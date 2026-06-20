package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"picoclip/internal/core/domain"
)

type EventRepository struct {
	db *sql.DB
}

func (r *EventRepository) Create(ctx context.Context, event domain.Event) error {
	dataJSON, _ := json.Marshal(event.Data)

	query := `
		INSERT INTO events (
			id, type, task_id, agent_id, run_id, message, data, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.ExecContext(ctx, query,
		event.ID, string(event.Type), event.TaskID, event.AgentID, event.RunID,
		event.Message, string(dataJSON), event.CreatedAt,
	)
	return err
}

func (r *EventRepository) ListByTask(ctx context.Context, taskID string) ([]domain.Event, error) {
	query := `
		SELECT id, type, task_id, agent_id, run_id, message, data, created_at
		FROM events WHERE task_id = ? ORDER BY created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []domain.Event
	for rows.Next() {
		ev, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func (r *EventRepository) ListRecent(ctx context.Context, limit int) ([]domain.Event, error) {
	query := `
		SELECT id, type, task_id, agent_id, run_id, message, data, created_at
		FROM events ORDER BY created_at DESC LIMIT ?
	`
	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []domain.Event
	for rows.Next() {
		ev, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func scanEvent(row scanner) (domain.Event, error) {
	var e domain.Event
	var typeStr, dataJSON string

	err := row.Scan(
		&e.ID, &typeStr, &e.TaskID, &e.AgentID, &e.RunID, &e.Message, &dataJSON, &e.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Event{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Event{}, err
	}
	e.Type = domain.EventType(typeStr)
	if dataJSON != "" && dataJSON != "null" {
		_ = json.Unmarshal([]byte(dataJSON), &e.Data)
	}
	return e, nil
}
