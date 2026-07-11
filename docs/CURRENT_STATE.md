# PicoClip — Estado Atual

Atualizado em: 2026-07-06

Este documento descreve o estado real atual do PicoClip. Ele deve ser lido junto com:

- [Project Map](PROJECT_MAP.md)
- [API Reference](API_REFERENCE.md)
- [Robustness](ROBUSTNESS.md)
- [Storage Architecture](STORAGE.md)
- [Roadmap](ROADMAP.md)
- [Documentation Policy](DOCUMENTATION_POLICY.md)
- [Improvement Backlog](IMPROVEMENT_BACKLOG.md)

## Resumo executivo

PicoClip é um motor local-first e leve de orquestração de agentes inspirado no Paperclip. O projeto já passou da fase de CRUD básico e hoje possui uma base funcional com:

- aplicação Go single-binary;
- storage SQLite padrão e storage em memória para testes/sessões temporárias;
- domínio com agents, tasks, runs, messages, events, workspaces, skills, runtimes, wakeups, usage, budgets, webhooks e auditorias semânticas de conclusão;
- scheduler/dispatcher/runner/reconciler para execução de tasks;
- task lifecycle com status Paperclip-like;
- execução por runtime adapters locais;
- UI server-rendered com Templ, HTMX, CSS simples e SSE;
- APIs administrativas e Agent API;
- diagnostics, backup/restore, settings, runtimes e webhooks;
- robustez inicial: checkout/locks, stale lock recovery, stalled run detection, retry wakeups com backoff e cancelamento por runtime.

O projeto ainda é experimental e não é recomendado para produção. As principais lacunas são enforcement completo de permissões, expansão da classificação retryable/non-retryable para mais erros determinísticos, dashboard de recovery, streaming/telemetria de runtime mais estruturados, workspace isolation por run e maturidade operacional.

## Filosofia preservada

O PicoClip deve continuar sendo:

- pequeno;
- local-first;
- portátil;
- econômico em RAM/tokens;
- modular por portas e adapters;
- fácil de entender por humanos e agentes;
- honesto sobre limitações;
- orientado a documentação e validação reais.

Evitar:

- frameworks pesados no core;
- SPA pesada;
- dependências sem necessidade clara;
- abstrações prematuras;
- comportamento “decorativo” que não muda execução real;
- documentação desatualizada ou aspiracional tratada como realidade.

## Stack atual

| Área | Estado atual |
| --- | --- |
| Linguagem | Go `1.25.10` no `go.mod`. |
| UI | Templ + HTMX + CSS plain. |
| Storage | SQLite via `modernc.org/sqlite`; memory adapter para testes. |
| E2E | Playwright. |
| Live reload | `air` via `make dev`. |
| Templates | `templ generate` via `make templ-generate`. |
| Runtimes | Crush, PicoClaw e Claurst adapters. |
| Logging | `slog` adapter. |
| Docs | Documentação canônica em `docs/` com política própria. |

## Estrutura macro

```text
cmd/picoclip/main.go
internal/core/domain/
internal/core/ports/
internal/core/services/
internal/adapters/storage/memory/
internal/adapters/storage/sqlite/
internal/adapters/storage/storagetest/
internal/adapters/events/
internal/adapters/runtimes/
internal/adapters/web/
internal/adapters/logger/
docs/
scripts/
e2e/
workspaces/
```

Mapa completo: [Project Map](PROJECT_MAP.md).

## Runtime e configuração

O entrypoint `cmd/picoclip/main.go` faz o wiring da aplicação:

1. configura logging;
2. escolhe storage;
3. abre SQLite e roda migrations quando aplicável;
4. cria event bus, clock e ID generator;
5. configura `RuntimeManager`;
6. registra runtimes;
7. inicia `Engine`;
8. cria services;
9. garante workspace default;
10. instala skills built-in;
11. inicia outbox worker e webhook worker;
12. cria diagnostics;
13. monta rotas web/API;
14. inicia servidor HTTP.

Variáveis principais:

| Variável | Padrão | Observação |
| --- | --- | --- |
| `BIND` | `0.0.0.0` | Interface HTTP. |
| `PORT` | `8080` no binário, `8088` no Makefile | Porta HTTP. |
| `PICOCLIP_STORAGE` | `sqlite` | `sqlite` ou `memory`. |
| `PICOCLIP_DB_PATH` | `data/picoclip.db` | Caminho SQLite. |
| `PICOCLIP_WORKSPACES` | `workspaces` | Base de projetos/workspaces. |
| `PICOCLIP_RUNTIMES` | `data/runtimes` | Estado/config dos runtimes. |
| `PICOCLIP_LOG_LEVEL` | `info` | Nível de log. |
| `PICOCLIP_DEBUG` | `false` | Modo debug. |
| `CRUSH_PATH` | `crush` | Binário Crush. |
| `PICOCLAW_PATH` | `picoclaw` | Binário PicoClaw. |
| `CLAURST_PATH` | `claurst` | Binário Claurst. |

## Domínio atual

### Agent

Arquivo: `internal/core/domain/agent.go`

Campos/conceitos importantes:

- identidade, nome, título e descrição;
- projeto (`ProjectID`);
- hierarquia (`ReportsToID`);
- tags;
- runtime type;
- system prompt e instruction file;
- capability;
- permissões;
- skills atribuídas;
- config/env/extra args;
- contadores de tokens na UI (agregados).

Capabilities atuais:

- `observer`
- `worker`
- `coordinator`
- `operator`
- `administrator`

Permissões atuais incluem leitura/escrita de projetos, agents, tasks, skills, settings, adapters e system.

Estado real: permissões existem no modelo e há enforcement parcial em rotas da Agent API. Ainda falta enforcement completo e consistente em todos os endpoints/ações.

### Task

Arquivo: `internal/core/domain/task.go`

Status atuais:

- `backlog`
- `todo`
- `in_progress`
- `waiting_next_cycle`
- `in_review`
- `blocked`
- `done`
- `cancelled`

Modos:

- `once`
- `continuous`

Campos de robustez relevantes:

- `NeedsRun`
- `Attempts`
- `MaxAttempts`
- `CheckoutRunID`
- `CheckedOutByAgentID`
- `ExecutionLockedAt`
- `LockExpiresAt`
- `CancelReason`
- timestamps de início/fim/conclusão/cancelamento

Estado real: tasks podem ser criadas, listadas, comentadas, delegadas, canceladas, acordadas, pausadas/retomadas quando contínuas e executadas pelo scheduler.

### Run

Arquivo: `internal/core/domain/run.go`

Representa uma tentativa de execução. Guarda:

- task/agent;
- driver/runtime;
- status;
- attempt;
- input/output/error;
- token counters;
- process id;
- last output heartbeat;
- stall timeout;
- started/finished timestamps.

Estado real: runs são criadas no checkout runnable e atualizadas durante a execução. Existe página de runs e detalhe de run.

### Message

Arquivo: `internal/core/domain/message.go`

Mensagens/comentários são a superfície de comunicação em tasks. Elas alimentam o prompt e a Agent API.

### Event

Arquivo: `internal/core/domain/event.go`

Eventos alimentam Activity, outbox, SSE e diagnósticos. Eventos importantes incluem criação/atualização de tasks, run output, run completed/failed/timeout/recovered, retry scheduled, budget blocked e driver missing.

### Workspace

Projetos/workspaces representam agrupamento e diretório local. O serviço garante um workspace default no startup.

### Skill

Skills existem como built-in/custom e podem conter instruções/arquivos. O sistema instala built-ins idempotentemente no startup.

### Wakeup

Wakeups são requests duráveis para tornar tasks executáveis por razões como retry, manual, assignment, schedule ou recovery.

### Usage e Budget

Há contadores agregados em runs/tasks/agents e um ledger persistente `UsageEvent` por run. O runner grava eventos de usage com tokens de entrada/saída; o detalhe do workspace também exibe o agregado do ledger por workspace, correlacionando cada evento à task correspondente. A API v1 expõe `/api/v1/usage` com filtros por `run_id`, `task_id` e `agent_id` e totais no `meta`. O `BudgetService` usa esses dados para bloquear execução por hard stop. Custo monetário real ainda é futuro: `cost_micros` existe no modelo/storage, mas permanece `0` até haver configuração de preço/modelo.

### Webhook

Settings incluem webhooks e deliveries, com worker para entrega e retry.

## Services atuais

| Serviço | Estado atual |
| --- | --- |
| `AgentService` | CRUD e atualização de agentes. |
| `TaskService` | CRUD, lifecycle, comments, delegation, checkout/release, wake, cancel e continuous task controls. |
| `TaskLifecycle` | Centraliza transições e efeitos colaterais. |
| `Dispatcher` | Reivindica tasks runnable respeitando slots de concorrência antes de claim. |
| `Runner` | Monta prompt, executa runtime, salva run, output, mensagens, events e usage. |
| `Scheduler` | Loop periódico: reconciler antes de dispatcher. |
| `Reconciler` | Ativa contínuas due, processa wakeups, detecta stalled runs e recupera órfãos. |
| `LockRecoveryService` | Limpa locks expirados e fecha runs associados. |
| `WakeupService` | Processa wakeups pendentes quando `DueAt` chega. |
| `RuntimeManager` | Gerencia runtimes, execução, health e cancelamento. |
| `BudgetService` | Verifica hard stops. |
| `DiagnosticsService` | Expõe health/config. |
| `SkillService` | CRUD e built-ins. |
| `WorkspaceService` | CRUD e diretórios. |
| `OutboxWorker` | Publica eventos persistidos no bus. |
| `WebhookDeliveryWorker` | Entrega webhooks e faz retry. |
| `Authorizer` | Enforcement parcial de permissões. |

## Fluxo de execução atual

1. Humano ou agente cria uma task.
2. Task fica runnable com `NeedsRun=true`.
3. Scheduler roda `Reconciler`.
4. Reconciler:
   - ativa continuous tasks due;
   - recupera stale locks;
   - processa wakeups;
   - detecta stalled runs;
   - recupera orphaned runs.
5. Dispatcher aguarda slot de concorrência.
6. Dispatcher faz `ClaimNextRunnable`.
7. Storage faz checkout atômico, cria run e locka task.
8. Runner recarrega task/agent, verifica budget e runtime.
9. Prompt é montado com task, agent, skills, mensagens e APIs disponíveis.
10. Runtime executa.
11. Output atualiza run e heartbeat.
12. Runner finaliza run/task ou agenda ciclo contínuo.
13. Falhas/stalls podem virar wakeups de retry com backoff.

Detalhes operacionais: [Robustness](ROBUSTNESS.md).

## Web/UI atual

A UI é server-rendered com Templ e HTMX. Páginas principais:

- `/` Dashboard;
- `/projects` e `/projects/{id}`;
- `/agents`, `/agents/new`, `/agents/{id}`;
- `/tasks`, `/tasks/{id}`;
- `/runs`, `/runs/{id}`;
- `/skills`, `/skills/{id}`;
- `/activity` com SSE;
- `/settings`, adapters/runtimes, budgets, webhooks, export/import/reset.

Padrões de UI: [Design System](DESIGN.md).

## APIs atuais

Há três grupos principais:

- API administrativa local: `/api/...`;
- Agent API: `/agent-api/...`;
- rotas web/HTMX/SSE.

Referência: [API Reference](API_REFERENCE.md).

A Agent API suporta aliases `tasks` e `issues` em várias rotas para alinhamento com Paperclip.

## Storage atual

SQLite é o padrão local-first:

- driver `modernc.org/sqlite`;
- WAL, foreign keys e busy timeout configurados no startup;
- migrations em Go;
- repositories por entidade;
- backup/restore pela Settings Danger Zone;
- memory adapter mantido em paridade via contract tests.

Detalhes: [Storage Architecture](STORAGE.md).

## Robustez atual

Recursos implementados:

- checkout/claim atômico com lock metadata;
- dispatcher respeita concorrência antes de claim;
- stale lock recovery;
- stalled run detection;
- orphaned run recovery;
- runtime cancellation;
- retry wakeups com backoff exponencial e cap de 5 minutos;
- retry metadata (`previous_run_id`, `attempt`, `backoff_seconds`, `retryable`, `reason`);
- eventos Activity para timeout, recovery e retry scheduled;
- continuous tasks respeitam próximo ciclo após recovery.

Limitações:

- retry classification ainda é básica;
- runtime liveness ainda depende muito de output/heartbeat;
- dashboard/API de recovery ainda não é completo;
- métricas agregadas ainda são limitadas;
- Windows process-tree cancellation ainda precisa Job Objects.

## Desenvolvimento e validação

Comandos principais:

```sh
make tools
npm install
npx playwright install chromium
make run
make dev
make test-go
make test-e2e
make check
```

`make check` roda:

1. `templ generate`;
2. `gofmt -w cmd internal`;
3. `go test ./...`;
4. `go vet ./...`;
5. build do binário;
6. Playwright E2E.

Guia completo: [Development Guide](DEVELOPMENT.md).

## Documentação atual

A documentação agora tem política formal:

- [README do diretório docs](README.md)
- [Project Map](PROJECT_MAP.md)
- [Documentation Policy](DOCUMENTATION_POLICY.md)
- [API Reference](API_REFERENCE.md)
- [Development Guide](DEVELOPMENT.md)
- [Storage Architecture](STORAGE.md)
- [Robustness](ROBUSTNESS.md)
- [Design System](DESIGN.md)
- [Improvement Backlog](IMPROVEMENT_BACKLOG.md)

Regra: novas funcionalidades devem atualizar documentação proporcional no mesmo conjunto de mudanças.

O backlog operacional de melhorias recorrentes é gerido no Hermes Kanban board `picoclip`; [Improvement Backlog](IMPROVEMENT_BACKLOG.md) documenta categorias, formato de cards, deduplicação, priorização e critérios de conclusão.

## Lacunas principais

Prioridades técnicas ainda abertas:

1. Enforcement completo de permissões.
2. Classificação retryable/non-retryable.
3. Recovery dashboard e métricas agregadas.
4. Workspace isolation por task/run.
5. Runtime events/streaming mais estruturados.
6. Ledger de usage/custos mais completo (além da visão agregada atual).
7. Skills como pacotes com múltiplos arquivos/import/export.
8. UI de inbox/atenção mais completa.
9. Windows Job Objects para cancelamento completo de árvore de processos.
10. Documentação de payloads detalhados por endpoint crítico.

## Como manter este documento

Atualize este arquivo quando:

- uma fase relevante do roadmap for concluída;
- arquitetura ou fluxo de execução mudar;
- novos módulos principais forem adicionados;
- uma limitação relevante deixar de existir;
- um documento canônico novo for criado.

Não use este arquivo para progresso temporário de tarefas. Use-o para estado real e durável do projeto.
