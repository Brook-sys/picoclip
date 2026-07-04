package sqlite

import (
	"context"
	"database/sql"

	_ "modernc.org/sqlite"

	"picoclip/internal/core/ports"
)

type txKeyType struct{}

var txKey = txKeyType{}

type Queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func getQueryer(ctx context.Context, db *sql.DB) Queryer {
	if tx, ok := ctx.Value(txKey).(*sql.Tx); ok {
		return tx
	}
	return db
}

type Storage struct {
	db *sql.DB
}

func NewStorage(db *sql.DB) *Storage {
	return &Storage{db: db}
}

func (s *Storage) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txKey).(*sql.Tx); ok {
		return fn(ctx)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	ctxWithTx := context.WithValue(ctx, txKey, tx)
	if err := fn(ctxWithTx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
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

func (s *Storage) Wakeups() ports.WakeupRepository {
	return &WakeupRepository{db: s.db}
}

func (s *Storage) Usage() ports.UsageRepository {
	return &UsageRepository{db: s.db}
}

func (s *Storage) Budgets() ports.BudgetRepository {
	return &BudgetRepository{db: s.db}
}

func (s *Storage) Webhooks() ports.WebhookRepository {
	return &WebhookRepository{db: s.db}
}

func (s *Storage) ResetAllData(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM webhook_deliveries;
		DELETE FROM webhook_subscriptions;
		DELETE FROM budgets;
		DELETE FROM wakeups;
		DELETE FROM usage_events;
		DELETE FROM messages;
		DELETE FROM outbox_events;
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
