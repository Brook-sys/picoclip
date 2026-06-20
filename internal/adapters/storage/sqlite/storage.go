package sqlite

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"

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

func (s *Storage) Tasks() ports.TaskRepository           { return nil }
func (s *Storage) Runs() ports.RunRepository             { return nil }
func (s *Storage) Events() ports.EventRepository         { return nil }
func (s *Storage) Messages() ports.MessageRepository     { return nil }
func (s *Storage) Skills() ports.SkillRepository         { return nil }
func (s *Storage) Workspaces() ports.WorkspaceRepository { return nil }
