# ADR: Redis Pub/Sub como adapter opcional de Event Bus

- Status: **Proposto; não implementado**
- Escopo: contrato de transporte de eventos, configuração, falhas, migração e rollback
- Decisão padrão preservada: `InMemoryBus`
- Fora de escopo: Redis como storage, fila durável, replay, consumer groups, cache ou dependência obrigatória

## Contexto

O PicoClip possui hoje a porta `ports.EventBus`, um `InMemoryBus`, eventos de domínio em `domain.Event` e um outbox SQLite consumido por `OutboxWorker`. O bus alimenta principalmente SSE e integrações em tempo real. A fonte autoritativa continua sendo o storage: Activity lê eventos persistidos e o outbox registra publicações pendentes.

Redis Pub/Sub é útil quando mais de um processo PicoClip precisa observar os mesmos eventos, mas não oferece retenção, confirmação por consumidor nem replay. Torná-lo obrigatório contrariaria a operação local-first e o binário simples. A decisão é, portanto, adicionar Redis somente como adapter explícito e opcional, mantendo o comportamento atual por padrão.

## Suposições verificadas no estado atual

- `internal/core/ports/event_bus.go` expõe `Publish(context.Context, domain.Event)` e `Subscribe(context.Context)`.
- `internal/adapters/events/inmemory.go` entrega em memória, não bloqueia publisher quando o buffer de um subscriber está cheio e pode descartar a entrega para esse subscriber.
- `internal/core/services/outbox_worker.go` lê até 50 eventos do outbox, publica no bus e remove a entrada após sucesso; falhas recebem retry de 5 segundos.
- SQLite limita a seleção a menos de 10 tentativas em `internal/adapters/storage/sqlite/event_repo.go`.
- A confirmação de `PUBLISH` pelo Redis significa apenas que o servidor aceitou a publicação; não confirma processamento por subscribers.
- O wiring atual em `cmd/picoclip/main.go` sempre cria `InMemoryBus`. Nenhuma variável Redis descrita neste ADR existe hoje.

## Decisão

### 1. Seleção do adapter

O event bus terá dois adapters:

- `memory`: adapter padrão e sem dependência externa;
- `redis`: adapter opt-in para distribuição de eventos entre processos.

A configuração proposta é:

| Variável | Padrão | Contrato |
| --- | --- | --- |
| `PICOCLIP_EVENT_BUS` | `memory` | Valores aceitos: `memory`, `redis`. Valor desconhecido é erro de configuração. |
| `PICOCLIP_REDIS_URL` | vazio | Obrigatória quando o adapter é `redis`. URL Redis; credenciais não podem aparecer em logs ou diagnostics. |
| `PICOCLIP_REDIS_CHANNEL_PREFIX` | `picoclip` | Namespace dos tópicos. Deve conter apenas letras ASCII, números, `_`, `-` e `.`; sem wildcard ou whitespace. |
| `PICOCLIP_REDIS_CONNECT_TIMEOUT` | `3s` | Timeout de conexão/operação inicial. Deve aceitar duração Go positiva e limitada pela implementação. |
| `PICOCLIP_REDIS_RECONNECT_MIN_BACKOFF` | `500ms` | Backoff inicial para subscriptions desconectadas. |
| `PICOCLIP_REDIS_RECONNECT_MAX_BACKOFF` | `30s` | Teto de backoff; deve ser maior ou igual ao mínimo. |

Regras:

1. Ausência de `PICOCLIP_EVENT_BUS` mantém exatamente o `InMemoryBus` atual.
2. `PICOCLIP_EVENT_BUS=redis` sem URL, com URL inválida ou com backoffs incoerentes impede o startup com erro sanitizado.
3. Indisponibilidade de rede/Redis depois de uma configuração válida não troca silenciosamente para memory. O processo continua com health degradado; publicações duráveis permanecem no outbox para retry e subscriptions reconectam com backoff.
4. Não haverá fallback automático para memory em modo Redis, pois isso dividiria os observers entre dois buses sem avisar o operador.
5. Senha, username, query parameters sensíveis e conteúdo integral da URL Redis nunca serão logados, persistidos em eventos ou devolvidos por diagnostics.

### 2. Contrato de tópicos

Cada publicação usa exatamente um canal Redis:

```text
{prefix}.events.{event_type}
```

Exemplos:

```text
picoclip.events.task.created
picoclip.events.run.completed
picoclip.events.runtime.stalled
```

Contrato normativo:

- `event_type` é o valor exato de `domain.Event.Type`, em lowercase e com segmentos separados por `.`;
- o publisher não publica uma segunda cópia em um tópico global;
- `EventBus.Subscribe`, cuja semântica atual é receber todos os eventos, usa `PSUBSCRIBE {prefix}.events.*`;
- consumidores externos podem assinar o wildcard global ou um canal exato;
- não se criam tópicos por task, agent, run ou workspace, evitando cardinalidade, ACLs frágeis e vazamento de identificadores no nome do canal;
- o canal é uma rota de transporte, não uma autorização: o subscriber ainda deve tratar payloads como dados potencialmente sensíveis;
- nomes de tópico são estáveis dentro da versão v1. Renomear `event_type` é breaking change e requer período de compatibilidade explícito.

Consumidores não devem assinar simultaneamente o wildcard global e canais exatos quando isso fizer o mesmo processo receber duas vezes por escolha própria.

### 3. Envelope JSON v1

O payload Redis não será o JSON implícito do struct Go. O adapter serializa um envelope versionado e independente da representação interna:

```json
{
  "specversion": "1.0",
  "id": "evt_...",
  "type": "task.completed",
  "source": "picoclip",
  "time": "2026-07-11T12:34:56.123456789Z",
  "subject": {
    "task_id": "task_...",
    "agent_id": "agent_...",
    "run_id": "run_..."
  },
  "delivery_mode": "durable_outbox",
  "message": "Task completed",
  "data": {
    "key": "value"
  }
}
```

Campos:

| Campo | Obrigatório | Regra |
| --- | --- | --- |
| `specversion` | sim | Literal `1.0`. Versões desconhecidas devem ser rejeitadas pelo adapter antes de converter para `domain.Event`. |
| `id` | sim | Igual a `domain.Event.ID`; chave de deduplicação para consumers. |
| `type` | sim | Igual ao sufixo `event_type` do canal. Divergência canal/payload é mensagem inválida. |
| `source` | sim | Literal `picoclip` na v1. Não representa identidade/autorização de instância. |
| `time` | sim | `CreatedAt` em UTC, RFC3339Nano. |
| `subject` | sim | Objeto com `task_id`, `agent_id` e `run_id`; valores vazios são omitidos. Objeto vazio é válido para eventos de sistema. |
| `delivery_mode` | sim | `durable_outbox` ou `ephemeral`. |
| `message` | sim | Mensagem humana do evento; pode ser vazia, não pode conter segredos intencionais. |
| `data` | sim | Mapa string→string; objeto vazio quando não houver metadata. |

Compatibilidade de schema:

- producers v1 podem adicionar campos opcionais sem alterar `specversion`;
- consumers devem ignorar campos desconhecidos;
- remover campo, mudar tipo, alterar semântica ou formato temporal exige nova versão;
- tamanho máximo aceito pelo adapter deve ser configurado em código com default de 256 KiB. Payload maior falha antes de `PUBLISH`, fica no fluxo de falha do outbox e nunca é truncado silenciosamente;
- JSON inválido, versão desconhecida, ID/type/time ausentes ou canal divergente são descartados no subscriber com log sanitizado e contador de mensagem inválida; não devem derrubar a subscription.

### 4. Semântica de entrega

O contrato é **best-effort em tempo real, com tentativa durável de publicação para eventos de outbox**:

- o commit do evento e de sua entrada no outbox é a garantia durável;
- o `OutboxWorker` tenta publicar entradas due;
- sucesso de `PUBLISH` permite remover a entrada do outbox;
- falha antes da confirmação mantém a entrada para retry;
- falha ao remover a entrada após `PUBLISH` pode republicar o mesmo `id`;
- subscribers offline, lentos ou desconectados podem perder eventos, porque Redis Pub/Sub não retém mensagens;
- ordering é apenas a ordem observada por uma conexão/canal em condições normais. Não há ordering global entre canais, processos, retries ou reconexões;
- consumers que executam efeitos devem deduplicar por `id` e ser idempotentes;
- quem precisa de histórico ou recuperação deve consultar o storage/API de eventos, não Redis Pub/Sub.

`delivery_mode=durable_outbox` identifica eventos publicados pelo worker após persistência. `delivery_mode=ephemeral` fica reservado aos sinais que hoje são deliberadamente transitórios, como atualizações de output em tempo real. Eventos ephemeral:

- podem ser publicados diretamente;
- não são inseridos no outbox;
- podem ser perdidos sem retry;
- não podem ser usados para decisões de lifecycle, auditoria ou integração que exija recuperação;
- não devem transportar stdout/stderr integral no envelope Redis. O contrato recomendado é transportar somente sinal compacto/contagem e manter output na superfície de run/log.

A implementação deve centralizar essa classificação; não deve inferi-la de forma divergente em cada producer. Na v1, todo evento persistido no outbox é `durable_outbox`; publicação direta é `ephemeral` e deve permanecer excepcional.

### 5. Política de falha

| Falha | Comportamento obrigatório |
| --- | --- |
| Selector/env inválido | Falhar startup; mensagem sem credenciais. |
| Redis indisponível ao publicar | `Publish` retorna erro; outbox mantém evento e agenda retry. A operação de domínio já commitada não é revertida. |
| Redis indisponível ao assinar | Subscription entra em reconnect com backoff e jitter; não cria fallback memory. |
| Context cancelado | Encerrar publish/subscription prontamente e fechar o canal Go uma única vez. |
| Subscriber local lento | Adapter usa buffer limitado; ao saturar, descarta para esse subscriber e incrementa métrica/log rate-limited, sem bloquear o leitor Redis global. |
| Envelope inválido | Descartar mensagem, registrar motivo sanitizado e continuar subscription. |
| Payload acima do limite | Retornar erro de publicação; não truncar. Para outbox, aplicar retry/terminal policy. |
| Outbox esgota tentativas | Preservar a linha como falha terminal para inspeção; não deletar. Expor contador/health degradado e último erro sanitizado. |
| Falha de delete após publish | Aceitar possível duplicata; retry com mesmo `id`. |

O limite atual de menos de 10 tentativas pode ser preservado na primeira implementação, mas a condição terminal precisa ficar observável. Hoje a linha deixa de ser selecionada silenciosamente; isso é uma lacuna a corrigir antes de considerar o adapter Redis pronto.

Logs e diagnostics mínimos:

- adapter selecionado e prefixo, sem URL;
- estado `healthy`/`degraded`;
- timestamp do último publish bem-sucedido;
- falhas de publish/reconnect por classe, com rate limiting;
- quantidade de outbox due e terminal;
- mensagens inválidas e drops por buffer cheio.

Falha do event bus não pode bloquear scheduler, runner, API administrativa nem persistência de eventos. Pode degradar SSE/integradores e deve ficar visível operacionalmente.

### 6. Segurança

- Redis deve ser tratado como extensão do boundary confiável da instalação.
- Para Redis fora do host/rede privada, usar TLS (`rediss://`) e ACL com usuário dedicado.
- A ACL deve permitir apenas conexão, `PUBLISH` nos canais `{prefix}.events.*` e `PSUBSCRIBE`/`SUBSCRIBE` necessários. O adapter não precisa de comandos de dados (`GET`, `SET`, `KEYS`, `FLUSH*`).
- Eventos não devem conter tokens, senhas, API keys, URLs credenciadas, prompts integrais ou output integral. Producers continuam responsáveis por sanitização antes da persistência/outbox.
- IDs no payload não são credenciais. Receber um evento não concede permissão para ler a task/run correspondente.
- O prefixo deve separar ambientes que compartilham Redis, por exemplo `picoclip.dev` e `picoclip.prod`.
- O adapter deve usar cliente Redis mantido, com timeouts, limites de pool e suporte a TLS; não deve implementar protocolo Redis manualmente.

## Alternativas consideradas

### Redis Streams

Ofereceria retenção, consumer groups e ack, mas transformaria o bus em fila durável paralela ao outbox, ampliando operação, recovery e semântica de ownership. Rejeitado para esta etapa. Pode ser novo adapter futuro com contrato próprio, não uma mudança transparente do adapter Pub/Sub.

### Publicar em um único canal global

Simplifica o publisher, mas força todos os consumers a receber e filtrar tudo e impede ACL/assinatura por tipo. Rejeitado em favor de um canal por `event_type` com wildcard para o contrato atual de Subscribe-all.

### Publicar simultaneamente em canal global e canal por tipo

Cria duplicatas para consumers que assinem ambos e dobra tráfego. Rejeitado.

### Redis obrigatório

Contraria local-first, aumenta dependências de startup e quebra instalações existentes. Rejeitado.

### Fallback automático Redis → memory

Mantém o processo aparentemente funcional, mas cria split-brain silencioso: eventos deixam de chegar aos outros processos. Rejeitado. Rollback deve ser explícito por configuração e restart.

### CloudEvents completo

O envelope usa nomes familiares (`specversion`, `id`, `type`, `source`, `time`), mas não declara conformidade CloudEvents porque `subject` é objeto e o contrato é deliberadamente pequeno. Conformidade formal pode ser avaliada em uma versão futura sem afirmar compatibilidade inexistente.

## Migração proposta

A implementação deve ser dividida em passos reversíveis:

1. Adicionar testes de contrato compartilhados para qualquer `EventBus`: publish/subscribe, cancelamento, fan-out, buffer lento e round-trip do evento.
2. Introduzir codec v1 isolado, com testes golden para envelope, UTC, campos opcionais, limite de tamanho, versão desconhecida e divergência canal/type.
3. Adicionar adapter Redis e testes de integração contra Redis descartável; nenhuma alteração de default.
4. Adicionar parser/validação de env no composition root. `memory` continua padrão.
5. Tornar explícita e central a origem `durable_outbox` versus `ephemeral`; remover publishes diretos redundantes de eventos que já têm outbox.
6. Implementar reconnect da subscription, health degradado, logs sanitizados e métricas/diagnostics mínimos.
7. Tornar falha terminal do outbox observável sem apagar o evento; adicionar runbook de retry/reprocessamento antes de habilitar Redis em ambiente compartilhado.
8. Habilitar `PICOCLIP_EVENT_BUS=redis` primeiro em ambiente de teste e validar dois processos: evento criado por um processo chega ao subscriber do outro.
9. Somente depois documentar Redis como adapter disponível no estado atual do produto. Até lá, este ADR permanece proposto.

Não é necessária migration de schema para selecionar o adapter. Qualquer mudança de schema do outbox para estado terminal/reprocessamento deve ser aditiva e manter leitura de bancos existentes.

## Compatibilidade

- Instalações sem novas env vars continuam usando memory e não precisam de Redis.
- `ports.EventBus` pode permanecer inicialmente com a assinatura atual; detalhes de envelope/tópico ficam no adapter/codec.
- Subscribers SSE continuam recebendo `domain.Event` após decode.
- O envelope externo é versionado e não depende de tags JSON futuras do struct Go.
- O mesmo evento pode ser entregue mais de uma vez; código existente que apenas invalida/atualiza UI permanece compatível, enquanto novos consumers com efeitos precisam deduplicação.
- Memory storage continua adequado a testes e sessões temporárias, mas seu outbox atual é no-op. Testes de integração do fluxo durável devem usar SQLite ou evoluir memory storage com semântica de outbox equivalente antes de alegar paridade.

## Rollback

Rollback operacional não exige remover código nem alterar banco:

1. definir `PICOCLIP_EVENT_BUS=memory` ou remover a variável;
2. reiniciar todos os processos PicoClip do mesmo ambiente;
3. confirmar health do bus memory e SSE local;
4. manter Redis disponível durante a janela de drenagem apenas se ainda houver processos antigos conectados;
5. inspecionar outbox terminal/due antes e depois da troca.

O rollback não reproduz mensagens Pub/Sub perdidas. Activity/histórico continua vindo da tabela de eventos. Não se deve alternar apenas parte das réplicas para memory, pois isso cria split-brain intencional.

## Critérios de aceite da implementação futura

- [ ] Sem env nova, o binário usa `InMemoryBus` e os testes atuais permanecem verdes.
- [ ] Configuração inválida falha startup sem expor credenciais.
- [ ] Dois processos com o mesmo Redis/prefixo trocam todos os tipos de evento pelo wildcard.
- [ ] Canal e envelope seguem exatamente o contrato v1 e testes golden.
- [ ] Desconectar Redis não derruba scheduler/API; health fica degradado e outbox retém eventos duráveis.
- [ ] Reconectar Redis retoma publish/subscription com backoff e sem intervenção manual.
- [ ] Repetição do mesmo `id` é demonstrada e consumer de teste deduplica corretamente.
- [ ] Subscriber offline perde Pub/Sub, mas recupera histórico pela API/storage conforme documentado.
- [ ] Payload inválido ou grande não derruba subscription nem vaza conteúdo sensível em logs.
- [ ] Outbox que esgota tentativas fica observável e reprocessável; não desaparece nem fica silencioso.
- [ ] `run.output` integral não é replicado no envelope Redis.
- [ ] `make check-docs`, testes focados do adapter/codec, `go test ./...` e `make check` passam.
- [ ] Runbook operacional e mapa do projeto são atualizados quando o adapter se tornar código real.

## Riscos residuais

- Redis Pub/Sub perde mensagens durante desconexões; o outbox reduz perda antes do publish, não depois da aceitação pelo Redis.
- Duplicatas continuam possíveis entre publish e delete do outbox.
- Um único prefixo compartilhado permite que qualquer subscriber autorizado veja IDs e metadata de todos os workspaces desse ambiente.
- Eventos atuais podem conter mensagens/data não desenhadas para sair do processo; uma auditoria de sanitização é pré-requisito para habilitação fora de localhost/rede confiável.
- O fluxo atual possui publishes diretos e paridade incompleta do outbox em memory storage; a classificação durable/ephemeral deve ser consolidada na implementação.
- Sem métricas de outbox terminal e reconnect, falhas podem permanecer invisíveis. Essas superfícies fazem parte dos critérios de aceite, não são opcionais.

## Decisão resumida

Adicionar futuramente Redis Pub/Sub como adapter opt-in, sem substituir o `InMemoryBus` padrão. Publicar um envelope JSON v1 em um canal por tipo de evento. Eventos persistidos usam outbox para tentar publicação, mas a entrega aos subscribers continua best-effort, sem replay e com possíveis duplicatas. Falhas Redis degradam o bus e preservam o core; configuração inválida falha cedo; nunca há fallback silencioso para memory.
