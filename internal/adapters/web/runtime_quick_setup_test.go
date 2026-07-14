package web

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestRuntimeQuickSetupSettingsRedactsSecretAndPrecedesAdvanced(t *testing.T) {
	server, config := newRuntimeQuickSetupServer(t)
	defer server.Close()
	res, err := server.Client().Get(server.URL + "/settings#runtimes")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	html := string(body)
	for _, expected := range []string{"Quick Setup", "Base URL", "API key", "Model", "Test Model", "Save Quick Setup", "Advanced configuration", `type="password"`, `autocomplete="new-password"`, "Configured", "runtime-quick-setup-heading", "runtime-quick-setup-section", "runtime-quick-setup-actions", "Provider endpoint", "Credentials & model"} {
		if !strings.Contains(html, expected) {
			t.Fatalf("missing %q", expected)
		}
	}
	if strings.Contains(html, "stored-secret") {
		t.Fatal("secret rendered")
	}
	if strings.Index(html, "Quick Setup") > strings.Index(html, "Advanced configuration") {
		t.Fatal("advanced rendered before quick setup")
	}
	if !strings.Contains(html, `<details class="runtime-advanced-config"`) {
		t.Fatal("advanced is not collapsed details")
	}
	before, _ := os.ReadFile(config)
	form := url.Values{"profile_id": {"openai-compatible"}, "base_url": {"https://new.example/v1"}, "model": {"new-model"}, "api_key": {""}, "revision": {"stale"}}
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/runtimes/crush/quick-setup", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	stale, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	stale.Body.Close()
	if stale.StatusCode != http.StatusConflict {
		t.Fatalf("status=%d", stale.StatusCode)
	}
	after, _ := os.ReadFile(config)
	if string(before) != string(after) {
		t.Fatal("stale form changed config")
	}
}

func TestRuntimeQuickSetupTestModelRejectsPrivateEndpointWithoutSaving(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("private model endpoint must not be reached")
	}))
	defer provider.Close()

	server, config := newRuntimeQuickSetupServer(t)
	defer server.Close()
	before, _ := os.ReadFile(config)
	form := url.Values{"profile_id": {"openai-compatible"}, "base_url": {provider.URL + "/v1"}, "model": {"unsaved-model"}, "api_key": {"unsaved-secret"}}
	res, err := server.Client().Post(server.URL+"/runtimes/crush/test-model", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(string(body), "public HTTP(S) endpoint") {
		t.Fatalf("status=%d body=%s", res.StatusCode, body)
	}
	after, _ := os.ReadFile(config)
	if string(after) != string(before) {
		t.Fatal("rejected test model request saved the quick setup form")
	}
}

func TestRuntimeAdvancedConfigRejectsUnknownAndInvalidYAML(t *testing.T) {
	server, _ := newRuntimeQuickSetupServer(t)
	defer server.Close()
	for _, tc := range []struct{ name, content string }{{"../../other", "{}"}, {".security.yml", "bad: ["}} {
		form := url.Values{"file_name": {tc.name}, "content": {tc.content}}
		res, err := server.Client().Post(server.URL+"/runtimes/crush/config", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s status=%d", tc.name, res.StatusCode)
		}
	}
}

func TestRuntimeAdvancedConfigPreservesRedactedSecretOnSave(t *testing.T) {
	server, config := newRuntimeQuickSetupServer(t)
	defer server.Close()
	contentBefore, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{"file_name": {"crush.json"}, "revision": {runtimeConfigRevision(contentBefore)}, "content": {`{"providers":{"picoclip-openai":{"type":"openai-compat","base_url":"https://changed.example/v1","api_key":"[REDACTED]","models":[{"id":"old-model","name":"old-model"}]}},"models":{"large":{"model":"old-model","provider":"picoclip-openai"},"small":{"model":"old-model","provider":"picoclip-openai"}}}`}}
	res, err := server.Client().Post(server.URL+"/runtimes/crush/config", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", res.StatusCode)
	}
	content, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "stored-secret") || strings.Contains(string(content), "[REDACTED]") {
		t.Fatalf("secret was not preserved: %s", content)
	}
	stale := url.Values{"file_name": {"crush.json"}, "revision": {"stale"}, "content": {string(content)}}
	staleRes, err := server.Client().Post(server.URL+"/runtimes/crush/config", "application/x-www-form-urlencoded", strings.NewReader(stale.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	if staleRes.Header.Get("HX-Refresh") != "true" {
		t.Fatal("missing HX-Refresh")
	}
	staleRes.Body.Close()
	if staleRes.StatusCode != http.StatusConflict {
		t.Fatalf("status=%d", staleRes.StatusCode)
	}
}

func TestRuntimeConfigRedactionFailsClosedAndRecognizesCommonSecretNames(t *testing.T) {
	invalid := domain.RuntimeConfigFile{Language: "json", Content: []byte(`{"apiKey":"leak"`)}
	if got := string(redactRuntimeConfig(invalid)); strings.Contains(got, "leak") {
		t.Fatalf("invalid config leaked: %q", got)
	}
	valid := domain.RuntimeConfigFile{Language: "json", Content: []byte(`{"apiKey":"a","access_token":"b","refresh_token":"refresh-value","id_token":"identity-value","bearer_token":"bearer-value","clientSecret":"c","extra_headers":{"Authorization":"Bearer d","Proxy-Authorization":"proxy-value","X-API-Key":"e"},"OPENAI_API_KEY":"f"}`)}
	got := string(redactRuntimeConfig(valid))
	for _, secret := range []string{`"a"`, `"b"`, "refresh-value", "identity-value", "bearer-value", `"c"`, "Bearer d", "proxy-value", `"e"`, `"f"`} {
		if strings.Contains(got, secret) {
			t.Fatalf("secret %q leaked: %s", secret, got)
		}
	}
}

func newRuntimeQuickSetupServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	ctx := context.Background()
	storage := memory.NewStorage()
	clock := services.SystemClock{}
	idGen := &services.TimeIDGenerator{}
	bus := events.NewInMemoryBus()
	agents := services.NewAgentService(storage, clock, idGen)
	tasks := services.NewTaskService(storage, clock, idGen, bus)
	manager := services.NewRuntimeManager(storage, t.TempDir(), clock)
	adapter := runtimes.NewCrushAdapter("")
	manager.Register(adapter)
	projects := services.NewWorkspaceService(storage, clock, idGen, t.TempDir())
	_, _ = projects.EnsureDefault(ctx)
	skills := services.NewSkillService(storage, clock, idGen)
	_ = skills.InstallBuiltins(ctx)
	dir := t.TempDir()
	config := filepath.Join(dir, "crush.json")
	binPath := filepath.Join(dir, "crush")
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\nprintf 'crush v-test\\n'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte(`{"providers":{"picoclip-openai":{"type":"openai-compat","base_url":"https://old.example/v1","api_key":"stored-secret","models":[{"id":"old-model","name":"old-model"}]}},"models":{"large":{"model":"old-model","provider":"picoclip-openai"},"small":{"model":"old-model","provider":"picoclip-openai"}}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := storage.Runtimes().Save(ctx, domain.RuntimeState{ID: "runtime_crush", RuntimeID: "crush", Enabled: true, BinPath: binPath, ConfigPath: config, SettingsJSON: "{}", MetadataJSON: "{}"}); err != nil {
		t.Fatal(err)
	}
	diagnostics := services.NewDiagnosticsService(storage, manager, services.DiagnosticsConfig{StorageType: "memory"})
	mux := http.NewServeMux()
	NewServer(agents, tasks, skills, projects, manager, diagnostics, storage, bus, true).Mount(mux)
	return httptest.NewServer(mux), config
}
