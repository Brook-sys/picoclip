# Robustez, Recovery e Aprendizado com Falhas

_Leia em [Inglês / English](ROBUSTNESS.md)._

O PicoClip é intencionalmente pequeno, mas deve se comportar como um sistema operacionalmente confiável: falhas precisam ser visíveis, decisões de retry precisam ser explícitas e recovery deve evitar piorar uma situação ruim.

Este documento explica o modelo atual de robustez e a direção de hardening do projeto.

## Objetivos de design

O trabalho de robustez do PicoClip segue estes princípios:

- **Falhar de forma visível**: falhas importantes devem criar eventos persistidos, não apenas linhas de log.
- **Recuperar de forma conservadora**: recovery deve destravar trabalho com segurança, sem criar runs ativos duplicados.
- **Evitar retry storms**: retry deve usar backoff e não pode burlar o próprio agendamento.
- **Aprender com falhas**: decisões de retry/recovery devem carregar metadata estruturada explicando o que aconteceu e por que o sistema reagiu.
- **Preservar simplicidade local-first**: robustez não deve depender de filas, bancos ou serviços externos.

## Visão geral do ciclo de execução

Fluxo simplificado:

1. Uma task é criada e marcada como executável.
2. O dispatcher reivindica uma task executável de forma atômica.
3. O runner cria um run e bloqueia a task para esse run.
4. Um runtime adapter executa o trabalho.
5. O run termina como completed, failed, canceled ou timed out.
6. O reconciler repara estado antigo e processa wakeups agendados.

A regra de segurança principal é que uma task não deve ter mais de um checkout/run ativo ao mesmo tempo.

## Locks e recuperação de locks antigos

Quando uma task é retirada para execução, o PicoClip salva metadata de lock na task, incluindo ID do run ativo e dados de expiração do lock.

O serviço de lock recovery varre locks expirados e limpa o estado de checkout antigo. Se o lock expirado pertencer a um run que ainda está marcado como running, o run é encerrado como timeout e um evento de recovery é persistido.

Isso evita que tasks fiquem permanentemente presas após crash, kill de processo ou worker perdido.

## Detecção de run travado

Um run pode definir um stall timeout. Se um run em execução não produzir output dentro dessa janela, o reconciler trata o run como travado.

Comportamento atual:

- marca o run como `timeout`;
- persiste um evento `run.timeout`;
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

## Metadata de retry

Wakeups de retry incluem payload estruturado:

```text
previous_run_id
attempt
backoff_seconds
retryable
reason
```

Para recovery de timeout, `reason` é atualmente `run_timeout` e `retryable=true`.

Essa metadata também é copiada para o evento de activity, para que humanos e futuras automações entendam por que o retry foi agendado.

## Eventos de Activity usados para diagnóstico

Eventos importantes de robustez:

| Evento | Significado |
| --- | --- |
| `run.timeout` | Um run parou de fazer progresso e foi encerrado como timeout. |
| `run.recovered` | PicoClip reparou estado antigo ou órfão de run. |
| `retry.scheduled` | PicoClip agendou retry e registrou por quê, quando e com qual backoff. |
| `budget.blocked` | Execução foi bloqueada por limite de budget. |
| `driver.missing` | Runtime/driver necessário não estava disponível. |

A página Activity transforma esses eventos em mensagens legíveis. Por exemplo, um evento de retry aparece como PicoClip aprendendo com um timeout e agendando retry após um número específico de segundos.

## Tasks contínuas

Tasks contínuas não são retentadas da mesma forma que tasks one-shot. Quando uma task contínua termina ou é recuperada, PicoClip agenda o próximo ciclo usando o delay do loop, a menos que a task tenha sido cancelada, concluída ou pausada.

Se o lock de uma task contínua expira enquanto um run ainda está ativo, recovery fecha o run como timeout, limpa o checkout e move a task para `waiting_next_cycle` com um novo `LoopNextRunAt`. Ele **não** cria um recovery wakeup imediato e **não** define `NeedsRun=true`. A task só fica executável quando o próximo ciclo do loop vence e o reconciler a ativa.

Isso mantém trabalho recorrente previsível e impede que recovery transforme um loop contínuo em um retry loop apertado.

## Limitações atuais

O sistema está mais robusto do que antes, mas ainda é experimental. Lacunas conhecidas:

- A classificação de retry ainda é básica. Timeouts são tratados como retryable, mas erros determinísticos ainda precisam ser separados entre retryable e non-retryable.
- A UI ainda não possui dashboard dedicado de recovery para locks antigos, retry queue, runtime health ou runs órfãos.
- Liveness de runtime ainda é inferido principalmente por output/heartbeat, não por um modelo completo de eventos estruturados de runtime.
- Cancelamento de árvore de processos no Windows ainda precisa de Job Objects para paridade com Unix process groups.
- Métricas aparecem em eventos/logs, mas métricas agregadas de confiabilidade ainda são limitadas.

## Checklist operacional

Ao investigar uma task presa ou falhando:

1. Abra o detalhe da task e inspecione o run mais recente.
2. Abra Activity e procure `run.timeout`, `run.recovered`, `retry.scheduled`, `driver.missing` ou `budget.blocked`.
3. Verifique se há retry wakeup pendente e se o `DueAt` está no futuro.
4. Confirme se `MaxAttempts` foi alcançado.
5. Verifique runtime configuration e disponibilidade do driver se o erro indicar runtime ausente.
6. Use diagnostics para inspecionar storage, runtime path, workspace path e saúde dos runtimes configurados.

## Checklist para mudanças de robustez

Ao alterar recovery, retry, cancellation, scheduling ou dispatcher:

1. Escreva ou atualize um teste de regressão primeiro.
2. Confirme que o teste falha pelo motivo esperado.
3. Implemente a menor mudança de comportamento possível.
4. Rode os testes focados do pacote.
5. Rode `make check` antes de mergear.
6. Confirme que novos caminhos de falha criam eventos ou diagnostics claros.
7. Evite qualquer retry sem cap, backoff e evento explicando a decisão.

## Próximos passos de hardening

Trabalhos recomendados:

1. Adicionar classificação explícita: `retryable`, `non_retryable` e `unknown`.
2. Persistir eventos `retry.skipped` ou `task.blocked` quando PicoClip decide não tentar de novo.
3. Expor retry queue e estado de recovery na UI/API.
4. Adicionar contadores agregados de confiabilidade: timeouts, recoveries, retries agendados, retries pulados e tentativas esgotadas.
5. Expandir eventos de runtime para que liveness use sinais estruturados, não apenas timing de output.
