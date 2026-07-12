package storagetest

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

type BudgetReservationRepositoryFactory func(t *testing.T) ports.BudgetReservationRepository

func RunBudgetReservationRepositoryContract(t *testing.T, factory BudgetReservationRepositoryFactory) {
	t.Helper()
	t.Run("reserve_and_settle_are_idempotent", func(t *testing.T) {
		ctx := context.Background()
		repository := factory(t)
		now := time.Date(2026, 7, 12, 20, 0, 0, 0, time.UTC)
		policy := domain.BudgetPolicy{
			ID:          "policy_workspace",
			Scope:       domain.BudgetPolicyScopeWorkspace,
			ScopeID:     "workspace_1",
			TokenLimit:  100,
			Enforcement: domain.BudgetPolicyEnforcementHard,
			Enabled:     true,
			PeriodStart: now,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if _, err := repository.UpsertPolicy(ctx, policy); err != nil {
			t.Fatal(err)
		}

		request := domain.BudgetReservationRequest{
			ReservationID: "reservation_1",
			RequestID:     "request_1",
			WorkspaceID:   "workspace_1",
			Tokens:        60,
			CostMicros:    600,
			CreatedAt:     now,
		}
		reservation, err := repository.TryReserve(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		if reservation.Status != domain.BudgetReservationStatusReserved || reservation.TokensReserved != request.Tokens || reservation.CostMicrosReserved != request.CostMicros {
			t.Fatalf("unexpected reservation: %#v", reservation)
		}
		duplicate, err := repository.TryReserve(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		if duplicate != reservation {
			t.Fatalf("duplicate reserve did not return original reservation: got %#v want %#v", duplicate, reservation)
		}

		account, err := repository.GetAccount(ctx, policy.ID)
		if err != nil {
			t.Fatal(err)
		}
		if account.ReservedTokens != 60 || account.ReservedCostMicros != 600 || account.SettledTokens != 0 || account.SettledCostMicros != 0 {
			t.Fatalf("unexpected account after reserve: %#v", account)
		}

		settlement := domain.BudgetReservationSettlement{ReservationID: reservation.ID, Tokens: 40, CostMicros: 400, SettledAt: now.Add(time.Minute)}
		settled, err := repository.Settle(ctx, settlement)
		if err != nil {
			t.Fatal(err)
		}
		if settled.Status != domain.BudgetReservationStatusSettled || settled.TokensSettled != 40 || settled.CostMicrosSettled != 400 {
			t.Fatalf("unexpected settled reservation: %#v", settled)
		}
		duplicateSettlement, err := repository.Settle(ctx, settlement)
		if err != nil {
			t.Fatal(err)
		}
		if duplicateSettlement != settled {
			t.Fatalf("duplicate settlement did not return original reservation: got %#v want %#v", duplicateSettlement, settled)
		}
		if _, err := repository.Settle(ctx, domain.BudgetReservationSettlement{ReservationID: reservation.ID, Tokens: 41, CostMicros: 400, SettledAt: settlement.SettledAt}); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("settlement with different usage error = %v, want ErrConflict", err)
		}

		account, err = repository.GetAccount(ctx, policy.ID)
		if err != nil {
			t.Fatal(err)
		}
		if account.ReservedTokens != 0 || account.ReservedCostMicros != 0 || account.SettledTokens != 40 || account.SettledCostMicros != 400 {
			t.Fatalf("unexpected account after settlement: %#v", account)
		}
	})

	t.Run("hard_limit_rejects_without_partial_reservation", func(t *testing.T) {
		ctx := context.Background()
		repository := factory(t)
		now := time.Date(2026, 7, 12, 20, 0, 0, 0, time.UTC)
		for _, policy := range []domain.BudgetPolicy{
			{ID: "policy_global", Scope: domain.BudgetPolicyScopeGlobal, TokenLimit: 100, Enforcement: domain.BudgetPolicyEnforcementHard, Enabled: true, PeriodStart: now, CreatedAt: now, UpdatedAt: now},
			{ID: "policy_agent", Scope: domain.BudgetPolicyScopeAgent, ScopeID: "agent_1", TokenLimit: 50, Enforcement: domain.BudgetPolicyEnforcementHard, Enabled: true, PeriodStart: now, CreatedAt: now, UpdatedAt: now},
		} {
			if _, err := repository.UpsertPolicy(ctx, policy); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := repository.TryReserve(ctx, domain.BudgetReservationRequest{ReservationID: "reservation_ok", RequestID: "request_ok", AgentID: "agent_1", Tokens: 40, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
		if _, err := repository.TryReserve(ctx, domain.BudgetReservationRequest{ReservationID: "reservation_rejected", RequestID: "request_rejected", AgentID: "agent_1", Tokens: 20, CreatedAt: now}); !errors.Is(err, domain.ErrBudgetExceeded) {
			t.Fatalf("reserve over hard limit error = %v, want ErrBudgetExceeded", err)
		}
		for _, policyID := range []string{"policy_global", "policy_agent"} {
			account, err := repository.GetAccount(ctx, policyID)
			if err != nil {
				t.Fatal(err)
			}
			if account.ReservedTokens != 40 {
				t.Fatalf("account %s reserved tokens = %d, want 40", policyID, account.ReservedTokens)
			}
		}
		if _, err := repository.GetReservation(ctx, "reservation_rejected"); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("rejected reservation lookup error = %v, want ErrNotFound", err)
		}
	})

	t.Run("concurrent_reservations_cannot_oversubscribe", func(t *testing.T) {
		ctx := context.Background()
		repository := factory(t)
		now := time.Date(2026, 7, 12, 20, 0, 0, 0, time.UTC)
		policy := domain.BudgetPolicy{ID: "policy_concurrent", Scope: domain.BudgetPolicyScopeWorkspace, ScopeID: "workspace_1", TokenLimit: 100, Enforcement: domain.BudgetPolicyEnforcementHard, Enabled: true, PeriodStart: now, CreatedAt: now, UpdatedAt: now}
		if _, err := repository.UpsertPolicy(ctx, policy); err != nil {
			t.Fatal(err)
		}

		const attempts = 20
		var accepted int
		var wg sync.WaitGroup
		var mu sync.Mutex
		errs := make(chan error, attempts)
		for i := range attempts {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := repository.TryReserve(ctx, domain.BudgetReservationRequest{ReservationID: fmt.Sprintf("reservation_%d", i), RequestID: fmt.Sprintf("request_%d", i), WorkspaceID: "workspace_1", Tokens: 10, CreatedAt: now})
				if err == nil {
					mu.Lock()
					accepted++
					mu.Unlock()
					return
				}
				if !errors.Is(err, domain.ErrBudgetExceeded) {
					errs <- err
				}
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Error(err)
		}
		if accepted != 10 {
			t.Fatalf("accepted reservations = %d, want 10", accepted)
		}
		account, err := repository.GetAccount(ctx, policy.ID)
		if err != nil {
			t.Fatal(err)
		}
		if account.ReservedTokens != 100 || account.SettledTokens != 0 {
			t.Fatalf("unexpected concurrent account balance: %#v", account)
		}
	})
}
