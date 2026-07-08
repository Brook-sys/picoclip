# PicoClip API Reference

Esta referência lista as rotas HTTP registradas atualmente em `internal/adapters/web/server.go` e serve como mapa para humanos e agentes. Ela não substitui testes nem leitura do handler quando o payload exato for crítico, mas deve ser atualizada sempre que endpoints mudarem.

## Convenções

- A aplicação usa `net/http` padrão do Go.
- As APIs retornam JSON quando chamadas por rotas `/api/...` ou `/agent-api/...`.
- Muitas ações da UI usam `POST` server-rendered/HTMX e podem responder HTML ou redirect.
- A Agent API aceita aliases `tasks` e `issues` em vários endpoints para alinhamento conceitual com Paperclip.
- O enforcement de permissões existe em partes da Agent API e ainda está em evolução. Não assuma cobertura total sem verificar o handler.

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
| `GET` | `/api/v1/tasks/{id}/events` | Lista eventos da task. |
| `GET` | `/api/v1/tasks/{id}/children` | Lista subtasks. |
| `POST` | `/api/v1/tasks/{id}/cancel` | Cancela task. |
| `POST` | `/api/v1/tasks/{id}/pause` | Pausa task contínua. |
| `POST` | `/api/v1/tasks/{id}/resume` | Retoma task contínua. |
| `POST` | `/api/v1/tasks/{id}/run-now` | Executa task contínua agora. |
| `POST` | `/api/v1/tasks/{id}/delegate` | Cria subtarefa. |
| `GET` | `/api/v1/runs` | Lista runs com filtros. |
| `GET` | `/api/v1/runs/{id}` | Detalhe de run. |
| `GET` | `/api/v1/skills` | Lista skills. |
| `POST` | `/api/v1/skills` | Cria skill. |
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

### Filtros comuns de tasks

`GET /api/tasks` aceita query params observados no handler:

| Parâmetro | Uso |
| --- | --- |
| `agent_id` | Filtra por agente. |
| `parent_id` | Filtra subtasks de uma task pai. |
| `project_id` | Filtra por workspace/projeto. |
| `status` | Filtra por um status ou lista parseada pelo helper de status. |

## Agent API

Superfície para agentes lerem contexto, atualizarem tasks, comentarem, delegarem e operarem via APIs.

| Método | Rota | Finalidade |
| --- | --- | --- |
| `GET` | `/agent-api/docs` | Documentação dinâmica para agentes. |
| `GET` | `/agent-api/me` | Identidade/contexto do agente chamador quando disponível. |
| `GET` | `/agent-api/agents/me/inbox-lite` | Inbox compacto para heartbeat/triagem. |
| `GET` | `/agent-api/agents` | Lista agentes. |
| `GET` | `/agent-api/tasks` | Lista tasks. |
| `GET` | `/agent-api/issues` | Alias de tasks. |
| `GET` | `/agent-api/tasks/{id}` | Detalhe da task. |
| `GET` | `/agent-api/issues/{id}` | Alias de detalhe. |
| `GET` | `/agent-api/tasks/{id}/heartbeat-context` | Contexto compacto para trabalhar em uma task, incluindo `execution_state` resumido de runs, locks, wakeups e eventos recentes. |
| `GET` | `/agent-api/issues/{id}/heartbeat-context` | Alias de heartbeat context. |
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
> **Nota Operacional:** Para triagem e debug de tasks presas via Agent API, consulte a seção **Triagem Rápida via Agent API** no [Operations Runbook](OPERATIONS.md).


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
