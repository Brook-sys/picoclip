# PicoClip Agents Guide

Este guia é o ponto de entrada para agentes de IA trabalhando no repositório PicoClip.

## Regra principal

Antes de alterar código, leia:

1. `docs/PROJECT_MAP.md` — mapa do projeto e onde cada coisa vive.
2. `docs/DOCUMENTATION_POLICY.md` — política obrigatória de documentação.
3. O documento específico da área que você vai mudar:
   - `docs/DEVELOPMENT.md` para comandos, testes e workflow local.
   - `docs/API_REFERENCE.md` para endpoints e contratos HTTP.
   - `docs/OPERATIONS.md` para runbooks, diagnostics, incidentes locais, backup/restore e troubleshooting operacional.
   - `docs/ROBUSTNESS.md` para scheduler, dispatcher, runner, reconciler, locks, retry e cancelamento.
   - `docs/STORAGE.md` para SQLite, migrations, repositories e restore.
   > **Dica:** Se precisar triar uma task que parece presa usando *Agent API* em vez da UI, veja a seção **Triagem Rápida via Agent API** em `docs/OPERATIONS.md`.

   - `docs/DESIGN.md` para UI, Templ, HTMX e padrões visuais.
   - `docs/AUTONOMOUS_IMPROVEMENT.md` para o ciclo Hermes Kanban/cron de melhoria contínua.
   - `docs/ROADMAP.md` e `docs/CURRENT_STATE.md` para contexto de produto.

Documentação faz parte do trabalho. Sempre que uma mudança alterar comportamento, API, comando, arquitetura, UI, storage, runtime ou operação, atualize os documentos correspondentes no mesmo conjunto de mudanças.

## Visão geral

PicoClip é um motor leve e local-first de orquestração de agentes inspirado no Paperclip. O objetivo é oferecer projetos/workspaces, agentes, tasks, runs, mensagens, delegação, capabilities, permissões, skills, runtimes e APIs para que humanos e agentes operem o sistema.

O projeto deve permanecer simples, portátil e econômico em recursos, seguindo a filosofia Go: binário pequeno, baixo uso de RAM, dependências moderadas e arquitetura modular.

## Filosofia

Preservar sempre:

- core pequeno;
- baixo consumo de recursos;
- local-first;
- modularidade por portas/adaptadores;
- runtimes plugáveis;
- storage plugável;
- UI leve com server-rendered HTML + HTMX + Templ;
- APIs documentadas para agentes;
- capabilities e permissões que mudam comportamento real;
- skills como pacotes reutilizáveis de contexto/instrução;
- documentação atualizada como parte do produto.

Evitar:

- frameworks pesados no core;
- abstrações excessivas;
- dependências sem valor claro;
- transformar o projeto em SPA pesada;
- implementar features que não mudam comportamento real;
- documentar comportamento futuro como se já existisse;
- deixar código e documentação divergirem.

## Arquitetura atual

Mapa completo: `docs/PROJECT_MAP.md`.

Resumo:

```text
cmd/picoclip/main.go                  # wiring e entrypoint
internal/core/domain/                 # entidades do domínio
internal/core/ports/                  # interfaces do core
internal/core/services/               # regras e orquestração
internal/adapters/storage/memory/     # storage em memória
internal/adapters/storage/sqlite/     # storage SQLite padrão
internal/adapters/events/             # event bus em memória
internal/adapters/runtimes/           # Crush, PicoClaw, Claurst
internal/adapters/web/                # HTTP, APIs, UI, HTMX, SSE
internal/adapters/web/assets/         # CSS/assets
docs/                                 # documentação canônica
workspaces/                           # projetos locais
```

## Fluxo de execução de tasks

Fluxo simplificado atual:

1. Task é criada e fica runnable (`NeedsRun=true`).
2. Scheduler roda reconciler antes do dispatcher.
3. Reconciler ativa tasks contínuas due, processa wakeups, detecta stalls e recupera locks/runs órfãos.
4. Dispatcher aguarda slot de concorrência e só então reivindica task runnable.
5. `ClaimNextRunnable` faz checkout atômico, cria run e aplica lock.
6. Runner carrega agent, runtime, skills, mensagens e prompt.
7. Runtime executa.
8. Runner salva output/error, mensagens, eventos e uso.
9. Task termina, bloqueia, cancela ou agenda próximo ciclo/retry conforme o caso.

Detalhes: `docs/ROBUSTNESS.md`.

## Comandos canônicos

Documentação completa: `docs/DEVELOPMENT.md`.

### Instalar ferramentas dev

```sh
make tools
npm install
npx playwright install chromium
```

### Rodar

```sh
make run
```

Padrão do Makefile: `0.0.0.0:8088`.

Configuração manual:

```sh
BIND=127.0.0.1 PORT=9090 go run cmd/picoclip/main.go
```

### Live reload

```sh
make dev
```

### Build

```sh
make build
```

### Validação

```sh
make test-go
make check
```

`make check` roda geração Templ, gofmt, testes Go, vet, build e E2E Playwright.

### Utilitário local deste ambiente

Existe um script local não versionado em `scripts/dev-local.sh`, ignorado pelo Git via `.gitignore`. Use quando disponível para tarefas repetitivas locais.

Comandos úteis:

```sh
scripts/dev-local.sh start
scripts/dev-local.sh stop
scripts/dev-local.sh restart
scripts/dev-local.sh status
scripts/dev-local.sh logs
scripts/dev-local.sh check
scripts/dev-local.sh full-check
scripts/dev-local.sh e2e
```

Regras:

- não commitar `scripts/dev-local.sh`;
- preferir `scripts/dev-local.sh start` para testes manuais locais na porta `8088` quando ele existir;
- antes de E2E local, confirmar `scripts/dev-local.sh status`;
- se o script não existir, seguir Makefile;
- se surgir atrito recorrente, avise antes de automatizar para decidirmos se vira comando local.

## APIs

Referência completa: `docs/API_REFERENCE.md`.

Principais grupos:

- API administrativa local: `/api/...`
- Agent API: `/agent-api/...`
- páginas web: `/`, `/projects`, `/agents`, `/tasks`, `/runs`, `/skills`, `/activity`, `/settings`
- live/partials: `/sse/...`, `/partials/...`

## Ciclo autônomo de melhorias

Melhorias recorrentes do PicoClip só podem ser executadas quando o card já possuir aprovação explícita do responsável pelo produto. Agentes e crons podem diagnosticar e recomendar, mas não podem criar features, épicos, subtarefas de implementação, mudanças de arquitetura ou dependências externas por iniciativa própria.

Melhorias aprovadas podem ser geridas pelo Hermes Kanban no board `picoclip`. Quando trabalhar nesse fluxo:

- leia `docs/AUTONOMOUS_IMPROVEMENT.md`;
- consulte o Kanban antes de escolher trabalho;
- execute no máximo uma melhoria por rodada;
- use `docs/IMPROVEMENT_BACKLOG.md` para formato de cards, categorias, deduplicação, priorização e critérios de conclusão;
- não crie cards novos sem aprovação explícita; apresente descobertas como recomendações fora da fila executável;
- antes de executar, confirme que a aprovação está registrada no card e deduplique contra itens existentes;
- comente início/fim, validações e commit no card;
- use o formato de relatório operacional de `docs/AUTONOMOUS_IMPROVEMENT.md` ao finalizar ciclos cron;
- pause o cron autônomo durante execução manual no mesmo workspace e reative ao final.

## Primeiros 15 minutos para agentes

Use esta trilha curta antes de qualquer mudança. Ela reduz contexto inicial sem substituir os documentos canônicos.

1. Leia este `AGENTS.md`, depois `docs/PROJECT_MAP.md` e `docs/DOCUMENTATION_POLICY.md`.
2. Leia o documento da área que será alterada: `docs/API_REFERENCE.md`, `docs/ROBUSTNESS.md`, `docs/STORAGE.md`, `docs/DESIGN.md`, `docs/OPERATIONS.md` ou `docs/DEVELOPMENT.md`.
3. Rode `git status --short` e preserve arquivos locais/não rastreados; neste ambiente, nunca adicione `graphify-out/`.
4. Se estiver em melhoria autônoma, leia `docs/AUTONOMOUS_IMPROVEMENT.md`, consulte o Kanban `picoclip` e escolha no máximo um card `ready`.
5. Antes de editar, leia os arquivos e testes relevantes. Para mudanças de comportamento, siga TDD: teste RED, implementação mínima, GREEN e refatoração pequena.
6. Atualize documentação proporcional quando mudar API, UI, storage, runtime, robustez, comandos, operação ou onboarding.
7. Valide com teste focado e com a validação canônica proporcional (`make check-docs`, `make test-go` ou `make check`) antes de commitar.
8. Finalize reportando exatamente o que mudou, comandos reais de validação, commit/push, estado do servidor e pendências.

## Storage

- SQLite é o padrão (`PICOCLIP_STORAGE=sqlite`).
- Memory storage é para testes/sessões temporárias (`PICOCLIP_STORAGE=memory`).
- SQLite usa `modernc.org/sqlite` para manter build sem CGO.
- Migrations ficam em `internal/adapters/storage/sqlite/migrations.go`.

Detalhes: `docs/STORAGE.md`.

## Capacidades e permissões

Capabilities atuais:

- `observer`
- `worker`
- `coordinator`
- `operator`
- `administrator`

Permissões vivem em `internal/core/domain/agent.go` e presets em `internal/core/services/capabilities.go`.

Importante: permissões existem no modelo e há enforcement parcial em fluxos da Agent API, mas a cobertura total ainda está em evolução. Ao alterar permissões, confira `Authorizer`, handlers e documentação.

## Política de documentação obrigatória

Antes de finalizar qualquer tarefa, verifique `docs/DOCUMENTATION_POLICY.md`.

Resumo prático:

- Nova API ou payload: atualize `docs/API_REFERENCE.md`.
- Novo módulo/fluxo/página/adapter: atualize `docs/PROJECT_MAP.md`.
- Mudança em scheduler/runner/retry/cancel/recovery: atualize `docs/ROBUSTNESS.md` e, se necessário, `.pt-BR.md`.
- Mudança em schema/repository/migration: atualize `docs/STORAGE.md`.
- Mudança de UI/componentes/HTMX: atualize `docs/DESIGN.md`.
- Mudança de comandos/workflow dev: atualize `docs/DEVELOPMENT.md`.
- Mudança macro de produto: atualize `docs/CURRENT_STATE.md` e `docs/ROADMAP.md`.
- Mudança que afeta onboarding: atualize `README.md`, `README.pt-BR.md` e este `AGENTS.md`.

## Cuidados com Git e arquivos locais

Antes de editar, confira:

```sh
git status --short
```

Não sobrescreva trabalho não rastreado sem confirmação. Neste ambiente, screenshots, bancos SQLite, binários, logs, `tmp/`, `node_modules/` e helpers locais podem existir e não devem ser commitados acidentalmente.

## Checklist final para agentes

- [ ] Li `docs/PROJECT_MAP.md` e o doc da área alterada.
- [ ] Preservei arquivos não relacionados e não rastreados.
- [ ] Atualizei documentação proporcional à mudança.
- [ ] Rodei validação real e reportei o comando.
- [ ] Não documentei plano futuro como estado atual.
- [ ] Não adicionei dependência pesada sem necessidade clara.
- [ ] Não quebrei a filosofia local-first/leve do projeto.
