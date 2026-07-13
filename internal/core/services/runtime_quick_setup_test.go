package services

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"picoclip/internal/adapters/storage/memory"
	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

type quickRuntimeAdapter struct {
	fakeRuntimeAdapter
	applied domain.RuntimeQuickSetupInput
	tested  domain.RuntimeQuickSetupInput
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
func (a *quickRuntimeAdapter) TestQuickSetup(_ context.Context, _ domain.RuntimeState, input domain.RuntimeQuickSetupInput) (domain.RuntimeModelTestResult, error) {
	a.tested = input
	return domain.RuntimeModelTestResult{Status: "ok", Message: "Model responded successfully", Output: "PONG"}, nil
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

type revisionRuntimeAdapter struct {
	fakeRuntimeAdapter
	mu       sync.Mutex
	revision string
}

func (a *revisionRuntimeAdapter) QuickSetupSchema() domain.RuntimeQuickSetupSchema {
	return domain.RuntimeQuickSetupSchema{ProfileID: "openai-compatible"}
}
func (a *revisionRuntimeAdapter) ReadQuickSetup(context.Context, domain.RuntimeState) (domain.RuntimeQuickSetupView, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return domain.RuntimeQuickSetupView{ProfileID: "openai-compatible", Revision: a.revision}, nil
}
func (a *revisionRuntimeAdapter) ApplyQuickSetup(_ context.Context, _ domain.RuntimeState, input domain.RuntimeQuickSetupInput) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if input.Revision != a.revision {
		return domain.ErrConfigurationChanged
	}
	a.revision = "next"
	return nil
}
func (a *revisionRuntimeAdapter) TestQuickSetup(context.Context, domain.RuntimeState, domain.RuntimeQuickSetupInput) (domain.RuntimeModelTestResult, error) {
	return domain.RuntimeModelTestResult{}, nil
}

func TestRuntimeManagerConcurrentQuickSetupWithSameRevisionCommitsOnce(t *testing.T) {
	ctx := context.Background()
	st := memory.NewStorage()
	m := NewRuntimeManager(st, t.TempDir(), fixedClock{t: time.Now()})
	adapter := &revisionRuntimeAdapter{fakeRuntimeAdapter: fakeRuntimeAdapter{id: "crush"}, revision: "same"}
	m.Register(adapter)
	if err := st.Runtimes().Save(ctx, domain.RuntimeState{ID: "runtime_crush", RuntimeID: "crush", Enabled: true, SettingsJSON: "{}", MetadataJSON: "{}"}); err != nil {
		t.Fatal(err)
	}
	input := domain.RuntimeQuickSetupInput{ProfileID: "openai-compatible", Values: map[string]string{}, Revision: "same"}
	errs := make(chan error, 2)
	var start sync.WaitGroup
	start.Add(1)
	for range 2 {
		go func() {
			start.Wait()
			_, err := m.ApplyQuickSetup(ctx, "crush", input)
			errs <- err
		}()
	}
	start.Done()
	var succeeded, conflicted int
	for range 2 {
		err := <-errs
		if err == nil {
			succeeded++
		} else if errors.Is(err, domain.ErrConfigurationChanged) {
			conflicted++
		} else {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("succeeded=%d conflicted=%d", succeeded, conflicted)
	}
}

type configRuntimeAdapter struct {
	fakeRuntimeAdapter
	mu      sync.Mutex
	content []byte
}

func (a *configRuntimeAdapter) ReadConfig(context.Context, domain.RuntimeState) ([]domain.RuntimeConfigFile, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return []domain.RuntimeConfigFile{{Name: "config.json", Content: append([]byte(nil), a.content...), Editable: true}}, nil
}
func (a *configRuntimeAdapter) WriteConfig(_ context.Context, _ domain.RuntimeState, _ string, content []byte) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.content = append([]byte(nil), content...)
	return nil
}

func TestRuntimeManagerConcurrentAdvancedConfigWithSameRevisionCommitsOnce(t *testing.T) {
	ctx := context.Background()
	st := memory.NewStorage()
	m := NewRuntimeManager(st, t.TempDir(), fixedClock{t: time.Now()})
	adapter := &configRuntimeAdapter{fakeRuntimeAdapter: fakeRuntimeAdapter{id: "crush"}, content: []byte(`{"value":"old"}`)}
	m.Register(adapter)
	if err := st.Runtimes().Save(ctx, domain.RuntimeState{ID: "runtime_crush", RuntimeID: "crush", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	revision := fmt.Sprintf("%x", sha256.Sum256(adapter.content))
	errs := make(chan error, 2)
	var start sync.WaitGroup
	start.Add(1)
	for _, value := range []string{"one", "two"} {
		go func() {
			start.Wait()
			errs <- m.UpdateConfig(ctx, "crush", "config.json", revision, func(domain.RuntimeConfigFile) ([]byte, error) {
				return []byte(value), nil
			})
		}()
	}
	start.Done()
	var succeeded, conflicted int
	for range 2 {
		err := <-errs
		if err == nil {
			succeeded++
		} else if errors.Is(err, domain.ErrConfigurationChanged) {
			conflicted++
		} else {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("succeeded=%d conflicted=%d", succeeded, conflicted)
	}
}

func TestRuntimeManagerTestQuickSetupDoesNotPersistInput(t *testing.T) {
	ctx := context.Background()
	st := memory.NewStorage()
	m := NewRuntimeManager(st, t.TempDir(), fixedClock{t: time.Now()})
	adapter := &quickRuntimeAdapter{fakeRuntimeAdapter: fakeRuntimeAdapter{id: "crush"}}
	m.Register(adapter)
	state := domain.RuntimeState{ID: "runtime_crush", RuntimeID: "crush", Enabled: true, SettingsJSON: "{}", MetadataJSON: "{}"}
	if err := st.Runtimes().Save(ctx, state); err != nil {
		t.Fatal(err)
	}
	input := domain.RuntimeQuickSetupInput{ProfileID: "openai-compatible", Values: map[string]string{"base_url": "https://unsaved.example/v1", "model": "unsaved-model"}, APIKey: "unsaved-" + "secret"}
	result, err := m.TestQuickSetup(ctx, "crush", input)
	if err != nil || result.Status != "ok" || adapter.tested.Values["model"] != "unsaved-model" {
		t.Fatalf("result=%#v tested=%#v err=%v", result, adapter.tested, err)
	}
	saved, _ := m.State(ctx, "crush")
	serialized, _ := json.Marshal(saved)
	if contains(string(serialized), "unsaved-secret") || contains(string(serialized), "unsaved-model") {
		t.Fatalf("test input persisted: %s", serialized)
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
