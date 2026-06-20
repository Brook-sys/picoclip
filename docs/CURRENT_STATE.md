# PicoClip — Estado Atual

Atualizado em: 2026-06-17

## 1. Resumo executivo

PicoClip deixou de ser apenas um CRUD de agentes/tarefas e passou a ter uma base inicial de orquestração leve inspirada no Paperclip:

- núcleo em Go separado em domínio, portas e serviços;
- adaptadores para web, storage em memória, drivers e eventos;
- tarefas com runs, eventos, mensagens e delegação;
- projetos/workspaces com pastas próprias;
- agentes com capacidades predefinidas que geram permissões reais;
- skills embutidas e customizadas como pacotes de instruções e arquivos;
- APIs JSON para UI e APIs documentadas para agentes;
- UI web com páginas separadas para Dashboard, Projetos, Agentes, Tarefas e Skills.

Ainda não é um Paperclip completo. O projeto tem uma fundação melhor, mas faltam persistência real, controle robusto de execução, logs em tempo real, permissões aplicadas em todos os endpoints, skill packages mais completos e UX de produto mais refinada.

## 2. Filosofia do projeto

PicoClip deve continuar sendo uma alternativa leve ao Paperclip:

- binário pequeno;
- baixo uso de RAM;
- local-first;
- modular;
- simples de estender;
- sem frameworks pesados no core;
- agentes e drivers plugáveis;
- UI server-rendered com HTMX em vez de SPA pesada;
- integrações por adaptadores/plugins.

O objetivo não é copiar complexidade desnecessária, mas preservar capacidades essenciais de orquestração de agentes.

## 3. Estrutura atual do repositório

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
ui/
AGENTS.md
go.mod
go.sum
picoclip
```

### 3.1 `cmd/picoclip/main.go`

Responsável por:

- criar storage em memória;
- criar bus de eventos em memória;
- criar clock e gerador de IDs;
- registrar drivers `crush` e `noop`;
- iniciar engine de execução;
- criar serviços de agentes, tarefas, workspaces e skills;
- criar workspace default;
- instalar skills embutidas;
- montar servidor HTTP;
- bindar por padrão em `0.0.0.0:8080`.

Variáveis de ambiente atuais:

- `CRUSH_PATH`: caminho do binário Crush; padrão `$HOME/crush/crush`.
- `PORT`: porta HTTP; padrão `8080`.
- `BIND`: interface de bind; padrão `0.0.0.0`.
- `PICOCLIP_WORKSPACES`: pasta base dos projetos; padrão `workspaces`.

## 4. Módulo e dependências

`go.mod`:

```go
module picoclip

go 1.25.10

require github.com/a-h/templ v0.3.1020 // indirect
```

Observações:

- `templ.Component` é usado como interface de renderização.
- Não há arquivos `.templ` nem geração por CLI atualmente.
- Views são componentes Go escritos manualmente em `internal/adapters/web/views.go`.
- HTMX está vendorizado como asset em `internal/adapters/web/assets/htmx.min.js`.

## 5. Domínio atual

### 5.1 Agent

Arquivo: `internal/core/domain/agent.go`

Campos principais:

- `ID`
- `ProjectID`
- `Name`
- `Type`
- `Description`
- `Enabled`
- `Capability`
- `Permissions`
- `SkillIDs`
- `Config`
- timestamps

Tipos relacionados:

- `AgentType`
- `AgentCapability`
- `AgentPermission`

Capacidades atuais:

- `observer`
- `worker`
- `coordinator`
- `operator`
- `administrator`

Permissões atuais:

- `agents.create`
- `agents.delete`
- `tasks.create`
- `tasks.delegate`
- `tasks.cancel`
- `skills.manage`
- `system.view`

Estado real:

- capacidades geram permissões por preset;
- permissões entram no contexto passado ao agente;
- ainda falta enforcement real em todos os endpoints e comandos executados pelo agente.

### 5.2 Task

Arquivo: `internal/core/domain/task.go`

Campos principais:

- `ID`
- `ParentID`
- `WorkspaceID`
- `AgentID`
- `Title`
- `Prompt`
- `Status`
- `Priority`
- `Attempts`
- `MaxAttempts`
- `CancelReason`
- timestamps de criação, atualização, início e fim

Status atuais:

- `pending`
- `queued`
- `running`
- `succeeded`
- `failed`
- `canceled`
- `timeout`

Estado real:

- tarefa pode ser criada;
- tarefa pode ser delegada criando subtarefa;
- tarefa pode receber mensagens;
- tarefa pode ser cancelada no storage;
- cancelamento não mata imediatamente processo externo já iniciado pelo driver, apenas impede início ou marca após retorno quando detectado.

### 5.3 Run

Arquivo: `internal/core/domain/run.go`

Representa uma tentativa de execução de uma tarefa.

Status atuais:

- `running`
- `succeeded`
- `failed`
- `canceled`
- `timeout`

Estado real:

- runs são criadas quando uma tarefa começa;
- output/erro final é salvo no run;
- ainda não há streaming de logs;
- ainda não há retry avançado.

### 5.4 Event

Arquivo: `internal/core/domain/event.go`

Eventos existentes:

- `agent.created`
- `task.created`
- `task.queued`
- `task.started`
- `task.completed`
- `task.failed`
- `task.canceled`
- `run.started`
- `run.output`
- `run.completed`
- `run.failed`
- `driver.missing`
- `task.delegated`
- `message.created`

Estado real:

- eventos existem no domínio;
- alguns são persistidos;
- muitos são apenas publicados no bus;
- precisa padronizar persistência e publicação;
- falta timeline detalhada por tarefa na UI.

### 5.5 Message

Arquivo: `internal/core/domain/message.go`

Papéis atuais:

- `user`
- `agent`
- `system`
- `delegated`

Estado real:

- mensagens podem ser adicionadas a uma tarefa;
- mensagens entram no prompt da execução;
- UI tem formulário de comando por tarefa;
- falta tela de detalhe com conversa completa e histórico.

### 5.6 Workspace / Project

Arquivo: `internal/core/domain/workspace.go`

Campos:

- `ID`
- `Name`
- `Description`
- `RootPath`
- timestamps

Estado real:

- projetos são criados como workspaces;
- cada projeto cria uma pasta em `workspaces/<id>`;
- tarefa e agente podem pertencer a projeto;
- ainda falta contexto documental por projeto, arquivos indexados, memória e isolamento forte.

### 5.7 Skill

Arquivo: `internal/core/domain/skill.go`

Campos:

- `ID`
- `ProjectID`
- `Name`
- `Description`
- `Instructions`
- `Files`
- `Kind`
- `Enabled`
- timestamps

Tipos:

- `SkillKindBuiltin`
- `SkillKindCustom`
- `SkillFile`

Estado real:

- skills embutidas são instaladas no boot;
- skills customizadas podem ser criadas pela UI;
- skill pode ter um arquivo opcional com path e conteúdo;
- skills entram no contexto do agente;
- ainda falta edição completa pela UI, múltiplos arquivos, import/export, versionamento e atribuição refinada por agente.

## 6. Portas atuais

Arquivo: `internal/core/ports/storage.go`

Interfaces:

- `Storage`
- `AgentRepository`
- `TaskRepository`
- `RunRepository`
- `EventRepository`
- `MessageRepository`
- `SkillRepository`
- `WorkspaceRepository`

Outras portas:

- `Driver`
- `EventBus`
- `Clock`
- `IDGenerator`
- `MemoryProvider`
- `SecretProvider`

Estado real:

- core depende de interfaces;
- storage atual é em memória;
- isso permite criar SQLite depois sem mexer no core.

## 7. Serviços atuais

### 7.1 AgentService

Arquivo: `internal/core/services/agent_service.go`

Faz:

- criar agentes;
- listar agentes;
- buscar agente;
- atualizar permissões;
- atualizar capacidade;
- atualizar skills associadas;
- excluir agente.

Estado real:

- criação usa capacidade predefinida;
- capacidade gera permissões;
- UI agora escolhe capacidade, não permissões soltas;
- ainda falta impedir exclusão de agente com tarefas ativas ou definir política clara.

### 7.2 CapabilityPresets

Arquivo: `internal/core/services/capabilities.go`

Presets atuais:

#### Observer

- Consulta sistema sem modificar.
- Permissão: `system.view`.

#### Worker

- Executa tarefas e cria tarefas dentro do escopo recebido.
- Permissões: `system.view`, `tasks.create`.

#### Coordinator

- Cria tarefas e delega subtarefas.
- Permissões: `system.view`, `tasks.create`, `tasks.delegate`.

#### Operator

- Cria, delega e cancela tarefas problemáticas.
- Permissões: `system.view`, `tasks.create`, `tasks.delegate`, `tasks.cancel`.

#### Administrator

- Administra agentes, skills e tarefas.
- Permissões: `system.view`, `tasks.create`, `tasks.delegate`, `tasks.cancel`, `agents.create`, `agents.delete`, `skills.manage`.

Estado real:

- presets são reais no modelo e no prompt;
- ainda falta enforcement nas APIs quando chamadas por agentes.

### 7.3 TaskService

Arquivo: `internal/core/services/task_service.go`

Faz:

- criar tarefa;
- criar tarefa em workspace;
- criar subtarefa;
- listar tarefas;
- buscar tarefa;
- buscar runs;
- adicionar mensagem;
- delegar tarefa;
- cancelar tarefa;
- buscar mensagens.

Estado real:

- delegação cria subtarefa com `ParentID`;
- cancelamento atualiza status para `canceled`;
- mensagem de delegação é registrada no parent;
- falta coordenação automática parent/child.

### 7.4 SkillService

Arquivo: `internal/core/services/skill_service.go`

Faz:

- instalar skills embutidas;
- criar skill customizada;
- criar skill com arquivos;
- listar skill;
- buscar skill;
- atualizar skill;
- deletar skill customizada.

Skills embutidas atuais:

- `PicoClip System API`
- `Delegation`
- `Task Control`

Estado real:

- builtins não podem ser deletadas;
- custom pode ser deletada;
- ainda falta edição pela UI e atribuição visual a agentes.

### 7.5 WorkspaceService

Arquivo: `internal/core/services/workspace_service.go`

Faz:

- criar workspace/projeto;
- garantir projeto default;
- listar;
- buscar;
- excluir.

Estado real:

- cria pasta do projeto;
- não indexa conteúdo da pasta;
- não cria metadados persistentes no disco.

### 7.6 Runner

Arquivo: `internal/core/services/runner.go`

Faz:

- pega tarefa;
- evita rodar se já está cancelada;
- marca como running;
- cria run;
- carrega agente;
- resolve driver;
- monta prompt com:
  - contexto de capacidade;
  - APIs disponíveis;
  - skills do agente;
  - arquivos das skills;
  - tarefa do usuário;
  - mensagens da conversa;
- executa driver;
- salva output ou erro;
- detecta cancelamento após retorno;
- salva status final.

Limitações:

- cancelamento não interrompe subprocesso imediatamente;
- eventos não são persistidos de forma uniforme;
- timeout vira run timeout, mas task ainda pode acabar como failed em alguns fluxos;
- não há streaming;
- input do run fica com prompt original, enquanto driver recebe prompt enriquecido via `task.Prompt` mutado localmente.

### 7.7 Dispatcher/Scheduler/Engine

Arquivos:

- `dispatcher.go`
- `scheduler.go`
- `engine.go`

Estado real:

- engine roda scheduler;
- scheduler chama dispatcher em intervalo;
- dispatcher busca pending e executa;
- houve mudança para execução síncrona anteriormente para reduzir estranheza de UI;
- isso limita concorrência real e precisa ser revisto.

## 8. Adaptadores atuais

### 8.1 Storage em memória

Pasta: `internal/adapters/storage/memory`

Repos:

- agentes;
- tarefas;
- runs;
- eventos;
- mensagens;
- skills;
- workspaces.

Estado real:

- dados somem ao reiniciar;
- bom para MVP/teste manual;
- ruim para uso real;
- SQLite é prioridade alta.

### 8.2 Event bus em memória

Arquivo: `internal/adapters/events/inmemory.go`

Estado real:

- publica eventos em canal;
- subscribe atual não é fan-out real;
- múltiplos subscribers competiriam pelos eventos;
- precisa reimplementar para SSE/event stream.

### 8.3 Drivers

Pasta: `internal/adapters/drivers`

Drivers atuais:

- `noop`: retorna `noop response: <prompt>`.
- `crush`: executa `crush run <prompt>` usando `exec.CommandContext`.

Limitações:

- driver Crush não streama logs;
- invocação real do CLI pode precisar ajuste;
- não injeta workspace como cwd;
- não passa config/env robustos;
- falta Picoclaw.

### 8.4 Web

Pasta: `internal/adapters/web`

Arquivos:

- `server.go`: rotas e handlers JSON principais;
- `api_handlers.go`: endpoints extras e agent-api docs;
- `html_handlers.go`: handlers HTML;
- `views.go`: componentes HTML em Go;
- `assets.go`: assets embedados;
- `assets/app.css`: estilo;
- `assets/htmx.min.js`: HTMX vendorizado.

Estado real:

- UI foi separada em páginas:
  - `/` Dashboard;
  - `/projects` Projetos;
  - `/agents` Agentes;
  - `/tasks` Tarefas;
  - `/skills` Skills.
- Antes tudo ficava numa única tela saturada; isso foi corrigido no código atual.
- Ainda falta polish visual e fluxos detalhados.

## 9. Rotas HTTP atuais

### 9.1 API JSON pública/interna

Agentes:

- `GET /api/agents`
- `POST /api/agents`
- `DELETE /api/agents/{id}`
- `POST /api/agents/{id}/permissions`
- `POST /api/agents/{id}/skills`

Tarefas:

- `GET /api/tasks`
- `POST /api/tasks`
- `POST /api/tasks/{id}/cancel`
- `POST /api/tasks/{id}/messages`
- `POST /api/tasks/{id}/delegate`

Skills:

- `GET /api/skills`
- `POST /api/skills`
- `PUT /api/skills/{id}`
- `DELETE /api/skills/{id}`

Projetos:

- `GET /api/projects`
- `POST /api/projects`

Capacidades:

- `GET /api/capabilities`

### 9.2 API documentada para agentes

- `GET /agent-api/docs`
- `GET /agent-api/agents`
- `GET /agent-api/tasks`
- `GET /agent-api/projects`
- `GET /agent-api/skills`
- `POST /agent-api/tasks/{id}/messages`
- `POST /agent-api/tasks/{id}/delegate`
- `POST /agent-api/tasks/{id}/cancel`

Estado real:

- endpoints existem;
- documentação básica existe em JSON;
- ainda falta autenticação/identidade de agente;
- ainda falta enforcement real por capability/permissão;
- ainda falta OpenAPI ou documentação mais amigável.

### 9.3 UI HTML

- `GET /`
- `GET /projects`
- `GET /agents`
- `GET /tasks`
- `GET /skills`
- `POST /projects`
- `POST /agents`
- `POST /agents/{id}/delete`
- `POST /agents/{id}/capability`
- `POST /tasks`
- `POST /tasks/{id}/cancel`
- `POST /tasks/{id}/messages`
- `POST /tasks/{id}/delegate`
- `POST /skills`
- `POST /skills/{id}/delete`
- `GET /partials/tasks`
- `GET /assets/*`

## 10. UI atual

### 10.1 Dashboard

Mostra:

- total de projetos;
- total de agentes;
- total de skills;
- total de tarefas;
- pendentes;
- rodando;
- concluídas;
- falhas/canceladas;
- cards de navegação para áreas principais.

### 10.2 Projetos

Mostra:

- formulário de criação de projeto;
- lista de projetos;
- path da pasta do projeto.

### 10.3 Agentes

Mostra:

- criação de agente;
- seleção de projeto;
- descrição;
- driver;
- capacidade predefinida;
- lista de agentes;
- alteração de capacidade;
- permissões geradas pela capacidade;
- exclusão de agente.

### 10.4 Tarefas

Mostra:

- criação de tarefa;
- seleção de projeto;
- seleção de agente;
- prompt;
- quadro de tarefas com polling;
- parar tarefa;
- enviar comando/mensagem;
- delegar subtarefa.

### 10.5 Skills

Mostra:

- criação de skill;
- projeto/global;
- nome;
- descrição;
- instruções;
- um arquivo opcional;
- biblioteca de skills;
- exclusão de skill customizada.

## 11. O que já foi feito

### 11.1 Renomeação/base

- Projeto renomeado para PicoClip.
- Módulo Go atualizado para `picoclip`.
- Entrada em `cmd/picoclip/main.go`.
- Binário `picoclip` gerado.

### 11.2 Arquitetura

- Estrutura antiga removida/substituída.
- Criado core com domínio, portas e serviços.
- Criados adaptadores para storage, web, eventos e drivers.
- Criado modelo de runs/eventos/mensagens/workspaces/skills.

### 11.3 Execução

- Engine com scheduler/dispatcher/runner.
- Driver registry.
- Driver noop.
- Driver crush.
- Timeout de task configurado.
- Runs salvas.

### 11.4 Interoperabilidade

- Mensagens por tarefa.
- Delegação criando subtarefas.
- ParentID em task.
- Mensagens entram no prompt.
- Skills entram no prompt.
- Capacidades entram no prompt.
- API local documentada para agentes.

### 11.5 Operação

- Cancelamento de tarefa.
- Exclusão de agente.
- Criação de projeto com pasta.
- Criação de skill customizada.
- Skills embutidas.
- Capacidades predefinidas.

### 11.6 Interface

- HTMX vendorizado.
- Assets embedados.
- UI separada em páginas.
- CSS refeito para layout mais profissional.
- Polling parcial de tarefas.

## 12. O que ainda precisa ser feito

### 12.1 Persistência

Prioridade alta.

- Criar adapter SQLite.
- Criar migrations/tabelas.
- Persistir agentes, projetos, tarefas, runs, eventos, mensagens e skills.
- Configurar por env:
  - `PICOCLIP_STORAGE=memory|sqlite`
  - `PICOCLIP_DB_PATH=picoclip.db`
- Preservar memory adapter para testes.

### 12.2 Permissões reais/enforcement

Prioridade alta.

Hoje capabilities geram permissões e entram no contexto, mas ainda não bloqueiam todas as ações.

Falta:

- identidade/autenticação de agente em `/agent-api`;
- middleware que resolve agente chamador;
- checagem por endpoint;
- negar delegação sem `tasks.delegate`;
- negar cancelamento sem `tasks.cancel`;
- negar criação de agente sem `agents.create`;
- negar gestão de skills sem `skills.manage`;
- auditar ações negadas.

### 12.3 Skills como pacotes completos

Prioridade alta.

Atual:

- instruções;
- um arquivo opcional pela UI;
- array `Files` no domínio.

Falta:

- múltiplos arquivos pela UI;
- editar skill;
- versionar skill;
- importar/exportar skill como diretório;
- manifest da skill;
- associar skills a agentes pela UI;
- skills por projeto e globais com herança clara;
- validar paths seguros;
- persistir arquivos em disco ou DB.

### 12.4 Projetos/contexto

Prioridade alta.

Atual:

- projeto cria pasta;
- task/agent podem ter ProjectID.

Falta:

- contexto do projeto entrar no prompt;
- arquivos do projeto serem visíveis ao agente por API;
- endpoint para listar arquivos permitidos;
- workspace como cwd do driver;
- separação forte por projeto;
- painel de projeto com agentes/tarefas/skills daquele projeto;
- metadata por projeto.

### 12.5 UI/UX profissional

Prioridade alta.

Já melhorou com telas separadas, mas falta:

- página de detalhe do projeto;
- página de detalhe do agente;
- página de detalhe da tarefa;
- página de detalhe da skill;
- edição de entidades;
- filtros e busca;
- breadcrumbs;
- feedback de erro/sucesso;
- empty states melhores;
- modais ou páginas dedicadas para ações destrutivas;
- visualização de conversas e timeline;
- status em tempo real sem sobrescrever formulários.

### 12.6 Execução e controle

Prioridade alta.

Falta:

- cancelamento matar processo externo;
- guardar handle/cancel func de execução;
- retries configuráveis;
- requeue;
- timeout virar status `timeout` corretamente;
- logs incrementais;
- stdout/stderr separados;
- eventos persistidos para cada transição;
- concorrência segura restaurada.

### 12.7 Eventos/timeline/SSE

Prioridade média/alta.

Falta:

- event store consistente;
- helper de emissão que persiste e publica;
- bus fan-out real;
- SSE para UI;
- timeline por tarefa;
- timeline por projeto;
- timeline por agente.

### 12.8 Drivers

Prioridade média.

Falta:

- Picoclaw driver;
- Crush driver robusto;
- workspace cwd;
- config/env por agente;
- streaming;
- adapters para outros CLIs.

### 12.9 API para agentes

Prioridade média/alta.

Falta:

- docs mais ricas;
- OpenAPI;
- schemas de payload;
- exemplos;
- endpoint de capacidades;
- endpoint de detalhes de tarefa;
- endpoint de mensagens;
- endpoint de runs;
- endpoint de eventos;
- autenticação/identidade;
- rate limiting local ou controles mínimos.

### 12.10 Qualidade/testes

Prioridade média.

Falta:

- testes unitários de services;
- testes de storage memory;
- testes HTTP handlers;
- testes de runner;
- testes de permissões;
- testes de skill packages;
- testes de cancelamento.

## 13. Riscos conhecidos

- Storage em memória faz o produto parecer instável após restart.
- Cancelamento atual não é cancelamento forte de processo.
- Permissões ainda são mais contexto/preset do que segurança aplicada.
- Skills ainda são simples demais para um produto maduro.
- Dispatcher síncrono limita concorrência.
- Event bus não suporta vários subscribers corretamente.
- UI ainda é HTML manual em Go, pode ficar difícil de manter sem `.templ` real.
- Falta gestão de estado/erros mais clara no frontend.

## 14. Estado de validação conhecido

Comandos que passaram recentemente:

```bash
gofmt -w cmd internal && go test ./... && go vet ./...
go build -o picoclip cmd/picoclip/main.go
```

Observação: após a última rodada, o servidor anterior estava rodando como background shell `001`, e uma tentativa de `job_kill` foi interrompida. Antes de reiniciar novamente, verificar o processo/porta ou o job se ainda existir.

## 15. Próximo passo recomendado

Antes de implementar qualquer feature nova:

1. Revisar esta documentação.
2. Atualizar `AGENTS.md` para refletir a arquitetura atual.
3. Criar SQLite storage.
4. Implementar enforcement real de capability/permissão.
5. Criar páginas de detalhe para projeto, agente, tarefa e skill.
6. Melhorar execução/cancelamento/logs.
