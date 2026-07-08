package web

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
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

func postForm(t *testing.T, client *http.Client, url string, body io.Reader) *http.Response {
	t.Helper()
	if body == nil {
		body = strings.NewReader("")
	}
	res, err := client.Post(url, "application/x-www-form-urlencoded", body)
	if err != nil {
		t.Fatalf("post form %s: %v", url, err)
	}
	return res
}

func readBodyString(t *testing.T, body io.Reader) string {
	t.Helper()
	b, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(b)
}

func TestAgentDocsAdvertisesPaperclipLikeWorkflow(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	res, err := ts.Client().Get(ts.URL + "/agent-api/docs")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("agent docs status = %d, want 200", res.StatusCode)
	}

	var docs struct {
		Endpoints []struct {
			Method      string `json:"method"`
			Path        string `json:"path"`
			Description string `json:"description"`
		} `json:"endpoints"`
		RecommendedFlow []string `json:"recommended_flow"`
	}
	if err := json.NewDecoder(res.Body).Decode(&docs); err != nil {
		t.Fatal(err)
	}

	wantPaths := []string{
		"/agent-api/agents/me/inbox-lite?agent_id=...",
		"/agent-api/tasks/{id}/heartbeat-context?include=execution_state,skills,apis",
		"/agent-api/issues/{id}/heartbeat-context?include=execution_state,skills,apis",
		"/agent-api/tasks/{id}/next-action",
		"/agent-api/issues/{id}/next-action",
		"/agent-api/issues/{id}/checkout",
		"/agent-api/issues/{id}/comments",
		"/agent-api/issues/{id}/release",
	}
	for _, want := range wantPaths {
		found := false
		for _, endpoint := range docs.Endpoints {
			if endpoint.Path == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("agent docs missing endpoint path %q in %#v", want, docs.Endpoints)
		}
	}

	flow := strings.Join(docs.RecommendedFlow, " -> ")
	for _, want := range []string{"inbox-lite", "next-action", "checkout", "heartbeat-context", "comment", "status", "release"} {
		if !strings.Contains(flow, want) {
			t.Fatalf("recommended flow %q missing %q", flow, want)
		}
	}
}

func TestAgentNextActionRecommendsCheckoutForRunnableTask(t *testing.T) {
	storage := memory.NewStorage()
	now := time.Now().UTC()
	agent := domain.Agent{ID: "agent_next_action", Name: "Next Action Agent", Type: "noop", Enabled: true, Capability: domain.CapabilityWorker, CreatedAt: now, UpdatedAt: now}
	task := domain.Task{ID: "task_next_action", AgentID: agent.ID, Title: "Decide what to do", Prompt: "Decide what to do", Status: domain.TaskStatusTodo, NeedsRun: true, CreatedAt: now, UpdatedAt: now}
	if err := storage.Agents().Create(t.Context(), agent); err != nil {
		t.Fatal(err)
	}
	if err := storage.Tasks().Create(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	ts := newTestServerWithStorage(t, storage, true)
	defer ts.Close()

	client := ts.Client()

	res, err := client.Get(ts.URL + "/agent-api/tasks/" + task.ID + "/next-action")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("next-action status = %d, want 200", res.StatusCode)
	}

	var got struct {
		TaskID      string   `json:"task_id"`
		Action      string   `json:"action"`
		Reason      string   `json:"reason"`
		Risks       []string `json:"risks"`
		UsefulLinks []string `json:"useful_links"`
	}
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.TaskID != task.ID {
		t.Fatalf("task_id = %q, want %q", got.TaskID, task.ID)
	}
	if got.Action != "checkout" {
		t.Fatalf("action = %q, want checkout", got.Action)
	}
	if !strings.Contains(got.Reason, "runnable") {
		t.Fatalf("reason = %q, want runnable explanation", got.Reason)
	}
	if len(got.UsefulLinks) == 0 || !strings.Contains(strings.Join(got.UsefulLinks, " "), "/checkout") {
		t.Fatalf("useful links should include checkout endpoint, got %#v", got.UsefulLinks)
	}
	if len(got.Risks) != 0 {
		t.Fatalf("runnable task should not report risks, got %#v", got.Risks)
	}
}

func TestAgentNextActionReportsOperationalBlockers(t *testing.T) {
	cases := []struct {
		name       string
		task       domain.Task
		runs       []domain.Run
		wakeups    []domain.WakeupRequest
		wantAction string
		wantRisk   string
	}{
		{
			name:       "active checkout waits",
			task:       domain.Task{ID: "task_locked", Status: domain.TaskStatusInProgress, NeedsRun: false, CheckoutRunID: "run_locked", CheckedOutByAgentID: "agent_next_action"},
			wantAction: "wait",
			wantRisk:   "active_checkout",
		},
		{
			name:       "max attempts blocks",
			task:       domain.Task{ID: "task_max_attempts", Status: domain.TaskStatusTodo, NeedsRun: true, Attempts: 2, MaxAttempts: 2},
			wantAction: "block",
			wantRisk:   "max_attempts_reached",
		},
		{
			name:       "future retry asks inspection",
			task:       domain.Task{ID: "task_retry", Status: domain.TaskStatusWaitingNextCycle, NeedsRun: false},
			wakeups:    []domain.WakeupRequest{{ID: "wakeup_retry", TaskID: "task_retry", AgentID: "agent_next_action", Reason: domain.WakeupReasonRetry, Status: domain.WakeupStatusPending, DueAt: time.Now().UTC().Add(time.Hour)}},
			wantAction: "inspect_retry",
			wantRisk:   "pending_wakeup",
		},
		{
			name:       "runtime unavailable asks human",
			task:       domain.Task{ID: "task_runtime", Status: domain.TaskStatusTodo, NeedsRun: true},
			wantAction: "ask_human",
			wantRisk:   "runtime_unavailable",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			storage := memory.NewStorage()
			now := time.Now().UTC()
			agentType := domain.AgentType("noop")
			if tc.wantRisk == "runtime_unavailable" {
				agentType = domain.AgentType("missing-runtime")
			}
			agent := domain.Agent{ID: "agent_next_action", Name: "Next Action Agent", Type: agentType, Enabled: true, Capability: domain.CapabilityWorker, CreatedAt: now, UpdatedAt: now}
			task := tc.task
			task.AgentID = agent.ID
			task.Title = tc.name
			task.Prompt = tc.name
			task.CreatedAt = now
			task.UpdatedAt = now
			if err := storage.Agents().Create(t.Context(), agent); err != nil {
				t.Fatal(err)
			}
			if err := storage.Tasks().Create(t.Context(), task); err != nil {
				t.Fatal(err)
			}
			for _, run := range tc.runs {
				if err := storage.Runs().Create(t.Context(), run); err != nil {
					t.Fatal(err)
				}
			}
			for _, wakeup := range tc.wakeups {
				if err := storage.Wakeups().Create(t.Context(), wakeup); err != nil {
					t.Fatal(err)
				}
			}
			ts := newTestServerWithStorage(t, storage, true)
			defer ts.Close()

			res, err := ts.Client().Get(ts.URL + "/agent-api/tasks/" + task.ID + "/next-action")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			var got struct {
				Action string   `json:"action"`
				Risks  []string `json:"risks"`
			}
			if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			if got.Action != tc.wantAction {
				t.Fatalf("action = %q, want %q", got.Action, tc.wantAction)
			}
			if !containsString(got.Risks, tc.wantRisk) {
				t.Fatalf("risks = %#v, want %q", got.Risks, tc.wantRisk)
			}
		})
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestAgentNextActionRecommendsReleaseForOwnActiveCheckout(t *testing.T) {
	storage := memory.NewStorage()
	now := time.Now().UTC()
	agent := domain.Agent{ID: "agent_next_action", Name: "Next Action Agent", Type: "noop", Enabled: true, Capability: domain.CapabilityWorker, CreatedAt: now, UpdatedAt: now}
	task := domain.Task{ID: "task_active_checkout", AgentID: agent.ID, Title: "Active checkout", Prompt: "Active checkout", Status: domain.TaskStatusInProgress, CheckoutRunID: "run_active", CheckedOutByAgentID: agent.ID, CreatedAt: now, UpdatedAt: now}
	if err := storage.Agents().Create(t.Context(), agent); err != nil {
		t.Fatal(err)
	}
	if err := storage.Tasks().Create(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	ts := newTestServerWithStorage(t, storage, true)
	defer ts.Close()

	res, err := ts.Client().Get(ts.URL + "/agent-api/tasks/" + task.ID + "/next-action?agent_id=" + agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var got struct {
		Action string   `json:"action"`
		Risks  []string `json:"risks"`
	}
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Action != "release" {
		t.Fatalf("action = %q, want release", got.Action)
	}
	if !containsString(got.Risks, "own_active_checkout") {
		t.Fatalf("risks = %#v, want own_active_checkout", got.Risks)
	}
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
	assertAgentAPIError(t, res, "forbidden", "permission tasks.run required")

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

func TestAgentAPICriticalTaskOperationsReturnStructuredErrors(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	client := ts.Client()
	agent := postJSON(t, client, ts.URL+"/api/agents", map[string]any{"name": "Operator", "type": "noop", "capability": "operator"})
	task := postJSON(t, client, ts.URL+"/api/tasks", map[string]any{"agent_id": agent["id"], "title": "Structured errors", "prompt": "Do"})
	taskID := task["id"].(string)
	agentID := agent["id"].(string)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		status int
		code   string
		hint   string
	}{
		{name: "checkout invalid json", method: http.MethodPost, path: "/agent-api/tasks/" + taskID + "/checkout", body: `{`, status: http.StatusBadRequest, code: "invalid_input", hint: "Fix request JSON and retry."},
		{name: "release missing agent", method: http.MethodPost, path: "/agent-api/tasks/" + taskID + "/release", body: `{}`, status: http.StatusBadRequest, code: "invalid_input", hint: "Provide a valid agent_id with the required permission."},
		{name: "patch invalid status", method: http.MethodPatch, path: "/agent-api/tasks/" + taskID, body: `{"agent_id":"` + agentID + `","status":"not-a-status"}`, status: http.StatusBadRequest, code: "invalid_input", hint: "Check the task state, payload, and allowed transition before retrying."},
		{name: "wake missing task", method: http.MethodPost, path: "/agent-api/issues/missing/wake", body: `{"agent_id":"` + agentID + `"}`, status: http.StatusNotFound, code: "not_found", hint: "Verify the resource id before retrying."},
		{name: "delegate missing assignee", method: http.MethodPost, path: "/agent-api/tasks/" + taskID + "/delegate", body: `{"from_agent_id":"` + agentID + `","prompt":"delegate"}`, status: http.StatusBadRequest, code: "invalid_input", hint: "Provide a valid agent_id with the required permission."},
		{name: "cancel invalid json", method: http.MethodPost, path: "/agent-api/tasks/" + taskID + "/cancel", body: `{`, status: http.StatusBadRequest, code: "invalid_input", hint: "Fix request JSON and retry."},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, ts.URL+tc.path, strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")
			res, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if res.StatusCode != tc.status {
				t.Fatalf("status = %d, want %d", res.StatusCode, tc.status)
			}
			assertAgentAPIError(t, res, tc.code, tc.hint)
		})
	}
}

func assertAgentAPIError(t *testing.T, res *http.Response, wantCode string, wantHint string) {
	t.Helper()
	if got := res.Header.Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	var decoded struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Hint    string `json:"hint"`
		} `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode structured error: %v", err)
	}
	if decoded.Error.Code != wantCode {
		t.Fatalf("error.code = %q, want %q (decoded=%+v)", decoded.Error.Code, wantCode, decoded.Error)
	}
	if decoded.Error.Message == "" {
		t.Fatalf("error.message should not be empty: %+v", decoded.Error)
	}
	if decoded.Error.Hint != wantHint {
		t.Fatalf("error.hint = %q, want %q", decoded.Error.Hint, wantHint)
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

func TestAPIV1UsageLedgerFiltersByRunTaskAndAgent(t *testing.T) {
	storage := memory.NewStorage()
	now := time.Now().UTC()
	if err := storage.Usage().Create(t.Context(), domain.UsageEvent{ID: "usage_run_a", RunID: "run_a", TaskID: "task_a", AgentID: "agent_a", Provider: "noop", InputTokens: 12, OutputTokens: 8, CreatedAt: now}); err != nil {
		t.Fatalf("create usage a: %v", err)
	}
	if err := storage.Usage().Create(t.Context(), domain.UsageEvent{ID: "usage_run_b", RunID: "run_b", TaskID: "task_b", AgentID: "agent_b", Provider: "noop", InputTokens: 5, OutputTokens: 3, CreatedAt: now.Add(time.Second)}); err != nil {
		t.Fatalf("create usage b: %v", err)
	}
	ts := newTestServerWithStorage(t, storage, true)
	defer ts.Close()

	res, err := ts.Client().Get(ts.URL + "/api/v1/usage?run_id=run_a&task_id=task_a&agent_id=agent_a")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("usage status = %d, want 200", res.StatusCode)
	}
	var decoded struct {
		Data []struct {
			RunID        string `json:"run_id"`
			TaskID       string `json:"task_id"`
			AgentID      string `json:"agent_id"`
			InputTokens  int    `json:"input_tokens"`
			OutputTokens int    `json:"output_tokens"`
		} `json:"data"`
		Meta map[string]any `json:"meta"`
	}
	if err := json.NewDecoder(res.Body).Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Data) != 1 {
		t.Fatalf("usage count = %d, want 1: %#v", len(decoded.Data), decoded.Data)
	}
	if decoded.Data[0].RunID != "run_a" || decoded.Data[0].TaskID != "task_a" || decoded.Data[0].AgentID != "agent_a" {
		t.Fatalf("unexpected usage event: %#v", decoded.Data[0])
	}
	if decoded.Meta["input_tokens"] != float64(12) || decoded.Meta["output_tokens"] != float64(8) || decoded.Meta["cost_micros"] != float64(0) {
		t.Fatalf("unexpected usage totals: %#v", decoded.Meta)
	}
}

func TestAPIV1DiagnosticsRecoveryLivenessReturnsCompactSnapshot(t *testing.T) {
	storage := memory.NewStorage()
	now := time.Now().UTC()
	expiredAt := now.Add(-time.Minute)
	future := now.Add(10 * time.Minute)
	if err := storage.Tasks().Create(t.Context(), domain.Task{
		ID:                  "task_expired_lock",
		AgentID:             "agent_1",
		Title:               "Expired lock",
		Status:              domain.TaskStatusInProgress,
		NeedsRun:            false,
		CheckoutRunID:       "run_expired_lock",
		CheckedOutByAgentID: "agent_1",
		LockExpiresAt:       &expiredAt,
		CreatedAt:           now,
		UpdatedAt:           now,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := storage.Runs().Create(t.Context(), domain.Run{ID: "run_running", TaskID: "task_expired_lock", AgentID: "agent_1", Status: domain.RunStatusRunning, StartedAt: now.Add(-5 * time.Minute), LastOutputAt: &expiredAt, StallTimeout: 30}); err != nil {
		t.Fatalf("create running run: %v", err)
	}
	if err := storage.Tasks().Create(t.Context(), domain.Task{ID: "task_timeout", AgentID: "agent_1", Title: "Timeout", Status: domain.TaskStatusTodo, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create timeout task: %v", err)
	}
	if err := storage.Runs().Create(t.Context(), domain.Run{ID: "run_timeout", TaskID: "task_timeout", AgentID: "agent_1", Status: domain.RunStatusTimeout, StartedAt: now.Add(-6 * time.Minute), FinishedAt: &now}); err != nil {
		t.Fatalf("create timeout run: %v", err)
	}
	if err := storage.Tasks().Create(t.Context(), domain.Task{ID: "task_retry", AgentID: "agent_1", Title: "Retry", Status: domain.TaskStatusTodo, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create retry task: %v", err)
	}
	for _, event := range []domain.Event{
		{ID: "evt_stalled", Type: domain.EventRuntimeStalled, TaskID: "task_expired_lock", RunID: "run_running", Message: "runtime stalled", CreatedAt: now.Add(-4 * time.Minute)},
		{ID: "evt_recovered", Type: domain.EventRunRecovered, TaskID: "task_recovered", RunID: "run_recovered", Message: "run recovered", CreatedAt: now.Add(-3 * time.Minute)},
		{ID: "evt_retry", Type: domain.EventRetryScheduled, TaskID: "task_retry", Message: "retry scheduled", CreatedAt: now.Add(-2 * time.Minute)},
	} {
		if err := storage.Events().Create(t.Context(), event); err != nil {
			t.Fatalf("create event %s: %v", event.ID, err)
		}
	}
	if err := storage.Wakeups().Create(t.Context(), domain.WakeupRequest{ID: "wakeup_retry", AgentID: "agent_1", TaskID: "task_retry", Reason: domain.WakeupReasonRetry, Status: domain.WakeupStatusPending, DueAt: future, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create wakeup: %v", err)
	}

	ts := newTestServerWithStorage(t, storage, true)
	defer ts.Close()

	res, err := ts.Client().Get(ts.URL + "/api/v1/diagnostics/recovery-liveness?limit=2")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var decoded struct {
		Data struct {
			Counts map[string]int `json:"counts"`
			Items  []struct {
				Kind   string `json:"kind"`
				TaskID string `json:"task_id"`
				RunID  string `json:"run_id,omitempty"`
			} `json:"items"`
		} `json:"data"`
		Meta map[string]any `json:"meta"`
	}
	if err := json.NewDecoder(res.Body).Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Data.Counts["runtime_stalled_events"] != 1 || decoded.Data.Counts["run_recovered_events"] != 1 || decoded.Data.Counts["retry_scheduled_events"] != 1 {
		t.Fatalf("event counts = %#v, want stalled/recovered/retry = 1", decoded.Data.Counts)
	}
	if decoded.Data.Counts["timeout_runs"] != 1 || decoded.Data.Counts["pending_retry_wakeups"] != 1 || decoded.Data.Counts["expired_locks"] != 1 {
		t.Fatalf("state counts = %#v, want timeout/retry/expired_lock = 1", decoded.Data.Counts)
	}
	if len(decoded.Data.Items) != 2 {
		t.Fatalf("items len = %d, want limit 2", len(decoded.Data.Items))
	}
	if decoded.Meta["limit"] != float64(2) {
		t.Fatalf("limit meta = %v, want 2", decoded.Meta["limit"])
	}
}

func TestAgentInboxLiteAndHeartbeatContext(t *testing.T) {
	storage := memory.NewStorage()
	ts := newTestServerWithStorage(t, storage, true)
	defer ts.Close()

	client := ts.Client()
	agent := postJSON(t, client, ts.URL+"/api/agents", map[string]any{"name": "Inbox Agent", "type": "noop", "capability": "coordinator"})
	task := postJSON(t, client, ts.URL+"/agent-api/tasks", map[string]any{"from_agent_id": agent["id"], "assignee_agent_id": agent["id"], "title": "Inbox task", "prompt": "Do inbox work"})
	_ = postJSON(t, client, ts.URL+"/agent-api/tasks/"+task["id"].(string)+"/comments", map[string]any{"from_id": agent["id"], "role": "user", "body": "Latest user comment"})
	_ = postJSON(t, client, ts.URL+"/agent-api/tasks/"+task["id"].(string)+"/delegate", map[string]any{"from_agent_id": agent["id"], "to_agent_id": agent["id"], "title": "Open child", "prompt": "Child work"})

	now := time.Now().UTC()
	storedTask, err := storage.Tasks().Get(t.Context(), task["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	storedTask.CheckoutRunID = "run_inbox_failed"
	storedTask.NeedsRun = true
	storedTask.UpdatedAt = now
	if err := storage.Tasks().Update(t.Context(), storedTask); err != nil {
		t.Fatal(err)
	}
	if err := storage.Runs().Create(t.Context(), domain.Run{ID: "run_inbox_failed", TaskID: storedTask.ID, AgentID: agent["id"].(string), Status: domain.RunStatusFailed, StartedAt: now.Add(-2 * time.Minute)}); err != nil {
		t.Fatal(err)
	}

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
			TaskID          string         `json:"task_id"`
			Title           string         `json:"title"`
			Status          string         `json:"status"`
			Reason          string         `json:"reason"`
			Attention       bool           `json:"attention"`
			Severity        string         `json:"severity"`
			LastActivityAt  string         `json:"last_activity_at"`
			NeedsRun        bool           `json:"needs_run"`
			CheckoutRunID   string         `json:"checkout_run_id"`
			Counts          map[string]int `json:"counts"`
			Messages        []any          `json:"messages"`
			Runs            []any          `json:"runs"`
			Wakeups         []any          `json:"wakeups"`
			Children        []any          `json:"children"`
			ExecutionEvents []any          `json:"execution_events"`
		} `json:"inbox"`
	}
	if err := json.NewDecoder(inboxRes.Body).Decode(&inbox); err != nil {
		t.Fatal(err)
	}
	if len(inbox.Inbox) == 0 || inbox.Inbox[0].TaskID != task["id"].(string) {
		t.Fatalf("unexpected inbox: %+v", inbox)
	}
	if inbox.Inbox[0].Reason != "comment" || !inbox.Inbox[0].Attention {
		t.Fatalf("inbox should mark recent comments as attention-worthy, got %+v", inbox.Inbox[0])
	}
	item := inbox.Inbox[0]
	if item.Severity != "high" {
		t.Fatalf("inbox severity = %q, want high for commented task with failed run", item.Severity)
	}
	if item.LastActivityAt == "" {
		t.Fatalf("inbox should expose last_activity_at compactly: %+v", item)
	}
	if !item.NeedsRun || item.CheckoutRunID != "run_inbox_failed" {
		t.Fatalf("inbox should expose execution signals, got needs_run=%v checkout_run_id=%q", item.NeedsRun, item.CheckoutRunID)
	}
	if item.Counts["pending_wakeups"] != 2 || item.Counts["failed_runs"] != 1 || item.Counts["open_children"] != 1 {
		t.Fatalf("inbox counts = %#v, want pending_wakeups=2 and failed_runs/open_children=1", item.Counts)
	}
	if item.Messages != nil || item.Runs != nil || item.Wakeups != nil || item.Children != nil || item.ExecutionEvents != nil {
		t.Fatalf("inbox-lite should not embed heavyweight arrays: %+v", item)
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
	included, ok := meta["included"].([]any)
	if !ok || len(included) != 1 || included[0] != "execution_state" {
		t.Fatalf("lean heartbeat context should report only requested allowed section: %+v", meta)
	}

	mixedCtxRes, err := client.Get(ts.URL + "/agent-api/tasks/" + task["id"].(string) + "/heartbeat-context?include=execution_state,skills")
	if err != nil {
		t.Fatal(err)
	}
	defer mixedCtxRes.Body.Close()
	if mixedCtxRes.StatusCode != http.StatusOK {
		t.Fatalf("mixed heartbeat context status = %d", mixedCtxRes.StatusCode)
	}
	var mixed map[string]any
	if err := json.NewDecoder(mixedCtxRes.Body).Decode(&mixed); err != nil {
		t.Fatal(err)
	}
	if _, ok := mixed["execution_state"]; !ok {
		t.Fatalf("mixed heartbeat context should include requested execution_state: %+v", mixed)
	}
	if _, ok := mixed["skills"]; !ok {
		t.Fatalf("mixed heartbeat context should include requested skills: %+v", mixed)
	}
	if _, ok := mixed["prompt"]; ok {
		t.Fatalf("mixed heartbeat context should omit unrequested prompt: %+v", mixed)
	}
	mixedMeta, ok := mixed["meta"].(map[string]any)
	if !ok {
		t.Fatalf("mixed heartbeat context should include meta: %+v", mixed)
	}
	mixedIncluded, ok := mixedMeta["included"].([]any)
	if !ok || len(mixedIncluded) != 2 {
		t.Fatalf("mixed heartbeat context should report two allowed sections: %+v", mixedMeta)
	}
	for _, want := range []string{"execution_state", "skills"} {
		found := false
		for _, got := range mixedIncluded {
			found = found || got == want
		}
		if !found {
			t.Fatalf("mixed heartbeat context meta.included missing %q: %+v", want, mixedMeta)
		}
	}

	unknownCtxRes, err := client.Get(ts.URL + "/agent-api/tasks/" + task["id"].(string) + "/heartbeat-context?include=execution_state,unknown")
	if err != nil {
		t.Fatal(err)
	}
	defer unknownCtxRes.Body.Close()
	if unknownCtxRes.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown include status = %d, want 400", unknownCtxRes.StatusCode)
	}
	body := readBodyString(t, unknownCtxRes.Body)
	if !strings.Contains(body, "invalid include section") || !strings.Contains(body, "unknown") {
		t.Fatalf("unknown include response should name invalid section, got %q", body)
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

func TestDashboardRendersOperationalAttentionInbox(t *testing.T) {
	storage := memory.NewStorage()
	now := time.Now().UTC()
	if err := storage.Agents().Create(t.Context(), domain.Agent{ID: "agent_attention", Name: "Attention Agent", Type: "noop", Enabled: true, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := storage.Tasks().Create(t.Context(), domain.Task{ID: "task_blocked", AgentID: "agent_attention", Title: "Blocked customer handoff", Prompt: "Needs a human decision", Status: domain.TaskStatusBlocked, CreatedAt: now.Add(-3 * time.Minute), UpdatedAt: now.Add(-2 * time.Minute)}); err != nil {
		t.Fatalf("create blocked task: %v", err)
	}
	if err := storage.Tasks().Create(t.Context(), domain.Task{ID: "task_failed_run", AgentID: "agent_attention", Title: "Retry broken export", Prompt: "Export failed", Status: domain.TaskStatusTodo, NeedsRun: true, CheckoutRunID: "run_failed", CreatedAt: now.Add(-5 * time.Minute), UpdatedAt: now.Add(-4 * time.Minute)}); err != nil {
		t.Fatalf("create failed task: %v", err)
	}
	if err := storage.Runs().Create(t.Context(), domain.Run{ID: "run_failed", TaskID: "task_failed_run", AgentID: "agent_attention", DriverType: "noop", Status: domain.RunStatusFailed, Error: "runtime failed", StartedAt: now.Add(-4 * time.Minute), FinishedAt: &now}); err != nil {
		t.Fatalf("create failed run: %v", err)
	}
	if err := storage.Wakeups().Create(t.Context(), domain.WakeupRequest{ID: "wakeup_retry", AgentID: "agent_attention", TaskID: "task_failed_run", Reason: domain.WakeupReasonRetry, Status: domain.WakeupStatusPending, DueAt: now.Add(time.Minute), CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create wakeup: %v", err)
	}

	ts := newTestServerWithStorage(t, storage, true)
	defer ts.Close()

	html := readHTML(t, ts.Client(), ts.URL+"/")
	for _, want := range []string{
		"Attention inbox",
		"Blocked customer handoff",
		"/tasks/task_blocked",
		"Retry broken export",
		"/runs/run_failed",
		"Pending wakeup: retry",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("dashboard attention inbox missing %q in:\n%s", want, html)
		}
	}
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

func cssDeclarations(css, selector string) map[string]string {
	for offset := 0; offset < len(css); {
		open := strings.Index(css[offset:], "{")
		if open == -1 {
			return nil
		}
		open += offset
		blockSelector := strings.TrimSpace(css[offset:open])

		close := strings.Index(css[open+1:], "}")
		if close == -1 {
			return nil
		}
		close += open + 1

		if blockSelector == selector {
			declarations := make(map[string]string)
			for _, raw := range strings.Split(css[open+1:close], ";") {
				property, value, ok := strings.Cut(raw, ":")
				if !ok {
					continue
				}
				property = strings.TrimSpace(property)
				value = strings.TrimSpace(value)
				if property != "" && value != "" {
					declarations[property] = value
				}
			}
			return declarations
		}

		offset = close + 1
	}
	return nil
}

func requireCSSDeclaration(t *testing.T, css, selector, property, want string) {
	t.Helper()
	declarations := cssDeclarations(css, selector)
	if declarations == nil {
		t.Fatalf("CSS selector %q not found", selector)
	}
	if got := declarations[property]; got != want {
		t.Fatalf("%s %s = %q, want %q", selector, property, got, want)
	}
}

func TestCSSRuleParserReadsDeclarationsWithFlexibleFormatting(t *testing.T) {
	t.Parallel()

	css := `.pc-btn-primary {
		background: linear-gradient(135deg, var(--brand), var(--brand-strong));
		color: var(--brand-ink);
		box-shadow: var(--button-shadow);
	}`

	declarations := cssDeclarations(css, ".pc-btn-primary")
	if declarations["background"] != "linear-gradient(135deg, var(--brand), var(--brand-strong))" {
		t.Fatalf("background declaration = %q", declarations["background"])
	}
	if declarations["box-shadow"] != "var(--button-shadow)" {
		t.Fatalf("box-shadow declaration = %q", declarations["box-shadow"])
	}
}

func TestDesignSystemCSSDefinesPicoClipIdentityTokens(t *testing.T) {
	css, err := os.ReadFile("assets/app.css")
	if err != nil {
		t.Fatal(err)
	}
	text := string(css)
	requireCSSDeclaration(t, text, ":root", "--brand", "#5e6ad2")
	requireCSSDeclaration(t, text, ":root", "--brand-strong", "#4f46e5")
	requireCSSDeclaration(t, text, ":root", "--brand-soft", "color-mix(in srgb, var(--brand) 12%, transparent)")
	requireCSSDeclaration(t, text, ":root", "--surface-gradient", "linear-gradient(180deg, color-mix(in srgb, var(--surface-elevated) 96%, var(--brand-soft)), var(--surface))")
	requireCSSDeclaration(t, text, ":root", "--focus-ring", "0 0 0 3px color-mix(in srgb, var(--brand) 24%, transparent)")
	requireCSSDeclaration(t, text, `[data-theme="dark"]`, "--brand", "#8b8cff")
	requireCSSDeclaration(t, text, `[data-theme="dark"]`, "--surface-raised", "linear-gradient(180deg, #1d1d26, #141419)")
	requireCSSDeclaration(t, text, `[data-theme="dark"]`, "--text-muted", "#b7b7c2")
	requireCSSDeclaration(t, text, "body", "font-feature-settings", `"cv01", "ss03"`)
	requireCSSDeclaration(t, text, ".brand-mark", "background", "linear-gradient(135deg, var(--brand), var(--brand-strong))")
	requireCSSDeclaration(t, text, ".page-title-icon", "background", "var(--surface-raised)")
	requireCSSDeclaration(t, text, ".pc-card", "background", "var(--surface-raised)")
	requireCSSDeclaration(t, text, ".card,\n.panel,\n.metric-card", "background", "var(--surface-gradient)")
	requireCSSDeclaration(t, text, ".brand-mark", "box-shadow", "var(--shadow-md)")
}

func TestDesignSystemCSSDefinesConsistentActionButtons(t *testing.T) {
	css, err := os.ReadFile("assets/app.css")
	if err != nil {
		t.Fatal(err)
	}
	text := string(css)
	requireCSSDeclaration(t, text, ":root", "--button-shadow", "0 12px 28px -20px var(--brand)")
	requireCSSDeclaration(t, text, ":root", "--button-hover-transform", "translateY(-1px)")
	requireCSSDeclaration(t, text, ".button,\nbutton,\n.action-menu summary", "background", "linear-gradient(135deg, var(--brand), var(--brand-strong))")
	requireCSSDeclaration(t, text, ".button,\nbutton,\n.action-menu summary", "box-shadow", "var(--button-shadow)")
	requireCSSDeclaration(t, text, ".button:hover,\nbutton:hover", "transform", "var(--button-hover-transform)")
	requireCSSDeclaration(t, text, ".button.secondary", "background", "var(--surface-gradient)")
	requireCSSDeclaration(t, text, ".pc-btn-primary", "background", "linear-gradient(135deg, var(--brand), var(--brand-strong))")
	requireCSSDeclaration(t, text, ".pc-btn-primary", "color", "var(--brand-ink)")
	requireCSSDeclaration(t, text, ".pc-btn-primary", "box-shadow", "var(--button-shadow)")
	requireCSSDeclaration(t, text, ".pc-btn-secondary", "background", "var(--surface-raised)")
	requireCSSDeclaration(t, text, ".pc-btn-secondary", "color", "var(--text)")
	requireCSSDeclaration(t, text, ".pc-btn-secondary", "border-color", "var(--border)")
	requireCSSDeclaration(t, text, ".pc-icon-btn", "background", "var(--surface-gradient)")
}

func TestDesignSystemCSSDefinesConsistentBadgesAndStatus(t *testing.T) {
	css, err := os.ReadFile("assets/app.css")
	if err != nil {
		t.Fatal(err)
	}
	text := string(css)
	requireCSSDeclaration(t, text, ":root", "--badge-border", "color-mix(in srgb, currentColor 18%, transparent)")
	requireCSSDeclaration(t, text, ":root", "--badge-radius", "999px")
	requireCSSDeclaration(t, text, ":root", "--status-dot-size", "8px")
	requireCSSDeclaration(t, text, ".badge", "border", "1px solid var(--badge-border)")
	requireCSSDeclaration(t, text, ".badge", "border-radius", "var(--badge-radius)")
	requireCSSDeclaration(t, text, ".badge", "letter-spacing", "0.04em")
	requireCSSDeclaration(t, text, ".pc-chip", "border-radius", "var(--badge-radius)")
	requireCSSDeclaration(t, text, ".pc-badge", "border", "1px solid var(--badge-border)")
	requireCSSDeclaration(t, text, ".pc-badge", "border-radius", "var(--badge-radius)")
	requireCSSDeclaration(t, text, ".status::before", "width", "var(--status-dot-size)")
	requireCSSDeclaration(t, text, ".status::before", "height", "var(--status-dot-size)")
	requireCSSDeclaration(t, text, ".status::before", "box-shadow", "0 0 0 3px color-mix(in srgb, currentColor 12%, transparent)")
}

func TestResponsiveShellCSSKeepsMobileNavigationCompact(t *testing.T) {
	css, err := os.ReadFile("assets/app.css")
	if err != nil {
		t.Fatal(err)
	}
	text := string(css)
	mobileBlock := "@media (max-width: 980px) {"
	idx := strings.Index(text, mobileBlock)
	if idx == -1 {
		t.Fatalf("responsive shell media query %q not found", mobileBlock)
	}
	nextMedia := strings.Index(text[idx+len(mobileBlock):], "@media")
	end := len(text)
	if nextMedia >= 0 {
		end = idx + len(mobileBlock) + nextMedia
	}
	mobileCSS := text[idx:end]
	for _, want := range []string{
		".app-shell { grid-template-columns: minmax(0, 1fr); max-width: 100vw; overflow-x: clip; }",
		".sidebar { position: sticky; top: 0; z-index:",
		"max-width: 100vw; min-width: 0;",
		".sidebar nav { display: flex; flex-wrap: nowrap; width: 100%; min-width: 0; max-width: 100vw; overflow-x: auto;",
		".sidebar nav a { flex: 0 0 auto;",
		".main { width: 100%; max-width: 100vw; min-width: 0; padding:",
		".brand-copy small { display: none; }",
	} {
		if !strings.Contains(mobileCSS, want) {
			t.Fatalf("mobile shell CSS missing %q in:\n%s", want, mobileCSS)
		}
	}
}

func TestPageShellExposesAccessibleAsyncFeedback(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	res, err := ts.Client().Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(res.Body); err != nil {
		t.Fatal(err)
	}
	html := buf.String()

	for _, want := range []string{
		`id="toast-root"`,
		`role="status"`,
		`aria-live="polite"`,
		`aria-atomic="true"`,
		`form.setAttribute('aria-busy', 'true')`,
		`form.removeAttribute('aria-busy')`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("page shell async feedback missing %q", want)
		}
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

func TestWebCleanupHistoryKeepsTasksAndTaskDeleteIsAccessible(t *testing.T) {
	storage := memory.NewStorage()
	ts := newTestServerWithStorage(t, storage, true)
	defer ts.Close()

	client := ts.Client()
	agent := postJSON(t, client, ts.URL+"/api/agents", map[string]any{"name": "Cleanup Agent", "type": "noop"})
	keep := postJSON(t, client, ts.URL+"/agent-api/tasks", map[string]any{"assignee_agent_id": agent["id"], "prompt": "keep task"})
	remove := postJSON(t, client, ts.URL+"/agent-api/tasks", map[string]any{"assignee_agent_id": agent["id"], "prompt": "remove task"})
	now := time.Now().UTC()
	for _, run := range []domain.Run{
		{ID: "run_web_done", TaskID: keep["id"].(string), AgentID: agent["id"].(string), Status: domain.RunStatusSucceeded, StartedAt: now},
		{ID: "run_web_running", TaskID: keep["id"].(string), AgentID: agent["id"].(string), Status: domain.RunStatusRunning, StartedAt: now.Add(time.Second)},
	} {
		if err := storage.Runs().Create(t.Context(), run); err != nil {
			t.Fatal(err)
		}
	}
	if err := storage.Events().Create(t.Context(), domain.Event{ID: "evt_web", Type: domain.EventRunCompleted, TaskID: keep["id"].(string), AgentID: agent["id"].(string), RunID: "run_web_done", Message: "done", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}

	tasksPage, err := client.Get(ts.URL + "/tasks")
	if err != nil {
		t.Fatal(err)
	}
	defer tasksPage.Body.Close()
	body := readBodyString(t, tasksPage.Body)
	if !strings.Contains(body, `hx-post="/tasks/`+remove["id"].(string)+`/delete"`) || !strings.Contains(body, "Delete task") {
		t.Fatalf("tasks menu does not expose quick delete action: %s", body)
	}

	res := postForm(t, client, ts.URL+"/runs/history/delete", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("delete runs status = %d", res.StatusCode)
	}
	if _, err := storage.Tasks().Get(t.Context(), keep["id"].(string)); err != nil {
		t.Fatalf("run cleanup removed task: %v", err)
	}
	runs, err := storage.Runs().ListByTask(t.Context(), keep["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != "run_web_running" {
		t.Fatalf("run cleanup should keep only running run, got %#v", runs)
	}

	res = postForm(t, client, ts.URL+"/activity/history/delete", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("delete activity status = %d", res.StatusCode)
	}
	if _, err := storage.Tasks().Get(t.Context(), keep["id"].(string)); err != nil {
		t.Fatalf("activity cleanup removed task: %v", err)
	}
	events, err := storage.Events().ListRecent(t.Context(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("expected empty activity after cleanup, got %#v", events)
	}

	res = postForm(t, client, ts.URL+"/tasks/"+remove["id"].(string)+"/delete", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("delete task status = %d", res.StatusCode)
	}
	if _, err := storage.Tasks().Get(t.Context(), remove["id"].(string)); err != domain.ErrNotFound {
		t.Fatalf("expected deleted task not found, got %v", err)
	}
	if _, err := storage.Tasks().Get(t.Context(), keep["id"].(string)); err != nil {
		t.Fatalf("delete task removed unrelated task: %v", err)
	}
}
