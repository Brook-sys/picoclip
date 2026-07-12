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
		{
			version: 4,
			name:    "add_token_tracking_columns",
			sql: `
				ALTER TABLE agents ADD COLUMN input_tokens INTEGER NOT NULL DEFAULT 0;
				ALTER TABLE agents ADD COLUMN output_tokens INTEGER NOT NULL DEFAULT 0;
				ALTER TABLE agents ADD COLUMN total_tokens INTEGER NOT NULL DEFAULT 0;
				ALTER TABLE tasks ADD COLUMN input_tokens INTEGER NOT NULL DEFAULT 0;
				ALTER TABLE tasks ADD COLUMN output_tokens INTEGER NOT NULL DEFAULT 0;
				ALTER TABLE tasks ADD COLUMN total_tokens INTEGER NOT NULL DEFAULT 0;
				ALTER TABLE runs ADD COLUMN input_tokens INTEGER NOT NULL DEFAULT 0;
				ALTER TABLE runs ADD COLUMN output_tokens INTEGER NOT NULL DEFAULT 0;
				ALTER TABLE runs ADD COLUMN total_tokens INTEGER NOT NULL DEFAULT 0;
			`,
		},
		{
			version: 5,
			name:    "add_task_lock_columns",
			sql: `
				ALTER TABLE tasks ADD COLUMN execution_locked_at TIMESTAMP;
				ALTER TABLE tasks ADD COLUMN lock_expires_at TIMESTAMP;
				CREATE INDEX IF NOT EXISTS idx_tasks_lock_expires_at ON tasks(lock_expires_at);
			`,
		},
		{
			version: 6,
			name:    "create_wakeups_table",
			sql: `
				CREATE TABLE IF NOT EXISTS wakeups (
					id TEXT PRIMARY KEY,
					agent_id TEXT NOT NULL,
					task_id TEXT NOT NULL DEFAULT '',
					reason TEXT NOT NULL,
					status TEXT NOT NULL,
					priority INTEGER NOT NULL DEFAULT 0,
					due_at TIMESTAMP NOT NULL,
					claimed_at TIMESTAMP,
					payload TEXT NOT NULL DEFAULT '{}',
					created_at TIMESTAMP NOT NULL,
					updated_at TIMESTAMP NOT NULL
				);
				CREATE INDEX IF NOT EXISTS idx_wakeups_pending ON wakeups(status, due_at, priority);
				CREATE INDEX IF NOT EXISTS idx_wakeups_agent_id ON wakeups(agent_id);
				CREATE INDEX IF NOT EXISTS idx_wakeups_task_id ON wakeups(task_id);
			`,
		},
		{
			version: 7,
			name:    "create_usage_events_table",
			sql: `
				CREATE TABLE IF NOT EXISTS usage_events (
					id TEXT PRIMARY KEY,
					run_id TEXT NOT NULL,
					task_id TEXT NOT NULL,
					agent_id TEXT NOT NULL,
					provider TEXT NOT NULL DEFAULT '',
					model TEXT NOT NULL DEFAULT '',
					input_tokens INTEGER NOT NULL DEFAULT 0,
					output_tokens INTEGER NOT NULL DEFAULT 0,
					total_tokens INTEGER NOT NULL DEFAULT 0,
					process_id INTEGER NOT NULL DEFAULT 0,
					last_output_at TIMESTAMP,
					stall_timeout INTEGER NOT NULL DEFAULT 0,
					started_at TIMESTAMP NOT NULL,
					finished_at TIMESTAMP
				);
				CREATE INDEX IF NOT EXISTS idx_usage_task ON usage_events(task_id);
				CREATE INDEX IF NOT EXISTS idx_usage_agent ON usage_events(agent_id);
				CREATE INDEX IF NOT EXISTS idx_usage_run ON usage_events(run_id);
			`,
		},
		{
			version: 8,
			name:    "add_run_liveness_columns",
			sql: `
				ALTER TABLE runs ADD COLUMN process_id INTEGER NOT NULL DEFAULT 0;
				ALTER TABLE runs ADD COLUMN last_output_at TIMESTAMP;
				ALTER TABLE runs ADD COLUMN stall_timeout INTEGER NOT NULL DEFAULT 0;
				CREATE INDEX IF NOT EXISTS idx_runs_last_output ON runs(last_output_at);
			`,
		},
		{
			version: 9,
			name:    "fix_run_started_finished_order",
			sql: `
				-- No-op: started_at/finished_at already exist in correct position since v1
			`,
		},
		{
			version: 10,
			name:    "create_budgets_table",
			sql: `
				CREATE TABLE IF NOT EXISTS budgets (
					id TEXT PRIMARY KEY,
					scope TEXT NOT NULL,
					workspace_id TEXT NOT NULL DEFAULT '',
					agent_id TEXT NOT NULL DEFAULT '',
					limit_tokens INTEGER NOT NULL DEFAULT 0,
					limit_runs INTEGER NOT NULL DEFAULT 0,
					limit_cost_micros INTEGER NOT NULL DEFAULT 0,
					hard_stop BOOLEAN NOT NULL DEFAULT 1,
					enabled BOOLEAN NOT NULL DEFAULT 1,
					created_at TIMESTAMP NOT NULL,
					updated_at TIMESTAMP NOT NULL
				);
				CREATE INDEX IF NOT EXISTS idx_budgets_scope ON budgets(scope);
				CREATE INDEX IF NOT EXISTS idx_budgets_workspace ON budgets(workspace_id);
				CREATE INDEX IF NOT EXISTS idx_budgets_agent ON budgets(agent_id);
			`,
		},
		{
			version: 11,
			name:    "fix_usage_events_ledger_columns",
			sql: `
					ALTER TABLE usage_events ADD COLUMN cached_tokens INTEGER NOT NULL DEFAULT 0;
					ALTER TABLE usage_events ADD COLUMN cost_micros INTEGER NOT NULL DEFAULT 0;
					ALTER TABLE usage_events ADD COLUMN created_at TIMESTAMP;
					UPDATE usage_events SET created_at = COALESCE(created_at, started_at, CURRENT_TIMESTAMP);
				`,
		},
		{
			version: 12,
			name:    "create_outbox_events_table",
			sql: `
					CREATE TABLE IF NOT EXISTS outbox_events (
						id TEXT PRIMARY KEY,
						type TEXT NOT NULL,
						payload TEXT NOT NULL,
						created_at TIMESTAMP NOT NULL
					);
					CREATE INDEX IF NOT EXISTS idx_outbox_created_at ON outbox_events(created_at);
				`,
		},
		{
			version: 13,
			name:    "add_outbox_retry_columns",
			sql: `
					ALTER TABLE outbox_events ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0;
					ALTER TABLE outbox_events ADD COLUMN next_attempt_at TIMESTAMP;
					ALTER TABLE outbox_events ADD COLUMN last_error TEXT NOT NULL DEFAULT '';
					CREATE INDEX IF NOT EXISTS idx_outbox_next_attempt ON outbox_events(next_attempt_at, created_at);
				`,
		},
		{
			version: 14,
			name:    "add_continuous_task_columns",
			sql: `
					ALTER TABLE tasks ADD COLUMN mode TEXT NOT NULL DEFAULT 'once';
					ALTER TABLE tasks ADD COLUMN loop_delay_seconds INTEGER NOT NULL DEFAULT 0;
					ALTER TABLE tasks ADD COLUMN loop_run_count INTEGER NOT NULL DEFAULT 0;
					ALTER TABLE tasks ADD COLUMN loop_next_run_at TIMESTAMP;
					ALTER TABLE tasks ADD COLUMN loop_paused_at TIMESTAMP;
					ALTER TABLE tasks ADD COLUMN loop_audit_prompt TEXT NOT NULL DEFAULT '';
					CREATE INDEX IF NOT EXISTS idx_tasks_loop_next_run ON tasks(mode, status, loop_next_run_at);
				`,
		},
		{
			version: 15,
			name:    "create_webhook_tables",
			sql: `
					CREATE TABLE IF NOT EXISTS webhook_subscriptions (
						id TEXT PRIMARY KEY,
						name TEXT NOT NULL,
						url TEXT NOT NULL,
						secret TEXT NOT NULL DEFAULT '',
						event_types TEXT NOT NULL DEFAULT '[]',
						enabled BOOLEAN NOT NULL DEFAULT 1,
						created_at TIMESTAMP NOT NULL,
						updated_at TIMESTAMP NOT NULL
					);
					CREATE INDEX IF NOT EXISTS idx_webhook_subscriptions_enabled ON webhook_subscriptions(enabled);

					CREATE TABLE IF NOT EXISTS webhook_deliveries (
						id TEXT PRIMARY KEY,
						subscription_id TEXT NOT NULL,
						event_id TEXT NOT NULL,
						event_type TEXT NOT NULL,
						url TEXT NOT NULL,
						status TEXT NOT NULL,
						attempts INTEGER NOT NULL DEFAULT 0,
						request_body TEXT NOT NULL DEFAULT '',
						response_status INTEGER NOT NULL DEFAULT 0,
						response_body TEXT NOT NULL DEFAULT '',
						last_error TEXT NOT NULL DEFAULT '',
						next_attempt_at TIMESTAMP,
						created_at TIMESTAMP NOT NULL,
						updated_at TIMESTAMP NOT NULL
					);
					CREATE UNIQUE INDEX IF NOT EXISTS idx_webhook_delivery_event_subscription ON webhook_deliveries(event_id, subscription_id);
					CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_due ON webhook_deliveries(status, next_attempt_at, created_at);
					CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_subscription ON webhook_deliveries(subscription_id, created_at);
				`,
		},
		{
			version: 16,
			name:    "create_completion_audits_table",
			sql: `
				CREATE TABLE IF NOT EXISTS completion_audits (
					id TEXT PRIMARY KEY,
					task_id TEXT NOT NULL,
					requested_by_agent_id TEXT NOT NULL DEFAULT '',
					outcome TEXT NOT NULL,
					summary TEXT NOT NULL DEFAULT '',
					findings_json TEXT NOT NULL DEFAULT '[]',
					requested_at TIMESTAMP NOT NULL,
					decided_at TIMESTAMP
				);
				CREATE INDEX IF NOT EXISTS idx_completion_audits_task_requested ON completion_audits(task_id, requested_at DESC);
			`,
		},
		{
			version: 17,
			name:    "create_budget_accounting_tables",
			sql: `
				CREATE TABLE IF NOT EXISTS budget_policies (
					id TEXT PRIMARY KEY,
					scope_type TEXT NOT NULL CHECK (scope_type IN ('global', 'workspace', 'agent')),
					scope_id TEXT NOT NULL DEFAULT '',
					period_kind TEXT NOT NULL CHECK (period_kind IN ('lifetime')),
					period_start TIMESTAMP NOT NULL,
					period_end TIMESTAMP,
					currency TEXT NOT NULL DEFAULT 'USD',
					token_limit INTEGER NOT NULL DEFAULT 0 CHECK (token_limit >= 0),
					cost_limit_micros INTEGER NOT NULL DEFAULT 0 CHECK (cost_limit_micros >= 0),
					enforcement TEXT NOT NULL CHECK (enforcement IN ('hard', 'warn')),
					enabled BOOLEAN NOT NULL DEFAULT 1,
					version INTEGER NOT NULL DEFAULT 1,
					created_at TIMESTAMP NOT NULL,
					updated_at TIMESTAMP NOT NULL,
					CHECK ((scope_type = 'global' AND scope_id = '') OR (scope_type IN ('workspace', 'agent') AND scope_id <> '')),
					CHECK (period_end IS NULL OR period_end > period_start),
					CHECK (token_limit > 0 OR cost_limit_micros > 0)
				);
				CREATE UNIQUE INDEX IF NOT EXISTS idx_budget_policies_active_hard_scope_period
					ON budget_policies(scope_type, scope_id, period_kind, period_start)
					WHERE enabled = 1 AND enforcement = 'hard';
				CREATE INDEX IF NOT EXISTS idx_budget_policies_scope_period
					ON budget_policies(scope_type, scope_id, period_kind, period_start);

				CREATE TABLE IF NOT EXISTS budget_accounts (
					policy_id TEXT PRIMARY KEY REFERENCES budget_policies(id) ON DELETE CASCADE,
					settled_tokens INTEGER NOT NULL DEFAULT 0 CHECK (settled_tokens >= 0),
					reserved_tokens INTEGER NOT NULL DEFAULT 0 CHECK (reserved_tokens >= 0),
					settled_cost_micros INTEGER NOT NULL DEFAULT 0 CHECK (settled_cost_micros >= 0),
					reserved_cost_micros INTEGER NOT NULL DEFAULT 0 CHECK (reserved_cost_micros >= 0),
					status TEXT NOT NULL CHECK (status IN ('active', 'suspended', 'exhausted', 'reconciling', 'disabled')),
					suspension_reason TEXT NOT NULL DEFAULT '',
					version INTEGER NOT NULL DEFAULT 1,
					created_at TIMESTAMP NOT NULL,
					updated_at TIMESTAMP NOT NULL
				);
				CREATE INDEX IF NOT EXISTS idx_budget_accounts_status ON budget_accounts(status);

				CREATE TABLE IF NOT EXISTS budget_reservations (
					id TEXT PRIMARY KEY,
					request_id TEXT NOT NULL UNIQUE,
					task_id TEXT NOT NULL DEFAULT '',
					run_id TEXT NOT NULL DEFAULT '',
					agent_id TEXT NOT NULL DEFAULT '',
					workspace_id TEXT NOT NULL DEFAULT '',
					provider TEXT NOT NULL DEFAULT '',
					model TEXT NOT NULL DEFAULT '',
					pricing_version TEXT NOT NULL DEFAULT '',
					currency TEXT NOT NULL DEFAULT 'USD',
					reserved_tokens INTEGER NOT NULL DEFAULT 0 CHECK (reserved_tokens >= 0),
					reserved_cost_micros INTEGER NOT NULL DEFAULT 0 CHECK (reserved_cost_micros >= 0),
					settled_tokens INTEGER NOT NULL DEFAULT 0 CHECK (settled_tokens >= 0),
					settled_cost_micros INTEGER NOT NULL DEFAULT 0 CHECK (settled_cost_micros >= 0),
					status TEXT NOT NULL CHECK (status IN ('reserved', 'sent', 'settled', 'aborted_before_send', 'reconcile_required', 'charged_conservatively')),
					provider_request_id TEXT NOT NULL DEFAULT '',
					lease_expires_at TIMESTAMP,
					settlement_deadline_at TIMESTAMP,
					created_at TIMESTAMP NOT NULL,
					updated_at TIMESTAMP NOT NULL,
					settled_at TIMESTAMP,
					CHECK (reserved_tokens > 0 OR reserved_cost_micros > 0)
				);
				CREATE INDEX IF NOT EXISTS idx_budget_reservations_status_deadline
					ON budget_reservations(status, settlement_deadline_at, created_at);
				CREATE INDEX IF NOT EXISTS idx_budget_reservations_scope_created
					ON budget_reservations(workspace_id, agent_id, created_at);

				CREATE TABLE IF NOT EXISTS pricing_catalog (
					id TEXT PRIMARY KEY,
					provider TEXT NOT NULL,
					model TEXT NOT NULL,
					version TEXT NOT NULL,
					currency TEXT NOT NULL DEFAULT 'USD',
					input_cost_micros_per_million INTEGER NOT NULL DEFAULT 0 CHECK (input_cost_micros_per_million >= 0),
					output_cost_micros_per_million INTEGER NOT NULL DEFAULT 0 CHECK (output_cost_micros_per_million >= 0),
					cached_cost_micros_per_million INTEGER NOT NULL DEFAULT 0 CHECK (cached_cost_micros_per_million >= 0),
					valid_from TIMESTAMP NOT NULL,
					valid_until TIMESTAMP,
					created_at TIMESTAMP NOT NULL,
					updated_at TIMESTAMP NOT NULL,
					UNIQUE (provider, model, version),
					CHECK (valid_until IS NULL OR valid_until > valid_from)
				);
				CREATE INDEX IF NOT EXISTS idx_pricing_catalog_lookup
					ON pricing_catalog(provider, model, currency, valid_from);
			`,
		},
	}
}
