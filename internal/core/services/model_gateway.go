package services

import (
	"context"
	"errors"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

// BudgetModelGateway reserves the estimated budget before calling a model and
// settles with the observed token count once the call has returned. A budget
// rejection is returned without invoking the runtime.
type BudgetModelGateway struct {
	reservations ports.BudgetReservationRepository
	executor     ports.RuntimeModelExecutor
	clock        ports.Clock
}

func NewBudgetModelGateway(reservations ports.BudgetReservationRepository, executor ports.RuntimeModelExecutor, clock ports.Clock) *BudgetModelGateway {
	return &BudgetModelGateway{reservations: reservations, executor: executor, clock: clock}
}

func (g *BudgetModelGateway) Execute(ctx context.Context, request ports.ModelRequest) (ports.RuntimeExecutionResult, error) {
	reservation, err := g.reservations.TryReserve(ctx, request.Reservation)
	if err != nil {
		return ports.RuntimeExecutionResult{}, err
	}

	result, executionErr := g.executor.Execute(ctx, request.RuntimeID, request.Execution)
	settlement := domain.BudgetReservationSettlement{
		ReservationID: reservation.ID,
		Tokens:        request.Execution.Run.InputTokens + estimateTokens(result.Output),
		SettledAt:     g.clock.Now(),
	}
	if executionErr != nil {
		settlement.Tokens = request.Execution.Run.InputTokens + estimateTokens(executionErr.Error())
	}
	if _, settleErr := g.reservations.Settle(ctx, settlement); settleErr != nil {
		if executionErr != nil {
			return result, errors.Join(executionErr, settleErr)
		}
		return result, settleErr
	}
	return result, executionErr
}
