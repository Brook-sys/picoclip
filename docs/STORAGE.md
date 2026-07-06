# Storage Architecture

PicoClip uses SQLite as the default local-first persistence layer and keeps the in-memory storage available for tests and temporary runs.

Read this with:

- [Project Map](PROJECT_MAP.md)
- [Current State](CURRENT_STATE.md)
- [Development Guide](DEVELOPMENT.md)
- [Robustness](ROBUSTNESS.md)
- [Documentation Policy](DOCUMENTATION_POLICY.md)

## Goals

The storage layer is designed for frequent product changes without making the system heavy:

- keep the core schema queryable and stable;
- allow experimental fields without immediate migrations;
- preserve single-binary distribution with `CGO_ENABLED=0`;
- support local-first usage with a simple file database;
- keep repositories aligned with the core `ports.Storage` interfaces;
- keep memory and SQLite behavior close enough that tests can exercise both;
- make backup/restore understandable and safe for local operation.

## Storage modes

| Mode | Use case | Persistence |
| --- | --- | --- |
| `sqlite` | Normal local-first operation. | Persistent file DB. |
| `memory` | Tests, demos, temporary sessions, behavior reference. | Process-local only. |

Runtime configuration:

```sh
PICOCLIP_STORAGE=sqlite
PICOCLIP_DB_PATH=data/picoclip.db
```

Examples:

```sh
PICOCLIP_STORAGE=sqlite PICOCLIP_DB_PATH=data/picoclip.db go run cmd/picoclip/main.go
PICOCLIP_STORAGE=memory go run cmd/picoclip/main.go
```

Use memory mode only for temporary, non-persistent sessions.

## SQLite driver

PicoClip uses `modernc.org/sqlite` instead of `github.com/mattn/go-sqlite3`.

Reasons:

- pure Go driver;
- no CGO requirement;
- easier cross-compilation for Linux, macOS and Windows;
- compatible with release builds that use `CGO_ENABLED=0`;
- simpler local-first distribution.

The application configures SQLite in `cmd/picoclip/main.go` with operational pragmas such as WAL, foreign keys and busy timeout.

## Storage boundaries

The core depends on `internal/core/ports.Storage`, not on SQLite directly.

Current storage interface exposes repositories for:

- agents;
- tasks;
- runs;
- runtimes;
- events;
- messages;
- skills;
- workspaces;
- settings;
- wakeups;
- usage;
- budgets;
- webhooks.

It also exposes:

- `ResetAllData(ctx)` for factory reset;
- `RestoreAllData(ctx, data)` for backup restore;
- `RunInTx(ctx, fn)` for storage-level transactions.

Rule: application services should go through ports/repositories. UI handlers should not hand-write SQL.

## Schema strategy

Each main entity has strong columns for important fields and JSON text columns for future expansion.

Strong columns are used for:

- IDs and relationships;
- names and display fields;
- status and lifecycle fields;
- timestamps;
- values used by filters, lists, ordering and indexes;
- lock/retry/runtime fields that participate in recovery logic.

JSON columns are used for:

- `metadata`: stable but flexible user/system metadata;
- `runtime_state`: internal state that can evolve between versions;
- `extra`: experimental feature data that should not require immediate migrations;
- payload/config fields that are naturally structured and not queried often.

Promote a JSON field to a real column when it becomes part of:

- list filters;
- ordering;
- lock/recovery logic;
- scheduler decisions;
- API contracts that need predictable indexing;
- dashboards/diagnostics that query it frequently.

## Migrations

Migrations are defined in Go in `internal/adapters/storage/sqlite/migrations.go` and tracked by:

```sql
schema_migrations (
  version INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  applied_at TIMESTAMP NOT NULL
)
```

Migration execution:

1. `Migrate(ctx)` ensures `schema_migrations` exists.
2. It walks `migrations()` in version order.
3. Already-applied versions are skipped.
4. Each migration runs inside a transaction.
5. On success, the version/name/applied timestamp is inserted.

Rules:

- append migrations; do not edit already-released migrations;
- keep migrations small and named;
- prefer additive changes;
- use JSON columns for unstable fields first;
- add indexes when a field becomes part of list/filter/scheduler paths;
- keep repositories responsible for mapping domain structs;
- update memory storage and contract tests when repository behavior changes;
- update this document when schema or backup behavior changes.

## Current migration history

| Version | Name | Purpose |
| --- | --- | --- |
| 1 | `create_core_tables` | Initial workspaces, agents, skills, tasks, runs, events, messages and settings. |
| 2 | `create_settings_table` | Compatibility/idempotent settings table creation. |
| 3 | `create_runtime_states` | Runtime adapter state/config. |
| 4 | `add_token_tracking_columns` | Token counters on agents, tasks and runs. |
| 5 | `add_task_lock_columns` | Task lock timestamps and lock expiration index. |
| 6 | `create_wakeups_table` | Durable wakeup requests for assignment/manual/retry/schedule/recovery. |
| 7 | `create_usage_events_table` | Usage event ledger foundation. |
| 8 | `add_run_liveness_columns` | Process/liveness fields for stalled run detection. |
| 9 | `fix_run_started_finished_order` | No-op compatibility migration. |
| 10 | `create_budgets_table` | Budget limits and hard-stop support. |
| 11 | `fix_usage_events_ledger_columns` | Cached token, cost and created-at fields. |
| 12 | `create_outbox_events_table` | Durable outbox events. |
| 13 | `add_outbox_retry_columns` | Outbox retry attempts, next attempt and last error. |
| 14 | `add_continuous_task_columns` | Continuous task mode, delay, counters and next run time. |
| 15 | `create_webhook_tables` | Webhook subscriptions and delivery retry state. |

## Current tables

Core product tables:

- `workspaces`
- `agents`
- `skills`
- `tasks`
- `runs`
- `events`
- `messages`
- `settings`

Runtime/reliability/operations tables:

- `runtime_states`
- `wakeups`
- `usage_events`
- `budgets`
- `outbox_events`
- `webhook_subscriptions`
- `webhook_deliveries`

Important indexes cover common UI, Agent API and scheduler paths:

- tasks by agent, workspace, parent, status, update time, lock expiration and loop next run;
- runs by task, agent, status, start time and last output time;
- messages by task and creation time;
- events by task and creation time;
- agents by project/reporting relationship;
- skills by project and built-in key;
- wakeups by pending status/due time/priority, agent and task;
- usage by task, agent and run;
- budgets by scope, workspace and agent;
- outbox/webhook deliveries by due retry state.

## Repository coverage

SQLite implements the full `ports.Storage` surface through repository files in `internal/adapters/storage/sqlite/`.

Memory storage implements the same behavior reference in `internal/adapters/storage/memory/`.

Current repository families:

| Repository | SQLite file | Memory file |
| --- | --- | --- |
| Agents | `agent_repo.go` | `agent_repo.go` |
| Tasks | `task_repo.go` | `task_repo.go` |
| Runs | `run_repo.go` | `run_repo.go` |
| Runtimes | `runtime_repo.go` | `runtime_repo.go` |
| Events/outbox | `event_repo.go` | `event_repo.go` |
| Messages | `message_repo.go` | `message_repo.go` |
| Skills | `skill_repo.go` | `skill_repo.go` |
| Workspaces | `workspace_repo.go` | `workspace_repo.go` |
| Settings | `settings_repo.go` | in-memory storage state |
| Wakeups | `wakeup_repo.go` | `wakeup_repo.go` |
| Usage | `usage_repo.go` | `usage_repo.go` |
| Budgets | `budget_repo.go` | `budget_repo.go` |
| Webhooks | `webhook_repo.go` | `webhook_repo.go` |

## Contract tests

Storage behavior should be verified against shared contract tests in `internal/adapters/storage/storagetest/`.

Run storage tests:

```sh
go test ./internal/adapters/storage/... -count=1
```

Run all Go tests:

```sh
go test ./...
```

When changing storage behavior:

1. update the domain model if needed;
2. update `ports.Storage`/repository interfaces if needed;
3. update SQLite migrations and repositories;
4. update memory repositories;
5. update storage contract tests;
6. update backup/restore mapping if the entity is durable;
7. update API/UI docs if the field is user-visible;
8. run storage tests and then broader validation.

## Atomic claim and lock-sensitive storage behavior

Task execution safety depends on storage semantics.

`TaskRepository.ClaimNextRunnable(ctx, now, lockTTL)` is responsible for atomically:

- finding a runnable task;
- setting checkout/lock fields;
- incrementing attempts;
- creating the associated run;
- returning the claimed task and run.

Dispatcher-level concurrency control ensures a slot exists before calling this method. Storage-level atomicity ensures competing dispatchers do not claim the same task.

Lock-sensitive fields include:

- `needs_run`;
- `checkout_run_id`;
- `checked_out_by_agent_id`;
- `execution_locked_at`;
- `lock_expires_at`;
- `attempts`;
- run `status`;
- run liveness fields such as `last_output_at` and `stall_timeout`.

Do not change these fields without reading [Robustness](ROBUSTNESS.md) and updating tests.

## Export and restore

Full database export/restore is available under Settings > Danger Zone.

Backup payload is represented by `ports.BackupData` and currently includes:

- settings;
- agents;
- workspaces/projects;
- skills;
- tasks;
- runs;
- runtimes;
- messages;
- events;
- wakeups;
- usage;
- budgets;
- webhook subscriptions.

Export:

- downloads a consolidated JSON payload for core durable state.

Restore:

- overwrites current storage from a JSON payload;
- SQLite restore runs inside a single transaction;
- partial restores are rolled back on error;
- callers should treat restore as destructive and use it only with an intentional backup file.

Before changing backup/restore shape:

1. update `ports.BackupData`;
2. update SQLite export/restore code;
3. update memory restore/reset behavior;
4. update UI/API text if the user-facing payload changes;
5. add/adjust tests;
6. update this document.

## Built-in skills and storage

Built-in skills are installed idempotently at startup.

Current behavior:

- missing built-ins are created;
- existing built-ins receive updated default metadata/instructions;
- user-modified built-ins preserve custom instructions/files until reset.

This means migrations and startup install logic must avoid overwriting user content unexpectedly.

## Operational checklist

When debugging storage issues:

1. Check storage mode:

   ```sh
   echo "$PICOCLIP_STORAGE"
   echo "$PICOCLIP_DB_PATH"
   ```

2. Confirm the server is using the expected DB path.
3. Check `/api/diagnostics` or the Settings diagnostics page.
4. For stuck tasks, inspect task lock fields and latest run state.
5. For retries, inspect pending wakeups and `DueAt`.
6. For webhook/outbox issues, inspect due delivery/outbox retry fields.
7. Prefer application APIs/UI for normal operation; use direct DB inspection only for debugging.

Useful local commands:

```sh
make test-go
go test ./internal/adapters/storage/... -count=1
go test ./internal/core/services -run 'TestReconciler|TestStalledRun|TestDispatcher|TestLockRecovery' -count=1
```

## Development checks

Minimum storage validation:

```sh
go test ./internal/adapters/storage/... -count=1
```

Recommended validation for schema/repository changes:

```sh
make check
```

`make check` runs templ generation, Go tests, vet, build and Playwright E2E. The default app mode uses SQLite unless `PICOCLIP_STORAGE=memory` is set.

## Known limitations

- Migrations are append-only but do not yet include a separate downgrade path.
- Some flexible fields remain JSON until query patterns stabilize.
- Backup/restore is local and destructive; there is not yet a partial restore UI.
- Reliability metrics are event/log based; dedicated aggregate reliability tables are still future work.
- SQLite is the default supported persistence layer; external databases are intentionally out of scope for now.
