package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"picoclip/internal/adapters/storage/memory"
	"picoclip/internal/core/domain"
)

func TestRuntimeManagerUninstallRemovesRuntimeState(t *testing.T) {
	ctx := context.Background()
	storage := memory.NewStorage()
	baseDir := t.TempDir()
	clock := SystemClock{}
	manager := NewRuntimeManager(storage, baseDir, clock)

	runtimeDir := filepath.Join(baseDir, "crush")
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		t.Fatal(err)
	}
	state := domain.RuntimeState{
		ID:           "runtime_crush",
		RuntimeID:    "crush",
		Mode:         domain.InstallModeExclusive,
		Enabled:      true,
		BinPath:      filepath.Join(runtimeDir, "bin", "crush"),
		SettingsJSON: "{}",
		MetadataJSON: "{}",
	}
	if err := storage.Runtimes().Save(ctx, state); err != nil {
		t.Fatal(err)
	}

	if err := manager.Uninstall(ctx, "crush"); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.Runtimes().GetByRuntimeID(ctx, "crush"); err != domain.ErrNotFound {
		t.Fatalf("expected runtime state to be removed, got %v", err)
	}
	if _, err := os.Stat(runtimeDir); !os.IsNotExist(err) {
		t.Fatalf("expected runtime directory to be removed, got %v", err)
	}
}

func TestRuntimeManagerTestAllConfiguredWithoutRuntimes(t *testing.T) {
	storage := memory.NewStorage()
	manager := NewRuntimeManager(storage, t.TempDir(), SystemClock{})
	manager.TestAllConfigured(context.Background(), NoopLogger{})
}
