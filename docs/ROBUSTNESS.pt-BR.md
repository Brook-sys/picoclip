# Robustez, Recovery e Aprendizado com Falhas

_Leia em [Inglês / English](ROBUSTNESS.md)._

O PicoClip é intencionalmente pequeno, mas deve se comportar como um sistema operacionalmente confiável: falhas precisam ser visíveis, decisões de retry precisam ser explícitas e recovery precisa evitar piorar uma situação ruim.

Este documento descreve o modelo atual de confiabilidade para scheduler, dispatcher, runner, reconciler, locks, retries, wakeups e cancelamento. Para o mapa geral da arquitetura, veja [Project Map](PROJECT_MAP.md). Para o estado atual do produto, veja [Current State](CURRENT_STATE.md).

## Objetivos de design

O trabalho de robustez segue estes princípios:

- **Falhar de forma visível**: falhas importantes devem criar eventos persistidos, não apenas linhas de log.
- **Reivindicar de forma conservadora**: tasks só devem ser reivindicadas quando o PicoClip realmente puder iniciar trabalho.
- **Recuperar de forma conservadora**: recovery deve destravar trabalho com segurança, sem criar runs ativos duplicados.
- **Evitar retry storms**: retry deve usar backoff e não pode burlar o próprio agendamento.
- **Aprender com falhas**: decisões de retry/recovery devem carregar metadata estruturada explicando o que aconteceu e por que o sistema reagiu.
- **Preservar simplicidade local-first**: robustez não deve depender de filas, bancos ou serviços externos.

## Visão geral do ciclo de execução

Fluxo simplificado atual:

1. Uma task é criada e marcada como executável com `NeedsRun=true`.
2. O scheduler roda o reconciler antes de despachar trabalho novo.
3. O reconciler ativa tasks contínuas due, processa wakeups, detecta stalls e recupera estado órfão.
4. O dispatcher aguarda um slot de concorrência.
5. Só depois que um slot está disponível, o dispatcher reivindica uma task executável de forma atômica.
6. `ClaimNextRunnable` cria um run e grava metadata de checkout/lock na task.
7. O runner recarrega contexto de task e agent, verifica budgets e runtime, monta o prompt e executa o runtime.
8. Output do runtime atualiza output do run e metadata de heartbeat.
9. O runner finaliza run/task, bloqueia a task, agenda retry ou agenda o próximo ciclo contínuo.
10. Passagens posteriores do reconciler reparam locks antigos, runs travados, runs órfãos e wakeups vencidos.

A regra de segurança principal é: uma task não deve ter mais de um checkout/run ativo ao mesmo tempo.

## Segurança de concorrência do dispatcher

O dispatcher usa um semáforo limitado para respeitar `maxConcurrentRuns`.

Comportamento atual importante:

- o dispatcher adquire um slot de concorrência **antes** de chamar `ClaimNextRunnable`;
- se o contexto é cancelado antes de existir slot, nenhuma task é reivindicada;
- se o claim retorna `ErrNoPendingTasks` ou outro erro, o slot é liberado imediatamente;
- quando uma goroutine inicia o runner, o slot só é liberado depois que `runner.Run` retorna.

Isso impede que uma task seja marcada como `in_progress`/checked out e que um run seja criado quando não existe capacidade real de runner. O teste de regressão que protege esse comportamento é `TestDispatcherDoesNotClaimTaskWhenConcurrencySlotUnavailable`.

## Locks e recuperação de locks antigos

Quando uma task é retirada para execução, o PicoClip salva metadata de lock na task:

- ID do run ativo;
- agent em checkout;
- timestamp de início do lock;
- timestamp de expiração do lock.

O serviço de lock recovery varre locks expirados e limpa o estado de checkout antigo. Se o lock expirado pertencer a um run que ainda está marcado como running, o run é encerrado como timeout e um evento de recovery é persistido.

Isso evita que tasks fiquem permanentemente presas após crash, kill de processo, worker perdido ou ciclo de scheduler interrompido.

## Detecção de run travado

Um run pode definir um stall timeout. Se um run em execução não produzir output dentro dessa janela, o reconciler trata o run como travado.

Comportamento atual:

- marca o run como `timeout`;
- persiste um evento `run.timeout`;
- persiste eventos estruturados de liveness do runtime (`runtime.stalled`, depois `runtime.cancel_requested` e `runtime.cancel_succeeded` ou `runtime.cancel_failed` quando há canceler configurado);
- pede ao runtime manager para cancelar o processo/sessão;
- destrava a task;
- agenda retry com backoff, bloqueia a task quando `MaxAttempts` foi esgotado ou agenda o próximo ciclo de uma task contínua.

Timeouts contam como eventos de atenção na UI porque normalmente merecem inspeção operacional.

## Agendamento de retry e backoff

Retries são agendados por wakeups. Um retry wakeup é um pedido durável dizendo: esta task pode rodar de novo, mas somente depois de `DueAt`.

Fórmula atual de backoff:

```text
attempt 1 -> 30 segundos
attempt 2 -> 60 segundos
attempt 3 -> 120 segundos
attempt 4 -> 240 segundos
attempt N -> limitado a 300 segundos / 5 minutos
```

Uma propriedade de segurança importante é que a task **não** fica imediatamente executável enquanto espera o retry. A task permanece `NeedsRun=false` até o wakeup vencer e ser processado. Isso impede o dispatcher de burlar o backoff e executar a task imediatamente.

## Classificação e metadata de retry

O PicoClip registra uma pequena classificação de retry em eventos de falha/retry para que humanos e agentes diferenciem falhas transitórias de bloqueios determinísticos.

Valores atuais de classificação:

| Classificação | Significado | Exemplos atuais |
| --- | --- | --- |
| `retryable` | O PicoClip pode executar a task novamente após backoff. | Timeout direto do runtime (`runtime_timeout`) e recovery de run travado (`run_timeout`). |
| `non_retryable` | O PicoClip não deve tentar automaticamente porque a mesma configuração tende a falhar de novo. | Runtime indisponível (`runtime_unavailable`), exposto em `driver.missing` e `task.failed`. |
| `unknown` | Um runtime retornou erro ainda não classificado. | Erro genérico de execução de runtime (`runtime_error`). |

Wakeups de retry incluem payload estruturado:

```text
previous_run_id
attempt
backoff_seconds
retryable
classification
reason
```

Para recovery de timeout, `reason` é `run_timeout`, `retryable=true` e `classification=retryable`.

Antes de criar um retry de timeout, o reconciler verifica se já existe um retry wakeup pendente para o mesmo `previous_run_id`. Isso mantém recovery idempotente em sweeps repetidos e evita wakeups/eventos duplicados para o mesmo run com falha.

Essa metadata também é copiada para o evento de activity, para que humanos e futuras automações entendam por que o retry foi agendado. Eventos de timeout direto do runner também carregam `classification=retryable`, enquanto eventos de runtime indisponível carregam `classification=non_retryable` e bloqueiam a task em vez de criar loop de retry.

## Eventos de Activity usados para diagnóstico

Eventos importantes de robustez:

| Evento | Significado |
| --- | --- |
| `run.timeout` | Um run parou de fazer progresso e foi encerrado como timeout. |
| `run.recovered` | PicoClip reparou estado antigo ou órfão de run. |
| `runtime.started` | Runner começou a executar um runtime configurado para um run. |
| `runtime.process_started` | Adapter informou o identificador do processo/sessão do runtime. |
| `runtime.heartbeat` | Adapter produziu output; payload inclui contagem de bytes em vez de output completo para evitar eventos persistidos ruidosos. |
| `runtime.completed` | Execução do runtime terminou com o status final do run. |
| `runtime.timeout` | Runner tratou um timeout direto do runtime. |
| `runtime.stalled` | Reconciler detectou ausência de output antes do stall timeout. |
| `runtime.cancel_requested` | PicoClip pediu cancelamento de um run de runtime travado. |
| `runtime.cancel_succeeded` / `runtime.cancel_failed` | Cancelamento do runtime retornou sucesso ou falha. |
| `retry.scheduled` | PicoClip agendou retry e registrou por quê, quando, com qual backoff e com qual classificação de retry. |
| `budget.blocked` | Execução foi bloqueada por limite de budget. |
| `driver.missing` | Runtime/driver necessário não estava disponível; payload marca como `non_retryable`. |

Payloads de eventos de liveness do runtime são intencionalmente compactos. Eles incluem campos estáveis como `runtime_id`, `phase`, `status`, `pid`, `stdout_bytes`, `stderr_bytes`, `reason` e `error` de cancelamento quando relevante. O output completo continua no run/output stream; heartbeats persistidos usam contagens de bytes para que output frequente não polua a Activity com payloads grandes.

A página Activity transforma esses eventos em mensagens legíveis. Por exemplo, um evento de retry aparece como PicoClip aprendendo com um timeout e agendando retry após um número específico de segundos.

## Tasks contínuas

Tasks contínuas não são retentadas da mesma forma que tasks one-shot. Quando uma task contínua termina ou é recuperada, PicoClip agenda o próximo ciclo usando o delay do loop, a menos que a task tenha sido cancelada, concluída ou pausada.

Se o lock de uma task contínua expira enquanto um run ainda está ativo, recovery fecha o run como timeout, limpa o checkout e move a task para `waiting_next_cycle` com um novo `LoopNextRunAt`. Ele **não** cria um recovery wakeup imediato e **não** define `NeedsRun=true`. A task só fica executável quando o próximo ciclo do loop vence e o reconciler a ativa.

Isso mantém trabalho recorrente previsível e impede que recovery transforme um loop contínuo em um retry loop apertado.

## Modelo de cancelamento

Cancelamento passa por services e runtime adapters:

- cancelamento de task passa pelo `TaskLifecycle` quando aplicável;
- estado ativo de checkout/lock é limpo;
- o run ativo é fechado como `canceled`;
- `RuntimeManager.CancelRun` encaminha o cancelamento para o adapter ativo;
- adapters Unix iniciam subprocessos em grupo próprio e cancelam o grupo com SIGTERM seguido de SIGKILL quando necessário.

Lacuna conhecida: cancelamento de árvore de processos no Windows ainda precisa de Job Objects para paridade com process groups no Unix.

## Checklist operacional

Ao investigar uma task presa ou falhando:

1. Abra o detalhe da task e inspecione o run mais recente.
2. Abra Activity e procure `run.timeout`, `run.recovered`, `retry.scheduled`, `driver.missing` ou `budget.blocked`.
3. Verifique se há retry wakeup pendente e se o `DueAt` está no futuro.
4. Confirme se `MaxAttempts` foi alcançado.
5. Verifique configuração de runtime e disponibilidade do driver se o erro indicar runtime ausente.
6. Use a página de diagnostics ou `/api/diagnostics` para inspecionar storage, runtime path, workspace path e saúde dos runtimes configurados.
7. Se uma task parece executável mas não é pega, confira a capacidade do dispatcher e se um run anterior ainda possui metadata de checkout/lock.

## Checklist para mudanças de robustez

Ao alterar recovery, retry, cancellation, scheduling, dispatcher, runner ou runtime:

1. Leia este documento e o [Development Guide](DEVELOPMENT.md).
2. Escreva ou atualize um teste de regressão primeiro.
3. Confirme que o teste falha pelo motivo esperado.
4. Implemente a menor mudança de comportamento possível.
5. Rode testes focados do pacote, por exemplo:

   ```sh
   go test ./internal/core/services -run 'TestReconciler|TestStalledRun|TestDispatcher|TestLockRecovery' -count=1
   ```

6. Rode `make check` antes de considerar a mudança concluída.
7. Confirme que novos caminhos de falha criam eventos ou diagnostics claros.
8. Evite qualquer retry sem cap, backoff e evento explicando a decisão.
9. Atualize este documento sempre que o contrato de scheduler/dispatcher/runner/reconciler mudar.

## Limitações atuais

O sistema está mais robusto do que antes, mas ainda é experimental. Lacunas conhecidas:

- A classificação de retry ainda é básica. Timeouts e falhas de runtime indisponível agora carregam metadata explícita `retryable`/`non_retryable`, mas a maioria dos erros genéricos de runtime ainda permanece `unknown` até ganharem categorias determinísticas.
- Ainda não há dashboard dedicado de recovery para locks antigos, retry queue, runtime health ou runs órfãos.
- Liveness de runtime agora possui eventos estruturados por run para início, processo iniciado, heartbeats de output, timeout direto, detecção de stall e resultado de cancelamento, mas diagnostics agregados e resumos específicos de UI ainda são limitados.
- Cancelamento de árvore de processos no Windows ainda precisa de Job Objects para paridade com Unix process groups.
- Métricas aparecem em eventos/logs, mas métricas agregadas de confiabilidade ainda são limitadas.

## Próximos passos de hardening

Trabalhos recomendados:

1. Expandir a classificação de retry além da base atual de timeout/runtime indisponível para cobrir mais erros determinísticos de runtime.
2. Persistir eventos `retry.skipped` ou `task.blocked` quando PicoClip decide não tentar de novo.
3. Expor retry queue e estado de recovery na UI/API.
4. Adicionar contadores agregados de confiabilidade: timeouts, recoveries, retries agendados, retries pulados, tentativas esgotadas e tasks atualmente lockadas.
5. Expor eventos de liveness do runtime em diagnostics compactos e resumos de UI para que agentes expliquem rapidamente se um run está vivo, travado, em cancelamento ou em timeout.
6. Adicionar suporte a Windows Job Objects para cancelamento completo de árvore de processos.
