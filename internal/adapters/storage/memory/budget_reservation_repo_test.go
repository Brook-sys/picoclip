package memory

import (
	"testing"

	"picoclip/internal/adapters/storage/storagetest"
	"picoclip/internal/core/ports"
)

func TestBudgetReservationRepositoryContract(t *testing.T) {
	storagetest.RunBudgetReservationRepositoryContract(t, func(t *testing.T) ports.BudgetReservationRepository {
		t.Helper()
		return NewBudgetReservationRepository()
	})
}
