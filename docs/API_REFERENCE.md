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
| `GET` | `/agent-api/tasks/{id}/heartbeat-context` | Contexto compacto para trabalhar em uma task. |
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
| `GET` | `/skills` | Lista skills. |
| `GET` | `/skills/{id}` | Detalhe de skill. |
| `GET` | `/activity` | Timeline de atividade. |
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
| `GET` | `/sse/activity` | Stream de eventos da Activity. |
| `GET` | `/sse/runs/{id}/logs` | Stream de logs de run. |
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
