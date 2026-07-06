# Documentação do PicoClip

Este diretório contém a documentação canônica do PicoClip. Use este índice para escolher o documento certo antes de alterar código, operar o sistema ou orientar um agente.

## Comece por aqui

| Documento | Para que serve |
| --- | --- |
| [Project Map](PROJECT_MAP.md) | Mapa geral do projeto: diretórios, módulos, fluxos e onde mexer. |
| [Documentation Policy](DOCUMENTATION_POLICY.md) | Política obrigatória para manter documentação junto com novas funcionalidades. |
| [Development Guide](DEVELOPMENT.md) | Manual de desenvolvimento local, comandos, testes e troubleshooting. |
| [Operations Runbook](OPERATIONS.md) | Guia operacional para diagnóstico, recovery, backup/restore e incidentes locais. |
| [Current State](CURRENT_STATE.md) | Estado real atual do projeto, capacidades implementadas e limitações. |
| [Roadmap](ROADMAP.md) | Plano de evolução e prioridades por fase. |

## Por área

| Área | Documento |
| --- | --- |
| APIs administrativas, Agent API e rotas web | [API Reference](API_REFERENCE.md) |
| Operação local, incidentes, diagnostics, backup/restore e runbooks | [Operations Runbook](OPERATIONS.md) |
| Robustez, locks, retry, wakeups, recovery e cancelamento | [Robustness](ROBUSTNESS.md) / [pt-BR](ROBUSTNESS.pt-BR.md) |
| SQLite, memory storage, migrations, backup/restore | [Storage Architecture](STORAGE.md) |
| UI, Templ, HTMX, componentes e design system | [Design System](DESIGN.md) |
| Alinhamento estratégico com Paperclip | [Paperclip Alignment](PAPERCLIP_ALIGNMENT.md) |
| Plano histórico de tasks contínuas | [Continuous Tasks Implementation Plan](CONTINUOUS_TASKS_IMPLEMENTATION_PLAN.md) |

## Trilhas de leitura

### Quero contribuir com código

1. [Project Map](PROJECT_MAP.md)
2. [Development Guide](DEVELOPMENT.md)
3. [Documentation Policy](DOCUMENTATION_POLICY.md)
4. Documento da área que você vai alterar.

### Quero entender a arquitetura

1. [Current State](CURRENT_STATE.md)
2. [Project Map](PROJECT_MAP.md)
3. [Storage Architecture](STORAGE.md)
4. [Robustness](ROBUSTNESS.md)
5. [Paperclip Alignment](PAPERCLIP_ALIGNMENT.md)

### Quero operar/debugar localmente

1. [Operations Runbook](OPERATIONS.md)
2. [Development Guide](DEVELOPMENT.md)
3. [API Reference](API_REFERENCE.md)
4. [Robustness](ROBUSTNESS.md)
5. [Storage Architecture](STORAGE.md)

### Sou um agente de IA trabalhando no repo

1. Leia `../AGENTS.md`.
2. Leia [Project Map](PROJECT_MAP.md).
3. Leia [Documentation Policy](DOCUMENTATION_POLICY.md).
4. Leia o documento da área que será modificada.
5. Atualize documentação junto com qualquer mudança de comportamento.

## Regra de manutenção

Sempre que um documento novo for criado em `docs/`, adicione-o a este índice. Sempre que um documento for renomeado, ajuste os links aqui, no README raiz e no `AGENTS.md` quando aplicável.
