# PicoClip API Reference

Esta referência lista as rotas HTTP registradas atualmente em `internal/adapters/web/server.go` e serve como mapa para humanos e agentes. Ela não substitui testes nem leitura do handler quando o payload exato for crítico, mas deve ser atualizada sempre que endpoints mudarem.

## Convenções

- A aplicação usa `net/http` padrão do Go.
- As APIs retornam JSON quando chamadas por rotas `/api/...` ou `/agent-api/...`.
- Muitas ações da UI usam `POST` server-rendered/HTMX e podem responder HTML ou redirect.
- A Agent API aceita aliases `tasks` e `issues` em vários endpoints para alinhamento conceitual com Paperclip.
- O enforcement de permissões existe em toda a Agent API. A maioria das ações requer `tasks.read`, `tasks.update`, `tasks.create`, `tasks.run` ou `tasks.cancel` no agente.

## Base URLs locais

Desenvolvimento pelo Makefile:

```text
http://127.0.0.1:8088
```

Binário sem variável `PORT`:

```text
http://127.0.0.1:8080
```

## API administrativa local

| Método | Rota | Finalidade |
| --- | --- | --- |
| `GET` | `/api/agents` | Lista agentes. |
| `POST` | `/api/agents` | Cria agente. |
| `DELETE` | `/api/agents/{id}` | Remove agente. |
| `POST` | `/api/agents/{id}/permissions` | Atualiza permissões do agente. |
| `POST` | `/api/agents/{id}/skills` | Atualiza skills do agente. |
| `GET` | `/api/tasks` | Lista tasks com filtros. |
| `POST` | `/api/tasks` | Cria task. |
| `POST` | `/api/tasks/{id}/cancel` | Cancela task. |
| `POST` | `/api/tasks/{id}/messages` | Adiciona mensagem/comentário. |
| `POST` | `/api/tasks/{id}/delegate` | Cria/delega subtarefa. |
| `GET` | `/api/skills` | Lista skills. |
| `POST` | `/api/skills` | Cria skill. |
| `PUT` | `/api/skills/{id}` | Atualiza skill. |
| `DELETE` | `/api/skills/{id}` | Remove skill. |
| `GET` | `/api/projects` | Lista projetos/workspaces. |
| `POST` | `/api/projects` | Cria projeto/workspace. |
| `GET` | `/api/capabilities` | Lista capabilities e permissões derivadas. |
| `GET` | `/api/search` | Busca para command palette. |
| `GET` | `/api/runtimes` | Lista estado dos runtimes. |
| `GET` | `/api/diagnostics` | Health/config diagnostics. |

## API v1

A API v1 usa envelope JSON consistente:

```json
{"data": {}, "meta": {}, "error": null}
```

Erros usam `error.code` estável, por exemplo `invalid_input`, `not_found`, `driver_unavailable` e `no_pending_tasks`.

| Método | Rota | Finalidade |
| --- | --- | --- |
| `GET` | `/api/v1/health` | Health check simples. |
| `GET` | `/api/v1/version` | Versão da API e runtime Go. |
| `GET` | `/api/v1/openapi.json` | Índice OpenAPI simples das rotas v1. |
| `GET` | `/api/v1/diagnostics/recovery-liveness` | Snapshot compacto de recovery/liveness para agentes e operadores. |
| `GET` | `/api/v1/dashboard` | Snapshot consolidado de dashboard. |
| `GET` | `/api/v1/projects` | Lista projetos. |
| `POST` | `/api/v1/projects` | Cria projeto. |
| `GET` | `/api/v1/projects/{id}` | Detalhe de projeto. |
| `GET` | `/api/v1/projects/{id}/agents` | Agentes do projeto. |
| `GET` | `/api/v1/projects/{id}/tasks` | Tasks do projeto. |
| `GET` | `/api/v1/projects/{id}/skills` | Skills do projeto. |
| `GET` | `/api/v1/agents` | Lista agentes, com filtro `tag`. |
| `POST` | `/api/v1/agents` | Cria agente completo. |
| `GET` | `/api/v1/agents/{id}` | Detalhe de agente. |
| `PATCH` | `/api/v1/agents/{id}` | Atualiza campos do agente. |
| `DELETE` | `/api/v1/agents/{id}` | Remove agente. |
| `GET` | `/api/v1/agents/{id}/tasks` | Tasks do agente. |
| `GET` | `/api/v1/agents/{id}/runs` | Runs do agente. |
| `PUT` | `/api/v1/agents/{id}/skills` | Define skills do agente. |
| `GET` | `/api/v1/tasks` | Lista tasks com filtros. |
| `POST` | `/api/v1/tasks` | Cria task. |
| `GET` | `/api/v1/tasks/{id}` | Detalhe compacto de task. |
| `GET` | `/api/v1/tasks/{id}/full` | Detalhe completo com mensagens, runs, eventos, wakeups e subtasks. |
| `GET` | `/api/v1/tasks/{id}/wakeups` | Lista wakeups da task para diagnóstico de retry/schedule/comment. |
| `GET` | `/api/v1/tasks/{id}/messages` | Lista mensagens da task. |
| `POST` | `/api/v1/tasks/{id}/messages` | Adiciona mensagem. |
| `GET` | `/api/v1/tasks/{id}/runs` | Lista runs da task. |
| `GET` | `/api/v1/tasks/{id}/events` | Lista eventos da task, incluindo eventos de auditoria de conclusão. |
| `GET` | `/api/v1/tasks/{id}/completion-audits` | Lista o histórico durável de decisões de auditoria semântica da task. |
| `GET` | `/api/v1/tasks/{id}/children` | Lista subtasks. |
| `POST` | `/api/v1/tasks/{id}/cancel` | Cancela task. |
| `POST` | `/api/v1/tasks/{id}/pause` | Pausa task contínua. |
| `POST` | `/api/v1/tasks/{id}/resume` | Retoma task contínua. |
| `POST` | `/api/v1/tasks/{id}/run-now` | Executa task contínua agora. |
| `POST` | `/api/v1/tasks/{id}/delegate` | Cria subtarefa. |
| `GET` | `/api/v1/runs` | Lista runs com filtros. |
| `GET` | `/api/v1/runs/{id}` | Detalhe de run. |
| `GET` | `/api/v1/usage` | Lista ledger compacto de UsageEvent com filtros por run, task ou agente. |
| `GET` | `/api/v1/skills` | Lista skills. |
| `POST` | `/api/v1/skills` | Cria skill. |
| `POST` | `/api/v1/skills/import` | Busca e importa uma skill YAML remota. |
| `GET` | `/api/v1/skills/{id}` | Detalhe de skill. |
| `PATCH` | `/api/v1/skills/{id}` | Atualiza skill. |
| `DELETE` | `/api/v1/skills/{id}` | Remove skill. |
| `POST` | `/api/v1/skills/{id}/reset` | Reseta skill built-in. |
| `PUT` | `/api/v1/skills/{id}/agents` | Define atribuições de skill. |
| `GET` | `/api/v1/webhooks` | Lista webhooks sem expor secrets. |
| `POST` | `/api/v1/webhooks` | Cria webhook. |
| `GET` | `/api/v1/webhooks/{id}/deliveries` | Lista deliveries de webhook. |
| `GET` | `/api/v1/events` | Lista eventos recentes com filtros e `limit` validado. |
| `GET` | `/api/v1/activity` | Alias de eventos recentes. |

### Importação remota de skills YAML

`POST /api/v1/skills/import` faz fetch síncrono de um único arquivo YAML e persiste a skill custom resultante imediatamente; portanto, workers que carregam skills a cada execução podem usá-la sem reiniciar o Dispatcher. O payload exige `source_url` absoluto com esquema `http` ou `https`; `project_id` é opcional.

```json
{"source_url":"https://example.com/skills/release.yaml","project_id":"prj_123"}
```

O YAML precisa ter `name` e `instructions`; pode conter `description`, `version` e `files` (`path` e `content`). A resposta usa o envelope v1, preserva a URL em `data.source` e a versão declarada em `data.version`. O fetch tem timeout de 15 segundos e tamanho máximo de 1 MiB. A URL e cada redirect devem resolver somente para endereços públicos; endereços privados, loopback, link-local, multicast, não especificados e a faixa compartilhada `100.64.0.0/10` são rejeitados. Respostas não-2xx, YAML inválido e arquivos sem os campos obrigatórios retornam erro; importação não agenda polling nem atualização automática e não permite credenciais embutidas na URL.

Exemplo:

```sh
curl -sS -X POST http://127.0.0.1:8088/api/v1/skills/import \
  -H 'Content-Type: application/json' \
  -d '{"source_url":"https://example.com/skills/release.yaml"}'
```

### Filtros comuns de API v1

`GET /api/v1/diagnostics/recovery-liveness` retorna um envelope com `data.counts` e `data.items` para triagem token-efficient de recovery/liveness. Ele agrega eventos recentes (`runtime.stalled`, `run.recovered`, `retry.scheduled`), runs em `timeout`, wakeups pendentes de retry e locks expirados. O parâmetro `limit` controla a lista compacta de itens retornados (padrão `20`, máximo `100`); contadores continuam representando o snapshot agregado. A rota não inclui env vars, secrets ou payloads grandes.

Exemplo:

```sh
curl -s 'http://127.0.0.1:8088/api/v1/diagnostics/recovery-liveness?limit=10' | jq
```

Campos principais:

| Campo | Uso |
| --- | --- |
| `data.counts.runtime_stalled_events` | Eventos recentes de stall de runtime. |
| `data.counts.run_recovered_events` | Eventos recentes de recovery de runs. |
| `data.counts.retry_scheduled_events` | Eventos recentes de retry agendado. |
| `data.counts.timeout_runs` | Runs em status `timeout` encontradas no snapshot. |
| `data.counts.pending_retry_wakeups` | Wakeups pendentes com `reason=retry`. |
| `data.counts.expired_locks` | Tasks com checkout/lock cujo `lock_expires_at` já passou. |
| `data.items[]` | Lista limitada de sinais compactos com `kind`, IDs e timestamps. |

`GET /api/v1/events` aceita filtros por `task_id`, `agent_id`, `type` e `limit`.

| Parâmetro | Uso |
| --- | --- |
| `task_id` | Retorna apenas eventos da task. |
| `agent_id` | Retorna apenas eventos do agente. |
| `type` | Retorna apenas eventos do tipo informado. |
| `limit` | Limita a busca inicial de eventos recentes. Padrão `100`, máximo `500`; valores inválidos retornam `400` com `error.code=invalid_input`. |

`GET /api/v1/usage` retorna eventos persistidos do ledger de tokens em envelope v1. A rota aceita filtros combináveis `run_id`, `task_id` e `agent_id`; `meta` inclui `count`, `input_tokens`, `output_tokens`, `cached_tokens` e `cost_micros` somados para o resultado filtrado. Hoje `cost_micros` permanece `0` porque o PicoClip ainda não tem tabela de preços/modelos.

Exemplo:

```sh
curl -s 'http://127.0.0.1:8088/api/v1/usage?agent_id=agt_123' | jq
```

### Filtros comuns de tasks

`GET /api/tasks` aceita query params observados no handler:

| Parâmetro | Uso |
| --- | --- |
| `agent_id` | Filtra por agente. |
| `parent_id` | Filtra subtasks de uma task pai. |
| `project_id` | Filtra por workspace/projeto. |
| `status` | Filtra por um status ou lista parseada pelo helper de status. |

### Auditoria semântica de conclusão

`GET /api/v1/tasks/{id}/completion-audits` é somente leitura e retorna o histórico de tentativas e decisões persistidas para a task. Em `completion_audit.v1.mode=enforce`, uma atualização Agent API para `status=done` só responde sucesso após decisão `approve`. Os resultados fail-closed retornam JSON com códigos estáveis: `409 semantic_audit_rejected`, `409 semantic_audit_superseded`, `503 semantic_audit_unavailable` e `504 semantic_audit_timeout`; nenhum deles representa conclusão bem-sucedida.

## Agent API

Superfície para agentes lerem contexto, atualizarem tasks, comentarem, delegarem e operarem via APIs.

| Método | Rota | Finalidade |
| --- | --- | --- |
| `GET` | `/agent-api/docs` | Documentação dinâmica para agentes, incluindo endpoints Paperclip-like, aliases `/issues`, `include` do heartbeat-context e fluxo recomendado. |
| `GET` | `/agent-api/me` | Identidade/contexto do agente chamador quando disponível. |
| `GET` | `/agent-api/agents/me/inbox-lite` | Inbox compacto para heartbeat/triagem. |
| `GET` | `/agent-api/agents` | Lista agentes. |
| `GET` | `/agent-api/tasks` | Lista tasks. |
| `GET` | `/agent-api/issues` | Alias de tasks. |
| `GET` | `/agent-api/tasks/{id}` | Detalhe da task. |
| `GET` | `/agent-api/issues/{id}` | Alias de detalhe. |
| `GET` | `/agent-api/tasks/{id}/heartbeat-context` | Contexto compacto para trabalhar em uma task, incluindo `execution_state` resumido de runs, locks, wakeups e eventos recentes. |
| `GET` | `/agent-api/issues/{id}/heartbeat-context` | Alias de heartbeat context. |
| `GET` | `/agent-api/tasks/{id}/next-action` | Recomendação compacta da próxima ação operacional para a task. |
| `GET` | `/agent-api/issues/{id}/next-action` | Alias de next-action. |
| `GET` | `/agent-api/tasks/{id}/comments` | Lista comentários/mensagens da task. |
| `GET` | `/agent-api/issues/{id}/comments` | Alias de comments. |
| `GET` | `/agent-api/projects` | Lista projetos. |
| `GET` | `/agent-api/skills` | Lista skills. |
| `POST` | `/agent-api/tasks` | Cria task. |
| `POST` | `/agent-api/issues` | Alias de criação. |
| `POST` | `/agent-api/tasks/{id}/comments` | Cria comentário. |
| `POST` | `/agent-api/issues/{id}/comments` | Alias de comentário. |
| `POST` | `/agent-api/tasks/{id}/messages` | Cria mensagem. |
| `POST` | `/agent-api/tasks/{id}/checkout` | Faz checkout/claim da task. |
| `POST` | `/agent-api/issues/{id}/checkout` | Alias de checkout. |
| `POST` | `/agent-api/tasks/{id}/release` | Libera checkout/lock. |
| `POST` | `/agent-api/issues/{id}/release` | Alias de release. |
| `PATCH` | `/agent-api/tasks/{id}` | Atualiza status/campos suportados. |
| `PATCH` | `/agent-api/issues/{id}` | Alias de update. |
| `POST` | `/agent-api/tasks/{id}/wake` | Agenda/acorda task. |
| `POST` | `/agent-api/issues/{id}/wake` | Alias de wake. |
| `POST` | `/agent-api/tasks/{id}/delegate` | Delega/cria subtarefa. |
| `POST` | `/agent-api/tasks/{id}/cancel` | Cancela task. |

### Comentários, wakeups e inbox de agentes

Comentários com `role=user` em `/agent-api/tasks/{id}/comments`, `/agent-api/tasks/{id}/messages` ou `/api/tasks/{id}/messages` são tratados como sinal operacional para o assignee da task:

- se a task está `todo`, `backlog`, `blocked` ou `in_review`, o comentário agenda/acorda a task com `WakeupRequest.reason=comment`; tasks `blocked` e `in_review` voltam para `todo` e `NeedsRun=true`;
- se a task está `in_progress`, o comentário cria/atualiza um wakeup pendente `reason=comment` para a inbox/heartbeat, mas não rouba o checkout ativo;
- se a task está `done`, o comentário cria uma subtarefa de follow-up e não cria wakeup de comentário na task concluída;
- se a task está `cancelled`, o comentário fica registrado, mas não acorda execução;
- wakeups pendentes de comentário são deduplicados por task/assignee: um novo comentário atualiza o payload do wakeup pendente com `message_id`, `from_id` e `to_id` em vez de multiplicar itens de inbox.

`GET /agent-api/agents/me/inbox-lite?agent_id=...` retorna cada task não terminal com `reason` e `attention`; comentários recentes aparecem como `reason="comment"` e `attention=true` para permitir triagem compacta. Use `heartbeat-context` para buscar o comentário completo quando necessário.

Quando o reconciler processa um wakeup due associado a task, o PicoClip preserva o fluxo atual de acordar a task para o dispatcher e registra também o evento compacto `agent.heartbeat_wakeup`. Esse é o modo piloto de heartbeat/wakeup: o payload aponta `wakeup_id`, `wake_reason`, `engine_mode=pilot` e `context_route=/agent-api/tasks/{id}/heartbeat-context`, permitindo que agentes correlacionem a razão do wakeup com a rota compacta sem trocar o scheduler atual por uma engine completa de heartbeat.

### Contexto compacto para agentes

`GET /agent-api/tasks/{id}/heartbeat-context` é a rota recomendada para agentes recuperarem percepção operacional antes de agir sem puxar o detalhe completo da task. Ela evita duplicar payloads grandes e retorna apenas campos resumidos:

- identidade e prompt da task;
- `last_user_comment` e `wake_reason`;
- skills compactas disponíveis ao agente;
- `execution_state`, com `needs_run`, checkout/lock atual, última run resumida, até 3 wakeups pendentes, até 5 eventos recentes e contadores totais.

Para economizar tokens, a rota aceita `include` com seções separadas por vírgula. Sem `include`, retorna o contexto padrão completo. Com `include`, retorna somente as seções solicitadas além dos campos básicos da task:

```text
GET /agent-api/tasks/{id}/heartbeat-context?include=execution_state
GET /agent-api/tasks/{id}/heartbeat-context?include=execution_state,skills
```

Seções disponíveis: `prompt`, `execution_state`, `skills`, `apis`. A allowlist é estrita: qualquer seção desconhecida em `include` retorna `400 Bad Request` com a seção inválida em vez de aparecer em `meta.included`. A resposta inclui `meta.mode` (`default` ou `selective`) e `meta.included` para o agente saber qual forma recebeu; em modo seletivo, `meta.included` contém somente seções válidas realmente solicitadas.

Use `/agent-api/tasks/{id}` somente quando o agente realmente precisar de mensagens/runs/eventos completos.

`GET /agent-api/tasks/{id}/next-action` é a rota mais curta quando o agente precisa apenas decidir o próximo passo operacional. Ela retorna JSON direto, sem envelope v1, com `task_id`, `action`, `reason`, `risks`, `links` e `useful_links`.

Ações atuais:

| `action` | Quando usar |
| --- | --- |
| `checkout` | Task `todo` com `needs_run=true`, sem checkout ativo e runtime disponível. |
| `wait` | Task já tem checkout/run ativo ou não está pronta para execução. |
| `inspect_retry` | Há wakeup pendente de retry/schedule/comment ou runs recentes com falha/timeout. |
| `block` | Tentativas máximas foram atingidas e a task precisa de triagem humana/operador. |
| `inspect` | Task terminal (`done`/`cancelled`); apenas inspecionar ou abrir follow-up. |
| `ask_human` | Runtime do agente atribuído não está disponível/configurado. |

Exemplo:

```sh
curl -s 'http://127.0.0.1:8088/agent-api/tasks/{id}/next-action' | jq
```

Resposta compacta:

```json
{
  "task_id": "tsk_123",
  "action": "checkout",
  "reason": "Task is runnable and ready for an agent checkout.",
  "risks": [],
  "links": {
    "heartbeat_context": "/agent-api/tasks/tsk_123/heartbeat-context?include=execution_state,skills,apis",
    "checkout": "/agent-api/tasks/tsk_123/checkout",
    "comments": "/agent-api/tasks/tsk_123/comments",
    "release": "/agent-api/tasks/tsk_123/release",
    "wake": "/agent-api/tasks/tsk_123/wake"
  },
  "useful_links": [
    "/agent-api/tasks/tsk_123/heartbeat-context?include=execution_state,skills,apis",
    "/agent-api/tasks/tsk_123/checkout",
    "/agent-api/tasks/tsk_123/comments"
  ]
}
```

> **Nota Operacional:** Para triagem e debug de tasks presas via Agent API, consulte a seção **Triagem Rápida via Agent API** no [Operations Runbook](OPERATIONS.md).

### Contratos JSON compactos da Agent API

Esta seção fixa o shape real dos endpoints compactos usados por agentes. Os handlers atuais retornam JSON direto em sucesso, sem envelope `data/meta/error` da API v1. Para as operações críticas de task/issue (`checkout`, `release`, `PATCH status`, `wake`, `delegate` e `cancel`), erros da Agent API usam envelope JSON estruturado:

```json
{"error":{"code":"invalid_input","message":"invalid input: agent_id is required","hint":"Provide a valid agent_id with the required permission."}}
```

`error.code` é estável para automação (`invalid_input`, `forbidden`, `not_found` ou `conflict`), `message` preserva o erro humano e `hint` traz uma orientação curta para retry/triagem. Endpoints legados ou compartilhados com a API administrativa ainda podem retornar erro em texto simples até serem migrados em fatias futuras.

#### `GET /agent-api/docs`

`/agent-api/docs` é o ponto de descoberta dinâmico para agentes. A resposta lista os endpoints agent-facing atuais, incluindo aliases Paperclip-like `/agent-api/issues...`, `inbox-lite`, `heartbeat-context` e a allowlist `include=prompt,execution_state,skills,apis`. Ela também expõe `recommended_flow`, que orienta o ciclo compacto recomendado:

1. consultar `GET /agent-api/agents/me/inbox-lite?agent_id=...`;
2. consultar `GET /agent-api/tasks/{id}/next-action` para decidir se deve fazer checkout, aguardar, inspecionar retry, bloquear ou pedir intervenção humana;
3. reivindicar trabalho com `POST /agent-api/tasks/{id}/checkout` quando `next-action.action="checkout"`;
4. carregar percepção com `GET /agent-api/tasks/{id}/heartbeat-context?include=execution_state,skills,apis`;
5. registrar progresso via comentário;
6. atualizar status quando concluir/bloquear;
7. liberar checkout se parar sem concluir.

Use essa rota quando um agente precisa descobrir a superfície HTTP disponível sem ler a documentação Markdown completa.

#### `GET /agent-api/agents/me/inbox-lite`

Query params:

| Campo | Obrigatório | Uso |
| --- | --- | --- |
| `agent_id` | Sim | ID do agente cuja inbox será resumida. `agent_id` é obrigatório e ausência retorna `400 Bad Request`. |

Resposta `200 OK`:

```json
{
  "agent_id": "agent_123",
  "inbox": [
    {
      "task_id": "task_123",
      "title": "Investigar timeout do runner",
      "status": "todo",
      "reason": "comment",
      "attention": true,
      "severity": "medium",
      "last_activity_at": "2026-07-08T10:00:01Z",
      "needs_run": true,
      "checkout_run_id": "run_123",
      "counts": {
        "pending_wakeups": 1,
        "failed_runs": 0,
        "open_children": 0
      }
    }
  ]
}
```

Campos:

| Campo | Tipo | Observação |
| --- | --- | --- |
| `agent_id` | string | Ecoa o agente solicitado. |
| `inbox[]` | array | Inclui tasks do agente que ainda não estão `done` nem `cancelled`. |
| `inbox[].task_id` | string | ID da task. |
| `inbox[].title` | string | Título atual da task. |
| `inbox[].status` | string | Status atual da task. |
| `inbox[].reason` | string | Razão do wakeup mais recente quando existir; hoje cai para `assignment` se não houver wakeups. |
| `inbox[].attention` | boolean | `true` quando há comentário recente, falha/timeout, wakeup pendente ou `needs_run=true`; sinaliza item que merece triagem. |
| `inbox[].severity` | string | Sinal compacto de priorização: `high` para falha/timeout, bloqueio ou checkout ativo; `medium` para comentário, wakeup pendente ou item runnable; `low` para restante. |
| `inbox[].last_activity_at` | string | Timestamp RFC3339 mais recente entre task, runs, wakeups e subtasks, sem embutir arrays completos. |
| `inbox[].needs_run` | boolean | Espelha `Task.NeedsRun` para agentes detectarem itens runnable. |
| `inbox[].checkout_run_id` | string | Run em checkout, quando existir; vazio se não houver lock/run ativa. |
| `inbox[].counts.pending_wakeups` | number | Total de wakeups pendentes da task. |
| `inbox[].counts.failed_runs` | number | Total compacto de runs `failed` ou `timeout` da task. |
| `inbox[].counts.open_children` | number | Subtasks que ainda não estão `done` nem `cancelled`. |

`inbox-lite` permanece propositalmente pequeno: não embute mensagens, runs, wakeups, eventos ou subtasks completos. Use `/agent-api/tasks/{id}` ou `heartbeat-context` quando esses detalhes forem necessários.

Erros observáveis:

| Status | Quando ocorre |
| --- | --- |
| `400 Bad Request` | `agent_id` ausente. |
| `500 Internal Server Error` | Falha inesperada ao listar tasks. |

#### `GET /agent-api/tasks/{id}/heartbeat-context`

Alias: `GET /agent-api/issues/{id}/heartbeat-context`.

Query params:

| Campo | Obrigatório | Uso |
| --- | --- | --- |
| `include` | Não | Lista separada por vírgulas. `include` aceita apenas `prompt`, `execution_state`, `skills` e `apis`. Se omitido, todas as seções são incluídas. Qualquer seção desconhecida retorna `400 Bad Request`. |

Resposta padrão `200 OK`:

```json
{
  "task_id": "task_123",
  "title": "Investigar timeout do runner",
  "last_user_comment": "Veja o log mais recente.",
  "status": "in_progress",
  "checkout_run_id": "run_123",
  "wake_reason": "comment",
  "prompt": "Diagnosticar e corrigir...",
  "execution_state": {
    "needs_run": false,
    "checked_out_by": "agent_123",
    "checkout_run_id": "run_123",
    "lock_expires_at": "2026-07-08T10:30:00Z",
    "pending_wakeups": [
      {"id": "wakeup_123", "reason": "comment", "due_at": "2026-07-08T10:00:00Z", "payload": {}}
    ],
    "recent_events": [
      {"id": "event_123", "type": "run.output", "run_id": "run_123", "message": "stdout resumido", "created_at": "2026-07-08T10:00:01Z"}
    ],
    "counts": {
      "runs": 1,
      "wakeups": 1,
      "pending_wakeups": 1,
      "recent_events": 1,
      "shown_wakeups": 1,
      "shown_events": 1
    },
    "latest_run": {
      "id": "run_123",
      "status": "running",
      "attempt": 1,
      "driver_type": "crush",
      "last_output_at": "2026-07-08T10:00:01Z",
      "started_at": "2026-07-08T09:59:00Z",
      "finished_at": "",
      "tokens": {"input": 10, "output": 20, "total": 30}
    }
  },
  "skills": [
    {"id": "skill_123", "name": "debugging", "description": "..."}
  ],
  "apis": ["/agent-api/tasks", "/agent-api/projects", "/agent-api/skills"],
  "meta": {"mode": "default", "included": ["prompt", "execution_state", "skills", "apis"]}
}
```

Campos e limites reais:

| Campo | Tipo | Observação |
| --- | --- | --- |
| `task_id`, `title`, `status`, `checkout_run_id` | string | Identidade e estado mínimo da task. |
| `last_user_comment` | string | Último comentário/mensagem com `role=user`; vazio se não existir. |
| `wake_reason` | string | Razão do wakeup mais recente ou `assignment` quando não há wakeups. |
| `meta.mode` | string | `default` sem `include`; `selective` quando `include` é usado. |
| `meta.included` | array | Seções realmente incluídas; no modo seletivo vem ordenado alfabeticamente. |
| `prompt` | string | Só presente quando a seção `prompt` está incluída. |
| `skills[]` | array | Skills habilitadas e aplicáveis ao agente da task, com `id`, `name` e `description`. |
| `apis[]` | array | Rotas agent-facing úteis para continuar a navegação compacta. |
| `execution_state.needs_run` | boolean | Espelha `Task.NeedsRun`. |
| `execution_state.checked_out_by` | string | Agente que segura o lock atual, se houver. |
| `execution_state.lock_expires_at` | string | Timestamp RFC3339 ou string vazia quando não há expiração. |
| `execution_state.pending_wakeups` | array | Mostra até 3 wakeups pendentes, com `id`, `reason`, `due_at` e `payload`. |
| `execution_state.recent_events` | array | Mostra até 5 eventos recentes; `message` é truncada para 180 caracteres mais `...`. |
| `execution_state.counts` | object | Contadores totais e quantidade mostrada para runs, wakeups e eventos. |
| `execution_state.latest_run` | object | Presente somente quando há ao menos uma run; inclui status, tentativa, runtime, timestamps e tokens. |

Erros observáveis:

| Status | Quando ocorre |
| --- | --- |
| `400 Bad Request` | `include` contém seção desconhecida. |
| `404 Not Found` | Task inexistente. |

#### Operações compactas de task/issue

Os aliases `/agent-api/issues...` são equivalentes aos endpoints `/agent-api/tasks...` para detalhe, heartbeat-context, comentários, criação, checkout, release, update e wake. A exceção atual é que delegate/cancel só estão registrados em `/agent-api/tasks/{id}/delegate` e `/agent-api/tasks/{id}/cancel`.

| Endpoint | Request JSON | Sucesso | Permissão exigida |
| --- | --- | --- | --- |
| `POST /agent-api/tasks` | `project_id`, `parent_id`, `agent_id`/`assignee_agent_id`, `from_agent_id`, `title`, `prompt` ou `message` | `200 OK` com task compacta; `prompt` cai para `message` quando vazio | `tasks:create` para `from_agent_id` |
| `POST /agent-api/tasks/{id}/comments` | `from_id`, `to_id`, `role`, `body`, `reopen` | `200 OK` com mensagem criada; `role` padrão é `user`; `reopen=true` também chama wake | `tasks:update` para `from_id` |
| `POST /agent-api/tasks/{id}/messages` | `from_id`, `to_id`, `role`, `body` | `200 OK` com mensagem criada; `role` padrão é `user` | `tasks:update` para `from_id` |
| `POST /agent-api/tasks/{id}/checkout` | `agent_id`, `run_id`, `expected_statuses` | `200 OK` com task compacta em checkout; se `run_id` vazio, usa `run_{task_id}_auto` | `tasks:run` para `agent_id` |
| `POST /agent-api/tasks/{id}/release` | `agent_id`, `comment` | `200 OK` com task compacta após liberar lock | `tasks:update` para `agent_id` |
| `PATCH /agent-api/tasks/{id}` | `agent_id`, `status`, `comment` | `200 OK` com task compacta após transição de status | `tasks:update` para `agent_id` |
| `POST /agent-api/tasks/{id}/wake` | `agent_id` | `200 OK` com task compacta após wake manual | `tasks:update` para `agent_id` |
| `POST /agent-api/tasks/{id}/delegate` | Mesmo payload de criação/delegação usado pela API administrativa | `200 OK` com subtarefa criada | Sem enforcement dedicado adicional no handler atual além das regras de criação quando aplicável |
| `POST /agent-api/tasks/{id}/cancel` | `agent_id`, `reason` | `200 OK` com task cancelada | `tasks:cancel`/regra do handler de cancelamento |

Erros comuns destes endpoints:

| Status | `error.code` | Quando ocorre |
| --- | --- | --- |
| `400 Bad Request` | `invalid_input` | JSON inválido, payload inválido, status/transição inválida ou `include` inválido em heartbeat-context. |
| `403 Forbidden` | `forbidden` | Agente informado não possui a permissão exigida pelo handler. |
| `404 Not Found` | `not_found` | Task/agente/recurso inexistente quando o serviço retorna `not found`. |
| `409 Conflict` | `conflict` | Lock/checkout ou transição conflita com o estado atual da task. |

Exemplo de erro estruturado em checkout sem permissão:

```sh
curl -s -X POST http://127.0.0.1:8088/agent-api/tasks/task_123/checkout \
  -H 'Content-Type: application/json' \
  -d '{"agent_id":"agent_observer"}' | jq
```

```json
{
  "error": {
    "code": "forbidden",
    "message": "forbidden: permission tasks.run required",
    "hint": "permission tasks.run required"
  }
}
```

## Páginas web e ações HTMX/server-rendered

Estas rotas são a interface humana principal. Elas podem retornar HTML completo, fragmentos HTMX, redirect ou responses específicas de formulário.

### Páginas principais

| Método | Rota | Finalidade |
| --- | --- | --- |
| `GET` | `/` | Dashboard. |
| `GET` | `/projects` | Lista projetos. |
| `GET` | `/projects/{id}` | Detalhe de projeto. |
| `GET` | `/agents` | Lista agentes. |
| `GET` | `/agents/new` | Form de novo agente. |
| `GET` | `/agents/{id}` | Detalhe de agente. |
| `GET` | `/tasks` | Lista tasks. |
| `GET` | `/tasks/{id}` | Detalhe de task. |
| `GET` | `/runs` | Lista runs. |
| `GET` | `/runs/{id}` | Detalhe de run. |
| `POST` | `/runs/history/delete` | Limpa histórico de runs finalizadas/falhas/canceladas e uso, preservando tasks e runs em execução. |
| `GET` | `/skills` | Lista skills. |
| `GET` | `/skills/{id}` | Detalhe de skill. |
| `GET` | `/activity` | Timeline de atividade. |
| `POST` | `/activity/history/delete` | Limpa eventos da timeline de atividade sem apagar tasks, agentes, projetos ou settings. |
| `GET` | `/settings` | Configurações. |
| `GET` | `/settings/adapters` | Aba de adapters/runtimes. |
| `GET` | `/settings/webhooks/{id}` | Detalhe de webhook. |

### Ações web

| Método | Rota | Finalidade |
| --- | --- | --- |
| `POST` | `/projects` | Cria projeto. |
| `POST` | `/agents` | Cria agente. |
| `POST` | `/agents/{id}/edit` | Edita agente. |
| `POST` | `/agents/{id}/edit-inline` | Edit inline de agente. |
| `POST` | `/agents/{id}/delete` | Remove agente. |
| `POST` | `/agents/{id}/capability` | Atualiza capability. |
| `POST` | `/agents/{id}/permissions` | Atualiza permissões. |
| `POST` | `/agents/{id}/skills` | Atualiza skills atribuídas. |
| `POST` | `/tasks` | Cria task. |
| `POST` | `/tasks/{id}/cancel` | Cancela task. |
| `POST` | `/tasks/{id}/delete` | Remove task e seu histórico associado (mensagens, eventos, runs, uso e wakeups), incluindo subtasks. |
| `POST` | `/tasks/{id}/status` | Atualiza status. |
| `POST` | `/tasks/{id}/wake` | Acorda/reagenda task. |
| `POST` | `/tasks/{id}/pause` | Pausa task contínua. |
| `POST` | `/tasks/{id}/resume` | Retoma task contínua. |
| `POST` | `/tasks/{id}/run-now` | Executa task contínua agora. |
| `POST` | `/tasks/{id}/edit-inline` | Edit inline de task. |
| `POST` | `/tasks/{id}/messages` | Adiciona comentário/mensagem. |
| `POST` | `/tasks/{id}/delegate` | Delega subtarefa. |
| `POST` | `/skills` | Cria skill. |
| `POST` | `/skills/{id}/edit` | Edita skill. |
| `POST` | `/skills/{id}/delete` | Remove skill. |
| `POST` | `/skills/{id}/reset` | Reseta skill built-in. |
| `POST` | `/skills/{id}/agents` | Atualiza agentes associados. |
| `POST` | `/skills/{id}/files/{index}` | Atualiza arquivo de skill. |

### Settings, runtimes e webhooks

| Método | Rota | Finalidade |
| --- | --- | --- |
| `POST` | `/settings/general` | Salva configurações gerais. |
| `POST` | `/settings/adapters` | Salva adapters. |
| `POST` | `/settings/adapters/test` | Testa adapters. |
| `POST` | `/settings/environment` | Salva ambiente. |
| `POST` | `/settings/budgets` | Cria budget. |
| `POST` | `/settings/budgets/{id}/toggle` | Ativa/desativa budget. |
| `POST` | `/settings/budgets/{id}/delete` | Remove budget. |
| `GET` | `/settings/export` | Exporta backup. |
| `POST` | `/settings/import` | Importa backup. |
| `POST` | `/settings/reset` | Factory reset. |
| `POST` | `/runtimes/{id}/install` | Instala runtime. |
| `POST` | `/runtimes/{id}/existing` | Usa binário existente. |
| `POST` | `/runtimes/{id}/test` | Testa runtime. |
| `POST` | `/runtimes/{id}/test-ai` | Teste AI do runtime. |
| `POST` | `/runtimes/{id}/uninstall` | Remove runtime. |
| `POST` | `/runtimes/{id}/config` | Salva config de runtime. |
| `POST` | `/runtimes/{id}/toggle` | Ativa/desativa runtime. |
| `POST` | `/settings/webhooks` | Cria webhook. |
| `POST` | `/settings/webhooks/{id}/edit` | Edita webhook. |
| `POST` | `/settings/webhooks/{id}/toggle` | Ativa/desativa webhook. |
| `POST` | `/settings/webhooks/{id}/test` | Testa webhook. |
| `POST` | `/settings/webhooks/{id}/delete` | Remove webhook. |
| `POST` | `/settings/webhook-deliveries/{id}/retry` | Reenvia delivery. |

## SSE e partials

| Método | Rota | Finalidade |
| --- | --- | --- |
| `GET` | `/sse/activity` | Stream global de eventos para a Activity. |
| `GET` | `/sse/tasks/{id}` | Stream filtrado por task para atualização live do detalhe da task. |
| `GET` | `/sse/runs/{id}/logs` | Stream de logs/eventos de run. |
| `GET` | `/partials/tasks` | Fragmento de lista de tasks. |
| `GET` | `/partials/tasks/{id}` | Fragmento de detalhe/live task. |
| `GET` | `/partials/runs/{id}` | Fragmento de detalhe/live run. |
| `GET` | `/assets/` | Assets estáticos. |

## Exemplos rápidos

Listar diagnostics:

```sh
curl http://127.0.0.1:8088/api/diagnostics
```

Listar tasks:

```sh
curl 'http://127.0.0.1:8088/api/tasks?status=todo'
```

Criar task administrativa:

```sh
curl -X POST http://127.0.0.1:8088/api/tasks \
  -H 'Content-Type: application/json' \
  -d '{"agent_id":"AGENT_ID","title":"Investigar bug","prompt":"Descreva e corrija o bug."}'
```

## Política de manutenção

Ao adicionar, remover ou alterar qualquer rota:

1. atualize `internal/adapters/web/server.go` e testes;
2. atualize este arquivo;
3. se afetar fluxo de agentes, atualize também `AGENTS.md` e/ou skills built-in;
4. se afetar UI/HTMX, atualize `docs/DESIGN.md` quando houver padrão novo;
5. rode validação proporcional (`go test ./...` ou `make check`).


### Histórico e exclusão seletiva

As ações destrutivas server-rendered/HTMX usam confirmação na UI. `POST /runs/history/delete` remove apenas runs que não estão `running` e o ledger de uso; tasks e configurações permanecem. `POST /activity/history/delete` remove eventos da timeline. `POST /tasks/{id}/delete` remove a task selecionada, subtasks e histórico associado, sem tocar em tasks não relacionadas.
