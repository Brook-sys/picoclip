# PicoClip — Roadmap Canônico (Paperclip Alignment)

Status: 2026-07-06

Este roadmap é a referência absoluta para evolução do PicoClip. Ele substitui versões anteriores e deve ser seguido em ordem.

O objetivo é transformar PicoClip em um sistema local-first que oferece as capacidades centrais do Paperclip (orquestração de agentes, tarefas/índices, heartbeat, checkout/locking, comentários/inbox, delegação, hierarquia, permissões, custos/tokens, recuperação, UI e APIs) com menor consumo de RAM, menor overhead de tokens e maior modularidade.

O roadmap define direção e fases. A fila operacional de melhorias recorrentes vive no Hermes Kanban board `picoclip`; o contrato do backlog — categorias, formato de cards, deduplicação, priorização e critérios de conclusão — está em [Improvement Backlog](IMPROVEMENT_BACKLOG.md).

Princípios que não podem ser violados:

- Core pequeno, sem frameworks pesados.
- Storage, runtimes, memória e segredos como adaptadores.
- UI server-rendered + HTMX.
- Permissões e capacidades devem mudar comportamento real.
- Skills são pacotes de contexto/ação.
- Projetos isolam contexto, arquivos e execução.
- Agentes devem operar o sistema via APIs documentadas.
- Menos tokens e menos RAM são prioridade.

## Visão de Estado Final

PicoClip deve oferecer:

- tarefas como issues com lifecycle completo;
- agentes com heartbeat, inbox, checkout, contexto e execução;
- sub-tarefas, delegação e hierarquia;
- comentários, mensagens e atenção/inbox;
- permissões reais e enforcement;
- custos/tokens como ledger + orçamentos;
- workspaces isolados por execução;
- runtimes como adaptadores eventados;
- UI e APIs que permitem operação completa por humanos e agentes;
- recuperação, retry e resiliência.

## Fases e Entregas

### Fase 0 — Fundação Estável (Concluída)

Entregue:

- Domínio: Task, Agent, Run, Message, Workspace, Skill, Event, RuntimeState.
- Serviços: TaskService, AgentService, Runner, Dispatcher, Scheduler, RuntimeManager, SkillService, WorkspaceService.
- Runtimes: noop, crush, picoclaw, claurst.
- UI: páginas separadas + detail redesign + inline edit + follow-ups.
- APIs: admin + agent-api.
- Persistência: SQLite completo.
- Tokens: contadores em Run/Task/Agent.
- Eventos/SSE.
- Skills built-in.

Estado: estável, mas sem lifecycle formal, heartbeat, inbox, ledger de custos, workspaces isolados, enforcement de permissões e recuperação forte.

### Fase 1 — Ciclo de Vida e Execução Robusta (Prioridade Máxima)

Objetivo: tornar o ciclo de trabalho confiável e próximo do Paperclip.

#### 1.1 Matriz de Transição de Tarefas

Entregas:

- `TaskLifecycle` com tabela de transições permitidas.
- Regras: comentário obrigatório em `blocked`/`done`/`cancelled`.
- Efeitos colaterais documentados e testados.
- `in_review` e `backlog` úteis.

Critério de aceite:

- todo transition tem teste.
- UI e Agent API respeitam matriz.

#### 1.2 Checkout Atômico e Locks

Entregas:

- `CheckoutRunID` obrigatório em checkout por run.
- `ExecutionLockedAt`, `LockExpiresAt`.
- Stale lock sweeper.
- 409 Conflict explícito no Agent API.
- Release endpoint.

Critério de aceite:

- tarefa não tem mais de um checkout ativo.
- run terminal não deixa lock pendente.
- stale locks são limpos periodicamente.

#### 1.3 Heartbeat/Wakeup Engine

Entregas:

- `WakeupRequest` (AgentID, Reason, TaskID, Priority, DueAt).
- Wake reasons: assignment, comment, manual, retry, schedule, recovery.
- Scheduler reconcilia primeiro, depois dispatcha wakeups.
- Runner inicia heartbeat com inbox-lite e heartbeat-context.

Critério de aceite:

- heartbeat pode ler inbox e decidir checkout.
- wakeups são priorizados e persistidos.

#### 1.4 Inbox e Heartbeat Context Compacto

Entregas:

- `GET /agent-api/agents/me/inbox-lite`
- `GET /agent-api/tasks/{id}/heartbeat-context`
- Razão de wakeup no contexto.
- Contexto inclui: título, descrição, último comentário do usuário, APIs disponíveis, skills relevantes.

Critério de aceite:

- prompt do primeiro heartbeat é compacto e útil.
- agente consegue descobrir o que fazer sem prompt gigante.

#### 1.5 Permissões com Enforcement Real

Entregas:

- `Authorizer` central.
- Checagem em endpoints de ação (create/update/delegate/cancel, skills, runtimes, settings).
- Eventos de auditoria.

Critério de aceite:

- observer não consegue criar/delegar/cancelar;
- coordinator consegue delegar;
- operator consegue cancelar;
- administrador consegue gerir agentes/skills/runtimes.

#### 1.6 Ledger de Uso e Custos

Entregas:

- `UsageEvent` (RunID, TaskID, AgentID, Provider, Model, Input/Output/Cached tokens, CostMicros).
- Agregação em Agent/Task/Project.
- Orçamentos simples (warn/disable).

Critério de aceite:

- tokens reais ou estimados são persistidos por run;
- UI mostra uso por agente/tarefa/projeto.

#### 1.7 Cancelamento e Recuperação Fortes

Entregas concluídas:

- `TaskService.Cancel` passa pelo `TaskLifecycle`, marca a task como `cancelled`, limpa checkout/lock e fecha o run ativo como `canceled`.
- `RuntimeManager.CancelRun` encaminha o cancelamento para o adapter ativo.
- Adapters Crush, Claurst e PicoClaw iniciam subprocessos em grupo próprio no Unix e cancelam o grupo inteiro com SIGTERM seguido de SIGKILL.
- Regressão automatizada cobre cancelamento de filhos que ignoram SIGTERM.
- Reconciler detecta runs sem output após `StallTimeout`, marca como `timeout`, chama cancelamento do runtime e reabre/reagenda a task quando cabível.
- Recovery de lock expirado também fecha o run associado como `timeout` e emite evento `run.recovered`.
- Runs órfãos sem task associada são marcados como `timeout`, cancelados no runtime e registrados em evento de recovery.
- Retry wakeups usam backoff exponencial com cap de 5 minutos e não deixam a task executável antes do `DueAt` ser processado.
- Retry de timeout persiste metadata de aprendizado (`previous_run_id`, `attempt`, `backoff_seconds`, `retryable`, `classification`, `reason`) e evento `retry.scheduled` visível na Activity UI.
- Recovery de lock expirado em task contínua agenda o próximo ciclo com `LoopNextRunAt`, sem wakeup imediato e sem burlar o delay do loop.
- Dispatcher aguarda um slot de concorrência antes de chamar `ClaimNextRunnable`, evitando que uma task seja lockada ou que um run seja criado quando não há capacidade real para iniciar o runner.
- Classificação inicial de retry diferencia timeouts/runtime stalled como `retryable`, runtime indisponível como `non_retryable` e erros genéricos como `unknown` nos eventos de falha.

Próximas entregas:

- Expandir classificação retryable vs non-retryable para mais falhas determinísticas e transitórias.
- UI/API de recovery para runs órfãos, travados ou parcialmente cancelados.
- Métricas agregadas de liveness/recovery no dashboard e diagnostics.
- Windows Job Objects para cancelamento completo de árvore de processos no Windows.

Critério de aceite:

- cancelamento de run mata subprocesso e filhos;
- run travado é detectado e recuperado;
- locks órfãos são reconciliados sem intervenção manual.

### Fase 2 — Heartbeat Completo e Inbox Operacional

Objetivo: agentes operam como no Paperclip.

#### 2.1 Wakeup por Eventos

Entregas:

- wakeup em assignment, comment, manual, retry, schedule, recovery.
- Scheduler processa wakeups em vez de só `NeedsRun`.

#### 2.2 Inbox e UI de Atenção

Entregas:

- Inbox page (assigned, blocked, in_review, failed, attention).
- Agent detail mostra inbox e load atual.
- Dashboard mostra filas de atenção.

#### 2.3 Delegation e Subtarefas

Entregas:

- `BlockParentUntilDone`.
- Delegation plan + batch create.
- Resumo de children no parent.
- Manager chain permission.

#### 2.4 Comentários e Thread

Entregas:

- `Message` com `CreatedByRunID`, `Metadata`, `Kind`.
- Comment-driven wakeup.
- Thread visível e resumido no prompt.

### Fase 3 — Workspaces, Runtimes e Skills Avançados

Objetivo: execução segura, repetível e extensível.

#### 3.1 Workspace Isolation

Entregas:

- Diretório de execução por task/run.
- Validação de path containment.
- Workspace hooks (after_create, before_run, after_run).
- `cmd.Dir` = workspace path.

#### 3.2 Runtime Adapter Eventado

Entregas:

- `RuntimeSession` e `RuntimeEvent`.
- `ExecuteStream` opcional.
- Cancel real.
- Liveness metadata.
- Parse de usage real (quando runtime expõe).

#### 3.3 Protocolo de Comunicação com Agente

Entregas:

- `.picoclip/status` (blocked, needs-human-review, done).
- MCP sidecar opcional.
- Env vars: `PICOCLIP_AGENT_ID`, `PICOCLIP_RUN_ID`, `PICOCLIP_TASK_ID`, `PICOCLIP_API_BASE`.

#### 3.4 Skills como Pacotes

Entregas:

- Skill package com `SKILL.md`, múltiplos arquivos, manifest.
- Import/export de diretório.
- Assignment por tags/permissões.
- Budget de tokens por skill.

### Fase 4 — UI e Operação Avançada

Objetivo: humanos conseguem operar com clareza.

#### 4.1 Páginas de Operação

Entregas:

- Inbox.
- Org chart.
- Usage/Cost dashboard.
- Recovery dashboard (stale locks, failed runs, runtime health, retry queue).
- Task detail com blockers, children, transcript e logs.

#### 4.2 Logs e Eventos em Tempo Real

Entregas:

- Event store consistente.
- SSE fan-out.
- Timeline por entidade.
- Logs incrementais.

#### 4.3 Self-Review e Verificação

Entregas:

- Comandos de verificação (lint, test, build).
- Self-review loop limitado.
- Diff + resultado no run.

### Fase 5 — Integrações e Portabilidade

Objetivo: conectar sem perder local-first.

#### 5.1 Skills e Backup

Entregas:

- Skill package import/export.
- Backup/restore robusto.

#### 5.2 Tracker Adapters (Opcional)

Entregas:

- Sync GitHub Issues / Jira / Linear (import/export, status, comments).
- Mantém Task como fonte de verdade.

#### 5.3 Service e Distribuição

Entregas:

- Systemd/launchctl service.
- Release artifacts.
- Compatibilidade diagnostics.

## Priorização e Sequência

Fase 1 é absoluta e deve ser concluída antes de Fase 2.

Ordem recomendada dentro da Fase 1:

1. Matriz de transição.
2. Checkout/locks + stale recovery.
3. Heartbeat/wakeup engine.
4. Inbox-lite + heartbeat-context.
5. Permissões enforcement.
6. UsageEvent ledger.
7. Cancelamento e liveness.

Depois:

- Fase 2 (heartbeat completo).
- Fase 3 (workspaces/runtimes/skills).
- Fase 4 (UI avançada).
- Fase 5 (integrações).

## Restrições que Devem Ser Mantidas

- Sem SPA pesada.
- Sem banco externo obrigatório.
- Sem Redis obrigatório.
- Sem dependências globais pesadas.
- Runtimes e storage como adaptadores.
- Prompts compactos por padrão.
- Sem conceitos de tracker específico no core.

## Critérios de Sucesso por Fase

Fase 1:

- Agente consegue fazer heartbeat, ver inbox, fazer checkout, trabalhar, comentar e atualizar status.
- Locks não ficam pendurados.
- Permissões são respeitadas.
- Uso de tokens é registrado.

Fase 2:

- Agente opera como heartbeat agent do Paperclip.
- Inbox e atenção são visíveis para humanos e agentes.
- Delegação e comentários funcionam de forma consistente.

Fase 3:

- Execução é isolada por workspace.
- Runtimes são canceláveis e observáveis.
- Skills são pacotes reais.

Fase 4:

- Humano consegue operar o sistema sem precisar inspecionar banco.
- Recuperação e atenção são tratadas como produto.

Fase 5:

- Skills e backups são portáveis.
- Tracker sync existe sem ser obrigatório.

## Fase 3 — Recursos inspirados em Paperclip (Controle e Operação)

Objetivo: trazer capacidades que Paperclip tem e que aumentam o valor real do controle plane sem violar a filosofia lean do PicoClip.

### 3.1 Budgets + Hard-stop

- Ledger de uso (`UsageEvent`) já existe.
- Adicionar `Budget` por workspace + agent com `limit_tokens`, `limit_runs`, `hard_stop`.
- Runner e Dispatcher respeitam orçamento e pausam automaticamente.
- UI mostra consumo vs limite + alerta de pausa.

Critério de aceite:
- tarefa é bloqueada quando orçamento estoura;
- wakeups de retry respeitam pausa por orçamento.

### 3.2 Approval Gates

- Adicionar `approver_agent_id` e `approval_status` em Task.
- Transições de status exigem aprovação explícita quando configurado.
- UI e Agent API expõem ações de approve/reject.
- Auditoria via Event.

Critério de aceite:
- task só avança para `done` ou `in_review` após aprovação quando gate existe.

### 3.3 Artifacts / Work Products

- Nova entidade `Artifact` (id, task_id, run_id, type, name, content_ref, metadata).
- Tipos: `text`, `markdown`, `diff`, `file`, `json`.
- Runner pode registrar artifacts ao final da execução.
- UI mostra lista de artifacts por task com preview simples.

Critério de aceite:
- output de run pode virar artifact persistente;
- artifacts são listáveis e recuperáveis via API.

### 3.4 Scheduled Routines

- Adicionar `Routine` (id, workspace_id, agent_id, prompt, schedule_cron, enabled).
- Scheduler avalia rotinas e cria wakeups ou tasks recorrentes.
- Suporte básico a cron simples (minuto, hora, dia).

Critério de aceite:
- rotina diária gera task automaticamente;
- logs de execução da rotina são visíveis.

### 3.5 Memória / Knowledge simples

- Entidade `Fact` (id, workspace_id, key, value, tags, updated_at).
- Skills podem referenciar facts.
- Agent API expõe `GET/POST /agent-api/facts`.
- UI básica de facts por workspace.

Critério de aceite:
- agente pode ler e escrever facts durante execução;
- facts sobrevivem a restarts.

## Manutenção do Roadmap

Este documento é a fonte de verdade. Mudanças devem ser feitas aqui primeiro e refletidas em `CURRENT_STATE.md` e `AGENTS.md` quando necessário.

Qualquer desvio deve ser justificado e registrado como decisão de arquitetura.

Fim do roadmap canônico.
