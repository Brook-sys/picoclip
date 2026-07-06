# PicoClip Agents Guide

Este guia descreve o estado atual do PicoClip e como trabalhar no repositório.

## Visão geral

PicoClip é um motor leve de orquestração local de agentes inspirado no Paperclip. O objetivo é oferecer projetos/workspaces, agentes, tarefas, runs, mensagens, delegação, capacidades, permissões, skills e APIs para que os próprios agentes interajam com o sistema.

O projeto deve permanecer simples, portátil e econômico em recursos, seguindo a filosofia Go: binário pequeno, baixo uso de RAM, dependências moderadas e arquitetura modular.

## Filosofia

Preservar sempre:

- core pequeno;
- baixo consumo de recursos;
- local-first;
- modularidade;
- drivers plugáveis;
- storage plugável;
- UI leve com server-rendered HTML + HTMX;
- APIs documentadas para agentes;
- capacidades reais, não apenas campos decorativos;
- skills como pacotes de contexto/instrução reutilizáveis.

Evitar:

- frameworks pesados no core;
- abstrações excessivas;
- dependências sem valor claro;
- transformar o projeto em uma SPA pesada;
- implementar features que não mudam comportamento real.

## Arquitetura atual

```text
cmd/picoclip/main.go
internal/core/domain
internal/core/ports
internal/core/services
internal/adapters/storage/memory
internal/adapters/events
internal/adapters/drivers
internal/adapters/web
internal/adapters/web/assets
workspaces/
docs/
```

### `cmd/picoclip`

Entrada da aplicação. Faz wiring de:

- storage em memória;
- event bus em memória;
- driver registry;
- drivers `noop` e `crush`;
- engine;
- serviços de agentes, tarefas, workspaces e skills;
- servidor web.

### `internal/core/domain`

Entidades de domínio:

- `Agent`
- `Task`
- `Run`
- `Event`
- `Message`
- `Workspace`
- `Skill`

### `internal/core/ports`

Interfaces do core:

- storage/repositories;
- driver;
- event bus;
- clock;
- id generator;
- memory provider;
- secret provider.

### `internal/core/services`

Regras de aplicação:

- criação/listagem/exclusão de agentes;
- capability presets;
- criação/listagem/cancelamento/delegação de tarefas;
- criação/listagem/remoção de skills;
- criação/listagem de workspaces;
- scheduler/dispatcher/runner;
- montagem de prompt com capacidade, skills, mensagens e APIs disponíveis.

### `internal/adapters/storage/memory`

Implementação em memória dos repositories. Dados são perdidos no restart.

Próximo grande passo recomendado: criar adapter SQLite.

### `internal/adapters/events`

Event bus em memória. Ainda não é fan-out real e deve ser refeito antes de SSE.

### `internal/adapters/drivers`

Drivers atuais:

- `noop`: driver de teste;
- `crush`: executa CLI Crush.

### `internal/adapters/web`

Servidor HTTP, API JSON, UI HTML e assets.

Páginas atuais:

- `/` Dashboard;
- `/projects` Projetos;
- `/agents` Agentes;
- `/tasks` Tarefas;
- `/skills` Skills.

## Execução de tarefas

Fluxo atual:

1. Tarefa é criada com status `pending`.
2. Scheduler chama dispatcher.
3. Dispatcher busca tarefa pendente.
4. Runner marca como `running`.
5. Runner cria `Run`.
6. Runner carrega agente, capacidade, skills e mensagens.
7. Runner monta prompt enriquecido.
8. Driver executa.
9. Runner salva output/erro e status final.

Limitações conhecidas:

- cancelamento agora sinaliza o grupo de processos do runtime, mas liveness/recovery ainda precisam ser ampliados;
- eventos não são persistidos de forma uniforme;
- concorrência do dispatcher precisa ser revista;
- logs não são streamados.

## Projetos/workspaces

Projetos são representados por `Workspace` e criam pasta em:

```text
workspaces/<project-id>/
```

Variável de ambiente:

```bash
PICOCLIP_WORKSPACES=/caminho/base
```

Se não definida, usa `workspaces`.

## Capacidades e permissões

Capacidades atuais:

- `observer`
- `worker`
- `coordinator`
- `operator`
- `administrator`

Cada capacidade gera permissões reais no modelo e no contexto do agente.

Permissões atuais:

- `agents.create`
- `agents.delete`
- `tasks.create`
- `tasks.delegate`
- `tasks.cancel`
- `skills.manage`
- `system.view`

Importante: o enforcement por endpoint ainda precisa ser implementado. Hoje as permissões já fazem parte do modelo e do prompt, mas ainda não bloqueiam todas as ações na camada HTTP.

## Skills

Skills são pacotes com:

- nome;
- descrição;
- instruções;
- arquivos opcionais;
- escopo global ou por projeto;
- tipo `builtin` ou `custom`.

Skills embutidas atuais:

- PicoClip System API;
- Delegation;
- Task Control.

Limitações:

- UI ainda só cria um arquivo opcional por skill;
- falta edição completa;
- falta atribuição visual de skills a agentes;
- falta import/export de diretórios de skill.

## APIs

### API administrativa/local

- `GET /api/agents`
- `POST /api/agents`
- `DELETE /api/agents/{id}`
- `POST /api/agents/{id}/permissions`
- `POST /api/agents/{id}/skills`
- `GET /api/tasks`
- `POST /api/tasks`
- `POST /api/tasks/{id}/cancel`
- `POST /api/tasks/{id}/messages`
- `POST /api/tasks/{id}/delegate`
- `GET /api/skills`
- `POST /api/skills`
- `PUT /api/skills/{id}`
- `DELETE /api/skills/{id}`
- `GET /api/projects`
- `POST /api/projects`
- `GET /api/capabilities`

### API para agentes

- `GET /agent-api/docs`
- `GET /agent-api/agents`
- `GET /agent-api/tasks`
- `GET /agent-api/projects`
- `GET /agent-api/skills`
- `POST /agent-api/tasks/{id}/messages`
- `POST /agent-api/tasks/{id}/delegate`
- `POST /agent-api/tasks/{id}/cancel`

## Comandos

Documentação completa: `docs/DEVELOPMENT.md`.

### Instalar ferramentas dev

```bash
make tools
npm install
npx playwright install chromium
```

### Gerar templ

O projeto atualmente não possui arquivos `.templ`, mas o comando deve permanecer no fluxo de validação:

```bash
make templ-generate
```

### Rodar

```bash
go run cmd/picoclip/main.go
```

Padrão oficial do projeto: `0.0.0.0:8088`.

Configuração:

```bash
BIND=127.0.0.1 PORT=9090 go run cmd/picoclip/main.go
```

### Utilitário local do agente

Existe um script local não versionado em `scripts/dev-local.sh`, ignorado pelo Git via `.gitignore`. Usar quando disponível para tarefas repetitivas deste ambiente.

Padrão do script local: `PORT=8088`, `BASE_URL=http://127.0.0.1:8088`, binário em `tmp/picoclip-dev` e logs em `tmp/picoclip-dev.log`.

Comandos úteis:

```bash
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
- preferir `scripts/dev-local.sh start` para testes manuais locais na porta `8088`;
- antes de E2E local, confirmar `scripts/dev-local.sh status`;
- se o script não existir, seguir os comandos padrão do Makefile;
- sempre que surgir um ponto recorrente de atrito no fluxo, tarefa repetitiva, validação manual frequente ou oportunidade clara de automação, comunicar ao usuário antes de implementar para planejarmos se deve virar comando no script shell local.

### Live reload

```bash
make dev
```

### Build

```bash
make build
```

### Validação

```bash
make check
```

Validação Go rápida:

```bash
gofmt -w cmd internal && go test ./... && go vet ./...
```

## Documentação complementar

- `docs/CURRENT_STATE.md`: documentação detalhada do estado atual, o que foi feito, limitações e riscos.
- `docs/ROADMAP.md`: roadmap técnico e produto com próximas fases.

## Próximas prioridades

1. Implementar SQLite.
2. Aplicar permissões de verdade nos endpoints.
3. Criar páginas de detalhe para projeto/agente/tarefa/skill.
4. Fortalecer cancelamento de execução.
5. Implementar logs/eventos em tempo real.
6. Evoluir skills para pacotes com múltiplos arquivos/import/export.
7. Melhorar drivers, especialmente Crush e Picoclaw.
