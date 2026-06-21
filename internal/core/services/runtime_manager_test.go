package services

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"picoclip/internal/adapters/storage/memory"
	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

type fakeRuntimeAdapter struct {
	id     domain.RuntimeID
	out    string
	err    error
	health domain.RuntimeHealth
}

func (a fakeRuntimeAdapter) ID() domain.RuntimeID                        { return a.id }
func (a fakeRuntimeAdapter) Name() string                                { return "fake" }
func (a fakeRuntimeAdapter) Kind() domain.RuntimeKind                    { return "fake" }
func (a fakeRuntimeAdapter) SupportedInstallModes() []domain.InstallMode { return nil }
func (a fakeRuntimeAdapter) ListVersions(context.Context, int) ([]domain.RuntimeVersion, error) {
	return nil, nil
}
func (a fakeRuntimeAdapter) Install(context.Context, domain.InstallMode, string, string) (domain.RuntimeState, error) {
	return domain.RuntimeState{}, nil
}
func (a fakeRuntimeAdapter) Resolve(context.Context, domain.RuntimeState) error { return nil }
func (a fakeRuntimeAdapter) Health(context.Context, domain.RuntimeState) domain.RuntimeHealth {
	return a.health
}
func (a fakeRuntimeAdapter) ReadConfig(context.Context, domain.RuntimeState) ([]domain.RuntimeConfigFile, error) {
	return nil, nil
}
func (a fakeRuntimeAdapter) WriteConfig(context.Context, domain.RuntimeState, string, []byte) error {
	return nil
}
func (a fakeRuntimeAdapter) Execute(ctx context.Context, state domain.RuntimeState, input ports.RuntimeExecutionInput) (ports.RuntimeExecutionResult, error) {
	return ports.RuntimeExecutionResult{Output: a.out}, a.err
}

func TestRuntimeManagerTestAISavesMetadataAndHandlesError(t *testing.T) {
	ctx := context.Background()
	storage := memory.NewStorage()
	baseDir := t.TempDir()
	clock := SystemClock{}
	manager := NewRuntimeManager(storage, baseDir, clock)

	adapter := fakeRuntimeAdapter{id: "crush", err: errors.New("raw failure banner")}
	manager.Register(adapter)

	state := domain.RuntimeState{
		ID:           "runtime_crush",
		RuntimeID:    "crush",
		Mode:         domain.InstallModeExisting,
		Enabled:      true,
		SettingsJSON: "{}",
		MetadataJSON: "{}",
	}
	if err := storage.Runtimes().Save(ctx, state); err != nil {
		t.Fatal(err)
	}

	result, err := manager.TestAI(ctx, "crush")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "error" {
		t.Fatalf("expected error, got %v", result.Status)
	}
	if result.Message != "AI request failed" {
		t.Fatalf("expected short message, got %s", result.Message)
	}
	if result.Output != "raw failure banner" {
		t.Fatalf("expected raw output, got %s", result.Output)
	}
}
func TestRuntimeManagerTestAISavesMetadata(t *testing.T) {
	ctx := context.Background()
	storage := memory.NewStorage()
	baseDir := t.TempDir()
	clock := SystemClock{}
	manager := NewRuntimeManager(storage, baseDir, clock)

	adapter := fakeRuntimeAdapter{id: "crush", out: "PONG"}
	manager.Register(adapter)

	state := domain.RuntimeState{
		ID:           "runtime_crush",
		RuntimeID:    "crush",
		Mode:         domain.InstallModeExisting,
		Enabled:      true,
		SettingsJSON: "{}",
		MetadataJSON: "{}",
	}
	if err := storage.Runtimes().Save(ctx, state); err != nil {
		t.Fatal(err)
	}

	result, err := manager.TestAI(ctx, "crush")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "ok" || result.Output != "PONG" {
		t.Fatalf("expected ok and PONG, got %v: %s", result.Status, result.Output)
	}

	saved, _ := storage.Runtimes().GetByRuntimeID(ctx, "crush")
	var meta struct {
		LastAITest *RuntimeAITestResult `json:"last_ai_test"`
	}
	if err := json.Unmarshal([]byte(saved.MetadataJSON), &meta); err != nil {
		t.Fatal(err)
	}
	if meta.LastAITest == nil || meta.LastAITest.Output != "PONG" {
		t.Fatalf("metadata missing or wrong: %v", meta.LastAITest)
	}
}

func TestRuntimeManagerTestSavesHealth(t *testing.T) {
	ctx := context.Background()
	storage := memory.NewStorage()
	clock := SystemClock{}
	manager := NewRuntimeManager(storage, t.TempDir(), clock)

	now := clock.Now()
	adapter := fakeRuntimeAdapter{id: "crush", health: domain.RuntimeHealth{Status: "ok", Version: "1.0", CheckedAt: now}}
	manager.Register(adapter)

	state := domain.RuntimeState{
		ID:           "runtime_crush",
		RuntimeID:    "crush",
		Mode:         domain.InstallModeExisting,
		Enabled:      true,
		SettingsJSON: "{}",
		MetadataJSON: "{}",
	}
	if err := storage.Runtimes().Save(ctx, state); err != nil {
		t.Fatal(err)
	}

	health, err := manager.Test(ctx, "crush")
	if err != nil {
		t.Fatal(err)
	}
	if health.Version != "1.0" {
		t.Fatalf("expected version 1.0, got %s", health.Version)
	}

	saved, _ := storage.Runtimes().GetByRuntimeID(ctx, "crush")
	var h domain.RuntimeHealth
	if err := json.Unmarshal([]byte(saved.LastHealthJSON), &h); err != nil {
		t.Fatal(err)
	}
	if h.Version != "1.0" {
		t.Fatalf("saved health version wrong: %v", h.Version)
	}
}

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
