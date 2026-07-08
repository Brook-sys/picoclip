package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

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
	q := getQueryer(ctx, r.db)
	_, err := q.ExecContext(ctx, query,
		event.ID, string(event.Type), event.TaskID, event.AgentID, event.RunID,
		event.Message, string(dataJSON), event.CreatedAt,
	)
	return err
}

func (r *EventRepository) CreateOutbox(ctx context.Context, event domain.Event) error {
	payload, _ := json.Marshal(event)
	query := `
		INSERT INTO outbox_events (id, type, payload, created_at)
		VALUES (?, ?, ?, ?)
	`
	q := getQueryer(ctx, r.db)
	_, err := q.ExecContext(ctx, query, event.ID, string(event.Type), string(payload), event.CreatedAt)
	return err
}

func (r *EventRepository) ListOutbox(ctx context.Context, limit int) ([]domain.Event, error) {
	query := `
		SELECT payload FROM outbox_events
		WHERE attempts < 10 AND (next_attempt_at IS NULL OR next_attempt_at <= CURRENT_TIMESTAMP)
		ORDER BY created_at ASC LIMIT ?
	`
	q := getQueryer(ctx, r.db)
	rows, err := q.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []domain.Event
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var ev domain.Event
		if err := json.Unmarshal([]byte(payload), &ev); err == nil {
			events = append(events, ev)
		}
	}
	return events, rows.Err()
}

func (r *EventRepository) DeleteOutbox(ctx context.Context, id string) error {
	query := `DELETE FROM outbox_events WHERE id = ?`
	q := getQueryer(ctx, r.db)
	_, err := q.ExecContext(ctx, query, id)
	return err
}

func (r *EventRepository) MarkOutboxFailed(ctx context.Context, id string, message string, nextAttemptAt time.Time) error {
	query := `
		UPDATE outbox_events
		SET attempts = attempts + 1, last_error = ?, next_attempt_at = ?
		WHERE id = ?
	`
	q := getQueryer(ctx, r.db)
	_, err := q.ExecContext(ctx, query, message, nextAttemptAt, id)
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

func (r *EventRepository) DeleteByTask(ctx context.Context, taskID string) error {
	q := getQueryer(ctx, r.db)
	_, err := q.ExecContext(ctx, `DELETE FROM events WHERE task_id = ?`, taskID)
	return err
}

func (r *EventRepository) DeleteAll(ctx context.Context) (int, error) {
	q := getQueryer(ctx, r.db)
	res, err := q.ExecContext(ctx, `DELETE FROM events`)
	if err != nil {
		return 0, err
	}
	deleted, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(deleted), nil
}
