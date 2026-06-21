package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"picoclip/internal/adapters/events"
	"picoclip/internal/adapters/storage/memory"
	"picoclip/internal/core/domain"
	"picoclip/internal/core/services"
)

func newTestServer(t *testing.T) *httptest.Server {
	return newTestServerWithDebug(t, true)
}

func newTestServerWithDebug(t *testing.T, debug bool) *httptest.Server {
	storage := memory.NewStorage()
	return newTestServerWithStorage(t, storage, debug)
}

func newTestServerWithStorage(t *testing.T, storage *memory.Storage, debug bool) *httptest.Server {
	t.Helper()
	clock := services.SystemClock{}
	idGen := &services.TimeIDGenerator{}
	bus := events.NewInMemoryBus()
	agents := services.NewAgentService(storage, clock, idGen)
	tasks := services.NewTaskService(storage, clock, idGen, bus)
	projects := services.NewWorkspaceService(storage, clock, idGen, t.TempDir())
	if _, err := projects.EnsureDefault(t.Context()); err != nil {
		t.Fatalf("ensure default project: %v", err)
	}
	skills := services.NewSkillService(storage, clock, idGen)
	if err := skills.InstallBuiltins(t.Context()); err != nil {
		t.Fatalf("install builtins: %v", err)
	}
	mux := http.NewServeMux()
	runtimes := services.NewRuntimeManager(storage, "data/runtimes", clock)
	diagnostics := services.NewDiagnosticsService(storage, runtimes, services.DiagnosticsConfig{StorageType: "memory", WorkspacePath: "workspaces", RuntimePath: "data/runtimes"})
	NewServer(agents, tasks, skills, projects, runtimes, diagnostics, storage, bus, debug).Mount(mux)
	return httptest.NewServer(mux)
}

func postJSON(t *testing.T, client *http.Client, url string, payload any) map[string]any {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	res, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post %s: %v", url, err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		t.Fatalf("post %s status = %d", url, res.StatusCode)
	}
	var decoded map[string]any
	if err := json.NewDecoder(res.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode %s: %v", url, err)
	}
	return decoded
}

func TestAgentTaskLifecycleAPI(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	client := ts.Client()
	agent := postJSON(t, client, ts.URL+"/api/agents", map[string]any{"name": "Lifecycle Agent", "type": "noop"})
	task := postJSON(t, client, ts.URL+"/agent-api/tasks", map[string]any{"assignee_agent_id": agent["id"], "prompt": "Lifecycle test"})
	if task["status"] != "todo" {
		t.Fatalf("created status = %v, want todo", task["status"])
	}

	checkout := postJSON(t, client, ts.URL+"/agent-api/tasks/"+task["id"].(string)+"/checkout", map[string]any{"agent_id": agent["id"], "expected_statuses": []string{"todo"}})
	if checkout["status"] != "in_progress" {
		t.Fatalf("checkout status = %v, want in_progress", checkout["status"])
	}

	blockedBody, _ := json.Marshal(map[string]any{"agent_id": agent["id"], "status": "blocked", "comment": "Need review."})
	req, err := http.NewRequest(http.MethodPatch, ts.URL+"/agent-api/tasks/"+task["id"].(string), bytes.NewReader(blockedBody))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("patch blocked status = %d", res.StatusCode)
	}

	_ = postJSON(t, client, ts.URL+"/agent-api/tasks/"+task["id"].(string)+"/comments", map[string]any{"role": "user", "body": "Continue."})

	detailRes, err := client.Get(ts.URL + "/agent-api/tasks/" + task["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	defer detailRes.Body.Close()
	var detail struct {
		Task struct {
			Status string `json:"status"`
		} `json:"task"`
		Messages []struct {
			Body string `json:"body"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(detailRes.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	if detail.Task.Status != "todo" {
		t.Fatalf("status after user comment = %s, want todo", detail.Task.Status)
	}
	if len(detail.Messages) < 2 {
		t.Fatalf("messages len = %d, want at least 2", len(detail.Messages))
	}
}

func TestAgentNewHidesNoopAndShowsRuntimeWarningWhenDebugDisabled(t *testing.T) {
	ts := newTestServerWithDebug(t, false)
	defer ts.Close()

	res, err := ts.Client().Get(ts.URL + "/agents/new")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(res.Body); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if strings.Contains(html, `value="noop"`) {
		t.Fatalf("noop should not be visible when debug is disabled")
	}
	if !strings.Contains(html, "No runtimes installed") {
		t.Fatalf("expected no runtime warning")
	}
}

func TestAgentNewHidesRuntimeWithMissingBinary(t *testing.T) {
	storage := memory.NewStorage()
	state := domain.RuntimeState{ID: "runtime_picoclaw", RuntimeID: "picoclaw", Mode: domain.InstallModeExclusive, Enabled: true, BinPath: "/tmp/definitely-missing-picoclaw", SettingsJSON: "{}", MetadataJSON: "{}"}
	if err := storage.Runtimes().Save(t.Context(), state); err != nil {
		t.Fatal(err)
	}
	ts := newTestServerWithStorage(t, storage, false)
	defer ts.Close()

	res, err := ts.Client().Get(ts.URL + "/agents/new")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(res.Body); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if strings.Contains(html, `value="picoclaw"`) {
		t.Fatalf("picoclaw should not be visible when its binary is missing")
	}
}

func TestWebCreateAgentRejectsUnavailableNoop(t *testing.T) {
	ts := newTestServerWithDebug(t, false)
	defer ts.Close()

	res, err := ts.Client().Post(ts.URL+"/agents", "application/x-www-form-urlencoded", strings.NewReader("name=Agent&type=noop"))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected bad request, got %d", res.StatusCode)
	}
}

func TestTaskDetailUsesPartialPollingOnly(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	client := ts.Client()
	agent := postJSON(t, client, ts.URL+"/api/agents", map[string]any{"name": "UI Agent", "type": "noop"})
	task := postJSON(t, client, ts.URL+"/agent-api/tasks", map[string]any{"assignee_agent_id": agent["id"], "prompt": "UI detail test"})

	res, err := client.Get(ts.URL + "/tasks/" + task["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(res.Body); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if strings.Contains(html, `hx-get="/tasks/`) || strings.Contains(html, `hx-trigger="every 2s" hx-target="body"`) || strings.Contains(html, `<section class="detail-grid" hx-`) {
		t.Fatalf("task detail should not poll and swap body")
	}
	if !strings.Contains(html, `id="task-live"`) || !strings.Contains(html, `/partials/tasks/`+task["id"].(string)) {
		t.Fatalf("task detail should use task-live partial polling")
	}

	partialRes, err := client.Get(ts.URL + "/partials/tasks/" + task["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	defer partialRes.Body.Close()
	partial := new(bytes.Buffer)
	if _, err := partial.ReadFrom(partialRes.Body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(partial.String(), "Conversation") || strings.Contains(partial.String(), "<html") {
		t.Fatalf("partial should render live fragment only")
	}
}
