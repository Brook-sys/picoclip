package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	runtimes := services.NewRuntimeManager(storage, "data/runtimes", clock)
	tasks.SetCanceler(runtimes)
	projects := services.NewWorkspaceService(storage, clock, idGen, t.TempDir())
	if _, err := projects.EnsureDefault(t.Context()); err != nil {
		t.Fatalf("ensure default project: %v", err)
	}
	skills := services.NewSkillService(storage, clock, idGen)
	if err := skills.InstallBuiltins(t.Context()); err != nil {
		t.Fatalf("install builtins: %v", err)
	}
	mux := http.NewServeMux()
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
	task := postJSON(t, client, ts.URL+"/agent-api/tasks", map[string]any{"from_agent_id": agent["id"], "assignee_agent_id": agent["id"], "prompt": "Lifecycle test"})
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

	_ = postJSON(t, client, ts.URL+"/agent-api/tasks/"+task["id"].(string)+"/comments", map[string]any{"from_id": agent["id"], "role": "user", "body": "Continue."})

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

func TestAgentAPIPermissionEnforcement(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	client := ts.Client()
	agent := postJSON(t, client, ts.URL+"/api/agents", map[string]any{"name": "Observer", "type": "noop", "capability": "observer"})
	task := postJSON(t, client, ts.URL+"/api/tasks", map[string]any{"agent_id": agent["id"], "title": "Protected", "prompt": "Do"})

	body, _ := json.Marshal(map[string]any{"agent_id": agent["id"], "expected_statuses": []string{"todo"}})
	res, err := client.Post(ts.URL+"/agent-api/tasks/"+task["id"].(string)+"/checkout", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("checkout status = %d, want 403", res.StatusCode)
	}

	cancelBody, _ := json.Marshal(map[string]any{"agent_id": agent["id"], "reason": "test"})
	cancelReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/tasks/"+task["id"].(string)+"/cancel", bytes.NewReader(cancelBody))
	cancelReq.Header.Set("Content-Type", "application/json")
	cancelRes, err := client.Do(cancelReq)
	if err != nil {
		t.Fatal(err)
	}
	defer cancelRes.Body.Close()
	if cancelRes.StatusCode != http.StatusForbidden {
		t.Fatalf("cancel status = %d, want 403", cancelRes.StatusCode)
	}
}

func TestAPIV1EventsSupportsValidatedLimit(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	client := ts.Client()
	agent := postJSON(t, client, ts.URL+"/api/agents", map[string]any{"name": "API Events Agent", "type": "noop"})
	_ = postJSON(t, client, ts.URL+"/api/v1/tasks", map[string]any{"agent_id": agent["id"], "title": "Event one", "prompt": "Expose event one"})
	_ = postJSON(t, client, ts.URL+"/api/v1/tasks", map[string]any{"agent_id": agent["id"], "title": "Event two", "prompt": "Expose event two"})

	limitedRes, err := client.Get(ts.URL + "/api/v1/events?limit=1")
	if err != nil {
		t.Fatal(err)
	}
	defer limitedRes.Body.Close()
	if limitedRes.StatusCode != http.StatusOK {
		t.Fatalf("limited events status = %d", limitedRes.StatusCode)
	}
	var limited struct {
		Data []map[string]any `json:"data"`
		Meta map[string]any   `json:"meta"`
	}
	if err := json.NewDecoder(limitedRes.Body).Decode(&limited); err != nil {
		t.Fatal(err)
	}
	if len(limited.Data) != 1 {
		t.Fatalf("limited events count = %d, want 1", len(limited.Data))
	}
	if limited.Meta["limit"] != float64(1) {
		t.Fatalf("events meta limit = %v, want 1", limited.Meta["limit"])
	}

	invalidRes, err := client.Get(ts.URL + "/api/v1/events?limit=invalid")
	if err != nil {
		t.Fatal(err)
	}
	defer invalidRes.Body.Close()
	if invalidRes.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid limit status = %d, want 400", invalidRes.StatusCode)
	}
	var invalid struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(invalidRes.Body).Decode(&invalid); err != nil {
		t.Fatal(err)
	}
	if invalid.Error.Code != "invalid_input" {
		t.Fatalf("invalid limit code = %q, want invalid_input", invalid.Error.Code)
	}
}

func TestAPIV1TaskFullIncludesWakeupsAndTaskWakeupsEndpoint(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	client := ts.Client()
	agent := postJSON(t, client, ts.URL+"/api/agents", map[string]any{"name": "API Wakeup Agent", "type": "noop"})
	task := postJSON(t, client, ts.URL+"/api/v1/tasks", map[string]any{"agent_id": agent["id"], "title": "Wakeup visibility", "prompt": "Expose wakeups"})
	taskID := task["data"].(map[string]any)["id"].(string)
	_ = postJSON(t, client, ts.URL+"/agent-api/tasks/"+taskID+"/comments", map[string]any{"from_id": agent["id"], "role": "user", "body": "Please wake this task."})

	fullRes, err := client.Get(ts.URL + "/api/v1/tasks/" + taskID + "/full")
	if err != nil {
		t.Fatal(err)
	}
	defer fullRes.Body.Close()
	if fullRes.StatusCode != http.StatusOK {
		t.Fatalf("full task status = %d", fullRes.StatusCode)
	}
	var full struct {
		Data struct {
			Wakeups []struct {
				TaskID string `json:"task_id"`
				Reason string `json:"reason"`
				Status string `json:"status"`
			} `json:"wakeups"`
		} `json:"data"`
	}
	if err := json.NewDecoder(fullRes.Body).Decode(&full); err != nil {
		t.Fatal(err)
	}
	if len(full.Data.Wakeups) < 2 {
		t.Fatalf("expected assignment and comment wakeups in full task response, got %#v", full.Data.Wakeups)
	}

	wakeupsRes, err := client.Get(ts.URL + "/api/v1/tasks/" + taskID + "/wakeups")
	if err != nil {
		t.Fatal(err)
	}
	defer wakeupsRes.Body.Close()
	if wakeupsRes.StatusCode != http.StatusOK {
		t.Fatalf("wakeups status = %d", wakeupsRes.StatusCode)
	}
	var wakeups struct {
		Data []struct {
			TaskID string `json:"task_id"`
			Reason string `json:"reason"`
		} `json:"data"`
		Meta map[string]any `json:"meta"`
	}
	if err := json.NewDecoder(wakeupsRes.Body).Decode(&wakeups); err != nil {
		t.Fatal(err)
	}
	if len(wakeups.Data) != len(full.Data.Wakeups) {
		t.Fatalf("wakeups endpoint count=%d full count=%d", len(wakeups.Data), len(full.Data.Wakeups))
	}
	for _, wakeup := range wakeups.Data {
		if wakeup.TaskID != taskID {
			t.Fatalf("unexpected wakeup task id: %#v", wakeup)
		}
	}
}

func TestAgentInboxLiteAndHeartbeatContext(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	client := ts.Client()
	agent := postJSON(t, client, ts.URL+"/api/agents", map[string]any{"name": "Inbox Agent", "type": "noop"})
	task := postJSON(t, client, ts.URL+"/agent-api/tasks", map[string]any{"from_agent_id": agent["id"], "assignee_agent_id": agent["id"], "title": "Inbox task", "prompt": "Do inbox work"})
	_ = postJSON(t, client, ts.URL+"/agent-api/tasks/"+task["id"].(string)+"/comments", map[string]any{"from_id": agent["id"], "role": "user", "body": "Latest user comment"})

	inboxRes, err := client.Get(ts.URL + "/agent-api/agents/me/inbox-lite?agent_id=" + agent["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	defer inboxRes.Body.Close()
	if inboxRes.StatusCode != http.StatusOK {
		t.Fatalf("inbox status = %d", inboxRes.StatusCode)
	}
	var inbox struct {
		Inbox []struct {
			TaskID string `json:"task_id"`
			Title  string `json:"title"`
		} `json:"inbox"`
	}
	if err := json.NewDecoder(inboxRes.Body).Decode(&inbox); err != nil {
		t.Fatal(err)
	}
	if len(inbox.Inbox) == 0 || inbox.Inbox[0].TaskID != task["id"].(string) {
		t.Fatalf("unexpected inbox: %+v", inbox)
	}

	ctxRes, err := client.Get(ts.URL + "/agent-api/tasks/" + task["id"].(string) + "/heartbeat-context")
	if err != nil {
		t.Fatal(err)
	}
	defer ctxRes.Body.Close()
	if ctxRes.StatusCode != http.StatusOK {
		t.Fatalf("heartbeat context status = %d", ctxRes.StatusCode)
	}
	var hb map[string]any
	if err := json.NewDecoder(ctxRes.Body).Decode(&hb); err != nil {
		t.Fatal(err)
	}
	if hb["last_user_comment"] != "Latest user comment" {
		t.Fatalf("last_user_comment = %v", hb["last_user_comment"])
	}
	if hb["wake_reason"] != "comment" {
		t.Fatalf("wake_reason = %v", hb["wake_reason"])
	}
	if _, ok := hb["skills"].([]any); !ok {
		t.Fatalf("skills missing from heartbeat context: %+v", hb)
	}
	executionState, ok := hb["execution_state"].(map[string]any)
	if !ok {
		t.Fatalf("execution_state missing from heartbeat context: %+v", hb)
	}
	if executionState["needs_run"] != true {
		t.Fatalf("execution_state needs_run = %v, want true", executionState["needs_run"])
	}
	counts, ok := executionState["counts"].(map[string]any)
	if !ok || counts["pending_wakeups"] == nil || counts["recent_events"] == nil {
		t.Fatalf("execution_state counts should expose compact wakeup/event counts: %+v", executionState)
	}
	pendingWakeups, ok := executionState["pending_wakeups"].([]any)
	if !ok || len(pendingWakeups) == 0 {
		t.Fatalf("execution_state should expose pending wakeups compactly: %+v", executionState)
	}
	recentEvents, ok := executionState["recent_events"].([]any)
	if !ok || len(recentEvents) == 0 {
		t.Fatalf("execution_state should expose recent events compactly: %+v", executionState)
	}
	if _, duplicatesPrompt := executionState["prompt"]; duplicatesPrompt {
		t.Fatalf("execution_state should stay compact and not duplicate prompt: %+v", executionState)
	}

	leanCtxRes, err := client.Get(ts.URL + "/agent-api/tasks/" + task["id"].(string) + "/heartbeat-context?include=execution_state")
	if err != nil {
		t.Fatal(err)
	}
	defer leanCtxRes.Body.Close()
	if leanCtxRes.StatusCode != http.StatusOK {
		t.Fatalf("lean heartbeat context status = %d", leanCtxRes.StatusCode)
	}
	var lean map[string]any
	if err := json.NewDecoder(leanCtxRes.Body).Decode(&lean); err != nil {
		t.Fatal(err)
	}
	if _, ok := lean["execution_state"]; !ok {
		t.Fatalf("lean heartbeat context should include requested execution_state: %+v", lean)
	}
	if _, ok := lean["skills"]; ok {
		t.Fatalf("lean heartbeat context should omit unrequested skills: %+v", lean)
	}
	if _, ok := lean["prompt"]; ok {
		t.Fatalf("lean heartbeat context should omit unrequested prompt: %+v", lean)
	}
	meta, ok := lean["meta"].(map[string]any)
	if !ok || meta["mode"] != "selective" {
		t.Fatalf("lean heartbeat context should advertise selective mode: %+v", lean)
	}
}

func readHTML(t *testing.T, client *http.Client, url string) string {
	t.Helper()
	res, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(res.Body); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestAgentNewHidesNoopAndShowsRuntimeWarningWhenDebugDisabled(t *testing.T) {
	ts := newTestServerWithDebug(t, false)
	defer ts.Close()

	html := readHTML(t, ts.Client(), ts.URL+"/agents/new")
	if strings.Contains(html, `value="noop"`) {
		t.Fatalf("noop should not be visible when debug is disabled")
	}
	if !strings.Contains(html, "No runtimes installed") {
		t.Fatalf("expected no runtime warning")
	}
	htmlModal := readHTML(t, ts.Client(), ts.URL+"/agents")
	if strings.Contains(htmlModal, `value="noop"`) {
		t.Fatalf("noop should not be visible in modal when debug is disabled")
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

	html := readHTML(t, ts.Client(), ts.URL+"/agents/new")
	if strings.Contains(html, `value="picoclaw"`) {
		t.Fatalf("picoclaw should not be visible when its binary is missing")
	}

	htmlModal := readHTML(t, ts.Client(), ts.URL+"/agents")
	if strings.Contains(htmlModal, `value="picoclaw"`) {
		t.Fatalf("picoclaw should not be visible in modal when its binary is missing")
	}
}

func TestAgentNewHidesDisabledRuntime(t *testing.T) {
	storage := memory.NewStorage()
	state := domain.RuntimeState{ID: "runtime_picoclaw", RuntimeID: "picoclaw", Mode: domain.InstallModeExisting, Enabled: false, BinPath: "/bin/sh", SettingsJSON: "{}", MetadataJSON: "{}"}
	if err := storage.Runtimes().Save(t.Context(), state); err != nil {
		t.Fatal(err)
	}
	ts := newTestServerWithStorage(t, storage, false)
	defer ts.Close()

	html := readHTML(t, ts.Client(), ts.URL+"/agents/new")
	if strings.Contains(html, `value="picoclaw"`) {
		t.Fatalf("picoclaw should not be visible when it is disabled")
	}

	htmlModal := readHTML(t, ts.Client(), ts.URL+"/agents")
	if strings.Contains(htmlModal, `value="picoclaw"`) {
		t.Fatalf("picoclaw should not be visible in modal when it is disabled")
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

func TestRunDetailUsesSSEDrivenPartialRefresh(t *testing.T) {
	storage := memory.NewStorage()
	started := time.Now().Add(-1 * time.Minute)
	agent := domain.Agent{ID: "agent_run_live", Name: "Run UI Agent", Type: "noop", CreatedAt: started, UpdatedAt: started}
	task := domain.Task{ID: "task_run_live", AgentID: agent.ID, Title: "Run live task", Prompt: "Stream run output", Status: domain.TaskStatusInProgress, CheckoutRunID: "run_live", CreatedAt: started, UpdatedAt: started}
	run := domain.Run{ID: "run_live", TaskID: task.ID, AgentID: agent.ID, Status: domain.RunStatusRunning, DriverType: "noop", StartedAt: started}
	if err := storage.Agents().Create(t.Context(), agent); err != nil {
		t.Fatal(err)
	}
	if err := storage.Tasks().Create(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	if err := storage.Runs().Create(t.Context(), run); err != nil {
		t.Fatal(err)
	}
	ts := newTestServerWithStorage(t, storage, true)
	defer ts.Close()

	client := ts.Client()
	res, err := client.Get(ts.URL + "/runs/" + run.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(res.Body); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if strings.Contains(html, `hx-trigger="every`) || strings.Contains(html, `<section class="detail-grid" hx-`) {
		t.Fatalf("run detail should not poll or refresh a full page region")
	}
	if !strings.Contains(html, `id="run-live"`) || !strings.Contains(html, `/partials/runs/`+run.ID) {
		t.Fatalf("run detail should keep a small run-live partial target")
	}
	if !strings.Contains(html, `data-run-id="`+run.ID+`"`) || !strings.Contains(html, `data-run-live-url="/partials/runs/`+run.ID+`"`) {
		t.Fatalf("run detail should expose data attributes for scoped live refresh")
	}
	if !strings.Contains(html, `new EventSource('/sse/runs/' + runId + '/logs')`) {
		t.Fatalf("run detail should subscribe to run-scoped SSE")
	}
	if !strings.Contains(html, `source.addEventListener('error', scheduleFallback)`) || !strings.Contains(html, `window.addEventListener('pagehide', stopRunLive`) {
		t.Fatalf("run detail SSE should degrade gracefully and clean up connections")
	}

	partialRes, err := client.Get(ts.URL + "/partials/runs/" + run.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer partialRes.Body.Close()
	partial := new(bytes.Buffer)
	if _, err := partial.ReadFrom(partialRes.Body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(partial.String(), "Console Output") || strings.Contains(partial.String(), "<html") {
		t.Fatalf("run partial should render live fragment only")
	}

	sseReq, err := http.NewRequestWithContext(t.Context(), http.MethodGet, ts.URL+"/sse/runs/"+run.ID+"/logs", nil)
	if err != nil {
		t.Fatal(err)
	}
	sseRes, err := client.Do(sseReq)
	if err != nil {
		t.Fatal(err)
	}
	defer sseRes.Body.Close()
	if sseRes.StatusCode != http.StatusOK {
		t.Fatalf("run SSE status = %d, want 200", sseRes.StatusCode)
	}
	if got := sseRes.Header.Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("run SSE content-type = %q, want text/event-stream", got)
	}
}

func TestTaskDetailUsesSSEDrivenPartialRefresh(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	client := ts.Client()
	agent := postJSON(t, client, ts.URL+"/api/agents", map[string]any{"name": "UI Agent", "type": "noop"})
	task := postJSON(t, client, ts.URL+"/agent-api/tasks", map[string]any{"from_agent_id": agent["id"], "assignee_agent_id": agent["id"], "prompt": "UI detail test"})
	taskID := task["id"].(string)

	res, err := client.Get(ts.URL + "/tasks/" + taskID)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(res.Body); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if strings.Contains(html, `hx-trigger="every`) || strings.Contains(html, `hx-get="/tasks/`) || strings.Contains(html, `<section class="detail-grid" hx-`) {
		t.Fatalf("task detail should not poll or refresh a full page region")
	}
	if !strings.Contains(html, `id="task-live"`) || !strings.Contains(html, `/partials/tasks/`+taskID) {
		t.Fatalf("task detail should keep a small task-live partial target")
	}
	if !strings.Contains(html, `data-task-id="`+taskID+`"`) || !strings.Contains(html, `new EventSource('/sse/tasks/' + taskID)`) {
		t.Fatalf("task detail should subscribe to task-scoped SSE")
	}
	if !strings.Contains(html, `source.addEventListener('error', scheduleFallback)`) || !strings.Contains(html, `window.addEventListener('pagehide', stopTaskLive`) {
		t.Fatalf("task detail SSE should degrade gracefully and clean up connections")
	}

	partialRes, err := client.Get(ts.URL + "/partials/tasks/" + taskID)
	if err != nil {
		t.Fatal(err)
	}
	defer partialRes.Body.Close()
	partial := new(bytes.Buffer)
	if _, err := partial.ReadFrom(partialRes.Body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(partial.String(), "Thread") || strings.Contains(partial.String(), "<html") {
		t.Fatalf("partial should render live fragment only")
	}

	sseReq, err := http.NewRequestWithContext(t.Context(), http.MethodGet, ts.URL+"/sse/tasks/"+taskID, nil)
	if err != nil {
		t.Fatal(err)
	}
	sseRes, err := client.Do(sseReq)
	if err != nil {
		t.Fatal(err)
	}
	defer sseRes.Body.Close()
	if sseRes.StatusCode != http.StatusOK {
		t.Fatalf("task SSE status = %d, want 200", sseRes.StatusCode)
	}
	if got := sseRes.Header.Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("task SSE content-type = %q, want text/event-stream", got)
	}
}
