package services

import (
	"errors"
	"testing"
	"time"

	"picoclip/internal/core/domain"
)

func TestTaskLifecycleTransitionMatrixMatchesFormalContract(t *testing.T) {
	lifecycle := NewTaskLifecycle()
	want := map[domain.TaskStatus][]domain.TaskStatus{
		domain.TaskStatusBacklog: {
			domain.TaskStatusTodo,
			domain.TaskStatusCancelled,
		},
		domain.TaskStatusTodo: {
			domain.TaskStatusBacklog,
			domain.TaskStatusInProgress,
			domain.TaskStatusWaitingNextCycle,
			domain.TaskStatusBlocked,
			domain.TaskStatusCancelled,
		},
		domain.TaskStatusInProgress: {
			domain.TaskStatusTodo,
			domain.TaskStatusInReview,
			domain.TaskStatusBlocked,
			domain.TaskStatusDone,
			domain.TaskStatusCancelled,
		},
		domain.TaskStatusWaitingNextCycle: {
			domain.TaskStatusTodo,
			domain.TaskStatusCancelled,
		},
		domain.TaskStatusInReview: {
			domain.TaskStatusTodo,
			domain.TaskStatusInProgress,
			domain.TaskStatusDone,
			domain.TaskStatusBlocked,
			domain.TaskStatusCancelled,
		},
		domain.TaskStatusBlocked: {
			domain.TaskStatusTodo,
			domain.TaskStatusInProgress,
			domain.TaskStatusCancelled,
		},
		domain.TaskStatusDone: {
			domain.TaskStatusTodo,
		},
		domain.TaskStatusCancelled: {
			domain.TaskStatusTodo,
		},
	}

	got := lifecycle.TransitionMatrix()
	if len(got) != len(want) {
		t.Fatalf("TransitionMatrix has %d states, want %d: %#v", len(got), len(want), got)
	}
	for from, wantAllowed := range want {
		gotAllowed, ok := got[from]
		if !ok {
			t.Fatalf("TransitionMatrix missing state %s", from)
		}
		if len(gotAllowed) != len(wantAllowed) {
			t.Fatalf("TransitionMatrix[%s] = %v, want %v", from, gotAllowed, wantAllowed)
		}
		for i, to := range wantAllowed {
			if gotAllowed[i] != to {
				t.Fatalf("TransitionMatrix[%s][%d] = %s, want %s", from, i, gotAllowed[i], to)
			}
			if !lifecycle.CanTransition(from, to) {
				t.Fatalf("CanTransition(%s, %s) = false, want true from matrix", from, to)
			}
		}
	}
}

func TestTaskLifecycleTransitionMatrixIsImmutableSnapshot(t *testing.T) {
	lifecycle := NewTaskLifecycle()
	matrix := lifecycle.TransitionMatrix()
	matrix[domain.TaskStatusBacklog][0] = domain.TaskStatusDone

	if lifecycle.CanTransition(domain.TaskStatusBacklog, domain.TaskStatusDone) {
		t.Fatal("mutating TransitionMatrix snapshot changed lifecycle rules")
	}
}

func TestTaskLifecycleCanTransitionRejectsInvalidEdges(t *testing.T) {
	lifecycle := NewTaskLifecycle()
	cases := []struct {
		name string
		from domain.TaskStatus
		to   domain.TaskStatus
	}{
		{"backlog to done", domain.TaskStatusBacklog, domain.TaskStatusDone},
		{"todo to done", domain.TaskStatusTodo, domain.TaskStatusDone},
		{"done to in progress", domain.TaskStatusDone, domain.TaskStatusInProgress},
		{"waiting next cycle to done", domain.TaskStatusWaitingNextCycle, domain.TaskStatusDone},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if lifecycle.CanTransition(tt.from, tt.to) {
				t.Fatalf("CanTransition(%s, %s) = true, want false", tt.from, tt.to)
			}
		})
	}
}

func TestTaskLifecycleRequiresCommentForTerminalOrBlocked(t *testing.T) {
	lifecycle := NewTaskLifecycle()
	now := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
	task := domain.Task{ID: "tsk_1", Status: domain.TaskStatusInProgress, NeedsRun: true, CreatedAt: now, UpdatedAt: now}

	for _, status := range []domain.TaskStatus{domain.TaskStatusBlocked, domain.TaskStatusDone, domain.TaskStatusCancelled} {
		t.Run(string(status), func(t *testing.T) {
			_, err := lifecycle.Apply(task, TaskTransition{From: task.Status, To: status, Now: now})
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("expected invalid input, got %v", err)
			}
		})
	}
}

func TestTaskLifecycleApplyDoneClearsExecutionState(t *testing.T) {
	lifecycle := NewTaskLifecycle()
	now := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
	task := domain.Task{
		ID:                  "tsk_1",
		Status:              domain.TaskStatusInProgress,
		NeedsRun:            true,
		CheckoutRunID:       "run_1",
		CheckedOutByAgentID: "agt_1",
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	got, err := lifecycle.Apply(task, TaskTransition{From: task.Status, To: domain.TaskStatusDone, Comment: "completed", Now: now})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if got.Status != domain.TaskStatusDone {
		t.Fatalf("Status = %s, want done", got.Status)
	}
	if got.NeedsRun {
		t.Fatal("NeedsRun = true, want false")
	}
	if got.CheckoutRunID != "" || got.CheckedOutByAgentID != "" {
		t.Fatalf("checkout state not cleared: run=%q agent=%q", got.CheckoutRunID, got.CheckedOutByAgentID)
	}
	if got.CompletedAt == nil || got.FinishedAt == nil {
		t.Fatal("completion timestamps were not set")
	}
}

func TestTaskLifecycleApplyTodoWakesTask(t *testing.T) {
	lifecycle := NewTaskLifecycle()
	now := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
	completedAt := now.Add(-time.Hour)
	task := domain.Task{
		ID:          "tsk_1",
		Status:      domain.TaskStatusDone,
		NeedsRun:    false,
		CompletedAt: &completedAt,
		FinishedAt:  &completedAt,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	got, err := lifecycle.Apply(task, TaskTransition{From: task.Status, To: domain.TaskStatusTodo, Now: now})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if got.Status != domain.TaskStatusTodo {
		t.Fatalf("Status = %s, want todo", got.Status)
	}
	if !got.NeedsRun {
		t.Fatal("NeedsRun = false, want true")
	}
	if got.CompletedAt != nil || got.FinishedAt != nil {
		t.Fatal("completion timestamps were not cleared")
	}
}

func TestTaskLifecycleApplyWaitingNextCycleDoesNotWakeTask(t *testing.T) {
	lifecycle := NewTaskLifecycle()
	now := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	nextRunAt := now.Add(time.Minute)
	task := domain.Task{
		ID:            "tsk_continuous",
		Status:        domain.TaskStatusTodo,
		Mode:          domain.TaskModeContinuous,
		NeedsRun:      true,
		LoopNextRunAt: &nextRunAt,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	got, err := lifecycle.Apply(task, TaskTransition{From: task.Status, To: domain.TaskStatusWaitingNextCycle, Now: now})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if got.Status != domain.TaskStatusWaitingNextCycle {
		t.Fatalf("Status = %s, want waiting_next_cycle", got.Status)
	}
	if got.NeedsRun {
		t.Fatal("NeedsRun = true, want false while waiting for next cycle")
	}
	if got.FinishedAt == nil {
		t.Fatal("FinishedAt was not set for completed cycle")
	}
	if got.LoopNextRunAt == nil || !got.LoopNextRunAt.Equal(nextRunAt) {
		t.Fatalf("LoopNextRunAt changed unexpectedly: %v", got.LoopNextRunAt)
	}
}

func TestTaskLifecycleRejectsInvalidTransition(t *testing.T) {
	lifecycle := NewTaskLifecycle()
	now := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
	task := domain.Task{ID: "tsk_1", Status: domain.TaskStatusBacklog, CreatedAt: now, UpdatedAt: now}

	_, err := lifecycle.Apply(task, TaskTransition{From: task.Status, To: domain.TaskStatusDone, Comment: "done", Now: now})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}
