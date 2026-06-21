package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

func TestSQLiteRestoreRollbackPreservesExistingData(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	storage := NewStorage(db)
	if err := storage.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	original := domain.Agent{ID: "agt_original", Name: "Original", Type: "noop", Enabled: true, CreatedAt: now, UpdatedAt: now}
	if err := storage.Agents().Create(ctx, original); err != nil {
		t.Fatal(err)
	}
	badBackup := ports.BackupData{
		Agents: []domain.Agent{{ID: "agt_bad", Name: "Bad", Type: "noop", Enabled: true, CreatedAt: now, UpdatedAt: now}},
	}
	badBackup.Agents = append(badBackup.Agents, domain.Agent{ID: "agt_bad", Name: "Bad 2", Type: "noop", Enabled: true, CreatedAt: now, UpdatedAt: now})
	if err := storage.RestoreAllData(ctx, badBackup); err == nil {
		t.Fatal("expected restore to fail")
	}
	got, err := storage.Agents().Get(ctx, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != original.Name {
		t.Fatalf("expected original preserved, got %#v", got)
	}
}
