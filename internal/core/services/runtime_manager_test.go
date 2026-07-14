package services

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"picoclip/internal/adapters/storage/memory"
	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

type fakeRuntimeAdapter struct {
	id           domain.RuntimeID
	out          string
	err          error
	health       domain.RuntimeHealth
	installState domain.RuntimeState
}

type failingRuntimeSaveStorage struct {
	ports.Storage
	runtimes ports.RuntimeRepository
}

func (s failingRuntimeSaveStorage) Runtimes() ports.RuntimeRepository { return s.runtimes }

type failingRuntimeSaveRepository struct {
	ports.RuntimeRepository
	err error
}

func (r failingRuntimeSaveRepository) Save(context.Context, domain.RuntimeState) error { return r.err }

type filesystemInstallAdapter struct {
	fakeRuntimeAdapter
}

func (a filesystemInstallAdapter) Install(_ context.Context, _ domain.InstallMode, destDir, _ string) (domain.RuntimeState, error) {
	binPath := filepath.Join(destDir, "bin", string(a.id))
	if err := os.MkdirAll(filepath.Dir(binPath), 0755); err != nil {
		return domain.RuntimeState{}, err
	}
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		return domain.RuntimeState{}, err
	}
	return domain.RuntimeState{BinPath: binPath}, nil
}

func (a fakeRuntimeAdapter) ID() domain.RuntimeID                        { return a.id }
func (a fakeRuntimeAdapter) Name() string                                { return "fake" }
func (a fakeRuntimeAdapter) Kind() domain.RuntimeKind                    { return "fake" }
func (a fakeRuntimeAdapter) SupportedInstallModes() []domain.InstallMode { return nil }
func (a fakeRuntimeAdapter) ListVersions(context.Context, int) ([]domain.RuntimeVersion, error) {
	return nil, nil
}
func (a fakeRuntimeAdapter) Install(context.Context, domain.InstallMode, string, string) (domain.RuntimeState, error) {
	return a.installState, nil
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
func (a fakeRuntimeAdapter) Cancel(ctx context.Context, state domain.RuntimeState, run domain.Run) error {
	return nil
}

func TestRuntimeManagerInstallPreservesPreexistingExclusiveDirectoryWhenStateSaveFails(t *testing.T) {
	ctx := context.Background()
	baseStorage := memory.NewStorage()
	storage := failingRuntimeSaveStorage{
		Storage: baseStorage,
		runtimes: failingRuntimeSaveRepository{
			RuntimeRepository: baseStorage.Runtimes(),
			err:               errors.New("database is readonly"),
		},
	}
	baseDir := t.TempDir()
	installDir := filepath.Join(baseDir, "crush")
	if err := os.MkdirAll(installDir, 0755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(installDir, "keep-me")
	if err := os.WriteFile(marker, []byte("user data"), 0644); err != nil {
		t.Fatal(err)
	}
	manager := NewRuntimeManager(storage, baseDir, SystemClock{})
	manager.Register(filesystemInstallAdapter{fakeRuntimeAdapter{
		id:     "crush",
		health: domain.RuntimeHealth{Status: "ok", Version: "test"},
	}})

	_, err := manager.Install(ctx, "crush", domain.InstallModeExclusive, "latest")
	if err == nil || !strings.Contains(err.Error(), "database is readonly") {
		t.Fatalf("expected save failure, got %v", err)
	}
	if content, readErr := os.ReadFile(marker); readErr != nil || string(content) != "user data" {
		t.Fatalf("preexisting runtime data must survive failed install, content=%q err=%v", content, readErr)
	}
}

func TestRuntimeManagerInstallRollsBackExclusiveFilesWhenStateSaveFails(t *testing.T) {
	ctx := context.Background()
	baseStorage := memory.NewStorage()
	storage := failingRuntimeSaveStorage{
		Storage: baseStorage,
		runtimes: failingRuntimeSaveRepository{
			RuntimeRepository: baseStorage.Runtimes(),
			err:               errors.New("database is readonly"),
		},
	}
	baseDir := t.TempDir()
	manager := NewRuntimeManager(storage, baseDir, SystemClock{})
	manager.Register(filesystemInstallAdapter{fakeRuntimeAdapter{
		id:     "crush",
		health: domain.RuntimeHealth{Status: "ok", Version: "test"},
	}})

	_, err := manager.Install(ctx, "crush", domain.InstallModeExclusive, "latest")
	if err == nil || !strings.Contains(err.Error(), "database is readonly") {
		t.Fatalf("expected save failure, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(baseDir, "crush")); !os.IsNotExist(statErr) {
		t.Fatalf("exclusive install files must be rolled back after persistence failure, stat err=%v", statErr)
	}
}

func TestRuntimeManagerInstallRejectsUnhealthyBinaryBeforeSaving(t *testing.T) {
	ctx := context.Background()
	storage := memory.NewStorage()
	manager := NewRuntimeManager(storage, t.TempDir(), SystemClock{})
	manager.Register(fakeRuntimeAdapter{
		id:           "claurst",
		installState: domain.RuntimeState{BinPath: "/tmp/claurst"},
		health:       domain.RuntimeHealth{Status: "error", Errors: []string{"Incompatible binary: glibc/musl mismatch"}},
	})

	_, err := manager.Install(ctx, "claurst", domain.InstallModeExclusive, "latest")
	if err == nil || !strings.Contains(err.Error(), "Incompatible binary") {
		t.Fatalf("expected health error, got %v", err)
	}
	if _, err := storage.Runtimes().GetByRuntimeID(ctx, "claurst"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unhealthy runtime was saved: %v", err)
	}
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

func TestRuntimeManagerCatalogIncludesClaurst(t *testing.T) {
	manager := NewRuntimeManager(memory.NewStorage(), t.TempDir(), SystemClock{})
	for _, manifest := range manager.Catalog() {
		if manifest.ID == "claurst" {
			if manifest.Name == "" || manifest.Description == "" || manifest.Repo == "" {
				t.Fatalf("claurst manifest incomplete: %#v", manifest)
			}
			return
		}
	}
	t.Fatal("expected claurst in runtime catalog")
}

func TestRuntimeManagerCatalogIncludesBwrapSandbox(t *testing.T) {
	manager := NewRuntimeManager(memory.NewStorage(), t.TempDir(), SystemClock{})
	for _, manifest := range manager.Catalog() {
		if manifest.ID == "bwrap" {
			if manifest.Name == "" || manifest.Description == "" || manifest.Repo == "" {
				t.Fatalf("bwrap manifest incomplete: %#v", manifest)
			}
			if manifest.Kind != domain.RuntimeKindSandbox {
				t.Fatalf("expected sandbox kind, got %q", manifest.Kind)
			}
			return
		}
	}
	t.Fatal("expected bwrap in runtime catalog")
}
