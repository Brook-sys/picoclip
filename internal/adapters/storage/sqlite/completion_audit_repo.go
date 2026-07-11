package sqlite

import (
	"context"
	"database/sql"
	"time"

	"picoclip/internal/core/domain"
)

type CompletionAuditRepository struct{ db *sql.DB }

func (r *CompletionAuditRepository) Create(ctx context.Context, audit domain.CompletionAudit) error {
	_, err := getQueryer(ctx, r.db).ExecContext(ctx, `INSERT INTO completion_audits (id, task_id, requested_by_agent_id, outcome, summary, findings_json, requested_at, decided_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, audit.ID, audit.TaskID, audit.RequestedByAgentID, string(audit.Outcome), audit.Summary, audit.FindingsJSON, audit.RequestedAt, audit.DecidedAt)
	return err
}
func (r *CompletionAuditRepository) Update(ctx context.Context, audit domain.CompletionAudit) error {
	res, err := getQueryer(ctx, r.db).ExecContext(ctx, `UPDATE completion_audits SET outcome=?, summary=?, findings_json=?, decided_at=? WHERE id=?`, string(audit.Outcome), audit.Summary, audit.FindingsJSON, audit.DecidedAt, audit.ID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}
func (r *CompletionAuditRepository) ListByTask(ctx context.Context, taskID string) ([]domain.CompletionAudit, error) {
	rows, err := getQueryer(ctx, r.db).QueryContext(ctx, `SELECT id, task_id, requested_by_agent_id, outcome, summary, findings_json, requested_at, decided_at FROM completion_audits WHERE task_id=? ORDER BY requested_at ASC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.CompletionAudit{}
	for rows.Next() {
		var a domain.CompletionAudit
		var decided sql.NullTime
		if err := rows.Scan(&a.ID, &a.TaskID, &a.RequestedByAgentID, &a.Outcome, &a.Summary, &a.FindingsJSON, &a.RequestedAt, &decided); err != nil {
			return nil, err
		}
		if decided.Valid {
			t := decided.Time
			a.DecidedAt = &t
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

var _ = time.Time{}
