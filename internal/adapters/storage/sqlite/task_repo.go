package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

type TaskRepository struct {
	db *sql.DB
}

func (r *TaskRepository) Create(ctx context.Context, task domain.Task) error {
	query := `
		INSERT INTO tasks (
			id, parent_id, workspace_id, agent_id, title, prompt, status, priority,
			attempts, max_attempts, needs_run, checkout_run_id, checked_out_by_agent_id,
			cancel_reason, created_at, updated_at, started_at, finished_at, completed_at, cancelled_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := r.db.ExecContext(ctx, query,
		task.ID, task.ParentID, task.WorkspaceID, task.AgentID, task.Title, task.Prompt, string(task.Status), task.Priority,
		task.Attempts, task.MaxAttempts, task.NeedsRun, task.CheckoutRunID, task.CheckedOutByAgentID,
		task.CancelReason, task.CreatedAt, task.UpdatedAt, task.StartedAt, task.FinishedAt, task.CompletedAt, task.CancelledAt,
	)
	return err
}

func (r *TaskRepository) Get(ctx context.Context, id string) (domain.Task, error) {
	query := `
		SELECT id, parent_id, workspace_id, agent_id, title, prompt, status, priority,
			attempts, max_attempts, needs_run, checkout_run_id, checked_out_by_agent_id,
			cancel_reason, created_at, updated_at, started_at, finished_at, completed_at, cancelled_at
		FROM tasks WHERE id = ?
	`
	row := r.db.QueryRowContext(ctx, query, id)
	return scanTask(row)
}

func (r *TaskRepository) List(ctx context.Context, filter ports.TaskFilter) ([]domain.Task, error) {
	query := `
		SELECT id, parent_id, workspace_id, agent_id, title, prompt, status, priority,
			attempts, max_attempts, needs_run, checkout_run_id, checked_out_by_agent_id,
			cancel_reason, created_at, updated_at, started_at, finished_at, completed_at, cancelled_at
		FROM tasks
		WHERE 1=1
	`
	var args []any

	if filter.AgentID != "" {
		query += " AND agent_id = ?"
		args = append(args, filter.AgentID)
	}
	if filter.ParentID != "" {
		query += " AND parent_id = ?"
		args = append(args, filter.ParentID)
	}
	if filter.WorkspaceID != "" {
		query += " AND workspace_id = ?"
		args = append(args, filter.WorkspaceID)
	}
	if filter.Status != "" {
		query += " AND status = ?"
		args = append(args, string(filter.Status))
	}
	if len(filter.Statuses) > 0 {
		query += " AND status IN ("
		for i, status := range filter.Statuses {
			if i > 0 {
				query += ", "
			}
			query += "?"
			args = append(args, string(status))
		}
		query += ")"
	}

	query += " ORDER BY created_at ASC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []domain.Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (r *TaskRepository) Update(ctx context.Context, task domain.Task) error {
	query := `
		UPDATE tasks SET
			parent_id = ?, workspace_id = ?, agent_id = ?, title = ?, prompt = ?, status = ?, priority = ?,
			attempts = ?, max_attempts = ?, needs_run = ?, checkout_run_id = ?, checked_out_by_agent_id = ?,
			cancel_reason = ?, updated_at = ?, started_at = ?, finished_at = ?, completed_at = ?, cancelled_at = ?
		WHERE id = ?
	`

	res, err := r.db.ExecContext(ctx, query,
		task.ParentID, task.WorkspaceID, task.AgentID, task.Title, task.Prompt, string(task.Status), task.Priority,
		task.Attempts, task.MaxAttempts, task.NeedsRun, task.CheckoutRunID, task.CheckedOutByAgentID,
		task.CancelReason, task.UpdatedAt, task.StartedAt, task.FinishedAt, task.CompletedAt, task.CancelledAt,
		task.ID,
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

func (r *TaskRepository) ClaimNextPending(ctx context.Context) (domain.Task, error) {
	// SQLite supports returning with UPDATE ... RETURNING * in modern versions, but we can do a transaction.
	// For simplicity in a single-file DB:
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Task{}, err
	}
	defer tx.Rollback()

	query := `
		SELECT id, parent_id, workspace_id, agent_id, title, prompt, status, priority,
			attempts, max_attempts, needs_run, checkout_run_id, checked_out_by_agent_id,
			cancel_reason, created_at, updated_at, started_at, finished_at, completed_at, cancelled_at
		FROM tasks 
		WHERE needs_run = 1 AND status != 'done' AND status != 'cancelled'
		ORDER BY created_at ASC LIMIT 1
	`
	row := tx.QueryRowContext(ctx, query)
	task, err := scanTask(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Task{}, domain.ErrNoPendingTasks
		}
		return domain.Task{}, err
	}

	task.NeedsRun = false
	updateQuery := `UPDATE tasks SET needs_run = 0 WHERE id = ?`
	if _, err := tx.ExecContext(ctx, updateQuery, task.ID); err != nil {
		return domain.Task{}, err
	}

	if err := tx.Commit(); err != nil {
		return domain.Task{}, err
	}

	return task, nil
}

func scanTask(row scanner) (domain.Task, error) {
	var t domain.Task
	var statusStr string

	err := row.Scan(
		&t.ID, &t.ParentID, &t.WorkspaceID, &t.AgentID, &t.Title, &t.Prompt, &statusStr, &t.Priority,
		&t.Attempts, &t.MaxAttempts, &t.NeedsRun, &t.CheckoutRunID, &t.CheckedOutByAgentID,
		&t.CancelReason, &t.CreatedAt, &t.UpdatedAt, &t.StartedAt, &t.FinishedAt, &t.CompletedAt, &t.CancelledAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Task{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Task{}, err
	}
	t.Status = domain.TaskStatus(statusStr)
	return t, nil
}
