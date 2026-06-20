package sqlite

import "context"

func (s *Storage) Migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
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
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_agents_created_at ON agents(created_at);
	`)
	return err
}
