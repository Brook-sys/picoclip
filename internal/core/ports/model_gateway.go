package ports

import (
	"context"

	"picoclip/internal/core/domain"
)

// ModelGateway wraps one model/runtime call with the budget reservation lifecycle.
// It reserves before execution and settles the reservation after execution, including
// when the runtime returns an error.
type ModelGateway interface {
	Execute(ctx context.Context, request ModelRequest) (RuntimeExecutionResult, error)
}

type ModelRequest struct {
	RuntimeID   domain.RuntimeID
	Execution   RuntimeExecutionInput
	Reservation domain.BudgetReservationRequest
}

// RuntimeModelExecutor is the narrow runtime dependency used by a ModelGateway.
type RuntimeModelExecutor interface {
	Execute(ctx context.Context, id domain.RuntimeID, input RuntimeExecutionInput) (RuntimeExecutionResult, error)
}
