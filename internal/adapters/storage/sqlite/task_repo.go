package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

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
			mode, loop_delay_seconds, loop_run_count, loop_next_run_at, loop_paused_at, loop_audit_prompt,
			attempts, max_attempts, needs_run, checkout_run_id, checked_out_by_agent_id,
			execution_locked_at, lock_expires_at, cancel_reason, input_tokens, output_tokens, total_tokens, created_at, updated_at, started_at, finished_at, completed_at, cancelled_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	q := getQueryer(ctx, r.db)
	_, err := q.ExecContext(ctx, query,
		task.ID, task.ParentID, task.WorkspaceID, task.AgentID, task.Title, task.Prompt, string(task.Status), task.Priority,
		string(task.Mode), task.LoopDelaySeconds, task.LoopRunCount, task.LoopNextRunAt, task.LoopPausedAt, task.LoopAuditPrompt,
		task.Attempts, task.MaxAttempts, task.NeedsRun, task.CheckoutRunID, task.CheckedOutByAgentID,
		task.ExecutionLockedAt, task.LockExpiresAt, task.CancelReason, task.InputTokens, task.OutputTokens, task.TotalTokens, task.CreatedAt, task.UpdatedAt, task.StartedAt, task.FinishedAt, task.CompletedAt, task.CancelledAt,
	)
	return err
}

func (r *TaskRepository) Get(ctx context.Context, id string) (domain.Task, error) {
	query := `
		SELECT id, parent_id, workspace_id, agent_id, title, prompt, status, priority,
			mode, loop_delay_seconds, loop_run_count, loop_next_run_at, loop_paused_at, loop_audit_prompt,
			attempts, max_attempts, needs_run, checkout_run_id, checked_out_by_agent_id,
			execution_locked_at, lock_expires_at, cancel_reason, input_tokens, output_tokens, total_tokens, created_at, updated_at, started_at, finished_at, completed_at, cancelled_at
		FROM tasks WHERE id = ?
	`
	q := getQueryer(ctx, r.db)
	row := q.QueryRowContext(ctx, query, id)
	return scanTask(row)
}

func (r *TaskRepository) List(ctx context.Context, filter ports.TaskFilter) ([]domain.Task, error) {
	query := `
		SELECT id, parent_id, workspace_id, agent_id, title, prompt, status, priority,
			mode, loop_delay_seconds, loop_run_count, loop_next_run_at, loop_paused_at, loop_audit_prompt,
			attempts, max_attempts, needs_run, checkout_run_id, checked_out_by_agent_id,
			execution_locked_at, lock_expires_at, cancel_reason, input_tokens, output_tokens, total_tokens, created_at, updated_at, started_at, finished_at, completed_at, cancelled_at
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
			mode = ?, loop_delay_seconds = ?, loop_run_count = ?, loop_next_run_at = ?, loop_paused_at = ?, loop_audit_prompt = ?,
			attempts = ?, max_attempts = ?, needs_run = ?, checkout_run_id = ?, checked_out_by_agent_id = ?,
			execution_locked_at = ?, lock_expires_at = ?, cancel_reason = ?, input_tokens = ?, output_tokens = ?, total_tokens = ?, updated_at = ?, started_at = ?, finished_at = ?, completed_at = ?, cancelled_at = ?
		WHERE id = ?
	`

	q := getQueryer(ctx, r.db)
	res, err := q.ExecContext(ctx, query,
		task.ParentID, task.WorkspaceID, task.AgentID, task.Title, task.Prompt, string(task.Status), task.Priority,
		string(task.Mode), task.LoopDelaySeconds, task.LoopRunCount, task.LoopNextRunAt, task.LoopPausedAt, task.LoopAuditPrompt,
		task.Attempts, task.MaxAttempts, task.NeedsRun, task.CheckoutRunID, task.CheckedOutByAgentID,
		task.ExecutionLockedAt, task.LockExpiresAt, task.CancelReason, task.InputTokens, task.OutputTokens, task.TotalTokens, task.UpdatedAt, task.StartedAt, task.FinishedAt, task.CompletedAt, task.CancelledAt,
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

func (r *TaskRepository) UpdateIfUnchanged(ctx context.Context, task domain.Task, precondition ports.TaskPrecondition) (bool, error) {
	query := `UPDATE tasks SET
		parent_id=?, workspace_id=?, agent_id=?, title=?, prompt=?, status=?, priority=?, mode=?, loop_delay_seconds=?, loop_run_count=?, loop_next_run_at=?, loop_paused_at=?, loop_audit_prompt=?, attempts=?, max_attempts=?, needs_run=?, checkout_run_id=?, checked_out_by_agent_id=?, execution_locked_at=?, lock_expires_at=?, cancel_reason=?, input_tokens=?, output_tokens=?, total_tokens=?, updated_at=?, started_at=?, finished_at=?, completed_at=?, cancelled_at=?
		WHERE id=? AND status=? AND updated_at=? AND checkout_run_id=?`
	res, err := getQueryer(ctx, r.db).ExecContext(ctx, query,
		task.ParentID, task.WorkspaceID, task.AgentID, task.Title, task.Prompt, string(task.Status), task.Priority, string(task.Mode), task.LoopDelaySeconds, task.LoopRunCount, task.LoopNextRunAt, task.LoopPausedAt, task.LoopAuditPrompt, task.Attempts, task.MaxAttempts, task.NeedsRun, task.CheckoutRunID, task.CheckedOutByAgentID, task.ExecutionLockedAt, task.LockExpiresAt, task.CancelReason, task.InputTokens, task.OutputTokens, task.TotalTokens, task.UpdatedAt, task.StartedAt, task.FinishedAt, task.CompletedAt, task.CancelledAt,
		task.ID, string(precondition.Status), precondition.UpdatedAt, precondition.CheckoutRunID,
	)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

func (r *TaskRepository) ClaimNextPending(ctx context.Context) (domain.Task, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Task{}, err
	}
	defer tx.Rollback()

	query := `
		SELECT id, parent_id, workspace_id, agent_id, title, prompt, status, priority,
			mode, loop_delay_seconds, loop_run_count, loop_next_run_at, loop_paused_at, loop_audit_prompt,
			attempts, max_attempts, needs_run, checkout_run_id, checked_out_by_agent_id,
			execution_locked_at, lock_expires_at, cancel_reason, input_tokens, output_tokens, total_tokens, created_at, updated_at, started_at, finished_at, completed_at, cancelled_at
		FROM tasks
		WHERE needs_run = 1
			AND status NOT IN ('done', 'cancelled')
			AND checkout_run_id = ''
			AND checked_out_by_agent_id = ''
			AND (max_attempts <= 0 OR attempts < max_attempts)
		ORDER BY priority DESC, created_at ASC LIMIT 1
	`
	row := tx.QueryRowContext(ctx, query)
	task, err := scanTask(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, domain.ErrNotFound) {
			return domain.Task{}, domain.ErrNoPendingTasks
		}
		return domain.Task{}, err
	}

	updateQuery := `
		UPDATE tasks SET needs_run = 0
		WHERE id = ?
			AND needs_run = 1
			AND status NOT IN ('done', 'cancelled')
			AND checkout_run_id = ''
			AND checked_out_by_agent_id = ''
			AND (max_attempts <= 0 OR attempts < max_attempts)
	`
	res, err := tx.ExecContext(ctx, updateQuery, task.ID)
	if err != nil {
		return domain.Task{}, err
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return domain.Task{}, err
	}
	if rowsAffected == 0 {
		return domain.Task{}, domain.ErrNoPendingTasks
	}

	task.NeedsRun = false
	if err := tx.Commit(); err != nil {
		return domain.Task{}, err
	}

	return task, nil
}

func (r *TaskRepository) ClaimNextRunnable(ctx context.Context, now time.Time, lockTTL time.Duration) (domain.Task, domain.Run, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Task{}, domain.Run{}, err
	}
	defer tx.Rollback()

	query := `
		SELECT id, parent_id, workspace_id, agent_id, title, prompt, status, priority,
			mode, loop_delay_seconds, loop_run_count, loop_next_run_at, loop_paused_at, loop_audit_prompt,
			attempts, max_attempts, needs_run, checkout_run_id, checked_out_by_agent_id,
			execution_locked_at, lock_expires_at, cancel_reason, input_tokens, output_tokens, total_tokens, created_at, updated_at, started_at, finished_at, completed_at, cancelled_at
		FROM tasks
		WHERE needs_run = 1
			AND status NOT IN ('done', 'cancelled')
			AND checkout_run_id = ''
			AND checked_out_by_agent_id = ''
			AND (max_attempts <= 0 OR attempts < max_attempts)
		ORDER BY priority DESC, created_at ASC LIMIT 1
	`
	row := tx.QueryRowContext(ctx, query)
	task, err := scanTask(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, domain.ErrNotFound) {
			return domain.Task{}, domain.Run{}, domain.ErrNoPendingTasks
		}
		return domain.Task{}, domain.Run{}, err
	}

	runID := "run_" + task.ID + "_" + time.Now().Format("20060102150405")
	lockExpires := now.Add(lockTTL)

	update := `
		UPDATE tasks SET
			needs_run = 0,
			status = 'in_progress',
			attempts = attempts + 1,
			checkout_run_id = ?,
			checked_out_by_agent_id = agent_id,
			started_at = ?,
			execution_locked_at = ?,
			lock_expires_at = ?,
			updated_at = ?
		WHERE id = ?
			AND needs_run = 1
			AND status NOT IN ('done', 'cancelled')
			AND checkout_run_id = ''
			AND checked_out_by_agent_id = ''
			AND (max_attempts <= 0 OR attempts < max_attempts)
	`
	res, err := tx.ExecContext(ctx, update, runID, now, now, lockExpires, now, task.ID)
	if err != nil {
		return domain.Task{}, domain.Run{}, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return domain.Task{}, domain.Run{}, err
	}
	if rows == 0 {
		return domain.Task{}, domain.Run{}, domain.ErrNoPendingTasks
	}

	row = tx.QueryRowContext(ctx, `
		SELECT id, parent_id, workspace_id, agent_id, title, prompt, status, priority,
			mode, loop_delay_seconds, loop_run_count, loop_next_run_at, loop_paused_at, loop_audit_prompt,
			attempts, max_attempts, needs_run, checkout_run_id, checked_out_by_agent_id,
			execution_locked_at, lock_expires_at, cancel_reason, input_tokens, output_tokens, total_tokens, created_at, updated_at, started_at, finished_at, completed_at, cancelled_at
		FROM tasks WHERE id = ?
	`, task.ID)
	task, err = scanTask(row)
	if err != nil {
		return domain.Task{}, domain.Run{}, err
	}

	run := domain.Run{
		ID:           task.CheckoutRunID,
		TaskID:       task.ID,
		AgentID:      task.AgentID,
		DriverType:   "",
		Status:       domain.RunStatusRunning,
		Attempt:      task.Attempts,
		StartedAt:    now,
		LastOutputAt: &now,
		StallTimeout: int(lockTTL.Seconds()),
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO runs (id, task_id, agent_id, driver_type, status, attempt, input, output, error, input_tokens, output_tokens, total_tokens, process_id, last_output_at, stall_timeout, started_at, finished_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, run.ID, run.TaskID, run.AgentID, run.DriverType, string(run.Status), run.Attempt, run.Input, run.Output, run.Error, run.InputTokens, run.OutputTokens, run.TotalTokens, run.ProcessID, run.LastOutputAt, run.StallTimeout, run.StartedAt, run.FinishedAt); err != nil {
		return domain.Task{}, domain.Run{}, err
	}

	if err := tx.Commit(); err != nil {
		return domain.Task{}, domain.Run{}, err
	}

	return task, run, nil
}

func scanTask(row scanner) (domain.Task, error) {
	var t domain.Task
	var statusStr, modeStr string

	err := row.Scan(
		&t.ID, &t.ParentID, &t.WorkspaceID, &t.AgentID, &t.Title, &t.Prompt, &statusStr, &t.Priority,
		&modeStr, &t.LoopDelaySeconds, &t.LoopRunCount, &t.LoopNextRunAt, &t.LoopPausedAt, &t.LoopAuditPrompt,
		&t.Attempts, &t.MaxAttempts, &t.NeedsRun, &t.CheckoutRunID, &t.CheckedOutByAgentID,
		&t.ExecutionLockedAt, &t.LockExpiresAt, &t.CancelReason, &t.InputTokens, &t.OutputTokens, &t.TotalTokens, &t.CreatedAt, &t.UpdatedAt, &t.StartedAt, &t.FinishedAt, &t.CompletedAt, &t.CancelledAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Task{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Task{}, err
	}
	t.Status = domain.TaskStatus(statusStr)
	t.Mode = domain.TaskMode(modeStr)
	return t, nil
}

func (r *TaskRepository) Delete(ctx context.Context, id string) error {
	q := getQueryer(ctx, r.db)
	res, err := q.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, id)
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
