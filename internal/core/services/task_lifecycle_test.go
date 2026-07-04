package services

import (
	"errors"
	"testing"
	"time"

	"picoclip/internal/core/domain"
)

func TestTaskLifecycleCanTransition(t *testing.T) {
	lifecycle := NewTaskLifecycle()
	cases := []struct {
		name string
		from domain.TaskStatus
		to   domain.TaskStatus
		want bool
	}{
		{"backlog to todo", domain.TaskStatusBacklog, domain.TaskStatusTodo, true},
		{"todo to in progress", domain.TaskStatusTodo, domain.TaskStatusInProgress, true},
		{"in progress to review", domain.TaskStatusInProgress, domain.TaskStatusInReview, true},
		{"review to done", domain.TaskStatusInReview, domain.TaskStatusDone, true},
		{"blocked to todo", domain.TaskStatusBlocked, domain.TaskStatusTodo, true},
		{"done to todo", domain.TaskStatusDone, domain.TaskStatusTodo, true},
		{"cancelled to todo", domain.TaskStatusCancelled, domain.TaskStatusTodo, true},
		{"backlog to done", domain.TaskStatusBacklog, domain.TaskStatusDone, false},
		{"todo to done", domain.TaskStatusTodo, domain.TaskStatusDone, false},
		{"done to in progress", domain.TaskStatusDone, domain.TaskStatusInProgress, false},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := lifecycle.CanTransition(tt.from, tt.to); got != tt.want {
				t.Fatalf("CanTransition(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.want)
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

func TestTaskLifecycleRejectsInvalidTransition(t *testing.T) {
	lifecycle := NewTaskLifecycle()
	now := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
	task := domain.Task{ID: "tsk_1", Status: domain.TaskStatusBacklog, CreatedAt: now, UpdatedAt: now}

	_, err := lifecycle.Apply(task, TaskTransition{From: task.Status, To: domain.TaskStatusDone, Comment: "done", Now: now})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}
