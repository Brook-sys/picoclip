package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"picoclip/internal/adapters/storage/storagetest"
	"picoclip/internal/core/ports"
)

func TestBudgetReservationRepositoryContract(t *testing.T) {
	storagetest.RunBudgetReservationRepositoryContract(t, func(t *testing.T) ports.BudgetReservationRepository {
		t.Helper()
		path := filepath.Join(t.TempDir(), "budget-reservations.db")
		db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", path))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		storage := NewStorage(db)
		if err := storage.Migrate(context.Background()); err != nil {
			t.Fatal(err)
		}
		return storage.BudgetReservations()
	})
}
