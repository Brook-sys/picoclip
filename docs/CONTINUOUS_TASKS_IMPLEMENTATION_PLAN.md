# Continuous Tasks Implementation Plan

Documento temporário de implementação para a funcionalidade **Continuous Task** no PicoClip.

Objetivo: permitir tarefas em loop, lentas, auditáveis e contínuas, especialmente úteis para modelos mais lentos ou gratuitos como NVIDIA NIM, onde o valor vem de ciclos sucessivos de melhoria, auditoria e coordenação.

## Decisões aprovadas

### Comportamento padrão

- Uma task pode ser normal (`once`) ou contínua (`continuous`).
- Continuous tasks rodam indefinidamente até o usuário pausar ou cancelar.
- Delay padrão entre ciclos: **1 minuto**.
- O delay começa a contar **após o término da execução anterior**.
- O agente principal sempre monitora subtasks delegadas.
- Perguntas ao usuário não bloqueiam o loop.
- Respostas do usuário não aceleram o próximo ciclo; elas entram no contexto da próxima execução programada.
- Deve existir ação específica: **Pause continuous task**.

### Filosofia da feature

Uma Continuous Task não é uma task que “falhou em terminar”. Ela é uma missão macro de longo prazo.

Cada ciclo deve:

1. Revisar o objetivo macro.
2. Revisar o histórico de ciclos anteriores.
3. Auditar o conteúdo gerado.
4. Verificar subtasks delegadas.
5. Detectar subtasks travadas, sem resposta ou com conteúdo fraco.
6. Melhorar incrementalmente o resultado.
7. Fazer perguntas ao usuário quando útil.
8. Continuar com suposições razoáveis se o usuário ainda não respondeu.
9. Planejar o próximo ciclo.

## Modelo de domínio

### Novos tipos

Adicionar em `internal/core/domain/task.go`:

```go
type TaskMode string

const (
    TaskModeOnce       TaskMode = "once"
    TaskModeContinuous TaskMode = "continuous"
)
```

Adicionar status:

```go
TaskStatusWaitingNextCycle TaskStatus = "waiting_next_cycle"
```

### Novos campos em `Task`

```go
Mode             TaskMode   `json:"mode"`
LoopDelaySeconds int        `json:"loop_delay_seconds"`
LoopRunCount     int        `json:"loop_run_count"`
LoopNextRunAt    *time.Time `json:"loop_next_run_at,omitempty"`
LoopPausedAt     *time.Time `json:"loop_paused_at,omitempty"`
LoopAuditPrompt  string     `json:"loop_audit_prompt,omitempty"`
```

### Defaults

Para task normal:

```go
Mode = TaskModeOnce
LoopDelaySeconds = 0
```

Para continuous task:

```go
Mode = TaskModeContinuous
LoopDelaySeconds = 60
MaxAttempts = 0
NeedsRun = true
```

`MaxAttempts = 0` significa sem limite de tentativas para continuous tasks.

## Storage

### SQLite migration

Adicionar migration nova:

```sql
ALTER TABLE tasks ADD COLUMN mode TEXT NOT NULL DEFAULT 'once';
ALTER TABLE tasks ADD COLUMN loop_delay_seconds INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN loop_run_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN loop_next_run_at TIMESTAMP;
ALTER TABLE tasks ADD COLUMN loop_paused_at TIMESTAMP;
ALTER TABLE tasks ADD COLUMN loop_audit_prompt TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_tasks_loop_next_run ON tasks(mode, status, loop_next_run_at);
```

### Repositórios

Atualizar:

- `internal/adapters/storage/sqlite/task_repo.go`
- `internal/adapters/storage/memory/task_repo.go` ou equivalente
- `scanTask`
- `Create`
- `Update`
- qualquer restore/export que serialize `Task`

## Scheduler / Reconciler

### Novo comportamento

Criar um reconciler específico ou estender o atual:

```go
continuous task where:
    mode = continuous
    status = waiting_next_cycle
    loop_paused_at IS NULL
    loop_next_run_at <= now
    checkout_run_id = ''
    checked_out_by_agent_id = ''
    status not in done/cancelled

then:
    needs_run = true
    status = todo
    updated_at = now
```

Recomendação: criar método claro, por exemplo:

```go
Reconciler.RequeueDueContinuousTasks(ctx)
```

Ou criar serviço dedicado:

```go
ContinuousTaskService.RequeueDue(ctx)
```

Inicialmente, o Reconciler pode chamar esse método em cada tick.

## Runner

### Ao iniciar ciclo

Quando a task for continuous, o prompt deve receber contexto especial.

Adicionar algo como:

```go
r.continuousTaskContext(ctx, task, run, messages, childTasks, childRuns)
```

### Prompt de Continuous Task

Injetar protocolo parecido com:

```text
You are executing a continuous task.

This task is not expected to finish in one run.
Your job is to make steady progress in cycles.

Each cycle must:
1. Re-read the macro objective.
2. Review previous messages, runs and outputs.
3. Audit what was produced so far.
4. Inspect delegated subtasks and their latest status.
5. Detect blocked, stalled, low-quality or unanswered subtasks.
6. Improve or research one meaningful increment.
7. Ask the user questions if needed, but do not stop waiting for answers.
8. Continue with explicit assumptions when user input is missing.
9. End with:
   - What changed this cycle
   - What was audited
   - Subtasks status
   - Open questions for the user
   - Plan for next cycle
```

### Ao finalizar ciclo com sucesso

Para task normal, manter comportamento atual.

Para continuous task:

```go
task.Status = TaskStatusWaitingNextCycle
task.NeedsRun = false
task.CheckoutRunID = ""
task.CheckedOutByAgentID = ""
task.ExecutionLockedAt = nil
task.LockExpiresAt = nil
task.LoopRunCount++
task.LoopNextRunAt = finishedAt + delay
task.UpdatedAt = finishedAt
```

Não marcar como done automaticamente.

### Ao falhar

Para continuous task, decidir política inicial:

Recomendação MVP:

- falhas de runtime deixam task `blocked`
- usuário pode corrigir runtime/config e usar resume/run now depois
- não tentar loop infinito em erro técnico

### Orçamento

Budget hard-stop continua bloqueando.

Continuous task bloqueada por budget deve ficar em `blocked`, não em `waiting_next_cycle`.

## Perguntas ao usuário

### Regra aprovada

Quando agente faz pergunta:

- cria mensagem visível ao usuário
- cria indicador prioritário no Dashboard
- não bloqueia execução
- resposta do usuário entra no contexto da próxima execução programada
- responder não antecipa o ciclo

### Implementação recomendada

Inicialmente, detectar perguntas pelo output do agente é difícil e frágil.

MVP recomendado:

- orientar o agente via prompt a terminar perguntas em uma seção explícita:

```text
Open questions for the user:
- ...
```

- na primeira fase, só aparecem no thread da task.
- fase seguinte: criar um tipo explícito de `Question`/`UserPrompt` persistido.

### UI no Dashboard

Criar widget ou painel:

```text
Questions for you
```

Fonte inicial possível:

- mensagens recentes de tasks continuous contendo marcador `Open questions for the user:`
- ou, melhor em fase posterior, tabela dedicada `questions`

Recomendação: não implementar parser frágil no MVP inicial. Primeiro consolidar loop. Depois criar estrutura explícita.

## Delegação supervisionada

Continuous task principal deve observar subtasks.

Contexto do ciclo deve incluir:

- subtasks abertas
- subtasks blocked
- subtasks in_progress há muito tempo
- subtasks sem runs recentes
- subtasks done com último output resumido
- subtasks que o agente principal delegou

Prompt deve instruir:

```text
You are responsible for supervising delegated subtasks.
If a child task is stalled, low quality, or has no answer, decide whether to wait, add a follow-up message, delegate again, or continue without it.
```

## UI

### Modal de criação de task

Adicionar:

- toggle `Continuous task`
- campo delay, default `60` seconds / `1 minute`
- explicação curta:

```text
Runs repeatedly until paused or cancelled. The next cycle starts after the previous one finishes plus the configured delay.
```

### Task list

Para continuous tasks mostrar:

- badge `Continuous`
- cycle count
- next cycle timestamp ou `Paused`
- status `waiting_next_cycle`

### Task detail

Adicionar painel:

```text
Continuous task
Mode: Continuous
Cycle: N
Delay: 1 min
Next cycle: ...
Main agent: ...
Watching subtasks: Yes
```

Ações MVP:

- Pause continuous task
- Resume continuous task
- Run now
- Cancel task (já existe)

### Dashboard

Adicionar seção futura:

- Questions for you
- Continuous tasks waiting next cycle
- Continuous tasks paused

## API / handlers

### Web forms

Atualizar criação de task em:

- modal de task
- handler POST `/tasks`
- API se aplicável

Campos de form:

```text
continuous=true|false
loop_delay_seconds=60
```

### Serviços

Adicionar inputs:

```go
type CreateTaskInput struct {
    AgentID string
    WorkspaceID string
    Title string
    Prompt string
    Mode domain.TaskMode
    LoopDelaySeconds int
}
```

Hoje `TaskService.CreateInWorkspace` recebe parâmetros soltos. Pode evoluir gradualmente criando novo método:

```go
CreateWithOptions(ctx, input CreateTaskInput)
```

E fazer os métodos antigos chamarem esse novo método.

## Fases de implementação

### Fase 1 — Modelo e storage

- adicionar `TaskMode`
- adicionar status `waiting_next_cycle`
- adicionar campos no domínio
- migration SQLite
- atualizar scan/create/update
- atualizar memory storage
- testes de storage

### Fase 2 — Criação pela UI

- toggle no modal
- delay default 60s
- handler cria continuous task
- task list mostra badge continuous

### Fase 3 — Loop básico

- Runner finaliza continuous task como `waiting_next_cycle`
- incrementa loop count
- agenda `loop_next_run_at`
- Reconciler requeue quando due
- continuous task roda indefinidamente até pause/cancel

### Fase 4 — Pause / Resume / Run now

- endpoints web
- botões no detalhe da task
- `loop_paused_at`
- resume recalcula ou mantém `loop_next_run_at`
- run now seta `needs_run=true`, `status=todo`, `loop_next_run_at=nil`

### Fase 5 — Prompt avançado e supervisão de subtasks

- injetar protocolo continuous
- incluir subtasks e runs recentes no contexto
- orientar agente principal a auditar e supervisionar

### Fase 6 — Questions UX

- definir estrutura explícita para perguntas
- destacar no Dashboard
- permitir resposta sem antecipar ciclo
- resposta vira mensagem para próxima execução

## Riscos / cuidados

### Loop infinito descontrolado

Mitigação:

- pause/cancel claros
- budget hard-stop
- delay mínimo de 60s no MVP
- UI mostra cycle count

### Modelos lentos e timeouts

Mitigação:

- permitir timeout maior por runtime/agent no futuro
- não depender de output frequente para considerar travado
- stall timeout deve ser revisto para continuous tasks

### Perguntas detectadas por texto

Evitar no core inicial.

Melhor criar um protocolo textual primeiro e depois persistência explícita.

### Subtasks travadas

O agente principal deve monitorar, mas o sistema também pode futuramente alertar:

- child task in_progress por tempo demais
- child task sem runs
- child task blocked

## Primeiro commit recomendado

Implementar somente Fase 1:

- domínio
- migration
- repositórios
- testes

Sem UI ainda.

Depois Fase 2 cria o toggle e permite criar continuous tasks reais.
