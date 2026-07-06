package services

import (
	"context"
	"errors"
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

func TestStaleLockRecoverySchedulesNextContinuousCycleWithoutImmediateDispatch(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 4, 11, 30, 0, 0, time.UTC)}
	idgen := &seqID{}
	agent := domain.Agent{ID: "agent_recover_cont", Name: "agent", Type: "noop", Enabled: true, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Agents().Create(context.Background(), agent); err != nil {
		t.Fatal(err)
	}
	expired := clock.t.Add(-time.Hour)
	task := domain.Task{
		ID:                  "task_recover_cont",
		AgentID:             agent.ID,
		Title:               "continuous",
		Prompt:              "watch",
		Status:              domain.TaskStatusInProgress,
		Mode:                domain.TaskModeContinuous,
		LoopDelaySeconds:    90,
		LoopRunCount:        2,
		NeedsRun:            false,
		CheckoutRunID:       "run_recover_cont",
		CheckedOutByAgentID: agent.ID,
		ExecutionLockedAt:   &expired,
		LockExpiresAt:       &expired,
		CreatedAt:           clock.t,
		UpdatedAt:           clock.t,
	}
	if err := st.Tasks().Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if err := st.Runs().Create(context.Background(), domain.Run{ID: "run_recover_cont", TaskID: task.ID, AgentID: agent.ID, Status: domain.RunStatusRunning, StartedAt: expired}); err != nil {
		t.Fatal(err)
	}

	recovered, err := NewLockRecoveryService(st, clock, noopBus{}, idgen).SweepStaleLocks(context.Background())
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if recovered != 1 {
		t.Fatalf("recovered=%d want 1", recovered)
	}

	got, err := st.Tasks().Get(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CheckoutRunID != "" || got.CheckedOutByAgentID != "" || got.ExecutionLockedAt != nil || got.LockExpiresAt != nil {
		t.Fatalf("expected checkout cleared, got %#v", got)
	}
	if got.Status != domain.TaskStatusWaitingNextCycle || got.NeedsRun {
		t.Fatalf("expected waiting next cycle without immediate run, got status=%s needs=%v", got.Status, got.NeedsRun)
	}
	wantNext := clock.t.Add(90 * time.Second)
	if got.LoopNextRunAt == nil || !got.LoopNextRunAt.Equal(wantNext) {
		t.Fatalf("next run at=%v want %v", got.LoopNextRunAt, wantNext)
	}
	if got.LoopRunCount != 3 {
		t.Fatalf("loop count=%d want 3", got.LoopRunCount)
	}

	claimed, _, err := st.Tasks().ClaimNextRunnable(context.Background(), clock.t, 30*time.Minute)
	if !errors.Is(err, domain.ErrNoPendingTasks) {
		t.Fatalf("expected no immediate dispatch after recovery, claimed=%#v err=%v", claimed, err)
	}

	wakeups, _ := st.Wakeups().ListPending(context.Background(), clock.t.Add(time.Hour), 10)
	if len(wakeups) != 0 {
		t.Fatalf("expected continuous recovery to rely on loop schedule, got wakeups=%#v", wakeups)
	}

	dueClock := fixedClock{t: wantNext}
	NewReconciler(st, dueClock, noopBus{}, idgen, testLogger{}).Reconcile(context.Background())
	claimed, _, err = st.Tasks().ClaimNextRunnable(context.Background(), wantNext, 30*time.Minute)
	if err != nil {
		t.Fatalf("expected claimable after next cycle due: %v", err)
	}
	if claimed.ID != task.ID {
		t.Fatalf("claimed task=%s want %s", claimed.ID, task.ID)
	}
}

func TestStaleLockRecoveryKeepsPausedContinuousTaskPaused(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 4, 11, 45, 0, 0, time.UTC)}
	idgen := &seqID{}
	pausedAt := clock.t.Add(-10 * time.Minute)
	expired := clock.t.Add(-time.Hour)
	task := domain.Task{
		ID:                  "task_paused_recover",
		AgentID:             "agent_paused",
		Title:               "continuous paused",
		Prompt:              "watch",
		Status:              domain.TaskStatusInProgress,
		Mode:                domain.TaskModeContinuous,
		LoopDelaySeconds:    90,
		LoopRunCount:        4,
		LoopPausedAt:        &pausedAt,
		CheckoutRunID:       "run_paused_recover",
		CheckedOutByAgentID: "agent_paused",
		ExecutionLockedAt:   &expired,
		LockExpiresAt:       &expired,
		CreatedAt:           clock.t,
		UpdatedAt:           clock.t,
	}
	if err := st.Tasks().Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if err := st.Runs().Create(context.Background(), domain.Run{ID: "run_paused_recover", TaskID: task.ID, AgentID: task.AgentID, Status: domain.RunStatusRunning, StartedAt: expired}); err != nil {
		t.Fatal(err)
	}

	_, err := NewLockRecoveryService(st, clock, noopBus{}, idgen).SweepStaleLocks(context.Background())
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	got, err := st.Tasks().Get(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.TaskStatusWaitingNextCycle || got.NeedsRun || got.LoopNextRunAt != nil {
		t.Fatalf("expected paused continuous task to stay waiting without next run, got %#v", got)
	}
	if got.LoopPausedAt == nil || !got.LoopPausedAt.Equal(pausedAt) {
		t.Fatalf("pause timestamp changed: %v want %v", got.LoopPausedAt, pausedAt)
	}
	if got.LoopRunCount != 4 {
		t.Fatalf("loop count=%d want unchanged 4", got.LoopRunCount)
	}
	wakeups, _ := st.Wakeups().ListPending(context.Background(), clock.t.Add(time.Hour), 10)
	if len(wakeups) != 0 {
		t.Fatalf("expected paused continuous recovery to avoid wakeups, got %#v", wakeups)
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

	queued, err := svc.RunContinuousNow(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if queued.Status != domain.TaskStatusTodo || !queued.NeedsRun || queued.LoopNextRunAt != nil || queued.LoopPausedAt != nil {
		t.Fatalf("unexpected queued task: %#v", queued)
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
		{ID: "msg_question", TaskID: task.ID, Role: domain.MessageRoleAgent, Body: "Which branch should I monitor?", CreatedAt: clock.t},
		{ID: "msg_user", TaskID: task.ID, Role: domain.MessageRoleUser, Body: "Use main.", CreatedAt: clock.t.Add(time.Second)},
	}
	_ = idgen
	contextText := NewPromptBuilder(st).taskProtocolContext(context.Background(), task, domain.Run{ID: "run_prompt"}, messages)
	for _, want := range []string{"Continuous Task Instructions", "Current cycle: 3", "non-blocking", "Child Tasks To Supervise", "do child", "latest run: succeeded", "investigated branch", "latest message: agent: summary done", "Open Questions Raised For User", "Which branch should I monitor?", "Latest User Comment", "Use main."} {
		if !strings.Contains(contextText, want) {
			t.Fatalf("expected context to contain %q:\n%s", want, contextText)
		}
	}
	if !strings.Contains(contextText, "compare the requested work") {
		t.Fatalf("expected supervision guidance in context")
	}
}
