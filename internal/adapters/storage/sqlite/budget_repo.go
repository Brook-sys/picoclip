package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"picoclip/internal/core/domain"
)

type BudgetRepository struct{ db *sql.DB }

func (r *BudgetRepository) Create(ctx context.Context, budget domain.Budget) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO budgets (id, scope, workspace_id, agent_id, limit_tokens, limit_runs, limit_cost_micros, hard_stop, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, budget.ID, string(budget.Scope), budget.WorkspaceID, budget.AgentID, budget.LimitTokens, budget.LimitRuns, budget.LimitCostMicros, budget.HardStop, budget.Enabled, budget.CreatedAt, budget.UpdatedAt)
	return err
}

func (r *BudgetRepository) Get(ctx context.Context, id string) (domain.Budget, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, scope, workspace_id, agent_id, limit_tokens, limit_runs, limit_cost_micros, hard_stop, enabled, created_at, updated_at
		FROM budgets WHERE id = ?
	`, id)
	budget, err := scanBudget(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Budget{}, domain.ErrNotFound
	}
	return budget, err
}

func (r *BudgetRepository) List(ctx context.Context) ([]domain.Budget, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, scope, workspace_id, agent_id, limit_tokens, limit_runs, limit_cost_micros, hard_stop, enabled, created_at, updated_at
		FROM budgets ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	budgets := make([]domain.Budget, 0)
	for rows.Next() {
		budget, err := scanBudget(rows)
		if err != nil {
			return nil, err
		}
		budgets = append(budgets, budget)
	}
	return budgets, rows.Err()
}

func (r *BudgetRepository) Update(ctx context.Context, budget domain.Budget) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE budgets SET scope = ?, workspace_id = ?, agent_id = ?, limit_tokens = ?, limit_runs = ?, limit_cost_micros = ?, hard_stop = ?, enabled = ?, created_at = ?, updated_at = ?
		WHERE id = ?
	`, string(budget.Scope), budget.WorkspaceID, budget.AgentID, budget.LimitTokens, budget.LimitRuns, budget.LimitCostMicros, budget.HardStop, budget.Enabled, budget.CreatedAt, budget.UpdatedAt, budget.ID)
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

func (r *BudgetRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM budgets WHERE id = ?`, id)
	return err
}

type budgetScanner interface {
	Scan(dest ...any) error
}

func scanBudget(row budgetScanner) (domain.Budget, error) {
	var budget domain.Budget
	var scope string
	if err := row.Scan(&budget.ID, &scope, &budget.WorkspaceID, &budget.AgentID, &budget.LimitTokens, &budget.LimitRuns, &budget.LimitCostMicros, &budget.HardStop, &budget.Enabled, &budget.CreatedAt, &budget.UpdatedAt); err != nil {
		return domain.Budget{}, err
	}
	budget.Scope = domain.BudgetScope(scope)
	return budget, nil
}
