package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"picoclip/internal/core/domain"
)

type UsageRepository struct {
	db *sql.DB
}

func (r *UsageRepository) Create(ctx context.Context, event domain.UsageEvent) error {
	query := `
		INSERT OR IGNORE INTO usage_events (id, run_id, task_id, agent_id, provider, model, input_tokens, output_tokens, cached_tokens, cost_micros, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.ExecContext(ctx, query,
		event.ID, event.RunID, event.TaskID, event.AgentID, event.Provider, event.Model,
		event.InputTokens, event.OutputTokens, event.CachedTokens, event.CostMicros, event.CreatedAt,
	)
	return err
}

func (r *UsageRepository) List(ctx context.Context) ([]domain.UsageEvent, error) {
	query := `
		SELECT id, run_id, task_id, agent_id, provider, model, input_tokens, output_tokens, cached_tokens, cost_micros, created_at
		FROM usage_events ORDER BY created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []domain.UsageEvent
	for rows.Next() {
		var ev domain.UsageEvent
		if err := rows.Scan(&ev.ID, &ev.RunID, &ev.TaskID, &ev.AgentID, &ev.Provider, &ev.Model,
			&ev.InputTokens, &ev.OutputTokens, &ev.CachedTokens, &ev.CostMicros, &ev.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}

func (r *UsageRepository) ListByTask(ctx context.Context, taskID string) ([]domain.UsageEvent, error) {
	query := `
		SELECT id, run_id, task_id, agent_id, provider, model, input_tokens, output_tokens, cached_tokens, cost_micros, created_at
		FROM usage_events WHERE task_id = ? ORDER BY created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []domain.UsageEvent
	for rows.Next() {
		var ev domain.UsageEvent
		if err := rows.Scan(&ev.ID, &ev.RunID, &ev.TaskID, &ev.AgentID, &ev.Provider, &ev.Model,
			&ev.InputTokens, &ev.OutputTokens, &ev.CachedTokens, &ev.CostMicros, &ev.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}

func (r *UsageRepository) SumByAgent(ctx context.Context, agentID string) (input, output, cached int, costMicros int64, err error) {
	query := `
		SELECT COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0), COALESCE(SUM(cached_tokens),0), COALESCE(SUM(cost_micros),0)
		FROM usage_events WHERE agent_id = ?
	`
	row := r.db.QueryRowContext(ctx, query, agentID)
	err = row.Scan(&input, &output, &cached, &costMicros)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, 0, 0, nil
	}
	return
}
