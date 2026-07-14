# PicoClip Development Guide

Este guia descreve o workflow local repetível para desenvolver, testar, depurar e validar o PicoClip. Para orientação arquitetural, leia também:

- [Project Map](PROJECT_MAP.md)
- [Current State](CURRENT_STATE.md)
- [Documentation Policy](DOCUMENTATION_POLICY.md)
- [API Reference](API_REFERENCE.md)
- [Robustness](ROBUSTNESS.md)
- [Storage Architecture](STORAGE.md)
- [Design System](DESIGN.md)
- [Autonomous Improvement](AUTONOMOUS_IMPROVEMENT.md)

## Pré-requisitos

Obrigatórios:

- Go compatível com o `go.mod`;
- Git;
- shell POSIX;
- Node/npm para Playwright E2E.

Ferramentas instaladas pelo projeto:

```sh
make tools
```

Isso cria wrappers locais em `bin/` para:

- `templ`;
- `air`.

Playwright:

```sh
npm install
npx playwright install chromium
```

Em Alpine, o Chromium baixado pelo Playwright pode não rodar por incompatibilidade de libs. Se necessário:

```sh
apk add chromium
PLAYWRIGHT_CHROMIUM_EXECUTABLE=/usr/bin/chromium make test-e2e
```

## Estrutura do projeto

Resumo operacional:

| Caminho | Finalidade |
| --- | --- |
| `cmd/picoclip/main.go` | Entrypoint e wiring. |
| `internal/core/domain/` | Entidades, enums e erros do domínio. |
| `internal/core/ports/` | Interfaces do core. |
| `internal/core/services/` | Regras de aplicação, scheduler, runner, recovery. |
| `internal/adapters/storage/memory/` | Storage em memória. |
| `internal/adapters/storage/sqlite/` | SQLite, migrations e repositories. |
| `internal/adapters/storage/storagetest/` | Contract tests de storage. |
| `internal/adapters/runtimes/` | Runtime adapters. |
| `internal/adapters/web/` | Rotas, handlers, APIs, Templ, HTMX e SSE. |
| `internal/adapters/web/assets/` | CSS/assets. |
| `scripts/seed/` | Seed de dados demo. |
| `scripts/run-e2e.sh` | Runner Playwright. |
| `e2e/` | Testes E2E. |
| `docs/` | Documentação canônica. |

Mapa completo: [Project Map](PROJECT_MAP.md).

## Configuração de runtime

Defaults principais:

```sh
BIND=127.0.0.1
PORT=8080      # padrão do binário
PORT=8088      # padrão dos comandos do Makefile
PICOCLIP_STORAGE=sqlite
PICOCLIP_DB_PATH=data/picoclip.db
PICOCLIP_WORKSPACES=workspaces
PICOCLIP_RUNTIMES=data/runtimes
PICOCLIP_LOG_LEVEL=info
PICOCLIP_DEBUG=false
BWRAP_PATH=bwrap  # Bubblewrap para o runtime sandbox opt-in
```

Rodar manualmente:

```sh
BIND=127.0.0.1 PORT=8088 go run cmd/picoclip/main.go
```

Runtimes podem ser sobrescritos com:

```sh
CRUSH_PATH=/path/to/crush \
PICOCLAW_PATH=/path/to/picoclaw \
CLAURST_PATH=/path/to/claurst \
go run cmd/picoclip/main.go
```

Use `PICOCLIP_STORAGE=memory` somente para sessões temporárias e testes. SQLite é o modo normal local-first.

## Comandos canônicos

```sh
make help
make tools
make run
make dev
make seed
make test-go
make test-docker-entrypoint
make test-e2e
make test-e2e-headed
make check-docs
make vet
make fmt
make build
make check
make clean
make kill-8088
```

### Rodar aplicação

```sh
make run
```

Equivalente:

```sh
BIND=127.0.0.1 PORT=8088 go run cmd/picoclip/main.go
```

### Live reload

```sh
make dev
```

Equivalente:

```sh
BIND=127.0.0.1 PORT=8088 ./bin/air -c .air.toml
```

Air builda em `tmp/picoclip` e reinicia quando Go, CSS, JS, HTML ou Templ mudam.

### Seed de dados demo

Com o servidor rodando:

```sh
make seed
```

Isso executa `scripts/seed/main.go` usando `scripts/seed/scenarios/full.json` contra `BASE_URL`.

## Segurança do repositório

O repositório público usa controles versionados e configurações do GitHub em conjunto:

- `.github/dependabot.yml` mantém dependências Go, npm, GitHub Actions e Docker atualizadas;
- `SECURITY.md` direciona vulnerabilidades para reporte privado;
- `.github/CODEOWNERS`, templates de PR e de issue formalizam revisão e triagem;
- `CONTRIBUTING.md` descreve o fluxo de contribuição e deixa explícito que features e mudanças de arquitetura exigem aprovação prévia;
- o repositório ainda não declara licença; a escolha deve ser feita pelo mantenedor antes de conceder permissões claras de reutilização;
- o ruleset `Protect main` exige pull request, o check `Check`, branch atualizada e conversas resolvidas, além de bloquear force-push e exclusão da `main`;
- Secret Scanning, Push Protection, Dependabot Security Updates e CodeQL devem permanecer habilitados.

Para mudanças em workflows ou políticas, preserve permissões mínimas do `GITHUB_TOKEN`, não inclua segredos e valide YAML mais `make check-docs`. Alertas CodeQL e Dependabot precisam ser triados; um scanner verde não substitui revisão da fronteira de confiança.

## Workflow de templates Templ

O projeto usa `github.com/a-h/templ`.

Gerar:

```sh
make templ-generate
```

Equivalente:

```sh
./bin/templ generate
```

Notas:

- arquivos `.templ` são a fonte;
- arquivos `*_templ.go` são gerados;
- rode geração antes de build/check quando templates mudarem;
- evite editar manualmente arquivos gerados, exceto se o projeto estiver temporariamente sem `.templ` correspondente e isso for explicitamente decidido.

### Validação de documentação

Para validar links Markdown locais, incluindo fragmentos de âncora como
`docs/DOCUMENTATION_POLICY.md#matriz-de-validação-mínima-por-tipo-de-mudança`, rode:

```sh
make check-docs
```

O comando usa `scripts/check_markdown_links.py`, sem dependências externas, e cobre
`README.md`, `README.pt-BR.md`, `AGENTS.md` e `docs/*.md`. Ele também roda como a
primeira etapa de `make check`.

## Build e validação

Validação rápida Go:

```sh
make test-go
```

Validação manual por etapas:

```sh
make fmt
make test-go
make vet
make build
```

Validação completa:

```sh
make check
```

`make check` roda:

1. `make check-docs`;
2. `templ generate`;
3. `gofmt -w cmd internal`;
4. `go test ./...`;
5. teste do entrypoint Docker contra volume legado com ownership incompatível;
6. `go vet ./...`;
7. `go build -o picoclip cmd/picoclip/main.go`;
8. Playwright E2E.

Use `make check` antes de considerar uma mudança relevante concluída, especialmente se ela toca UI, APIs, scheduler, storage, runtimes ou documentação com comandos. Para escolher o mínimo proporcional por tipo de mudança, consulte a matriz em [Documentation Policy](DOCUMENTATION_POLICY.md#matriz-de-validação-mínima-por-tipo-de-mudança).

## Ciclo autônomo via Hermes Kanban

O PicoClip pode ser melhorado por um ciclo autônomo do Hermes que consulta o board Kanban `picoclip`, cria novas demandas objetivas e executa uma melhoria pequena por rodada.

Política completa: [Autonomous Improvement](AUTONOMOUS_IMPROVEMENT.md). O contrato de backlog — categorias, formato de cards, deduplicação, priorização e critérios de conclusão — fica em [Improvement Backlog](IMPROVEMENT_BACKLOG.md).

Resumo operacional:

```sh
hermes kanban boards switch picoclip
hermes kanban list --tenant picoclip --sort priority-desc
hermes kanban stats
```

Regras principais:

- o Kanban é a fila operacional; roadmap/docs continuam sendo referência estratégica;
- cada rodada executa no máximo um card;
- cada rodada cria no máximo três cards novos;
- não iniciar nova melhoria se `git status --short` mostrar trabalho rastreado não relacionado;
- pausar o cron autônomo durante execução manual no mesmo workspace;
- validar, revisar diff/segredos, commitar, pushar e atualizar o card antes de concluir;
- não adicionar `graphify-out/` ou artefatos locais ao commit.

## Testes Go

Testes ficam próximos ao pacote testado. Locais importantes:

```text
internal/adapters/web/server_test.go
internal/core/services/*_test.go
internal/adapters/storage/*/*_test.go
internal/adapters/runtimes/*_test.go
```

Rodar todos:

```sh
go test ./...
```

Rodar pacote de services:

```sh
go test ./internal/core/services -count=1
```

Rodar testes focados de robustez:

```sh
go test ./internal/core/services -run 'TestReconciler|TestStalledRun|TestDispatcher|TestLockRecovery' -count=1
```

Rodar storage contract tests:

```sh
go test ./internal/adapters/storage/... -count=1
```

Os testes cobrem, entre outros:

- Agent task lifecycle API;
- checkout/release/block/comment/reopen;
- contratos HTMX de task detail;
- stale lock recovery;
- stalled run timeout;
- retry wakeups e metadata;
- orphaned run recovery;
- paridade memory/SQLite;
- runtime process cancellation em Unix;
- diagnostics.

## E2E Playwright

Rodar E2E usando o script do projeto:

```sh
make test-e2e
```

Headed:

```sh
make test-e2e-headed
```

O E2E atual cobre:

- páginas principais sem erros de console/requisição;
- criação de agent/task via UI;
- estabilidade de task detail durante polling HTMX;
- command palette;
- diagnostics API;
- Agent API com lifecycle Paperclip-like;
- factory reset.

Artifacts:

```text
e2e/test-results/
e2e/playwright-report/
```

Esses artifacts não devem ser commitados.

## Helper local deste ambiente

Neste ambiente existe um script local não versionado:

```sh
scripts/dev-local.sh
```

Ele é ignorado pelo Git e deve permanecer local.

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

Use quando disponível para tarefas repetitivas locais. Se ele não existir em outro ambiente, use o Makefile.

## Qual validação rodar por tipo de mudança

| Mudança | Validação mínima | Validação recomendada |
| --- | --- | --- |
| Docs apenas | check de links internos | `go test ./...` se docs referem comandos/contratos |
| Handler/API | teste do pacote web | `make check` |
| UI/Templ/HTMX | `make templ-generate`, testes web | `make test-e2e` e `make check` |
| Scheduler/dispatcher/runner/reconciler | teste focado de services | `make check` |
| Storage/migrations | storage contract tests | `make check` |
| Runtime adapters/cancelamento | runtime tests focados | `make check` |
| Makefile/scripts/dev workflow | comando alterado real | `make check` se possível |

## Regras de TDD para mudanças comportamentais

Para bugfixes e mudanças de comportamento:

1. Escreva teste de regressão primeiro.
2. Rode o teste e confirme falha pelo motivo esperado.
3. Implemente a menor correção.
4. Rode o teste focado e confirme sucesso.
5. Rode suíte mais ampla proporcional.
6. Atualize documentação quando o comportamento mudar.

Não corrija bug de scheduler, retry, storage, API ou runtime sem teste de regressão, salvo decisão explícita.

## HTMX quality rules

Evite polling que troca a página inteira.

Ruim:

```html
<section hx-get="/tasks/{id}" hx-trigger="every 2s" hx-target="body" hx-swap="outerHTML">
```

Bom:

```html
<div id="task-live" hx-get="/partials/tasks/{id}" hx-trigger="every 3s" hx-swap="innerHTML">
```

Regras:

- Poll apenas fragments pequenos.
- Mantenha forms, botões críticos e modals fora de containers com polling.
- Use rotas `/partials/...` para regiões live.
- Não re-renderize `<body>` em timers.
- Confira console do browser depois de mudanças HTMX.
- Veja também [Design System](DESIGN.md).

## Debug checklist

### UI ou handlers

1. Rode `make test-go`.
2. Inicie app com `make dev` ou `scripts/dev-local.sh start`.
3. Abra a página afetada.
4. Verifique console do browser.
5. Verifique Network tab ou falhas Playwright.
6. Rode `make test-e2e`.
7. Rode `make check` antes de finalizar.

### Retry, recovery, cancellation, scheduler, dispatcher ou runtime

1. Leia [Robustness](ROBUSTNESS.md).
2. Escreva/atualize teste de regressão antes do código.
3. Confirme falha esperada.
4. Faça a menor mudança.
5. Rode teste focado.
6. Confirme eventos/diagnostics quando aplicável.
7. Rode `make check`.

### Storage ou migrations

1. Leia [Storage Architecture](STORAGE.md).
2. Atualize domain/ports/repositories/migrations juntos.
3. Atualize memory adapter quando o contrato mudar.
4. Rode:

```sh
go test ./internal/adapters/storage/... -count=1
```

5. Rode `make check`.

### Agent API

1. Leia [API Reference](API_REFERENCE.md).
2. Confira permissões em `Authorizer` quando aplicável.
3. Atualize handler e tests.
4. Atualize docs de API.
5. Rode testes web/services e E2E se o fluxo for crítico.

## Documentação durante desenvolvimento

A política está em [Documentation Policy](DOCUMENTATION_POLICY.md).

Resumo:

- nova API: atualize [API Reference](API_REFERENCE.md);
- novo módulo/fluxo: atualize [Project Map](PROJECT_MAP.md);
- robustez: atualize [Robustness](ROBUSTNESS.md);
- storage: atualize [Storage Architecture](STORAGE.md);
- UI: atualize [Design System](DESIGN.md);
- workflow/comando: atualize este guia;
- mudança macro: atualize [Current State](CURRENT_STATE.md) e [Roadmap](ROADMAP.md).

## Check de links internos

Antes de finalizar mudanças grandes de docs:

```sh
python3 - <<'PY'
from pathlib import Path
import re
files = [Path('README.md'), Path('README.pt-BR.md'), Path('AGENTS.md'), *Path('docs').glob('*.md')]
missing = []
for f in files:
    if not f.exists():
        continue
    text = f.read_text(errors='ignore')
    for m in re.finditer(r'\]\(([^)]+)\)', text):
        link = m.group(1).strip()
        if link.startswith(('http://', 'https://', 'mailto:')):
            continue
        path = link.split('#', 1)[0]
        if not path:
            continue
        target = (f.parent / path).resolve()
        if not target.exists():
            missing.append((str(f), link, str(target)))
print('missing=', missing)
raise SystemExit(1 if missing else 0)
PY
```

## Git hygiene

Antes de começar e antes de terminar:

```sh
git status --short
git diff --stat
```

Não commitar acidentalmente:

- `node_modules/`;
- `tmp/`;
- bancos em `data/`;
- logs;
- binários gerados (`picoclip`, `tmp/picoclip*`);
- Playwright artifacts;
- screenshots locais;
- `scripts/dev-local.sh`.

## Troubleshooting rápido

### Porta 8088 ocupada

```sh
make kill-8088
```

ou use outro `PORT`:

```sh
PORT=9090 make run
```

### E2E falha por browser no Alpine

```sh
apk add chromium
PLAYWRIGHT_CHROMIUM_EXECUTABLE=/usr/bin/chromium make test-e2e
```

### SQLite travando/esquisito em desenvolvimento

- confira `PICOCLIP_DB_PATH`;
- confira se outro processo está usando o mesmo DB;
- para E2E, prefira DB separado;
- para sessão descartável, use `PICOCLIP_STORAGE=memory`.

### Runtime indisponível

- verifique Settings > Adapters;
- confira `CRUSH_PATH`, `PICOCLAW_PATH`, `CLAURST_PATH`;
- confira `/api/diagnostics`;
- rode teste de runtime pela UI quando disponível.

### Mudança Templ não apareceu

```sh
make templ-generate
make build
```

ou reinicie `make dev`.

## Critério de pronto

Uma tarefa está pronta quando:

- código ou docs foram atualizados no lugar certo;
- testes/validação proporcionais foram executados;
- documentação relacionada está consistente;
- `git status --short` foi revisado;
- limitações conhecidas foram mencionadas;
- nenhum artifact local foi incluído por acidente.
