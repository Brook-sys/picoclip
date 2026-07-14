package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"picoclip/internal/adapters/events"
	"picoclip/internal/adapters/runtimes"
	"picoclip/internal/adapters/storage/memory"
	"picoclip/internal/core/domain"
	"picoclip/internal/core/services"
)

func TestRuntimeCardsRefreshHealthAfterInstalledBinaryDisappears(t *testing.T) {
	ts, binPath := newRuntimeReliabilityServer(t)
	defer ts.Close()

	if err := os.Remove(binPath); err != nil {
		t.Fatal(err)
	}

	res, err := ts.Client().Get(ts.URL + "/api/runtimes")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}

	var cards []RuntimeCardView
	if err := json.NewDecoder(res.Body).Decode(&cards); err != nil {
		t.Fatal(err)
	}
	var crush RuntimeCardView
	for _, card := range cards {
		if card.ID == "crush" {
			crush = card
			break
		}
	}
	if !crush.Configured {
		t.Fatal("persisted runtime should remain configured so it can be repaired or uninstalled")
	}
	if crush.Functional || crush.Health.Status != "error" {
		t.Fatalf("missing binary must be reported as unhealthy, got functional=%v health=%#v", crush.Functional, crush.Health)
	}
	if len(crush.Checks) == 0 || crush.Checks[0].Name != "binary_exists" || crush.Checks[0].Status != "error" {
		t.Fatalf("expected binary_exists error check, got %#v", crush.Checks)
	}

	settings, err := ts.Client().Get(ts.URL + "/settings#runtimes")
	if err != nil {
		t.Fatal(err)
	}
	defer settings.Body.Close()
	body := readBodyString(t, settings.Body)
	if !strings.Contains(body, "Repair runtime") {
		t.Fatal("broken persisted runtime must expose a repair action")
	}
	if strings.Contains(body, `id="runtime-quick-setup-crush"`) {
		t.Fatal("quick setup must stay hidden until the runtime CLI is functional")
	}
}

func TestRuntimeCLITestReportsFailureWhenHealthIsError(t *testing.T) {
	ts, binPath := newRuntimeReliabilityServer(t)
	defer ts.Close()

	if err := os.Remove(binPath); err != nil {
		t.Fatal(err)
	}

	res, err := ts.Client().Post(ts.URL+"/runtimes/crush/test", "application/x-www-form-urlencoded", strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	trigger := res.Header.Get("HX-Trigger")
	if !strings.Contains(trigger, "CLI check failed") {
		t.Fatalf("expected failure toast, got %q", trigger)
	}
	if strings.Contains(trigger, "CLI check successful") {
		t.Fatalf("unhealthy CLI must not report success: %q", trigger)
	}
}

func newRuntimeReliabilityServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	ctx := context.Background()
	storage := memory.NewStorage()
	clock := services.SystemClock{}
	idGen := &services.TimeIDGenerator{}
	bus := events.NewInMemoryBus()
	agents := services.NewAgentService(storage, clock, idGen)
	tasks := services.NewTaskService(storage, clock, idGen, bus)
	baseDir := t.TempDir()
	manager := services.NewRuntimeManager(storage, baseDir, clock)
	manager.Register(runtimes.NewCrushAdapter(""))
	projects := services.NewWorkspaceService(storage, clock, idGen, t.TempDir())
	if _, err := projects.EnsureDefault(ctx); err != nil {
		t.Fatal(err)
	}
	skills := services.NewSkillService(storage, clock, idGen)
	if err := skills.InstallBuiltins(ctx); err != nil {
		t.Fatal(err)
	}

	binPath := filepath.Join(baseDir, "crush", "bin", "crush")
	if err := os.MkdirAll(filepath.Dir(binPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\nprintf 'crush v-test\\n'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	state := domain.RuntimeState{
		ID:           "runtime_crush",
		RuntimeID:    "crush",
		Mode:         domain.InstallModeExclusive,
		Enabled:      true,
		BinPath:      binPath,
		SettingsJSON: "{}",
		MetadataJSON: "{}",
	}
	if err := storage.Runtimes().Save(ctx, state); err != nil {
		t.Fatal(err)
	}
	health, err := manager.Test(ctx, "crush")
	if err != nil || health.Status != "ok" {
		t.Fatalf("seed health = %#v, err=%v", health, err)
	}

	diagnostics := services.NewDiagnosticsService(storage, manager, services.DiagnosticsConfig{StorageType: "memory", RuntimePath: baseDir})
	mux := http.NewServeMux()
	NewServer(agents, tasks, skills, projects, manager, diagnostics, storage, bus, true).Mount(mux)
	return httptest.NewServer(mux), binPath
}
