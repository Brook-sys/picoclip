package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type migration struct {
	version int
	name    string
	sql     string
}

func (s *Storage) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMP NOT NULL
		);
	`); err != nil {
		return err
	}

	for _, migration := range migrations() {
		applied, err := s.migrationApplied(ctx, migration.version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		if err := s.applyMigration(ctx, migration); err != nil {
			return err
		}
	}
	return nil
}

func (s *Storage) migrationApplied(ctx context.Context, version int) (bool, error) {
	var current int
	err := s.db.QueryRowContext(ctx, `SELECT version FROM schema_migrations WHERE version = ?`, version).Scan(&current)
	if err == nil {
		return true, nil
	}
	if err == sql.ErrNoRows {
		return false, nil
	}
	return false, err
}

func (s *Storage) applyMigration(ctx context.Context, migration migration) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, migration.sql); err != nil {
		return fmt.Errorf("migration %d %s failed: %w", migration.version, migration.name, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`, migration.version, migration.name, time.Now().UTC()); err != nil {
		return err
	}
	return tx.Commit()
}

func migrations() []migration {
	return []migration{
		{
			version: 1,
			name:    "create_core_tables",
			sql: `
				CREATE TABLE IF NOT EXISTS workspaces (
					id TEXT PRIMARY KEY,
					name TEXT NOT NULL,
					description TEXT NOT NULL DEFAULT '',
					root_path TEXT NOT NULL DEFAULT '',
					metadata TEXT NOT NULL DEFAULT '{}',
					extra TEXT NOT NULL DEFAULT '{}',
					version INTEGER NOT NULL DEFAULT 1,
					created_at TIMESTAMP NOT NULL,
					updated_at TIMESTAMP NOT NULL
				);

				CREATE TABLE IF NOT EXISTS agents (
					id TEXT PRIMARY KEY,
					project_id TEXT NOT NULL DEFAULT '',
					name TEXT NOT NULL,
					title TEXT NOT NULL DEFAULT '',
					reports_to_id TEXT NOT NULL DEFAULT '',
					tags TEXT NOT NULL DEFAULT 'null',
					type TEXT NOT NULL,
					description TEXT NOT NULL DEFAULT '',
					system_prompt TEXT NOT NULL DEFAULT '',
					instruction_file TEXT NOT NULL DEFAULT '',
					enabled INTEGER NOT NULL,
					capability TEXT NOT NULL DEFAULT '',
					permissions TEXT NOT NULL DEFAULT 'null',
					skill_ids TEXT NOT NULL DEFAULT 'null',
					config TEXT NOT NULL DEFAULT 'null',
					env TEXT NOT NULL DEFAULT 'null',
					extra_args TEXT NOT NULL DEFAULT 'null',
					metadata TEXT NOT NULL DEFAULT '{}',
					runtime_state TEXT NOT NULL DEFAULT '{}',
					extra TEXT NOT NULL DEFAULT '{}',
					version INTEGER NOT NULL DEFAULT 1,
					created_at TIMESTAMP NOT NULL,
					updated_at TIMESTAMP NOT NULL
				);
				CREATE INDEX IF NOT EXISTS idx_agents_created_at ON agents(created_at);
				CREATE INDEX IF NOT EXISTS idx_agents_project_id ON agents(project_id);
				CREATE INDEX IF NOT EXISTS idx_agents_reports_to_id ON agents(reports_to_id);

				CREATE TABLE IF NOT EXISTS skills (
					id TEXT PRIMARY KEY,
					project_id TEXT NOT NULL DEFAULT '',
					name TEXT NOT NULL,
					slug TEXT NOT NULL DEFAULT '',
					description TEXT NOT NULL DEFAULT '',
					license TEXT NOT NULL DEFAULT '',
					compatibility TEXT NOT NULL DEFAULT '',
					allowed_tools TEXT NOT NULL DEFAULT '',
					metadata TEXT NOT NULL DEFAULT 'null',
					instructions TEXT NOT NULL DEFAULT '',
					default_instructions TEXT NOT NULL DEFAULT '',
					files TEXT NOT NULL DEFAULT 'null',
					default_files TEXT NOT NULL DEFAULT 'null',
					kind TEXT NOT NULL,
					builtin_key TEXT NOT NULL DEFAULT '',
					permission TEXT NOT NULL DEFAULT '',
					agent_ids TEXT NOT NULL DEFAULT 'null',
					allowed_agent_types TEXT NOT NULL DEFAULT 'null',
					allowed_permissions TEXT NOT NULL DEFAULT 'null',
					source TEXT NOT NULL DEFAULT '',
					version TEXT NOT NULL DEFAULT '',
					enabled INTEGER NOT NULL,
					is_modified INTEGER NOT NULL,
					runtime_state TEXT NOT NULL DEFAULT '{}',
					extra TEXT NOT NULL DEFAULT '{}',
					schema_version INTEGER NOT NULL DEFAULT 1,
					created_at TIMESTAMP NOT NULL,
					updated_at TIMESTAMP NOT NULL
				);
				CREATE INDEX IF NOT EXISTS idx_skills_project_id ON skills(project_id);
				CREATE INDEX IF NOT EXISTS idx_skills_builtin_key ON skills(builtin_key);

				CREATE TABLE IF NOT EXISTS tasks (
					id TEXT PRIMARY KEY,
					parent_id TEXT NOT NULL DEFAULT '',
					workspace_id TEXT NOT NULL DEFAULT '',
					agent_id TEXT NOT NULL DEFAULT '',
					title TEXT NOT NULL DEFAULT '',
					prompt TEXT NOT NULL DEFAULT '',
					status TEXT NOT NULL,
					priority INTEGER NOT NULL DEFAULT 0,
					attempts INTEGER NOT NULL DEFAULT 0,
					max_attempts INTEGER NOT NULL DEFAULT 0,
					needs_run INTEGER NOT NULL DEFAULT 0,
					checkout_run_id TEXT NOT NULL DEFAULT '',
					checked_out_by_agent_id TEXT NOT NULL DEFAULT '',
					cancel_reason TEXT NOT NULL DEFAULT '',
					metadata TEXT NOT NULL DEFAULT '{}',
					runtime_state TEXT NOT NULL DEFAULT '{}',
					extra TEXT NOT NULL DEFAULT '{}',
					version INTEGER NOT NULL DEFAULT 1,
					created_at TIMESTAMP NOT NULL,
					updated_at TIMESTAMP NOT NULL,
					started_at TIMESTAMP,
					finished_at TIMESTAMP,
					completed_at TIMESTAMP,
					cancelled_at TIMESTAMP
				);
				CREATE INDEX IF NOT EXISTS idx_tasks_agent_id ON tasks(agent_id);
				CREATE INDEX IF NOT EXISTS idx_tasks_workspace_id ON tasks(workspace_id);
				CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
				CREATE INDEX IF NOT EXISTS idx_tasks_parent_id ON tasks(parent_id);
				CREATE INDEX IF NOT EXISTS idx_tasks_updated_at ON tasks(updated_at);

				CREATE TABLE IF NOT EXISTS runs (
					id TEXT PRIMARY KEY,
					task_id TEXT NOT NULL,
					agent_id TEXT NOT NULL,
					driver_type TEXT NOT NULL,
					status TEXT NOT NULL,
					attempt INTEGER NOT NULL DEFAULT 0,
					input TEXT NOT NULL DEFAULT '',
					output TEXT NOT NULL DEFAULT '',
					error TEXT NOT NULL DEFAULT '',
					metadata TEXT NOT NULL DEFAULT '{}',
					runtime_state TEXT NOT NULL DEFAULT '{}',
					extra TEXT NOT NULL DEFAULT '{}',
					version INTEGER NOT NULL DEFAULT 1,
					started_at TIMESTAMP NOT NULL,
					finished_at TIMESTAMP
				);
				CREATE INDEX IF NOT EXISTS idx_runs_task_id ON runs(task_id);
				CREATE INDEX IF NOT EXISTS idx_runs_agent_id ON runs(agent_id);
				CREATE INDEX IF NOT EXISTS idx_runs_status ON runs(status);
				CREATE INDEX IF NOT EXISTS idx_runs_started_at ON runs(started_at);

				CREATE TABLE IF NOT EXISTS events (
					id TEXT PRIMARY KEY,
					type TEXT NOT NULL,
					task_id TEXT NOT NULL DEFAULT '',
					agent_id TEXT NOT NULL DEFAULT '',
					run_id TEXT NOT NULL DEFAULT '',
					message TEXT NOT NULL DEFAULT '',
					data TEXT NOT NULL DEFAULT 'null',
					metadata TEXT NOT NULL DEFAULT '{}',
					extra TEXT NOT NULL DEFAULT '{}',
					version INTEGER NOT NULL DEFAULT 1,
					created_at TIMESTAMP NOT NULL
				);
				CREATE INDEX IF NOT EXISTS idx_events_task_id ON events(task_id);
				CREATE INDEX IF NOT EXISTS idx_events_created_at ON events(created_at);

				CREATE TABLE IF NOT EXISTS messages (
					id TEXT PRIMARY KEY,
					task_id TEXT NOT NULL,
					from_id TEXT NOT NULL DEFAULT '',
					to_id TEXT NOT NULL DEFAULT '',
					role TEXT NOT NULL,
					body TEXT NOT NULL DEFAULT '',
					metadata TEXT NOT NULL DEFAULT '{}',
					extra TEXT NOT NULL DEFAULT '{}',
					version INTEGER NOT NULL DEFAULT 1,
					created_at TIMESTAMP NOT NULL
				);
				CREATE INDEX IF NOT EXISTS idx_messages_task_id_created_at ON messages(task_id, created_at);

				CREATE TABLE IF NOT EXISTS settings (
					key TEXT PRIMARY KEY,
					value TEXT NOT NULL,
					updated_at TIMESTAMP NOT NULL
				);
			`,
		},
		{
			version: 2,
			name:    "create_settings_table",
			sql: `
				CREATE TABLE IF NOT EXISTS settings (
					key TEXT PRIMARY KEY,
					value TEXT NOT NULL,
					updated_at TIMESTAMP NOT NULL
				);
			`,
		},
		{
			version: 3,
			name:    "create_runtime_states",
			sql: `
				CREATE TABLE IF NOT EXISTS runtime_states (
					id TEXT PRIMARY KEY,
					runtime_id TEXT NOT NULL UNIQUE,
					mode TEXT NOT NULL,
					enabled INTEGER NOT NULL DEFAULT 1,
					version TEXT NOT NULL DEFAULT '',
					bin_path TEXT NOT NULL DEFAULT '',
					config_path TEXT NOT NULL DEFAULT '',
					home_path TEXT NOT NULL DEFAULT '',
					data_path TEXT NOT NULL DEFAULT '',
					logs_path TEXT NOT NULL DEFAULT '',
					source TEXT NOT NULL DEFAULT '',
					source_url TEXT NOT NULL DEFAULT '',
					checksum TEXT NOT NULL DEFAULT '',
					installed_at TIMESTAMP NOT NULL,
					updated_at TIMESTAMP NOT NULL,
					last_health_at TIMESTAMP,
					last_health_json TEXT NOT NULL DEFAULT '{}',
					settings_json TEXT NOT NULL DEFAULT '{}',
					metadata_json TEXT NOT NULL DEFAULT '{}'
				);
				CREATE INDEX IF NOT EXISTS idx_runtime_states_runtime_id ON runtime_states(runtime_id);
			`,
		},
	}
}
