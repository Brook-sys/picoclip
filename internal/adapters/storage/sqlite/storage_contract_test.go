package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"picoclip/internal/adapters/storage/storagetest"
	"picoclip/internal/core/ports"
)

func TestSQLiteStorageContract(t *testing.T) {
	storagetest.RunStorageContract(t, func(t *testing.T) ports.Storage {
		t.Helper()
		path := filepath.Join(t.TempDir(), "test.db")
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		storage := NewStorage(db)
		if err := storage.Migrate(context.Background()); err != nil {
			t.Fatal(err)
		}
		return storage
	})
}
