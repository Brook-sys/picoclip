package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"picoclip/internal/core/domain"
)

type RunRepository struct {
	db *sql.DB
}

func (r *RunRepository) Create(ctx context.Context, run domain.Run) error {
	query := `
		INSERT INTO runs (
			id, task_id, agent_id, driver_type, status, attempt,
			input, output, error, input_tokens, output_tokens, total_tokens,
			process_id, last_output_at, stall_timeout, started_at, finished_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.ExecContext(ctx, query,
		run.ID, run.TaskID, run.AgentID, string(run.DriverType), string(run.Status), run.Attempt,
		run.Input, run.Output, run.Error, run.InputTokens, run.OutputTokens, run.TotalTokens,
		run.ProcessID, run.LastOutputAt, run.StallTimeout, run.StartedAt, run.FinishedAt,
	)
	return err
}

func (r *RunRepository) Get(ctx context.Context, id string) (domain.Run, error) {
	query := `
		SELECT id, task_id, agent_id, driver_type, status, attempt,
			input, output, error, input_tokens, output_tokens, total_tokens,
			process_id, last_output_at, stall_timeout, started_at, finished_at
		FROM runs WHERE id = ?
	`
	row := r.db.QueryRowContext(ctx, query, id)
	return scanRun(row)
}

func (r *RunRepository) ListRunning(ctx context.Context) ([]domain.Run, error) {
	query := `
		SELECT id, task_id, agent_id, driver_type, status, attempt,
			input, output, error, input_tokens, output_tokens, total_tokens,
			process_id, last_output_at, stall_timeout, started_at, finished_at
		FROM runs WHERE status = 'running' ORDER BY started_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []domain.Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return runs, nil
}

func (r *RunRepository) ListByTask(ctx context.Context, taskID string) ([]domain.Run, error) {
	query := `
		SELECT id, task_id, agent_id, driver_type, status, attempt,
			input, output, error, input_tokens, output_tokens, total_tokens,
			process_id, last_output_at, stall_timeout, started_at, finished_at
		FROM runs WHERE task_id = ? ORDER BY started_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []domain.Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return runs, nil
}

func (r *RunRepository) Update(ctx context.Context, run domain.Run) error {
	query := `
		UPDATE runs SET
			task_id = ?, agent_id = ?, driver_type = ?, status = ?, attempt = ?,
			input = ?, output = ?, error = ?, input_tokens = ?, output_tokens = ?, total_tokens = ?,
			process_id = ?, last_output_at = ?, stall_timeout = ?, started_at = ?, finished_at = ?
		WHERE id = ?
	`
	res, err := r.db.ExecContext(ctx, query,
		run.TaskID, run.AgentID, string(run.DriverType), string(run.Status), run.Attempt,
		run.Input, run.Output, run.Error, run.InputTokens, run.OutputTokens, run.TotalTokens,
		run.ProcessID, run.LastOutputAt, run.StallTimeout, run.StartedAt, run.FinishedAt,
		run.ID,
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

func scanRun(row scanner) (domain.Run, error) {
	var r domain.Run
	var driverStr, statusStr string

	err := row.Scan(
		&r.ID, &r.TaskID, &r.AgentID, &driverStr, &statusStr, &r.Attempt,
		&r.Input, &r.Output, &r.Error, &r.InputTokens, &r.OutputTokens, &r.TotalTokens,
		&r.ProcessID, &r.LastOutputAt, &r.StallTimeout, &r.StartedAt, &r.FinishedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Run{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Run{}, err
	}
	r.DriverType = driverStr
	r.Status = domain.RunStatus(statusStr)
	return r, nil
}

func (r *RunRepository) DeleteByTask(ctx context.Context, taskID string) error {
	q := getQueryer(ctx, r.db)
	_, err := q.ExecContext(ctx, `DELETE FROM runs WHERE task_id = ?`, taskID)
	return err
}

func (r *RunRepository) DeleteHistory(ctx context.Context) (int, error) {
	q := getQueryer(ctx, r.db)
	res, err := q.ExecContext(ctx, `DELETE FROM runs WHERE status <> 'running'`)
	if err != nil {
		return 0, err
	}
	deleted, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(deleted), nil
}
