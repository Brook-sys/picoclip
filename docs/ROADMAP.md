# PicoClip — Roadmap Técnico e Produto

## Objetivo

Transformar PicoClip em uma alternativa leve ao Paperclip, com foco em orquestração local de agentes, projetos, skills, permissões, tarefas, delegação e operação via UI/API.

## Princípios

1. Core pequeno e desacoplado.
2. Storage, drivers, memória, segredos e integrações como adaptadores.
3. UI profissional, mas leve: server-rendered + HTMX.
4. Permissões e capacidades precisam mudar comportamento real, não apenas aparecer na tela.
5. Skills devem ser pacotes reais de conhecimento/ações, não apenas texto solto.
6. Projetos devem isolar contexto, arquivos, agentes, tarefas e skills.
7. Agentes devem conseguir entender e operar o próprio PicoClip via APIs documentadas.

## Fase 0 — Estado atual

Status: em andamento, parcialmente implementado.

Entregue:

- Core/adapters.
- Memory storage.
- Engine básico.
- Agentes.
- Tarefas.
- Runs.
- Mensagens.
- Delegação.
- Projetos com pasta.
- Skills built-in/custom.
- Capacidades predefinidas.
- API para agentes.
- UI separada por páginas.

Ainda frágil:

- persistência;
- permissões aplicadas;
- cancelamento forte;
- logs em tempo real;
- detalhes de entidades;
- skill packages completos.

## Fase 1 — Produto utilizável localmente

Prioridade: alta.

### 1.1 Persistência SQLite

Entregar:

- adapter `internal/adapters/storage/sqlite`;
- schema/migrations;
- configuração por env;
- persistência de todas entidades;
- testes básicos.

Critério de aceite:

- criar projeto/agente/tarefa/skill;
- reiniciar servidor;
- dados continuam existindo.

### 1.2 Permissões reais

Entregar:

- identidade de agente para `/agent-api`;
- middleware de permissão;
- checagem nos endpoints de ação;
- eventos de auditoria.

Critério de aceite:

- Observer não consegue criar/delegar/cancelar;
- Coordinator consegue delegar;
- Operator consegue cancelar;
- Administrator consegue gerir agentes/skills.

### 1.3 Detalhes de entidade

Entregar páginas:

- `/projects/{id}`;
- `/agents/{id}`;
- `/tasks/{id}`;
- `/skills/{id}`.

Critério de aceite:

- tarefa mostra mensagens, runs, eventos, subtarefas e output;
- projeto mostra agentes, tarefas, skills e pasta;
- agente mostra capability, permissões, skills e tarefas;
- skill mostra instruções e arquivos.

### 1.4 Cancelamento real

Entregar:

- controle de execução ativa;
- cancel func por task/run;
- driver recebe context cancelável;
- cancelar interrompe subprocesso.

Critério de aceite:

- tarefa running muda para canceled rapidamente;
- processo externo é encerrado;
- run fica canceled.

## Fase 2 — Workspace de agentes

Prioridade: alta/média.

### 2.1 Projeto como contexto real

Entregar:

- driver executando com cwd no projeto;
- endpoint para listar arquivos do projeto;
- endpoint para ler arquivos permitidos;
- contexto do projeto no prompt;
- arquivos `.picoclip/context.md` ou similar.

Critério de aceite:

- agente consegue descobrir arquivos do projeto via API;
- agente recebe contexto do projeto automaticamente.

### 2.2 Skills como pacotes

Entregar:

- skill manifest;
- múltiplos arquivos;
- edição de skill;
- import/export de diretório;
- atribuição de skills a agentes;
- validação de paths.

Formato sugerido:

```text
skills/
  my-skill/
    skill.json
    SKILL.md
    references/
    scripts/
    examples/
```

Critério de aceite:

- criar skill com múltiplos arquivos;
- atribuir a agente;
- agente recebe todos os arquivos relevantes no prompt/contexto.

### 2.3 Interoperabilidade melhor

Entregar:

- comandos estruturados;
- protocolo de agent actions;
- parent task sabe subtarefas;
- resumo de subtarefas volta ao parent;
- conversa entre agentes visível.

Critério de aceite:

- agente coordinator delega subtarefa;
- subtarefa aparece no parent;
- conclusão da subtarefa pode ser incorporada ao parent.

## Fase 3 — Operação e observabilidade

Prioridade: média.

### 3.1 Logs e eventos em tempo real

Entregar:

- event store consistente;
- bus fan-out;
- SSE;
- timeline por tarefa/projeto/agente;
- logs incrementais.

Critério de aceite:

- UI atualiza sem polling destrutivo;
- output aparece progressivamente.

### 3.2 Execução robusta

Entregar:

- retry;
- requeue;
- max attempts configurável;
- timeout correto;
- status detalhados;
- stderr/stdout separados.

### 3.3 Drivers

Entregar:

- Picoclaw driver;
- Crush driver robusto;
- driver config por agente;
- seleção de cwd/env;
- logs streaming.

## Fase 4 — Integrações

Prioridade: média/baixa.

Possíveis integrações:

- Sops como secret provider;
- Engram como memory provider;
- Beads como task graph/issue graph;
- Gentle-AI como referência de comportamento/UX;
- outros CLIs locais.

## Fase 5 — Qualidade e distribuição

Entregar:

- testes unitários;
- testes HTTP;
- testes de storage;
- testes de runner;
- testes de permissões;
- release build;
- documentação de instalação;
- exemplos de uso.

## Backlog detalhado

### UI

- Melhorar visual com design system consistente.
- Adicionar breadcrumbs.
- Adicionar busca/filtros.
- Adicionar feedback de ação.
- Adicionar confirmação para delete/cancel.
- Adicionar edição inline ou telas de edição.
- Evitar polling substituir formulários abertos.

### API

- Criar OpenAPI.
- Documentar schemas.
- Padronizar erros JSON.
- Separar API pública/admin de agent-api.
- Criar autenticação local simples.
- Expor settings headless para prompts, env vars e adapters.

### Prompts e adapters

- Separar prompts de sistema em arquivos dedicados.
- Implementar cascata de prompts: global, projeto, adapter, permissões, agente, skills e task.
- Permitir edição de prompt global pela UI/API.
- Permitir prompt custom opcional na criação/edição de agente.
- Implementar variáveis de ambiente globais, por projeto, por adapter e por agente.
- Criar settings globais por adapter, começando por Crush: binário, args padrão, timeout, cwd strategy e env.

### Segurança

- Enforce de permissões.
- Path traversal protection em skills/projetos.
- Não logar segredos.
- Limitar leitura de arquivos.
- Auditar ações destrutivas.

### Storage

- SQLite.
- Migrações.
- Backup/export.
- Import.

### Agentes

- Modelo permission-first no lugar de presets rígidos.
- Permissões explícitas por agente: leitura, escrita, criar tasks, delegar, cancelar, criar agentes, gerenciar skills, settings e adapters.
- Permission skills embutidas e editáveis para ensinar o agente a usar cada permissão.
- Reset de permission skills embutidas para o valor padrão.
- Agent profile mais rico.
- Agent memory.
- Agent environment variables.
- Prompt custom opcional por agente.
- Configuração por adapter/agente.

### Tarefas

- Prioridade real.
- Fila por projeto/agente.
- Cancelamento forte.
- Retry.
- Subtasks agregadas.
- Dependencies.

### Skills

- Seguir o padrão Agent Skills (`SKILL.md` com YAML frontmatter, `scripts/`, `references/`, `assets/`).
- Múltiplos arquivos.
- Upload/import.
- Manifest derivado de `SKILL.md`.
- Versionamento.
- Validação de nome, descrição, frontmatter e paths.
- Teste de skill.
- Catálogos/lojas de skills como ClawHub, skill.sh e outros registries compatíveis.
- Instalação de skill em um clique pela UI.
- Atualização/removal de skills instaladas por loja.
- Política de uso por agente, tipo de agente ou permissão.

## Ordem recomendada imediata

1. Atualizar `AGENTS.md` com arquitetura real.
2. Garantir que servidor atual foi reiniciado com a UI separada.
3. Implementar SQLite.
4. Implementar enforcement de permissões.
5. Criar páginas de detalhe.
6. Fortalecer cancelamento.
7. Evoluir skill packages.
