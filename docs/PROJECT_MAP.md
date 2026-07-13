# Mapa do Projeto PicoClip

Este documento é o mapa de orientação rápida para humanos e agentes. Use-o para entender onde cada parte do PicoClip vive, quais fluxos existem e qual documento ler em seguida.

## Leitura rápida recomendada

Para chegar produtivo no projeto:

1. `README.md` ou `README.pt-BR.md` — visão geral e quick start.
2. `AGENTS.md` — regras operacionais para agentes no repositório.
3. Este arquivo — mapa da arquitetura e dos fluxos.
4. Documento específico da área:
   - `docs/DEVELOPMENT.md` para ambiente, comandos e validação.
   - `docs/API_REFERENCE.md` para APIs e rotas.
   - `docs/ROBUSTNESS.md` para recovery, retry, locks e cancelamento.
   - `docs/STORAGE.md` para SQLite, migrations e restore.
   - `docs/PLUGINS.md` para arquitetura de plugins gRPC.
   - `docs/DESIGN.md` para UI, HTMX e componentes.
   - `docs/DOCUMENTATION_POLICY.md` para manter documentação atualizada.

## Princípios de arquitetura

PicoClip deve permanecer:

- local-first;
- leve em RAM e tokens;
- simples de rodar como binário Go;
- modular por portas e adaptadores;
- server-rendered na UI, com HTMX para interação progressiva;
- orientado a APIs para humanos e agentes;
- explícito sobre limitações e estado real.

## Estrutura de diretórios

```text
cmd/picoclip/main.go                  # entrypoint e wiring da aplicação
api/plugin/v1/                        # Contratos Protobuf e gRPC
internal/core/domain/                 # entidades e enums do domínio
internal/core/ports/                  # interfaces do core
internal/core/services/               # regras de aplicação e orquestração
internal/adapters/storage/memory/     # storage em memória para testes/sessões temporárias
internal/adapters/storage/sqlite/     # storage persistente padrão e migrations
internal/adapters/storage/storagetest/# contrato compartilhado de storage
internal/adapters/events/             # event bus em memória
internal/adapters/runtimes/           # adapters Crush, PicoClaw e Claurst
internal/adapters/drivers/            # drivers legados
internal/adapters/logger/             # logging estruturado
internal/adapters/web/                # servidor HTTP, APIs, HTML, Templ, HTMX, SSE
internal/adapters/web/assets/         # CSS e assets estáticos
docs/                                 # documentação canônica
.github/                              # CI, Dependabot, CODEOWNERS e templates
SECURITY.md                           # política de reporte privado de vulnerabilidades
scripts/seed/                         # seed de dados demo via API
scripts/run-e2e.sh                    # runner E2E Playwright
scripts/dev-local.sh                  # helper local não versionado neste ambiente
workspaces/                           # workspaces/projetos locais
```

## Entrypoint e wiring

Arquivo: `cmd/picoclip/main.go`

Responsabilidades principais:

- selecionar storage (`sqlite` por padrão, `memory` opcional);
- configurar SQLite com `busy_timeout`, WAL e foreign keys;
- migrar schema;
- criar event bus, clock e gerador de IDs;
- criar `RuntimeManager` e registrar adapters `crush`, `picoclaw`, `claurst`;
- iniciar `Engine`, `OutboxWorker` e `WebhookDeliveryWorker`;
- criar serviços de agents, tasks, workspaces, skills e diagnostics;
- garantir workspace default;
- instalar skills built-in;
- montar servidor HTTP;
- iniciar `ListenAndServe`.

Variáveis importantes:

| Variável | Padrão | Uso |
| --- | --- | --- |
| `BIND` | `127.0.0.1` | Interface HTTP local por padrão; containers sobrescrevem para `0.0.0.0`. |
| `PORT` | `8080` no binário, `8088` no Makefile | Porta HTTP. |
| `PICOCLIP_STORAGE` | `sqlite` | `sqlite` ou `memory`. |
| `PICOCLIP_DB_PATH` | `data/picoclip.db` | Banco SQLite. |
| `PICOCLIP_WORKSPACES` | `workspaces` | Diretório base de projetos. |
| `PICOCLIP_RUNTIMES` | `data/runtimes` | Estado/config dos runtimes. |
| `PICOCLIP_LOG_LEVEL` | `info` | Nível de log. |
| `PICOCLIP_DEBUG` | `false` | Modo debug. |
| `CRUSH_PATH` | `crush` | Executável Crush. |
| `PICOCLAW_PATH` | `picoclaw` | Executável PicoClaw. |
| `CLAURST_PATH` | `claurst` | Executável Claurst. |
| `BWRAP_PATH` | `bwrap` | Executável Bubblewrap usado pelo runtime sandbox fail-closed. |

## Core domain

Diretório: `internal/core/domain/`

| Arquivo | Conceito | Observações |
| --- | --- | --- |
| `agent.go` | Agent, capabilities, permissions | Capabilities: `observer`, `worker`, `coordinator`, `operator`, `administrator`. |
| `task.go` | Task e modos | Status: `backlog`, `todo`, `in_progress`, `waiting_next_cycle`, `in_review`, `blocked`, `done`, `cancelled`. Modos: `once`, `continuous`. |
| `run.go` | Run de execução | Guarda status, runtime, output/error, tokens, PID e heartbeat de output. |
| `message.go` | Comentários/mensagens | Superfície de comunicação em tasks. |
| `event.go` | Eventos de domínio/activity | Base para Activity, outbox, SSE e diagnóstico. |
| `workspace.go` | Projeto/workspace | Isola contexto e pasta local. |
| `skill.go` | Skills built-in/custom | Pacotes de instrução/contexto. |
| `runtime.go` | Estado de runtimes | Instalação, paths e metadata. |
| `wakeup.go` | Wakeup requests | Retry, manual, assignment, schedule e recovery. |
| `usage.go` | Uso/tokens | Base para ledger/contadores. |
| `budget.go` | Orçamentos | Hard stop/warn para uso. |
| `internal/core/workflow/` | Workflow YAML declarativo | Contrato versionado v1, parser e validação de grafo acíclico; não executa nodes. |
| `webhook.go` | Webhook subscriptions/deliveries | Entrega de eventos externos. |
| `completion_audit.go` | CompletionAudit | Histórico de decisões semânticas antes de concluir uma task. |
| `diagnostics.go` | Health/diagnostics | Dados para diagnostics API/UI. |
| `errors.go` | Erros canônicos | `ErrNotFound`, `ErrNoPendingTasks`, etc. |

## Event bus e outbox

Estado atual:

- `internal/core/ports/event_bus.go` define publish/subscribe de `domain.Event`;
- `internal/adapters/events/inmemory.go` é o único adapter;
- o outbox SQLite tenta publicar eventos persistidos e alimenta também webhook deliveries;
- o projeto não possui nem planeja automaticamente adapters externos de Event Bus. Qualquer expansão desse escopo exige aprovação explícita do responsável pelo produto.

## Ports

Diretório: `internal/core/ports/`

As portas mantêm o core independente de implementação concreta:

- `Storage` e repositories;
- `RuntimeAdapter` e tipos de execução;
- `EventBus`;
- `Clock`;
- `IDGenerator`;
- `Logger`;
- `MemoryProvider`;
- `SecretProvider`;
- `CompletionAuditor` e repositório de auditorias de conclusão;
- drivers legados.

Regra: serviços do core devem depender de `ports`, não de adapters concretos.

## Services

Diretório: `internal/core/services/`

| Serviço | Papel |
| --- | --- |
| `AgentService` | Criar, listar, atualizar e deletar agents. |
| `TaskService` | Criar, listar, comentar, delegar, checkout, release, wake, cancel e atualizar tasks. |
| `TaskLifecycle` | Regras de transição de status e efeitos colaterais. |
| `TaskService` / `RuntimeCompletionAuditor` | Faz o gate semântico em transições não-terminais para `done`; executa o auditor diretamente, sem criar task/run normal. |
| `Runner` | Monta prompt, executa runtime, salva run/output/mensagens/eventos/uso. |
| `Dispatcher` | Reivindica tasks runnable respeitando concorrência e cria runs. |
| `Scheduler` | Ciclo periódico: reconciler antes, dispatcher depois. |
| `Reconciler` | Ativa contínuas, processa wakeups, detecta stalls e recupera órfãos. |
| `LockRecoveryService` | Limpa locks expirados e fecha runs associados. |
| `WakeupService` | Processa wakeups pendentes/due. |
| `RuntimeManager` | Registro, estado, execução, teste e cancelamento de runtimes. |
| `PromptBuilder` | Constrói contexto enviado ao agent/runtime. |
| `BudgetService` | Verifica hard stops e uso. |
| `DiagnosticsService` | Monta health checks para UI/API. |
| `SkillService` | CRUD e instalação de skills built-in. |
| `WorkspaceService` | CRUD e pastas de workspaces. |
| `WebhookDeliveryWorker` | Entrega/retry de webhooks. |
| `OutboxWorker` | Publica eventos persistidos no bus. |
| `Authorizer` | Enforcement parcial de permissões em fluxos de Agent API. |

## Storage

Documentação detalhada: `docs/STORAGE.md`.

Adapters:

- `internal/adapters/storage/sqlite/` — persistência padrão.
- `internal/adapters/storage/memory/` — referência comportamental, testes e sessões temporárias.
- `internal/adapters/storage/storagetest/` — contrato compartilhado entre adapters.

Regras importantes:

- SQLite usa `modernc.org/sqlite` para manter build sem CGO.
- Migrations ficam em `internal/adapters/storage/sqlite/migrations.go`.
- Mudanças em storage devem manter paridade com memory via contract tests.
- Backup/restore fica na Settings Danger Zone.

## Runtime adapters

Diretório: `internal/adapters/runtimes/`

Adapters atuais:

- `crush`;
- `picoclaw`;
- `claurst`.

Responsabilidades:

- resolver executável/configuração;
- executar processo;
- capturar stdout/stderr;
- registrar PID;
- cancelar processo/grupo de processos quando suportado;
- reportar health/status.

Notas:

- Unix usa grupo de processos para cancelar árvore.
- Windows ainda precisa Job Objects para paridade completa.
- Claurst pode precisar imagem Debian/glibc em vez de Alpine/musl.
- O runtime `bwrap` é um sandbox opt-in apenas para Linux: ele só executa comandos absolutos aprovados sob `/bin` ou `/usr/bin`, sem rede (`--unshare-all`) e sem fallback para execução no host. A raiz é tmpfs mínima, sem mounts do root, homes, `/run` ou sockets do host; workspace opcional precisa ficar sob `PICOCLIP_WORKSPACES`. O operador deve instalar Bubblewrap no host e, quando necessário, sobrescrever o executável com `BWRAP_PATH`.

## Web, UI e APIs

Diretório: `internal/adapters/web/`

Responsabilidades:

- registrar rotas em `server.go`;
- expor APIs admin, Agent API e API v1 envelopada;
- oferecer endpoints compactos para agentes, como `heartbeat-context`, para reduzir tokens ao consultar estado de execução;
- renderizar UI server-side com Templ;
- servir assets;
- expor SSE e partials HTMX;
- montar view models.

Documentos relacionados:

- `docs/API_REFERENCE.md` para endpoints.
- `docs/DESIGN.md` para layout, componentes e HTMX.

Principais páginas web:

| Área | Rotas |
| --- | --- |
| Dashboard | `/` |
| Projects | `/projects`, `/projects/{id}` |
| Agents | `/agents`, `/agents/new`, `/agents/{id}` |
| Tasks | `/tasks`, `/tasks/{id}` |
| Runs | `/runs`, `/runs/{id}` |
| Skills | `/skills`, `/skills/{id}` |
| Activity/live streams | `/activity`, `/sse/activity`, `/sse/tasks/{id}`, `/sse/runs/{id}/logs` |
| Settings | `/settings`, `/settings/adapters`, webhook/settings/runtime actions, `RUNTIME_PROVIDER_QUICK_SETUP.md` |

## Fluxo de execução de task

Fluxo simplificado atual:

1. Task é criada com `NeedsRun=true`.
2. Scheduler chama reconciler.
3. Reconciler ativa contínuas due, processa wakeups, recupera locks/runs.
4. Scheduler chama dispatcher.
5. Dispatcher espera slot de concorrência antes de reivindicar task.
6. `ClaimNextRunnable` faz checkout atômico, incrementa attempt e cria run.
7. Runner carrega agent, runtime, skills, mensagens e contexto.
8. Runner executa runtime com timeout.
9. Output/error atualizam run e heartbeat.
10. Runner finaliza task/run ou agenda ciclo contínuo.
11. Reconciler posterior corrige stalls, órfãos e wakeups.

Documentação detalhada: `docs/ROBUSTNESS.md`.

## Continuous tasks

Tasks podem rodar em modo:

- `once` — execução única;
- `continuous` — ciclos com `LoopDelaySeconds`, pause/resume/run-now e `waiting_next_cycle`.

Recovery de task contínua deve respeitar o próximo ciclo e não transformar falhas em loop apertado.

## Agent API

A Agent API é a superfície para agentes operarem o sistema.

Rotas principais:

- `/agent-api/docs`;
- `/agent-api/me`;
- `/agent-api/agents/me/inbox-lite`;
- `/agent-api/tasks` e alias `/agent-api/issues`;
- `/agent-api/tasks/{id}/heartbeat-context`;
- `/agent-api/tasks/{id}/comments`;
- checkout/release/update/wake/delegate/cancel.

Detalhes: `docs/API_REFERENCE.md`.

## Comandos canônicos

Veja `docs/DEVELOPMENT.md` para detalhes.

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

Neste ambiente existe um helper local não versionado:

```sh
scripts/dev-local.sh status
scripts/dev-local.sh start
scripts/dev-local.sh stop
scripts/dev-local.sh full-check
```

Não commitar `scripts/dev-local.sh`.

## Onde mexer por tipo de mudança

| Mudança | Comece por | Atualize docs |
| --- | --- | --- |
| Novo status ou regra de task | `domain/task.go`, `services/task_lifecycle.go` | `PROJECT_MAP`, `API_REFERENCE`, `ROBUSTNESS`, `ROADMAP` |
| Retry/recovery/cancelamento | `services/reconciler.go`, `lock_recovery.go`, `runner.go`, runtimes | `ROBUSTNESS`, `CURRENT_STATE`, `ROADMAP` |
| Novo endpoint | `adapters/web/server.go` e handler correspondente | `API_REFERENCE`, `PROJECT_MAP` |
| Nova página UI | `*_templ.go`, `html_handlers.go`, viewmodels | `DESIGN`, `PROJECT_MAP`, E2E |
| Novo storage field/table | `domain`, `ports/storage.go`, sqlite/memory repos, migrations | `STORAGE`, `PROJECT_MAP` |
| Novo runtime adapter | `ports/runtime.go`, `adapters/runtimes/`, settings UI | `PROJECT_MAP`, `DEVELOPMENT`, `API_REFERENCE` se exposto |
| Nova skill built-in | `services/skill_service.go` | `PROJECT_MAP`, README se for conceito principal |
| Mudança de comando/dev workflow | `Makefile`, scripts, package/go toolchain | `DEVELOPMENT`, README se afetar quick start |

## Estado e limitações importantes

- PicoClip é experimental e não recomendado para produção.
- Permissões existem no modelo e há enforcement parcial; ainda falta cobertura total.
- Retry classification ainda é básica.
- Runtime liveness ainda depende bastante de output/heartbeat.
- Recovery dashboard e métricas agregadas ainda são roadmap.
- Windows process-tree cancellation ainda precisa Job Objects.

## Regra de manutenção deste mapa

Sempre que uma mudança criar ou mover um conceito, módulo, endpoint, página, adapter, fluxo operacional ou comando, atualize este arquivo no mesmo conjunto de mudanças.
