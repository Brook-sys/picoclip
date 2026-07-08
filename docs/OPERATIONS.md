# PicoClip Operations Runbook

Este runbook descreve como operar, diagnosticar e recuperar uma instalação local do PicoClip. Ele complementa:

- [Development Guide](DEVELOPMENT.md) — setup, comandos de desenvolvimento e validação.
- [Robustness](ROBUSTNESS.md) — modelo de scheduler, dispatcher, runner, reconciler, locks, retry e cancelamento.
- [Storage Architecture](STORAGE.md) — SQLite, migrations, backup/restore e contract tests.
- [API Reference](API_REFERENCE.md) — rotas HTTP, Agent API, SSE e partials.

## Objetivo operacional

PicoClip é local-first e experimental, mas deve ser operável com clareza. Em uma investigação, a pessoa mantenedora deve conseguir responder rapidamente:

- O servidor está rodando?
- Qual storage está em uso?
- Qual banco SQLite está sendo usado?
- Os runtimes estão configurados e saudáveis?
- Existem tasks presas por lock, retry, budget ou runtime ausente?
- Há wakeups pendentes ou retries agendados?
- Há eventos de falha/recovery suficientes para explicar o comportamento?
- É seguro exportar, restaurar ou resetar dados?
## Triagem Rápida via Agent API

Para agentes ou ferramentas diagnosticando o estado de tasks de forma rápida, não faça scrapping nem force requisições gigantes. Use as chamadas curtas abaixo. Substitua `http://127.0.0.1:8088` conforme necessidade e injete cabeçalho de auth se não for no mesmo host.

### 1. Uma task parece travada ou não inicia
Recupere o **heartbeat-context** (compacto).
```bash
curl -s "http://127.0.0.1:8088/agent-api/tasks/{id}/heartbeat-context?include=execution_state" | jq
```
*O que buscar?*
- `execution_state.is_runnable`: se for `false`, o scheduler não vai alocá-la (pode não estar em status `ready`/`running` ou faltar `needs_run`).
- `execution_state.locked`: se for `true` e `execution_state.lock_owner` apontar para um runner morto, a task está pendente de reconciliação (Reconciler reseta locks vencidos a cada intervalo).
- `execution_state.next_wakeup`: se estiver no futuro, a task está aguardando (sleep, backoff de retry).
- `execution_state.recent_events`: verifique se há erros recentes indicando runtime falho (ex: `Crush falhou`) ou budget excedido (ex: `max_runs_reached`).

### 2. A task foi bloqueada por tentativas sucessivas (Retry Storm)
Se o scheduler interromper uma task repetidamente e ela estiver marcada com erro permanente ou esgotou limites, analise suas runs passadas.
```bash
curl -s "http://127.0.0.1:8088/agent-api/tasks/{id}/runs" | jq '.runs | map(select(.status == "failed")) | length'
```
Se `failed` dominar as últimas runs, o runtime associado pode não ter as *capabilities* necessárias, estar offline (ex: token de LLM inválido), ou faltar dependências de ambiente. A task provavelmente passará por backoff.

### 3. Verificar o runtime atual configurado e se o agente tem permissões
Se a task tentar agir e falhar com erros de permissão ou runtime indisponível:
```bash
curl -s "http://127.0.0.1:8088/agent-api/tasks/{id}/heartbeat-context?include=skills" | jq '.agent_snapshot.capabilities'
curl -s "http://127.0.0.1:8088/agent-api/agents/me" | jq
```
Isso ajuda o agente diagnosticador a ver os limites que o agente de operação da task possui. Se a task precisar executar código local e não tiver runtime associado a `capabilities=["operator"]`, ela falhará na validação do dispatcher.

## Comandos rápidos
## Comandos rápidos

### Rodar localmente

```sh
make run
```

Padrão do Makefile: `0.0.0.0:8088`.

Rodar manualmente:

```sh
BIND=127.0.0.1 PORT=8088 PICOCLIP_STORAGE=sqlite go run cmd/picoclip/main.go
```

### Live reload

```sh
make dev
```

### Validação rápida

```sh
go test ./...
```

### Validação completa

```sh
make check
```

### Diagnóstico HTTP básico

Com servidor local em `8088`:

```sh
curl -s http://127.0.0.1:8088/api/diagnostics
curl -s http://127.0.0.1:8088/api/runtimes
curl -s 'http://127.0.0.1:8088/api/tasks'
```

## Variáveis de ambiente importantes

| Variável | Padrão | Quando conferir |
| --- | --- | --- |
| `BIND` | `0.0.0.0` | Servidor inacessível ou exposto demais. |
| `PORT` | `8080` binário / `8088` Makefile | Conflito de porta ou URL errada. |
| `PICOCLIP_STORAGE` | `sqlite` | Dados sumindo ou modo temporário acidental. |
| `PICOCLIP_DB_PATH` | `data/picoclip.db` | Banco errado, restore ou debug SQLite. |
| `PICOCLIP_WORKSPACES` | `workspaces` | Workspace/path incorreto. |
| `PICOCLIP_RUNTIMES` | `data/runtimes` | Runtime install/config ausente. |
| `PICOCLIP_LOG_LEVEL` | `info` | Precisa de logs mais detalhados. |
| `PICOCLIP_DEBUG` | `false` | Debug local. |
| `CRUSH_PATH` | `crush` | Crush não encontrado. |
| `PICOCLAW_PATH` | `picoclaw` | PicoClaw não encontrado. |
| `CLAURST_PATH` | `claurst` | Claurst não encontrado. |

## Checklist de saúde local

Use este checklist quando o sistema parecer instável ou antes de uma sessão longa de desenvolvimento.

1. Confirme branch e alterações locais:

   ```sh
   git status --short
   git branch --show-current
   ```

2. Confirme porta e processo:

   ```sh
   make kill-8088 # se precisar limpar processo antigo
   make run
   ```

3. Confirme diagnostics:

   ```sh
   curl -s http://127.0.0.1:8088/api/diagnostics
   ```

4. Confirme runtimes:

   ```sh
   curl -s http://127.0.0.1:8088/api/runtimes
   ```

5. Abra a UI:

   ```text
   http://127.0.0.1:8088
   ```

6. Verifique Activity para eventos recentes.
7. Se houve mudança de UI, rode E2E ou ao menos abra as páginas afetadas e confira console/network.

## Runbook: servidor não inicia

Sintomas:

- `make run` falha;
- porta indisponível;
- erro ao abrir SQLite;
- erro em migration.

Passos:

1. Confira porta:

   ```sh
   make kill-8088
   ```

2. Rode com logs explícitos:

   ```sh
   PICOCLIP_LOG_LEVEL=debug BIND=127.0.0.1 PORT=8088 go run cmd/picoclip/main.go
   ```

3. Confira storage:

   ```sh
   echo "$PICOCLIP_STORAGE"
   echo "$PICOCLIP_DB_PATH"
   ```

4. Se o erro citar SQLite/migration, leia [Storage Architecture](STORAGE.md) antes de mexer em migrations.
5. Se o banco local parece corrompido, exporte quando possível antes de resetar/remover arquivos.

## Runbook: task não executa

Sintomas:

- task fica `todo` ou `waiting_next_cycle` sem execução;
- task parece runnable, mas não recebe run novo;
- scheduler roda, mas nada acontece.

Passos:

1. Abra o detalhe da task na UI.
2. Confira `status`, agent atribuído, modo (`once`/`continuous`) e tentativas.
3. Veja Activity para:
   - `driver.missing`;
   - `budget.blocked`;
   - `retry.scheduled`;
   - `run.timeout`;
   - `run.recovered`;
   - `runtime.started`, `runtime.heartbeat`, `runtime.stalled`, `runtime.timeout` e `runtime.cancel_*` para entender liveness, stall e resultado de cancelamento do runtime.
4. Confira se há retry wakeup pendente com `DueAt` futuro.
5. Confira se `MaxAttempts` foi atingido.
6. Confira runtime/agent:

   ```sh
   curl -s http://127.0.0.1:8088/api/runtimes
   curl -s http://127.0.0.1:8088/api/agents
   ```

7. Se a task parece lockada, leia [Robustness](ROBUSTNESS.md) e investigue checkout/lock/latest run.

Possíveis causas:

| Causa | Sinal | Ação |
| --- | --- | --- |
| Retry ainda não due | `retry.scheduled`, wakeup futuro | Esperar ou acordar manualmente se fizer sentido. |
| Runtime ausente | `driver.missing` | Configurar runtime em Settings > Adapters. |
| Budget bloqueou | `budget.blocked` | Ajustar budget ou revisar consumo. |
| Lock antigo | checkout/run antigo | Deixar reconciler recuperar ou investigar storage. |
| Continuous task pausada | `waiting_next_cycle`/pause | Retomar pela UI. |
| Sem capacidade de dispatcher | muitos runs ativos | Aguardar runs ou investigar travamento. |

## Runbook: run travado ou sem output

Sintomas:

- run fica `running` por muito tempo;
- output não cresce;
- processo externo não termina;
- task não avança.

Passos:

1. Abra `/runs/{id}`.
2. Confira `last_output_at` e `stall_timeout` quando disponíveis.
3. Veja Activity para `run.timeout`.
4. Aguarde o reconciler processar stall timeout.
5. Se necessário, cancele a task pela UI/API.
6. Verifique runtime adapter e processo externo.

Comportamento esperado:

- stalled run é marcado como `timeout`;
- runtime manager tenta cancelar processo/sessão;
- task é desbloqueada;
- retry é agendado com backoff ou task é bloqueada se tentativas acabaram.

## Runbook: retry storm ou execução repetida demais

Sintomas:

- runs aparecem em sequência rápida;
- task falha e volta imediatamente;
- Activity mostra muitos retries.

Estado esperado do PicoClip:

- retries passam por wakeups;
- `NeedsRun` deve ficar `false` até `DueAt`;
- backoff é exponencial com cap de 5 minutos;
- dispatcher não deve burlar backoff.

Passos:

1. Veja Activity para `retry.scheduled`.
2. Confira metadata do retry:
   - `previous_run_id`;
   - `attempt`;
   - `backoff_seconds`;
   - `retryable`;
   - `reason`.
3. Verifique se `MaxAttempts` é adequado.
4. Se a task está rodando antes de `DueAt`, tratar como bug de robustez e escrever teste de regressão.

## Runbook: runtime ausente ou mal configurado

Sintomas:

- evento `driver.missing`;
- run falha imediatamente;
- settings mostra adapter ausente;
- binário não encontrado.

Passos:

1. Confira env vars:

   ```sh
   echo "$CRUSH_PATH"
   echo "$PICOCLAW_PATH"
   echo "$CLAURST_PATH"
   ```

2. Confira runtimes:

   ```sh
   curl -s http://127.0.0.1:8088/api/runtimes
   ```

3. Use Settings > Adapters para instalar/testar runtime.
4. Se estiver no Alpine e Claurst exigir glibc, considere ambiente Debian/glibc para esse runtime.
5. Documente qualquer requisito novo em [Development Guide](DEVELOPMENT.md) e [Project Map](PROJECT_MAP.md) se for estrutural.

## Runbook: SQLite ou dados inconsistentes

Sintomas:

- dados somem após restart;
- DB path inesperado;
- migration falha;
- restore parcial;
- task/run existe em estado impossível.

Passos:

1. Confirme modo e DB path:

   ```sh
   echo "$PICOCLIP_STORAGE"
   echo "$PICOCLIP_DB_PATH"
   ```

2. Confirme se não está em `PICOCLIP_STORAGE=memory`.
3. Leia [Storage Architecture](STORAGE.md).
4. Para mudanças em schema/repository, rode:

   ```sh
   go test ./internal/adapters/storage/... -count=1
   ```

5. Para estado de task/run inconsistente, veja Activity e latest run antes de editar qualquer dado manualmente.
6. Prefira export/restore pela UI Settings > Danger Zone em vez de manipulação manual do DB.

## Backup, restore e reset

### Exportar backup

Use Settings > Danger Zone > Export Backup.

O backup inclui estado durável central como settings, agents, workspaces, skills, tasks, runs, runtimes, messages, events, wakeups, usage, budgets e webhooks.

### Restaurar backup

Use Settings > Danger Zone > Restore Backup.

Atenção:

- restore sobrescreve o estado atual;
- SQLite restore roda em transação;
- erro deve fazer rollback do restore parcial;
- mantenha cópia do backup original.

### Factory reset

Use somente quando você quer limpar estado local intencionalmente. Antes de resetar, exporte backup se houver qualquer dado útil.

## Observabilidade atual

Superfícies disponíveis:

- Activity page;
- `/api/diagnostics`;
- `/api/runtimes`;
- `/runs` e `/runs/{id}`;
- `/tasks` e `/tasks/{id}`;
- SSE de activity/logs;
- logs de processo.

Eventos importantes:

| Evento | Ação operacional |
| --- | --- |
| `run.timeout` | Investigar runtime/liveness e retry. |
| `run.recovered` | Verificar lock antigo/orphaned state. |
| `retry.scheduled` | Conferir backoff e motivo. |
| `budget.blocked` | Revisar limites e consumo. |
| `driver.missing` | Configurar runtime/adapter. |

## Critérios de prontidão após mudança operacional

Depois de alterar comportamento operacional, robustez, storage, runtime ou comandos:

1. Atualize documentação correspondente.
2. Rode teste focado.
3. Rode validação ampla proporcional.
4. Confira links internos se docs mudaram.
5. Abra UI/API afetada quando possível.
6. Registre limitações conhecidas em docs, não apenas em comentários de código.

## Validação recomendada

Para mudanças apenas em documentação operacional:

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

Para mudanças que citam comandos/contratos de runtime/storage/API:

```sh
go test ./...
```

Para mudanças completas de comportamento:

```sh
make check
```

## Limitações operacionais conhecidas

- Não há dashboard dedicado de recovery/retry queue ainda.
- Métricas agregadas de confiabilidade ainda são limitadas.
- Classificação retryable/non-retryable ainda é básica.
- Cancelamento de árvore de processos no Windows ainda precisa Job Objects.
- External databases são intencionalmente fora de escopo no momento; SQLite é o storage persistente suportado.
