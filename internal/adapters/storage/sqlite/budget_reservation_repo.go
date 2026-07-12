package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"picoclip/internal/core/domain"
)

type BudgetReservationRepository struct{ db *sql.DB }

func (r *BudgetReservationRepository) UpsertPolicy(ctx context.Context, policy domain.BudgetPolicy) (domain.BudgetPolicy, error) {
	if err := validateSQLiteBudgetPolicy(policy); err != nil {
		return domain.BudgetPolicy{}, err
	}
	if policy.CreatedAt.IsZero() {
		policy.CreatedAt = time.Now().UTC()
	}
	if policy.UpdatedAt.IsZero() {
		policy.UpdatedAt = policy.CreatedAt
	}

	err := r.runInTx(ctx, func(tx *sql.Tx) error {
		var existingCreatedAt time.Time
		err := tx.QueryRowContext(ctx, `SELECT created_at FROM budget_policies WHERE id = ?`, policy.ID).Scan(&existingCreatedAt)
		if err == nil {
			policy.CreatedAt = existingCreatedAt
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO budget_policies (id, scope_type, scope_id, period_kind, period_start, period_end, token_limit, cost_limit_micros, enforcement, enabled, created_at, updated_at)
			VALUES (?, ?, ?, 'lifetime', ?, NULLIF(?, ?), ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				scope_type = excluded.scope_type, scope_id = excluded.scope_id, period_start = excluded.period_start,
				period_end = excluded.period_end, token_limit = excluded.token_limit, cost_limit_micros = excluded.cost_limit_micros,
				enforcement = excluded.enforcement, enabled = excluded.enabled, updated_at = excluded.updated_at
		`, policy.ID, string(policy.Scope), policy.ScopeID, policy.PeriodStart, policy.PeriodEnd, time.Time{}, policy.TokenLimit, policy.CostLimitMicros, string(policy.Enforcement), policy.Enabled, policy.CreatedAt, policy.UpdatedAt); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO budget_accounts (policy_id, status, created_at, updated_at)
			VALUES (?, 'active', ?, ?)
			ON CONFLICT(policy_id) DO NOTHING
		`, policy.ID, policy.CreatedAt, policy.UpdatedAt)
		return err
	})
	if err != nil {
		return domain.BudgetPolicy{}, err
	}
	return policy, nil
}

func (r *BudgetReservationRepository) GetAccount(ctx context.Context, policyID string) (domain.BudgetAccount, error) {
	var account domain.BudgetAccount
	err := r.db.QueryRowContext(ctx, `
		SELECT policy_id, settled_tokens, reserved_tokens, settled_cost_micros, reserved_cost_micros, created_at, updated_at
		FROM budget_accounts WHERE policy_id = ?
	`, policyID).Scan(&account.PolicyID, &account.SettledTokens, &account.ReservedTokens, &account.SettledCostMicros, &account.ReservedCostMicros, &account.CreatedAt, &account.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.BudgetAccount{}, domain.ErrNotFound
	}
	return account, err
}

func (r *BudgetReservationRepository) GetReservation(ctx context.Context, id string) (domain.BudgetReservation, error) {
	return scanSQLiteBudgetReservation(r.db.QueryRowContext(ctx, budgetReservationSelect+` WHERE id = ?`, id))
}

func (r *BudgetReservationRepository) TryReserve(ctx context.Context, request domain.BudgetReservationRequest) (domain.BudgetReservation, error) {
	if err := validateSQLiteReservationRequest(request); err != nil {
		return domain.BudgetReservation{}, err
	}
	var reservation domain.BudgetReservation
	err := r.runInTx(ctx, func(tx *sql.Tx) error {
		existing, err := scanSQLiteBudgetReservation(tx.QueryRowContext(ctx, budgetReservationSelect+` WHERE request_id = ?`, request.RequestID))
		if err == nil {
			if !sameSQLiteReservationRequest(existing, request) {
				return domain.ErrConflict
			}
			reservation = existing
			return nil
		}
		if !errors.Is(err, domain.ErrNotFound) {
			return err
		}

		policies, err := sqliteApplicablePolicies(ctx, tx, request)
		if err != nil {
			return err
		}
		for _, policy := range policies {
			if policy.Enforcement != domain.BudgetPolicyEnforcementHard {
				continue
			}
			result, err := tx.ExecContext(ctx, `
				UPDATE budget_accounts
				SET reserved_tokens = reserved_tokens + ?, reserved_cost_micros = reserved_cost_micros + ?, updated_at = ?
				WHERE policy_id = ?
				AND (? = 0 OR settled_tokens + reserved_tokens + ? <= ?)
				AND (? = 0 OR settled_cost_micros + reserved_cost_micros + ? <= ?)
			`, request.Tokens, request.CostMicros, request.CreatedAt, policy.ID,
				policy.TokenLimit, request.Tokens, policy.TokenLimit,
				policy.CostLimitMicros, request.CostMicros, policy.CostLimitMicros)
			if err != nil {
				return err
			}
			n, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if n != 1 {
				return domain.ErrBudgetExceeded
			}
		}
		for _, policy := range policies {
			if policy.Enforcement == domain.BudgetPolicyEnforcementHard {
				continue
			}
			if _, err := tx.ExecContext(ctx, `UPDATE budget_accounts SET reserved_tokens = reserved_tokens + ?, reserved_cost_micros = reserved_cost_micros + ?, updated_at = ? WHERE policy_id = ?`, request.Tokens, request.CostMicros, request.CreatedAt, policy.ID); err != nil {
				return err
			}
		}

		reservation = domain.BudgetReservation{ID: request.ReservationID, RequestID: request.RequestID, TaskID: request.TaskID, RunID: request.RunID, AgentID: request.AgentID, WorkspaceID: request.WorkspaceID, TokensReserved: request.Tokens, CostMicrosReserved: request.CostMicros, Status: domain.BudgetReservationStatusReserved, CreatedAt: request.CreatedAt, UpdatedAt: request.CreatedAt}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO budget_reservations (id, request_id, task_id, run_id, agent_id, workspace_id, reserved_tokens, reserved_cost_micros, settled_tokens, settled_cost_micros, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, 0, 'reserved', ?, ?)
		`, reservation.ID, reservation.RequestID, reservation.TaskID, reservation.RunID, reservation.AgentID, reservation.WorkspaceID, reservation.TokensReserved, reservation.CostMicrosReserved, reservation.CreatedAt, reservation.UpdatedAt)
		return err
	})
	if err != nil {
		return domain.BudgetReservation{}, err
	}
	return reservation, nil
}

func (r *BudgetReservationRepository) Settle(ctx context.Context, settlement domain.BudgetReservationSettlement) (domain.BudgetReservation, error) {
	if settlement.ReservationID == "" || settlement.Tokens < 0 || settlement.CostMicros < 0 {
		return domain.BudgetReservation{}, domain.ErrInvalidInput
	}
	if settlement.SettledAt.IsZero() {
		settlement.SettledAt = time.Now().UTC()
	}
	var reservation domain.BudgetReservation
	err := r.runInTx(ctx, func(tx *sql.Tx) error {
		stored, err := scanSQLiteBudgetReservation(tx.QueryRowContext(ctx, budgetReservationSelect+` WHERE id = ?`, settlement.ReservationID))
		if err != nil {
			return err
		}
		if stored.Status == domain.BudgetReservationStatusSettled {
			if stored.TokensSettled != settlement.Tokens || stored.CostMicrosSettled != settlement.CostMicros {
				return domain.ErrConflict
			}
			reservation = stored
			return nil
		}
		policies, err := sqliteApplicablePolicies(ctx, tx, domain.BudgetReservationRequest{AgentID: stored.AgentID, WorkspaceID: stored.WorkspaceID})
		if err != nil {
			return err
		}
		for _, policy := range policies {
			result, err := tx.ExecContext(ctx, `
				UPDATE budget_accounts
				SET reserved_tokens = reserved_tokens - ?, reserved_cost_micros = reserved_cost_micros - ?, settled_tokens = settled_tokens + ?, settled_cost_micros = settled_cost_micros + ?, updated_at = ?
				WHERE policy_id = ? AND reserved_tokens >= ? AND reserved_cost_micros >= ?
			`, stored.TokensReserved, stored.CostMicrosReserved, settlement.Tokens, settlement.CostMicros, settlement.SettledAt, policy.ID, stored.TokensReserved, stored.CostMicrosReserved)
			if err != nil {
				return err
			}
			n, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if n != 1 {
				return fmt.Errorf("budget account missing or inconsistent: %w", domain.ErrConflict)
			}
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE budget_reservations SET settled_tokens = ?, settled_cost_micros = ?, status = 'settled', settled_at = ?, updated_at = ?
			WHERE id = ? AND status = 'reserved'
		`, settlement.Tokens, settlement.CostMicros, settlement.SettledAt, settlement.SettledAt, stored.ID)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return domain.ErrConflict
		}
		stored.TokensSettled, stored.CostMicrosSettled = settlement.Tokens, settlement.CostMicros
		stored.Status, stored.SettledAt, stored.UpdatedAt = domain.BudgetReservationStatusSettled, settlement.SettledAt, settlement.SettledAt
		reservation = stored
		return nil
	})
	if err != nil {
		return domain.BudgetReservation{}, err
	}
	return reservation, nil
}

const budgetReservationSelect = `SELECT id, request_id, task_id, run_id, agent_id, workspace_id, reserved_tokens, reserved_cost_micros, settled_tokens, settled_cost_micros, status, created_at, updated_at, settled_at FROM budget_reservations`

type sqliteBudgetPolicy struct {
	ID              string
	Enforcement     domain.BudgetPolicyEnforcement
	TokenLimit      int
	CostLimitMicros int64
}

func sqliteApplicablePolicies(ctx context.Context, tx *sql.Tx, request domain.BudgetReservationRequest) ([]sqliteBudgetPolicy, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, enforcement, token_limit, cost_limit_micros FROM budget_policies
		WHERE enabled = 1 AND ((scope_type = 'global' AND scope_id = '') OR (scope_type = 'workspace' AND scope_id = ?) OR (scope_type = 'agent' AND scope_id = ?))
		ORDER BY id ASC
	`, request.WorkspaceID, request.AgentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var policies []sqliteBudgetPolicy
	for rows.Next() {
		var policy sqliteBudgetPolicy
		if err := rows.Scan(&policy.ID, &policy.Enforcement, &policy.TokenLimit, &policy.CostLimitMicros); err != nil {
			return nil, err
		}
		policies = append(policies, policy)
	}
	return policies, rows.Err()
}

func scanSQLiteBudgetReservation(row interface{ Scan(...any) error }) (domain.BudgetReservation, error) {
	var reservation domain.BudgetReservation
	var status string
	var settledAt sql.NullTime
	err := row.Scan(&reservation.ID, &reservation.RequestID, &reservation.TaskID, &reservation.RunID, &reservation.AgentID, &reservation.WorkspaceID, &reservation.TokensReserved, &reservation.CostMicrosReserved, &reservation.TokensSettled, &reservation.CostMicrosSettled, &status, &reservation.CreatedAt, &reservation.UpdatedAt, &settledAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.BudgetReservation{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.BudgetReservation{}, err
	}
	reservation.Status = domain.BudgetReservationStatus(status)
	if settledAt.Valid {
		reservation.SettledAt = settledAt.Time
	}
	return reservation, nil
}

func (r *BudgetReservationRepository) runInTx(ctx context.Context, fn func(*sql.Tx) error) error {
	for attempt := 0; attempt < 8; attempt++ {
		tx, err := r.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		err = fn(tx)
		if err == nil {
			err = tx.Commit()
		} else {
			_ = tx.Rollback()
		}
		if err == nil || ctx.Err() != nil || !isSQLiteBusy(err) {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * time.Millisecond)
	}
	return fmt.Errorf("sqlite remained busy: %w", context.DeadlineExceeded)
}

func isSQLiteBusy(err error) bool {
	return err != nil && (contains(err.Error(), "database is locked") || contains(err.Error(), "database is busy"))
}

func contains(s, part string) bool {
	for i := 0; i+len(part) <= len(s); i++ {
		if s[i:i+len(part)] == part {
			return true
		}
	}
	return false
}

func validateSQLiteBudgetPolicy(policy domain.BudgetPolicy) error {
	if policy.ID == "" || policy.TokenLimit < 0 || policy.CostLimitMicros < 0 || (policy.TokenLimit == 0 && policy.CostLimitMicros == 0) || (policy.Enforcement != domain.BudgetPolicyEnforcementHard && policy.Enforcement != domain.BudgetPolicyEnforcementWarn) {
		return domain.ErrInvalidInput
	}
	switch policy.Scope {
	case domain.BudgetPolicyScopeGlobal:
		if policy.ScopeID != "" {
			return domain.ErrInvalidInput
		}
	case domain.BudgetPolicyScopeWorkspace, domain.BudgetPolicyScopeAgent:
		if policy.ScopeID == "" {
			return domain.ErrInvalidInput
		}
	default:
		return domain.ErrInvalidInput
	}
	return nil
}

func validateSQLiteReservationRequest(request domain.BudgetReservationRequest) error {
	if request.ReservationID == "" || request.RequestID == "" || request.Tokens < 0 || request.CostMicros < 0 || (request.Tokens == 0 && request.CostMicros == 0) {
		return domain.ErrInvalidInput
	}
	return nil
}

func sameSQLiteReservationRequest(reservation domain.BudgetReservation, request domain.BudgetReservationRequest) bool {
	return reservation.ID == request.ReservationID && reservation.TokensReserved == request.Tokens && reservation.CostMicrosReserved == request.CostMicros && reservation.TaskID == request.TaskID && reservation.RunID == request.RunID && reservation.AgentID == request.AgentID && reservation.WorkspaceID == request.WorkspaceID
}
