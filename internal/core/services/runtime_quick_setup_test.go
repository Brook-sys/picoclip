package services

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"picoclip/internal/adapters/storage/memory"
	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

type quickRuntimeAdapter struct {
	fakeRuntimeAdapter
	applied domain.RuntimeQuickSetupInput
}

func (a *quickRuntimeAdapter) QuickSetupSchema() domain.RuntimeQuickSetupSchema {
	return domain.RuntimeQuickSetupSchema{ProfileID: "openai-compatible"}
}
func (a *quickRuntimeAdapter) ReadQuickSetup(context.Context, domain.RuntimeState) (domain.RuntimeQuickSetupView, error) {
	return domain.RuntimeQuickSetupView{ProfileID: "openai-compatible", Values: map[string]string{"base_url": "https://x/v1", "model": "m"}, Revision: "r2", Configured: true}, nil
}
func (a *quickRuntimeAdapter) ApplyQuickSetup(_ context.Context, _ domain.RuntimeState, input domain.RuntimeQuickSetupInput) error {
	a.applied = input
	return nil
}
func (a *quickRuntimeAdapter) ResolveExistingPaths(bin string) domain.RuntimeState {
	return domain.RuntimeState{BinPath: bin, ConfigPath: "/native/config.json", HomePath: "/native"}
}

func TestRuntimeManagerQuickSetupClearsStaleAITestAndDoesNotPersistSecret(t *testing.T) {
	ctx := context.Background()
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 13, 4, 0, 0, 0, time.UTC)}
	m := NewRuntimeManager(st, t.TempDir(), clock)
	adapter := &quickRuntimeAdapter{fakeRuntimeAdapter: fakeRuntimeAdapter{id: "crush"}}
	m.Register(adapter)
	bin := filepath.Join(t.TempDir(), "runtime")
	if err := os.WriteFile(bin, []byte("x"), 0755); err != nil {
		t.Fatal(err)
	}
	state := domain.RuntimeState{ID: "runtime_crush", RuntimeID: "crush", Enabled: true, BinPath: bin, ConfigPath: "/tmp/config", SettingsJSON: "{}", MetadataJSON: `{"last_ai_test":{"status":"ok"},"keep":true}`}
	if err := st.Runtimes().Save(ctx, state); err != nil {
		t.Fatal(err)
	}
	view, err := m.ApplyQuickSetup(ctx, "crush", domain.RuntimeQuickSetupInput{ProfileID: "openai-compatible", Values: map[string]string{"base_url": "https://x/v1", "model": "m"}, APIKey: "super-secret", Revision: "r1"})
	if err != nil {
		t.Fatal(err)
	}
	if view.Revision != "r2" {
		t.Fatalf("view=%#v", view)
	}
	saved, err := m.State(ctx, "crush")
	if err != nil {
		t.Fatal(err)
	}
	serialized, _ := json.Marshal(saved)
	if string(serialized) == "" || contains(string(serialized), "super-secret") {
		t.Fatal("secret persisted")
	}
	var metadata map[string]any
	if err = json.Unmarshal([]byte(saved.MetadataJSON), &metadata); err != nil {
		t.Fatal(err)
	}
	if _, ok := metadata["last_ai_test"]; ok {
		t.Fatal("stale ai result retained")
	}
	if metadata["keep"] != true {
		t.Fatal("unrelated metadata lost")
	}
}

func TestRuntimeManagerQuickSetupRejectsUnsupportedAndMissing(t *testing.T) {
	ctx := context.Background()
	st := memory.NewStorage()
	m := NewRuntimeManager(st, t.TempDir(), fixedClock{t: time.Now()})
	m.Register(fakeRuntimeAdapter{id: "crush"})
	if _, _, err := m.QuickSetup(ctx, "crush"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing err=%v", err)
	}
	if err := st.Runtimes().Save(ctx, domain.RuntimeState{ID: "runtime_crush", RuntimeID: "crush", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.QuickSetup(ctx, "crush"); !errors.Is(err, domain.ErrQuickSetupUnsupported) {
		t.Fatalf("unsupported err=%v", err)
	}
}

func TestConfigureExistingMergesResolvedNativePaths(t *testing.T) {
	ctx := context.Background()
	st := memory.NewStorage()
	m := NewRuntimeManager(st, t.TempDir(), fixedClock{t: time.Now()})
	bin := filepath.Join(t.TempDir(), "runtime")
	if err := os.WriteFile(bin, []byte("x"), 0755); err != nil {
		t.Fatal(err)
	}
	adapter := &quickRuntimeAdapter{fakeRuntimeAdapter: fakeRuntimeAdapter{id: "crush"}}
	m.Register(adapter)
	state, err := m.ConfigureExisting(ctx, "crush", bin)
	if err != nil {
		t.Fatal(err)
	}
	if state.ConfigPath != "/native/config.json" || state.HomePath != "/native" || state.BinPath != bin {
		t.Fatalf("state=%#v", state)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

var _ ports.RuntimeQuickConfigurator = (*quickRuntimeAdapter)(nil)
