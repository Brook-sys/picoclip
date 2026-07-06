package services

import (
	"context"
	"testing"
	"time"

	"picoclip/internal/adapters/storage/memory"
	"picoclip/internal/core/domain"
)

func TestWakeupServiceCreateAndListDue(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 6, 25, 14, 0, 0, 0, time.UTC)}
	idgen := &seqID{}
	svc := NewWakeupService(st, clock, idgen)

	created, err := svc.Create(context.Background(), CreateWakeupInput{
		AgentID:  "agt_1",
		TaskID:   "tsk_1",
		Reason:   domain.WakeupReasonAssignment,
		Priority: 10,
	})
	if err != nil {
		t.Fatalf("create wakeup: %v", err)
	}
	if created.Status != domain.WakeupStatusPending {
		t.Fatalf("status=%s want pending", created.Status)
	}

	due, err := svc.ListDue(context.Background(), 10)
	if err != nil {
		t.Fatalf("list due: %v", err)
	}
	if len(due) != 1 || due[0].ID != created.ID {
		t.Fatalf("due=%v want created wakeup", due)
	}

	completed, err := svc.MarkCompleted(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("complete wakeup: %v", err)
	}
	if completed.Status != domain.WakeupStatusCompleted {
		t.Fatalf("status=%s want completed", completed.Status)
	}
	due, err = svc.ListDue(context.Background(), 10)
	if err != nil {
		t.Fatalf("list due after complete: %v", err)
	}
	if len(due) != 0 {
		t.Fatalf("due len=%d want 0", len(due))
	}
}

func TestWakeupServiceProcessDueWakesTask(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 6, 25, 15, 0, 0, 0, time.UTC)}
	idgen := &seqID{}
	bus := noopBus{}
	taskSvc := NewTaskService(st, clock, idgen, bus)

	agent := domain.Agent{ID: "agt_1", Name: "a", Type: "internal", Enabled: true, Capability: domain.CapabilityWorker, CreatedAt: clock.t, UpdatedAt: clock.t}
	_ = st.Agents().Create(context.Background(), agent)

	task, err := taskSvc.Create(context.Background(), agent.ID, "t", "do")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	task.Status = domain.TaskStatusBlocked
	task.NeedsRun = false
	if err := st.Tasks().Update(context.Background(), task); err != nil {
		t.Fatalf("update task: %v", err)
	}

	wakeupSvc := NewWakeupService(st, clock, idgen)
	created, err := wakeupSvc.Create(context.Background(), CreateWakeupInput{AgentID: agent.ID, TaskID: task.ID, Reason: domain.WakeupReasonManual, Priority: 1})
	if err != nil {
		t.Fatalf("create wakeup: %v", err)
	}
	processed, err := wakeupSvc.ProcessDue(context.Background(), 10)
	if err != nil {
		t.Fatalf("process due: %v", err)
	}
	if processed != 2 {
		t.Fatalf("processed=%d want 2", processed)
	}

	got, err := st.Tasks().Get(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Status != domain.TaskStatusTodo || !got.NeedsRun {
		t.Fatalf("expected todo/needs_run, got status=%s needs_run=%v", got.Status, got.NeedsRun)
	}
	wakeup, err := st.Wakeups().Get(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get wakeup: %v", err)
	}
	if wakeup.Status != domain.WakeupStatusCompleted {
		t.Fatalf("wakeup status=%s want completed", wakeup.Status)
	}
}

func TestWakeupServiceDoesNotWakeContinuousTaskBeforeNextRun(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 6, 25, 15, 0, 0, 0, time.UTC)}
	idgen := &seqID{}
	bus := noopBus{}
	taskSvc := NewTaskService(st, clock, idgen, bus)

	agent := domain.Agent{ID: "agt_cont", Name: "c", Type: "internal", Enabled: true, Capability: domain.CapabilityWorker, CreatedAt: clock.t, UpdatedAt: clock.t}
	_ = st.Agents().Create(context.Background(), agent)

	task, err := taskSvc.CreateWithOptions(context.Background(), CreateTaskInput{AgentID: agent.ID, Title: "t", Prompt: "do", Mode: domain.TaskModeContinuous, LoopDelaySeconds: 60})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	task.Status = domain.TaskStatusWaitingNextCycle
	task.NeedsRun = false
	next := clock.t.Add(60 * time.Second)
	task.LoopNextRunAt = &next
	if err := st.Tasks().Update(context.Background(), task); err != nil {
		t.Fatalf("update task: %v", err)
	}

	wakeupSvc := NewWakeupService(st, clock, idgen)
	created, err := wakeupSvc.Create(context.Background(), CreateWakeupInput{AgentID: agent.ID, TaskID: task.ID, Reason: domain.WakeupReasonComment, Priority: 1})
	if err != nil {
		t.Fatalf("create wakeup: %v", err)
	}
	processed, err := wakeupSvc.ProcessDue(context.Background(), 10)
	if err != nil {
		t.Fatalf("process due: %v", err)
	}
	if processed != 2 {
		t.Fatalf("processed=%d want 2", processed)
	}

	got, err := st.Tasks().Get(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Status != domain.TaskStatusWaitingNextCycle || got.NeedsRun {
		t.Fatalf("expected continuous task to remain waiting, got status=%s needs_run=%v", got.Status, got.NeedsRun)
	}

	wakeup, err := st.Wakeups().Get(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get wakeup: %v", err)
	}
	if wakeup.Status != domain.WakeupStatusCompleted {
		t.Fatalf("wakeup status=%s want completed", wakeup.Status)
	}
}
