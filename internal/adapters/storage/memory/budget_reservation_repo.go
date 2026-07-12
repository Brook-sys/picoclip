package memory

import (
	"context"
	"sync"
	"time"

	"picoclip/internal/core/domain"
)

type budgetReservationRepository struct {
	mu                      sync.RWMutex
	policies                map[string]domain.BudgetPolicy
	accounts                map[string]domain.BudgetAccount
	reservations            map[string]domain.BudgetReservation
	reservationIDsByRequest map[string]string
}

func NewBudgetReservationRepository() *budgetReservationRepository {
	return &budgetReservationRepository{
		policies:                make(map[string]domain.BudgetPolicy),
		accounts:                make(map[string]domain.BudgetAccount),
		reservations:            make(map[string]domain.BudgetReservation),
		reservationIDsByRequest: make(map[string]string),
	}
}

func (r *budgetReservationRepository) UpsertPolicy(ctx context.Context, policy domain.BudgetPolicy) (domain.BudgetPolicy, error) {
	if err := ctx.Err(); err != nil {
		return domain.BudgetPolicy{}, err
	}
	if err := validateBudgetPolicy(policy); err != nil {
		return domain.BudgetPolicy{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.policies[policy.ID]; ok {
		policy.CreatedAt = existing.CreatedAt
		if policy.UpdatedAt.IsZero() {
			policy.UpdatedAt = time.Now().UTC()
		}
	} else {
		if policy.CreatedAt.IsZero() {
			policy.CreatedAt = time.Now().UTC()
		}
		if policy.UpdatedAt.IsZero() {
			policy.UpdatedAt = policy.CreatedAt
		}
	}
	r.policies[policy.ID] = policy
	account, ok := r.accounts[policy.ID]
	if !ok {
		r.accounts[policy.ID] = domain.BudgetAccount{PolicyID: policy.ID, CreatedAt: policy.CreatedAt, UpdatedAt: policy.UpdatedAt}
	} else {
		account.UpdatedAt = policy.UpdatedAt
		r.accounts[policy.ID] = account
	}
	return policy, nil
}

func (r *budgetReservationRepository) GetAccount(ctx context.Context, policyID string) (domain.BudgetAccount, error) {
	if err := ctx.Err(); err != nil {
		return domain.BudgetAccount{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	account, ok := r.accounts[policyID]
	if !ok {
		return domain.BudgetAccount{}, domain.ErrNotFound
	}
	return account, nil
}

func (r *budgetReservationRepository) GetReservation(ctx context.Context, id string) (domain.BudgetReservation, error) {
	if err := ctx.Err(); err != nil {
		return domain.BudgetReservation{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	reservation, ok := r.reservations[id]
	if !ok {
		return domain.BudgetReservation{}, domain.ErrNotFound
	}
	return reservation, nil
}

func (r *budgetReservationRepository) TryReserve(ctx context.Context, request domain.BudgetReservationRequest) (domain.BudgetReservation, error) {
	if err := ctx.Err(); err != nil {
		return domain.BudgetReservation{}, err
	}
	if request.ReservationID == "" || request.RequestID == "" || request.Tokens < 0 || request.CostMicros < 0 || request.Tokens == 0 && request.CostMicros == 0 {
		return domain.BudgetReservation{}, domain.ErrInvalidInput
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if id, ok := r.reservationIDsByRequest[request.RequestID]; ok {
		reservation := r.reservations[id]
		if reservation.ID != request.ReservationID || reservation.TokensReserved != request.Tokens || reservation.CostMicrosReserved != request.CostMicros || reservation.TaskID != request.TaskID || reservation.RunID != request.RunID || reservation.AgentID != request.AgentID || reservation.WorkspaceID != request.WorkspaceID {
			return domain.BudgetReservation{}, domain.ErrConflict
		}
		return reservation, nil
	}
	if _, ok := r.reservations[request.ReservationID]; ok {
		return domain.BudgetReservation{}, domain.ErrConflict
	}

	applicablePolicies := r.applicablePoliciesLocked(request)
	for _, policy := range applicablePolicies {
		account := r.accounts[policy.ID]
		if policy.Enforcement == domain.BudgetPolicyEnforcementHard && exceedsPolicy(policy, account, request.Tokens, request.CostMicros) {
			return domain.BudgetReservation{}, domain.ErrBudgetExceeded
		}
	}

	now := request.CreatedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	reservation := domain.BudgetReservation{ID: request.ReservationID, RequestID: request.RequestID, TaskID: request.TaskID, RunID: request.RunID, AgentID: request.AgentID, WorkspaceID: request.WorkspaceID, TokensReserved: request.Tokens, CostMicrosReserved: request.CostMicros, Status: domain.BudgetReservationStatusReserved, CreatedAt: now, UpdatedAt: now}
	r.reservations[reservation.ID] = reservation
	r.reservationIDsByRequest[reservation.RequestID] = reservation.ID
	for _, policy := range applicablePolicies {
		account := r.accounts[policy.ID]
		account.ReservedTokens += request.Tokens
		account.ReservedCostMicros += request.CostMicros
		account.UpdatedAt = now
		r.accounts[policy.ID] = account
	}
	return reservation, nil
}

func (r *budgetReservationRepository) Settle(ctx context.Context, settlement domain.BudgetReservationSettlement) (domain.BudgetReservation, error) {
	if err := ctx.Err(); err != nil {
		return domain.BudgetReservation{}, err
	}
	if settlement.ReservationID == "" || settlement.Tokens < 0 || settlement.CostMicros < 0 {
		return domain.BudgetReservation{}, domain.ErrInvalidInput
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	reservation, ok := r.reservations[settlement.ReservationID]
	if !ok {
		return domain.BudgetReservation{}, domain.ErrNotFound
	}
	if reservation.Status == domain.BudgetReservationStatusSettled {
		if reservation.TokensSettled != settlement.Tokens || reservation.CostMicrosSettled != settlement.CostMicros {
			return domain.BudgetReservation{}, domain.ErrConflict
		}
		return reservation, nil
	}
	now := settlement.SettledAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	reservation.TokensSettled = settlement.Tokens
	reservation.CostMicrosSettled = settlement.CostMicros
	reservation.Status = domain.BudgetReservationStatusSettled
	reservation.SettledAt = now
	reservation.UpdatedAt = now
	r.reservations[reservation.ID] = reservation
	for _, policy := range r.applicablePoliciesLocked(domain.BudgetReservationRequest{AgentID: reservation.AgentID, WorkspaceID: reservation.WorkspaceID}) {
		account := r.accounts[policy.ID]
		account.ReservedTokens -= reservation.TokensReserved
		account.ReservedCostMicros -= reservation.CostMicrosReserved
		account.SettledTokens += settlement.Tokens
		account.SettledCostMicros += settlement.CostMicros
		account.UpdatedAt = now
		r.accounts[policy.ID] = account
	}
	return reservation, nil
}

func validateBudgetPolicy(policy domain.BudgetPolicy) error {
	if policy.ID == "" || policy.TokenLimit < 0 || policy.CostLimitMicros < 0 || policy.TokenLimit == 0 && policy.CostLimitMicros == 0 || policy.Enforcement != domain.BudgetPolicyEnforcementHard && policy.Enforcement != domain.BudgetPolicyEnforcementWarn {
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

func (r *budgetReservationRepository) applicablePoliciesLocked(request domain.BudgetReservationRequest) []domain.BudgetPolicy {
	policies := make([]domain.BudgetPolicy, 0, len(r.policies))
	for _, policy := range r.policies {
		if !policy.Enabled {
			continue
		}
		switch policy.Scope {
		case domain.BudgetPolicyScopeGlobal:
			policies = append(policies, policy)
		case domain.BudgetPolicyScopeWorkspace:
			if policy.ScopeID == request.WorkspaceID {
				policies = append(policies, policy)
			}
		case domain.BudgetPolicyScopeAgent:
			if policy.ScopeID == request.AgentID {
				policies = append(policies, policy)
			}
		}
	}
	return policies
}

func exceedsPolicy(policy domain.BudgetPolicy, account domain.BudgetAccount, tokens int, costMicros int64) bool {
	return policy.TokenLimit > 0 && account.SettledTokens+account.ReservedTokens+tokens > policy.TokenLimit || policy.CostLimitMicros > 0 && account.SettledCostMicros+account.ReservedCostMicros+costMicros > policy.CostLimitMicros
}
