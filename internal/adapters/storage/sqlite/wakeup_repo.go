package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"picoclip/internal/core/domain"
)

type WakeupRepository struct {
	db *sql.DB
}

func (r *WakeupRepository) Create(ctx context.Context, wakeup domain.WakeupRequest) error {
	query := `
		INSERT INTO wakeups (
			id, agent_id, task_id, reason, status, priority, due_at, claimed_at, payload, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	payloadJSON := "{}"
	if len(wakeup.Payload) > 0 {
		b, _ := json.Marshal(wakeup.Payload)
		payloadJSON = string(b)
	}
	_, err := r.db.ExecContext(ctx, query,
		wakeup.ID, wakeup.AgentID, wakeup.TaskID, string(wakeup.Reason), string(wakeup.Status), wakeup.Priority,
		wakeup.DueAt, wakeup.ClaimedAt, payloadJSON, wakeup.CreatedAt, wakeup.UpdatedAt,
	)
	return err
}

func (r *WakeupRepository) Get(ctx context.Context, id string) (domain.WakeupRequest, error) {
	query := `
		SELECT id, agent_id, task_id, reason, status, priority, due_at, claimed_at, payload, created_at, updated_at
		FROM wakeups WHERE id = ?
	`
	row := r.db.QueryRowContext(ctx, query, id)
	return scanWakeup(row)
}

func (r *WakeupRepository) ListPending(ctx context.Context, now time.Time, limit int) ([]domain.WakeupRequest, error) {
	query := `
		SELECT id, agent_id, task_id, reason, status, priority, due_at, claimed_at, payload, created_at, updated_at
		FROM wakeups
		WHERE status = 'pending' AND due_at <= ?
		ORDER BY priority DESC, due_at ASC
	`
	if limit > 0 {
		query += " LIMIT ?"
	}
	args := []any{now}
	if limit > 0 {
		args = append(args, limit)
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var wakeups []domain.WakeupRequest
	for rows.Next() {
		wakeup, err := scanWakeup(rows)
		if err != nil {
			return nil, err
		}
		wakeups = append(wakeups, wakeup)
	}
	return wakeups, rows.Err()
}

func (r *WakeupRepository) ListByTask(ctx context.Context, taskID string) ([]domain.WakeupRequest, error) {
	query := `
		SELECT id, agent_id, task_id, reason, status, priority, due_at, claimed_at, payload, created_at, updated_at
		FROM wakeups WHERE task_id = ? ORDER BY created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var wakeups []domain.WakeupRequest
	for rows.Next() {
		wakeup, err := scanWakeup(rows)
		if err != nil {
			return nil, err
		}
		wakeups = append(wakeups, wakeup)
	}
	return wakeups, rows.Err()
}

func (r *WakeupRepository) Update(ctx context.Context, wakeup domain.WakeupRequest) error {
	query := `
		UPDATE wakeups SET
			agent_id = ?, task_id = ?, reason = ?, status = ?, priority = ?, due_at = ?, claimed_at = ?, payload = ?, updated_at = ?
		WHERE id = ?
	`
	payloadJSON := "{}"
	if len(wakeup.Payload) > 0 {
		b, _ := json.Marshal(wakeup.Payload)
		payloadJSON = string(b)
	}
	res, err := r.db.ExecContext(ctx, query,
		wakeup.AgentID, wakeup.TaskID, string(wakeup.Reason), string(wakeup.Status), wakeup.Priority,
		wakeup.DueAt, wakeup.ClaimedAt, payloadJSON, wakeup.UpdatedAt, wakeup.ID,
	)
	if err != nil {
		return err
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func scanWakeup(row scanner) (domain.WakeupRequest, error) {
	var w domain.WakeupRequest
	var reasonStr, statusStr string
	var payloadJSON string
	err := row.Scan(
		&w.ID, &w.AgentID, &w.TaskID, &reasonStr, &statusStr, &w.Priority,
		&w.DueAt, &w.ClaimedAt, &payloadJSON, &w.CreatedAt, &w.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.WakeupRequest{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.WakeupRequest{}, err
	}
	w.Reason = domain.WakeupReason(reasonStr)
	w.Status = domain.WakeupStatus(statusStr)
	if payloadJSON != "" && payloadJSON != "{}" {
		_ = json.Unmarshal([]byte(payloadJSON), &w.Payload)
	}
	return w, nil
}

func (r *WakeupRepository) DeleteByTask(ctx context.Context, taskID string) error {
	q := getQueryer(ctx, r.db)
	_, err := q.ExecContext(ctx, `DELETE FROM wakeups WHERE task_id = ?`, taskID)
	return err
}
