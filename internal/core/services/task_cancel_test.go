package services

import (
	"context"
	"testing"
	"time"

	"picoclip/internal/adapters/storage/memory"
	"picoclip/internal/core/domain"
)

type recordingRunCanceler struct {
	calls []domain.Run
	err   error
}

func (c *recordingRunCanceler) CancelRun(ctx context.Context, run domain.Run) error {
	c.calls = append(c.calls, run)
	return c.err
}

func TestTaskServiceCancelMarksActiveRunCanceledAndInvokesCanceler(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 6, 11, 0, 0, 0, time.UTC)}
	svc := NewTaskService(st, clock, &seqID{}, noopBus{})
	canceler := &recordingRunCanceler{}
	svc.SetCanceler(canceler)

	started := clock.t.Add(-time.Minute)
	expires := clock.t.Add(time.Minute)
	task := domain.Task{
		ID:                  "task_cancel",
		AgentID:             "agent_cancel",
		Title:               "cancel me",
		Prompt:              "long run",
		Status:              domain.TaskStatusInProgress,
		NeedsRun:            false,
		CheckoutRunID:       "run_cancel",
		CheckedOutByAgentID: "agent_cancel",
		ExecutionLockedAt:   &started,
		LockExpiresAt:       &expires,
		CreatedAt:           clock.t,
		UpdatedAt:           clock.t,
	}
	if err := st.Tasks().Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	run := domain.Run{ID: "run_cancel", TaskID: task.ID, AgentID: task.AgentID, DriverType: "crush", Status: domain.RunStatusRunning, ProcessID: 4242, StartedAt: started}
	if err := st.Runs().Create(context.Background(), run); err != nil {
		t.Fatal(err)
	}

	got, err := svc.Cancel(context.Background(), task.ID, "user stopped it")
	if err != nil {
		t.Fatalf("cancel task: %v", err)
	}
	if got.Status != domain.TaskStatusCancelled || got.NeedsRun {
		t.Fatalf("expected cancelled task without needs_run, got %#v", got)
	}
	if got.CheckoutRunID != "" || got.CheckedOutByAgentID != "" || got.ExecutionLockedAt != nil || got.LockExpiresAt != nil {
		t.Fatalf("expected execution lock cleared, got %#v", got)
	}
	if len(canceler.calls) != 1 || canceler.calls[0].ID != run.ID {
		t.Fatalf("expected canceler called with active run, got %#v", canceler.calls)
	}

	savedRun, err := st.Runs().Get(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if savedRun.Status != domain.RunStatusCanceled {
		t.Fatalf("run status = %s, want canceled", savedRun.Status)
	}
	if savedRun.Error != "user stopped it" {
		t.Fatalf("run error = %q, want cancel reason", savedRun.Error)
	}
	if savedRun.FinishedAt == nil || !savedRun.FinishedAt.Equal(clock.t) {
		t.Fatalf("run finished_at = %v, want %v", savedRun.FinishedAt, clock.t)
	}
}
