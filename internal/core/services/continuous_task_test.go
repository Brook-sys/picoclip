package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"picoclip/internal/adapters/storage/memory"
	"picoclip/internal/core/domain"
)

func TestRunnerSchedulesNextContinuousCycle(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)}
	idgen := &seqID{}
	agent := domain.Agent{ID: "agent_cont", Name: "agent", Type: "noop", Enabled: true, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Agents().Create(context.Background(), agent); err != nil {
		t.Fatal(err)
	}
	task := domain.Task{ID: "task_cont", AgentID: agent.ID, Title: "continuous", Prompt: "do", Status: domain.TaskStatusTodo, Mode: domain.TaskModeContinuous, LoopDelaySeconds: 60, MaxAttempts: 0, NeedsRun: true, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Tasks().Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(st, clock, idgen, noopBus{}, nil, NoopMemoryProvider{}, testLogger{}, Config{})
	runner.Run(context.Background(), task)

	got, err := st.Tasks().Get(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.TaskStatusWaitingNextCycle || got.NeedsRun {
		t.Fatalf("expected waiting next cycle without needs_run, got status=%s needs=%v", got.Status, got.NeedsRun)
	}
	if got.LoopRunCount != 1 {
		t.Fatalf("expected loop count 1, got %d", got.LoopRunCount)
	}
	if got.LoopNextRunAt == nil || !got.LoopNextRunAt.Equal(clock.t.Add(60*time.Second)) {
		t.Fatalf("expected next run at +60s, got %v", got.LoopNextRunAt)
	}
	if got.CheckoutRunID != "" || got.CheckedOutByAgentID != "" {
		t.Fatalf("expected checkout cleared, got run=%q agent=%q", got.CheckoutRunID, got.CheckedOutByAgentID)
	}
}

func TestRunnerClearsCheckoutWhenContinuousTaskPausedMidRun(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 4, 10, 30, 0, 0, time.UTC)}
	idgen := &seqID{}
	pausedAt := clock.t.Add(-time.Second)
	startedAt := clock.t.Add(-time.Minute)
	expiresAt := clock.t.Add(time.Minute)
	task := domain.Task{ID: "task_paused_mid_run", AgentID: "agent_cont", Title: "continuous", Prompt: "do", Status: domain.TaskStatusInProgress, Mode: domain.TaskModeContinuous, LoopDelaySeconds: 60, MaxAttempts: 0, LoopPausedAt: &pausedAt, CheckoutRunID: "run_paused", CheckedOutByAgentID: "agent_cont", ExecutionLockedAt: &startedAt, LockExpiresAt: &expiresAt, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Tasks().Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(st, clock, idgen, noopBus{}, nil, NoopMemoryProvider{}, testLogger{}, Config{})
	runner.completeContinuousCycle(context.Background(), task, domain.Run{ID: "run_paused"}, clock.t)

	got, err := st.Tasks().Get(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CheckoutRunID != "" || got.CheckedOutByAgentID != "" || got.ExecutionLockedAt != nil || got.LockExpiresAt != nil {
		t.Fatalf("expected paused task checkout cleared, got %#v", got)
	}
	if got.Status != domain.TaskStatusWaitingNextCycle || got.NeedsRun || got.LoopNextRunAt != nil {
		t.Fatalf("expected paused waiting state without next run, got %#v", got)
	}
}

func TestReconcilerActivatesDueContinuousTask(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 4, 11, 0, 0, 0, time.UTC)}
	idgen := &seqID{}
	next := clock.t.Add(-time.Second)
	task := domain.Task{ID: "task_due", AgentID: "agent_due", Title: "continuous", Prompt: "do", Status: domain.TaskStatusWaitingNextCycle, Mode: domain.TaskModeContinuous, LoopDelaySeconds: 60, LoopRunCount: 1, LoopNextRunAt: &next, NeedsRun: false, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Tasks().Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}

	reconciler := NewReconciler(st, clock, noopBus{}, idgen, testLogger{})
	reconciler.Reconcile(context.Background())

	got, err := st.Tasks().Get(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.TaskStatusTodo || !got.NeedsRun {
		t.Fatalf("expected todo/needs_run, got status=%s needs=%v", got.Status, got.NeedsRun)
	}
	if got.LoopNextRunAt != nil {
		t.Fatalf("expected next run cleared, got %v", got.LoopNextRunAt)
	}
}

func TestTaskServiceControlsContinuousTask(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)}
	idgen := &seqID{}
	svc := NewTaskService(st, clock, idgen, noopBus{})
	next := clock.t.Add(time.Minute)
	task := domain.Task{ID: "task_control", AgentID: "agent_control", Title: "continuous", Prompt: "do", Status: domain.TaskStatusWaitingNextCycle, Mode: domain.TaskModeContinuous, LoopDelaySeconds: 60, LoopNextRunAt: &next, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Tasks().Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}

	paused, err := svc.PauseContinuous(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if paused.LoopPausedAt == nil || paused.LoopNextRunAt != nil || paused.NeedsRun {
		t.Fatalf("unexpected paused task: %#v", paused)
	}

	resumed, err := svc.ResumeContinuous(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.LoopPausedAt != nil || resumed.LoopNextRunAt == nil || resumed.NeedsRun {
		t.Fatalf("unexpected resumed task: %#v", resumed)
	}

	resumed, err = svc.Wake(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Status != domain.TaskStatusWaitingNextCycle || resumed.NeedsRun || resumed.LoopNextRunAt == nil {
		t.Fatalf("wake should not bypass continuous delay: %#v", resumed)
	}

	queued, err := svc.RunContinuousNow(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if queued.Status != domain.TaskStatusTodo || !queued.NeedsRun || queued.LoopNextRunAt != nil || queued.LoopPausedAt != nil {
		t.Fatalf("unexpected queued task: %#v", queued)
	}
}

func TestRateLimitBackoffDuration(t *testing.T) {
	cases := []struct {
		failures int
		want     time.Duration
	}{
		{0, 6 * time.Second},
		{1, 6 * time.Second},
		{2, 18 * time.Second},
		{3, 54 * time.Second},
		{4, 162 * time.Second},
		{20, 2 * time.Hour},
	}
	for _, tc := range cases {
		if got := rateLimitBackoffDuration(tc.failures); got != tc.want {
			t.Fatalf("failures %d: got %s, want %s", tc.failures, got, tc.want)
		}
	}
}

func TestTransientProviderErrorBackoff(t *testing.T) {
	if !isTransientProviderError(`{"message":"Internal server error","type":"internal_server_error","code":500}`) {
		t.Fatal("expected provider 500 to be transient")
	}
	cases := []struct {
		failures int
		want     time.Duration
	}{
		{0, 2 * time.Minute},
		{1, 2 * time.Minute},
		{2, 4 * time.Minute},
		{3, 8 * time.Minute},
		{4, 16 * time.Minute},
		{20, time.Hour},
	}
	for _, tc := range cases {
		if got := transientProviderBackoffDuration(tc.failures); got != tc.want {
			t.Fatalf("failures %d: got %s, want %s", tc.failures, got, tc.want)
		}
	}
}

func TestSkillCatalogKeepsFullInstructionsOutOfPrompt(t *testing.T) {
	runner := NewRunner(memory.NewStorage(), fixedClock{t: time.Now()}, &seqID{}, noopBus{}, nil, NoopMemoryProvider{}, testLogger{}, Config{})
	skill := domain.Skill{ID: "skill_large", Name: "Large Skill", Description: "Use for large operations", Instructions: strings.Repeat("very detailed instruction ", 100), Kind: domain.SkillKindBuiltin, Enabled: true}
	catalog := runner.skillCatalogContext([]domain.Skill{skill})
	if !strings.Contains(catalog, "GET /agent-api/skills") || !strings.Contains(catalog, "Large Skill") {
		t.Fatalf("unexpected catalog: %s", catalog)
	}
	if strings.Contains(catalog, "very detailed instruction") {
		t.Fatalf("catalog should not include full skill instructions: %s", catalog)
	}
	full := runner.skillContext(skill)
	if !strings.Contains(full, "very detailed instruction") {
		t.Fatalf("manual skill context should keep instructions: %s", full)
	}
}

func TestContinuousTaskProtocolContextIncludesLoopGuidance(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 4, 13, 0, 0, 0, time.UTC)}
	idgen := &seqID{}
	task := domain.Task{ID: "task_prompt", AgentID: "agent_prompt", Title: "watch", Prompt: "watch", Status: domain.TaskStatusInProgress, Mode: domain.TaskModeContinuous, LoopDelaySeconds: 60, LoopRunCount: 2, CreatedAt: clock.t, UpdatedAt: clock.t}
	child := domain.Task{ID: "task_child", ParentID: task.ID, AgentID: "agent_child", Title: "child work", Prompt: "do child", Status: domain.TaskStatusTodo, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Tasks().Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if err := st.Tasks().Create(context.Background(), child); err != nil {
		t.Fatal(err)
	}
	if err := st.Runs().Create(context.Background(), domain.Run{ID: "run_child", TaskID: child.ID, AgentID: child.AgentID, Status: domain.RunStatusSucceeded, Output: "investigated branch", StartedAt: clock.t}); err != nil {
		t.Fatal(err)
	}
	if err := st.Messages().Create(context.Background(), domain.Message{ID: "msg_child", TaskID: child.ID, Role: domain.MessageRoleAgent, Body: "summary done", CreatedAt: clock.t}); err != nil {
		t.Fatal(err)
	}
	messages := []domain.Message{
		{ID: "msg_question", TaskID: task.ID, Role: domain.MessageRoleAgent, Body: "Pergunta para você: Qual branch devo monitorar? Opções: main, develop, outra. Padrão seguro: main.", CreatedAt: clock.t},
		{ID: "msg_user", TaskID: task.ID, Role: domain.MessageRoleUser, Body: "Use main.", CreatedAt: clock.t.Add(time.Second)},
	}
	runner := NewRunner(st, clock, idgen, noopBus{}, nil, NoopMemoryProvider{}, testLogger{}, Config{})
	contextText := runner.taskProtocolContext(context.Background(), task, domain.Run{ID: "run_prompt"}, messages)
	for _, want := range []string{"Continuous Task Rules", "Cycle 3", "safe assumptions", "Child Tasks To Supervise", "do child", "latest run: succeeded", "investigated branch", "latest message: agent: summary done", "Open Questions Raised For User", "Qual branch devo monitorar?", "Latest User Comment", "Use main."} {
		if !strings.Contains(contextText, want) {
			t.Fatalf("expected context to contain %q:\n%s", want, contextText)
		}
	}
	if !strings.Contains(contextText, "compare the requested work") {
		t.Fatalf("expected supervision guidance in context")
	}
}
