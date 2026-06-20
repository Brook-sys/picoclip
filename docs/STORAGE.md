# Storage Architecture

PicoClip uses SQLite as the default local persistence layer and keeps the in-memory storage available for tests and temporary runs.

## Goals

The database is designed for frequent feature changes:

- keep the core schema queryable and stable;
- allow new experimental fields without immediate migrations;
- preserve single-binary distribution with `CGO_ENABLED=0`;
- support local-first usage with a simple file database;
- keep repositories aligned with the core `ports.Storage` interfaces.

## Driver

PicoClip uses `modernc.org/sqlite` instead of `github.com/mattn/go-sqlite3`.

Reason:

- pure Go driver;
- no CGO requirement;
- easier cross-compilation for Linux, macOS and Windows;
- compatible with the GoReleaser setup that builds with `CGO_ENABLED=0`.

## Runtime configuration

Default:

```sh
PICOCLIP_STORAGE=sqlite
PICOCLIP_DB_PATH=data/picoclip.db
```

Available modes:

```sh
PICOCLIP_STORAGE=sqlite PICOCLIP_DB_PATH=data/picoclip.db go run cmd/picoclip/main.go
PICOCLIP_STORAGE=memory go run cmd/picoclip/main.go
```

Use memory mode only for temporary, non-persistent sessions.

## Schema strategy

Each main entity has strong columns for important fields and JSON text columns for future expansion.

Strong columns are used for:

- IDs and relationships;
- names and display fields;
- status and lifecycle fields;
- timestamps;
- values used by filters, lists and indexes.

JSON columns are used for:

- `metadata`: stable but flexible user/system metadata;
- `runtime_state`: internal state that can evolve between versions;
- `extra`: experimental feature data that should not require immediate migrations.

This lets new features land quickly while keeping important queries indexed and explicit. When an experimental field becomes central to the product, promote it from JSON to a real column in a new migration.

## Migrations

Migrations are defined in Go in `internal/adapters/storage/sqlite/migrations.go` and tracked by:

```sql
schema_migrations (
  version INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  applied_at TIMESTAMP NOT NULL
)
```

Rules:

- append migrations; do not edit already-released migrations;
- keep migrations small and named;
- prefer additive changes;
- use JSON columns for unstable fields first;
- add indexes when a field becomes part of list/filter paths;
- keep repositories responsible for mapping domain structs, not UI handlers.

## Current tables

Core tables:

- `workspaces`
- `agents`
- `skills`
- `tasks`
- `runs`
- `events`
- `messages`
- `settings`

Important indexes exist for common UI and Agent API access paths:

- task status, parent, workspace, assignee and update time;
- runs by task, agent, status and start time;
- messages by task and creation time;
- events by task and creation time;
- agents by project/reporting relationship;
- skills by project and built-in key.

## Repository coverage

SQLite implements the full `ports.Storage` surface:

- `AgentRepository`
- `TaskRepository`
- `RunRepository`
- `EventRepository`
- `MessageRepository`
- `SkillRepository`
- `WorkspaceRepository`
- `SettingsRepository`

Both SQLite and memory storage also expose `ResetAllData`, used by the Settings danger zone factory reset flow.

The memory adapter remains useful as a behavior reference and fallback.

## Export and Restore

Full database dumps and restorations are available under the Settings > Danger Zone.

- **Export Backup**: Downloads all core tables and settings as a consolidated JSON payload.
- **Restore Backup**: Overwrites the current database from a JSON payload (resets all rows and streams the JSON array elements back into storage).

## Built-in skills

Built-in skills are installed idempotently at startup. Existing built-ins are updated with new default metadata/instructions, but user-modified built-ins preserve their customized instructions/files until reset.

## Development checks

Run:

```sh
make check
```

This runs templ generation, Go tests, vet, build, and Playwright E2E. The default E2E server uses SQLite unless `PICOCLIP_STORAGE=memory` is set.
