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
	runner := NewRunner(st, clock, idgen, noopBus{}, nil, NoopMemoryProvider{}, testLogger{}, Config{})
	contextText := runner.taskProtocolContext(context.Background(), task, domain.Run{ID: "run_prompt"}, messages)
	for _, want := range []string{"Continuous Task Instructions", "Current cycle: 3", "non-blocking", "Child Tasks To Supervise", "do child", "latest run: succeeded", "investigated branch", "latest message: agent: summary done", "Open Questions Raised For User", "Which branch should I monitor?", "Latest User Comment", "Use main."} {
		if !strings.Contains(contextText, want) {
			t.Fatalf("expected context to contain %q:\n%s", want, contextText)
		}
	}
	if !strings.Contains(contextText, "compare the requested work") {
		t.Fatalf("expected supervision guidance in context")
	}
}
