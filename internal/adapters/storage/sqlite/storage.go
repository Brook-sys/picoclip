package sqlite

import (
	"context"
	"database/sql"

	_ "modernc.org/sqlite"

	"picoclip/internal/core/ports"
)

type Storage struct {
	db *sql.DB
}

func NewStorage(db *sql.DB) *Storage {
	return &Storage{db: db}
}

func (s *Storage) Agents() ports.AgentRepository {
	return &AgentRepository{db: s.db}
}

func (s *Storage) Tasks() ports.TaskRepository {
	return &TaskRepository{db: s.db}
}

func (s *Storage) Runs() ports.RunRepository {
	return &RunRepository{db: s.db}
}

func (s *Storage) Runtimes() ports.RuntimeRepository {
	return &RuntimeRepository{db: s.db}
}

func (s *Storage) Events() ports.EventRepository {
	return &EventRepository{db: s.db}
}

func (s *Storage) Messages() ports.MessageRepository {
	return &MessageRepository{db: s.db}
}

func (s *Storage) Skills() ports.SkillRepository {
	return &SkillRepository{db: s.db}
}

func (s *Storage) Workspaces() ports.WorkspaceRepository {
	return &WorkspaceRepository{db: s.db}
}

func (s *Storage) Settings() ports.SettingsRepository {
	return &SettingsRepository{db: s.db}
}

func (s *Storage) ResetAllData(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM messages;
		DELETE FROM events;
		DELETE FROM runs;
		DELETE FROM tasks;
		DELETE FROM skills;
		DELETE FROM agents;
		DELETE FROM workspaces;
		DELETE FROM settings;
		DELETE FROM runtime_states;
	`)
	return err
}
