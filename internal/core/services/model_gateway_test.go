package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"picoclip/internal/adapters/storage/memory"
	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

type recordingModelExecutor struct {
	calls  int
	result ports.RuntimeExecutionResult
	err    error
}

func (e *recordingModelExecutor) Execute(context.Context, domain.RuntimeID, ports.RuntimeExecutionInput) (ports.RuntimeExecutionResult, error) {
	e.calls++
	return e.result, e.err
}

type failingSettlementRepository struct {
	ports.BudgetReservationRepository
	err error
}

func (r failingSettlementRepository) Settle(context.Context, domain.BudgetReservationSettlement) (domain.BudgetReservation, error) {
	return domain.BudgetReservation{}, r.err
}

func TestBudgetModelGatewayReservesBeforeExecutionAndSettlesActualUsage(t *testing.T) {
	ctx := context.Background()
	storage := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 12, 20, 0, 0, 0, time.UTC)}
	if _, err := storage.BudgetReservations().UpsertPolicy(ctx, domain.BudgetPolicy{
		ID:          "policy_global",
		Scope:       domain.BudgetPolicyScopeGlobal,
		TokenLimit:  100,
		Enforcement: domain.BudgetPolicyEnforcementHard,
		Enabled:     true,
		CreatedAt:   clock.t,
		UpdatedAt:   clock.t,
	}); err != nil {
		t.Fatal(err)
	}
	executor := &recordingModelExecutor{result: ports.RuntimeExecutionResult{Output: "one two three"}}
	gateway := NewBudgetModelGateway(storage.BudgetReservations(), executor, clock)

	result, err := gateway.Execute(ctx, modelRequest("reservation_success", "request_success", 8))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Output != "one two three" || executor.calls != 1 {
		t.Fatalf("unexpected execution result=%#v calls=%d", result, executor.calls)
	}
	reservation, err := storage.BudgetReservations().GetReservation(ctx, "reservation_success")
	if err != nil {
		t.Fatal(err)
	}
	if reservation.Status != domain.BudgetReservationStatusSettled || reservation.TokensReserved != 8 || reservation.TokensSettled != 12 {
		t.Fatalf("unexpected reservation: %#v", reservation)
	}
	account, err := storage.BudgetReservations().GetAccount(ctx, "policy_global")
	if err != nil {
		t.Fatal(err)
	}
	if account.ReservedTokens != 0 || account.SettledTokens != 12 {
		t.Fatalf("unexpected account: %#v", account)
	}
}

func TestBudgetModelGatewayRejectsBeforeRuntimeExecution(t *testing.T) {
	ctx := context.Background()
	storage := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 12, 20, 0, 0, 0, time.UTC)}
	if _, err := storage.BudgetReservations().UpsertPolicy(ctx, domain.BudgetPolicy{
		ID:          "policy_global",
		Scope:       domain.BudgetPolicyScopeGlobal,
		TokenLimit:  7,
		Enforcement: domain.BudgetPolicyEnforcementHard,
		Enabled:     true,
		CreatedAt:   clock.t,
		UpdatedAt:   clock.t,
	}); err != nil {
		t.Fatal(err)
	}
	executor := &recordingModelExecutor{}
	gateway := NewBudgetModelGateway(storage.BudgetReservations(), executor, clock)

	_, err := gateway.Execute(ctx, modelRequest("reservation_rejected", "request_rejected", 8))
	if !errors.Is(err, domain.ErrBudgetExceeded) {
		t.Fatalf("expected budget exceeded, got %v", err)
	}
	if executor.calls != 0 {
		t.Fatalf("runtime called %d times after rejected reservation", executor.calls)
	}
}

func TestBudgetModelGatewaySettlesWhenRuntimeFailsAndPreservesRuntimeError(t *testing.T) {
	ctx := context.Background()
	storage := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 12, 20, 0, 0, 0, time.UTC)}
	if _, err := storage.BudgetReservations().UpsertPolicy(ctx, domain.BudgetPolicy{
		ID:          "policy_global",
		Scope:       domain.BudgetPolicyScopeGlobal,
		TokenLimit:  100,
		Enforcement: domain.BudgetPolicyEnforcementHard,
		Enabled:     true,
		CreatedAt:   clock.t,
		UpdatedAt:   clock.t,
	}); err != nil {
		t.Fatal(err)
	}
	runtimeErr := errors.New("runtime failed")
	executor := &recordingModelExecutor{err: runtimeErr}
	gateway := NewBudgetModelGateway(storage.BudgetReservations(), executor, clock)

	_, err := gateway.Execute(ctx, modelRequest("reservation_failed", "request_failed", 8))
	if !errors.Is(err, runtimeErr) {
		t.Fatalf("expected runtime error, got %v", err)
	}
	reservation, err := storage.BudgetReservations().GetReservation(ctx, "reservation_failed")
	if err != nil {
		t.Fatal(err)
	}
	if reservation.Status != domain.BudgetReservationStatusSettled || reservation.TokensSettled != 10 {
		t.Fatalf("unexpected reservation after runtime failure: %#v", reservation)
	}
	account, err := storage.BudgetReservations().GetAccount(ctx, "policy_global")
	if err != nil {
		t.Fatal(err)
	}
	if account.ReservedTokens != 0 || account.SettledTokens != 10 {
		t.Fatalf("unexpected account after runtime failure: %#v", account)
	}
}

func TestBudgetModelGatewayPreservesResultWhenSettlementFails(t *testing.T) {
	ctx := context.Background()
	storage := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 12, 20, 0, 0, 0, time.UTC)}
	if _, err := storage.BudgetReservations().UpsertPolicy(ctx, domain.BudgetPolicy{
		ID:          "policy_global",
		Scope:       domain.BudgetPolicyScopeGlobal,
		TokenLimit:  100,
		Enforcement: domain.BudgetPolicyEnforcementHard,
		Enabled:     true,
		CreatedAt:   clock.t,
		UpdatedAt:   clock.t,
	}); err != nil {
		t.Fatal(err)
	}
	settlementErr := errors.New("settlement failed")
	executor := &recordingModelExecutor{result: ports.RuntimeExecutionResult{Output: "runtime output"}}
	gateway := NewBudgetModelGateway(failingSettlementRepository{BudgetReservationRepository: storage.BudgetReservations(), err: settlementErr}, executor, clock)

	result, err := gateway.Execute(ctx, modelRequest("reservation_settlement_failed", "request_settlement_failed", 8))
	if !errors.Is(err, settlementErr) {
		t.Fatalf("expected settlement error, got %v", err)
	}
	if result.Output != "runtime output" || executor.calls != 1 {
		t.Fatalf("expected preserved runtime result and one call, got result=%#v calls=%d", result, executor.calls)
	}
}

func modelRequest(reservationID, requestID string, inputTokens int) ports.ModelRequest {
	return ports.ModelRequest{
		RuntimeID: "crush",
		Execution: ports.RuntimeExecutionInput{Run: domain.Run{ID: "run_1", InputTokens: inputTokens}},
		Reservation: domain.BudgetReservationRequest{
			ReservationID: reservationID,
			RequestID:     requestID,
			TaskID:        "task_1",
			RunID:         "run_1",
			AgentID:       "agent_1",
			WorkspaceID:   "workspace_1",
			Tokens:        inputTokens,
			CreatedAt:     time.Date(2026, 7, 12, 20, 0, 0, 0, time.UTC),
		},
	}
}
