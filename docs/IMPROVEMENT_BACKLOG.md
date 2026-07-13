# PicoClip — Backlog Canônico de Melhorias

Este documento descreve como o backlog de melhorias do PicoClip é mantido. Ele não substitui o board Hermes Kanban; o objetivo é deixar claro o processo, os critérios de priorização e as categorias de trabalho para humanos e agentes.

Documentos relacionados:

- [Autonomous Improvement](AUTONOMOUS_IMPROVEMENT.md)
- [Roadmap](ROADMAP.md)
- [Current State](CURRENT_STATE.md)
- [Documentation Policy](DOCUMENTATION_POLICY.md)
- [Development Guide](DEVELOPMENT.md)

## Fonte operacional

A fila operacional de melhorias recorrentes vive no Hermes Kanban:

```text
board: picoclip
tenant: picoclip
```

Use o Kanban para saber o que está `ready`, `running`, `blocked` ou `done`:

```sh
hermes kanban boards switch picoclip
hermes kanban list --tenant picoclip --sort priority-desc
hermes kanban stats
```

O roadmap e o estado atual continuam sendo referências estratégicas e arquiteturais. O Kanban é a fila executável por rodada.

## O que este documento registra

Este documento registra critérios estáveis:

- como demandas entram no backlog;
- quais categorias de melhoria são esperadas;
- como deduplicar e priorizar;
- qual formato mínimo um card deve ter;
- quais validações e documentos devem acompanhar a entrega.

Ele não deve listar todos os cards atuais. Para isso, consulte o Kanban.

## Descoberta de demandas

Agentes podem identificar lacunas reais em documentação, código, testes, UI ou operação local, mas devem apresentá-las como recomendações para decisão humana.

A descoberta não autoriza criar card, épico, subtarefa, roadmap ou implementação. A entrada no backlog exige aprovação explícita do responsável pelo produto. O registro da aprovação deve constar no card antes de qualquer atribuição ou execução.

Fontes comuns:

- divergência entre documentação e comportamento real;
- TODOs ou limitações explícitas em docs/código;
- regressões potenciais sem teste;
- inconsistências visuais ou de design-system;
- endpoints agent-facing com payload excessivo, contrato ambíguo ou erro pouco acionável;
- pontos frágeis de scheduler, runner, reconciler, retry, locks ou cancelamento;
- runbooks incompletos para incidentes locais;
- storage/migrations sem paridade ou sem validação proporcional.

Antes de criar um card, procure duplicatas por título, intenção, área e arquivos prováveis em cards `ready`, `blocked` e `done`. Se a lacuna é apenas evidência adicional para um card existente, comente no card em vez de criar outro.

## Formato esperado de cards

Cada card novo deve ser pequeno, vertical e verificável. Use uma `--idempotency-key` estável para evitar duplicatas entre execuções autônomas.

Modelo recomendado:

```text
Contexto:
Por que esta melhoria importa e qual lacuna real foi observada.

Arquivos prováveis:
- caminho/arquivo.go
- docs/AREA.md

Critério de aceite:
- comportamento ou documentação esperada;
- limite de escopo;
- estado vazio/erro quando aplicável.

Validação esperada:
- teste focado;
- make templ-generate quando templates mudarem;
- make check-docs para docs;
- make check quando a mudança for relevante.

Nota de documentação:
Quais documentos devem mudar ou por que não precisam mudar.
```

Evite cards genéricos como “melhorar UI” ou “refatorar backend”. Prefira recortes observáveis, por exemplo:

- “Expor métricas agregadas de recovery no dashboard”;
- “Adicionar teste de contrato para erro 409 no checkout Agent API”;
- “Padronizar estado vazio da lista de runs filtrada”.

## Categorias atuais de melhoria

| Categoria | Foco | Exemplos de cards bons | Docs normalmente afetadas |
| --- | --- | --- | --- |
| Design-system e frontend | UI server-rendered, HTMX, Templ, responsividade, acessibilidade, estados empty/loading/error e consistência visual. | Migrar um conjunto específico de componentes para helpers Templ; adicionar feedback ARIA em uma ação icon-only; corrigir contraste em uma página crítica. | [Design System](DESIGN.md), [Project Map](PROJECT_MAP.md) quando houver página/componente novo. |
| APIs agent-facing | Agent API, payloads compactos, aliases Paperclip-like, erros estruturados, permissões e contratos JSON. | Documentar/validar um endpoint crítico; reduzir payload de contexto; adicionar envelope de erro consistente para uma ação. | [API Reference](API_REFERENCE.md), [Operations Runbook](OPERATIONS.md) quando afetar triagem por agentes. |
| Robustez de execução | Scheduler, dispatcher, runner, reconciler, locks, wakeups, retry, cancelamento e liveness. | Reproduzir uma falha com teste RED; tornar recovery observável; classificar erro determinístico como non-retryable. | [Robustness](ROBUSTNESS.md), [Robustez pt-BR](ROBUSTNESS.pt-BR.md), [Operations Runbook](OPERATIONS.md). |
| Storage e dados | SQLite, migrations, repositories, memory adapter, backup/restore e contract tests. | Adicionar paridade memory/SQLite para novo campo; validar migration idempotente; documentar restore de um caso operacional. | [Storage Architecture](STORAGE.md), [Project Map](PROJECT_MAP.md) se criar/mover módulo. |
| Operação e documentação | Runbooks, diagnóstico local, ciclo autônomo, validação, onboarding e navegação documental. | Adicionar runbook de incidente; criar lint de documentação; atualizar matriz de validação para novo tipo de mudança. | [Operations Runbook](OPERATIONS.md), [Development Guide](DEVELOPMENT.md), [Documentation Policy](DOCUMENTATION_POLICY.md), este documento. |

## Priorização

Ao escolher trabalho para uma rodada autônoma, aplique esta ordem:

1. preservar workspace limpo e artefatos locais;
2. respeitar bloqueios e dependências explícitas;
3. escolher maior prioridade no Kanban;
4. preferir escopo pequeno, reversível e com validação clara;
5. priorizar valor operacional: percepção de agentes, robustez, UX operacional e documentação acionável;
6. confirmar no código/documentação que o card ainda representa uma lacuna real.

Cada rodada deve executar no máximo um card. Se uma descoberta exigir trabalho adicional, crie follow-up específico em vez de ampliar o escopo.

## Critérios de conclusão

Um card só deve ser concluído quando houver evidência real de validação.

Checklist mínimo:

- card comentado no início com escopo e validação prevista;
- mudança pequena e vertical;
- teste focado quando houver comportamento de código;
- documentação proporcional atualizada;
- `make check-docs` para docs e `make check` para mudanças relevantes de código/UI/API/runtime/storage;
- revisão pré-commit com `git status --short`, `git diff --check` e `git diff --stat`;
- diff sem segredos e sem artefatos locais como `graphify-out/`;
- commit e push quando a validação passar;
- comentário final no Kanban com resumo, validações e commit.

## Relação com roadmap

O [Roadmap](ROADMAP.md) define direção e fases. O Kanban transforma essa direção em cards executáveis. Este documento define o contrato operacional do backlog para que novas demandas sejam pequenas, deduplicadas, rastreáveis e validadas.

Quando uma entrega muda o estado real do produto, atualize também [Current State](CURRENT_STATE.md). Quando altera comandos, APIs, UI, storage, robustez ou operação, atualize o documento de área correspondente conforme a [Documentation Policy](DOCUMENTATION_POLICY.md).
