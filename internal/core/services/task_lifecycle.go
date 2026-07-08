package services

import (
	"fmt"
	"strings"
	"time"

	"picoclip/internal/core/domain"
)

type TaskLifecycle struct{}

type TaskTransition struct {
	From      domain.TaskStatus
	To        domain.TaskStatus
	Comment   string
	Now       time.Time
	UpdatedBy string
}

func NewTaskLifecycle() TaskLifecycle {
	return TaskLifecycle{}
}

func (l TaskLifecycle) Apply(task domain.Task, transition TaskTransition) (domain.Task, error) {
	if transition.To == "" {
		task.UpdatedAt = transition.Now
		return task, nil
	}
	if !validTaskStatus(transition.To) {
		return domain.Task{}, fmt.Errorf("%w: invalid task status", domain.ErrInvalidInput)
	}
	if !l.CanTransition(transition.From, transition.To) {
		return domain.Task{}, fmt.Errorf("%w: cannot transition task from %s to %s", domain.ErrConflict, transition.From, transition.To)
	}
	if requiresTransitionComment(transition.To) && strings.TrimSpace(transition.Comment) == "" {
		return domain.Task{}, fmt.Errorf("%w: comment is required for %s", domain.ErrInvalidInput, transition.To)
	}

	task.Status = transition.To
	task.UpdatedAt = transition.Now

	switch transition.To {
	case domain.TaskStatusBacklog:
		task.NeedsRun = false
		task.StartedAt = nil
		task.FinishedAt = nil
		task.CompletedAt = nil
		task.CancelledAt = nil
		task.CheckoutRunID = ""
		task.CheckedOutByAgentID = ""
		task.ExecutionLockedAt = nil
		task.LockExpiresAt = nil
	case domain.TaskStatusTodo:
		task.NeedsRun = true
		task.FinishedAt = nil
		task.CompletedAt = nil
		task.CancelledAt = nil
		task.CheckoutRunID = ""
		task.CheckedOutByAgentID = ""
		task.ExecutionLockedAt = nil
		task.LockExpiresAt = nil
	case domain.TaskStatusInProgress:
		task.NeedsRun = true
		if task.StartedAt == nil {
			task.StartedAt = &transition.Now
		}
		task.FinishedAt = nil
		task.CompletedAt = nil
		task.CancelledAt = nil
	case domain.TaskStatusWaitingNextCycle:
		task.NeedsRun = false
		if task.FinishedAt == nil {
			task.FinishedAt = &transition.Now
		}
		task.CompletedAt = nil
		task.CancelledAt = nil
		task.CheckoutRunID = ""
		task.CheckedOutByAgentID = ""
		task.ExecutionLockedAt = nil
		task.LockExpiresAt = nil
	case domain.TaskStatusInReview:
		task.NeedsRun = false
		task.FinishedAt = nil
		task.CompletedAt = nil
		task.CancelledAt = nil
		task.CheckoutRunID = ""
		task.CheckedOutByAgentID = ""
		task.ExecutionLockedAt = nil
		task.LockExpiresAt = nil
	case domain.TaskStatusBlocked:
		task.NeedsRun = false
		task.FinishedAt = nil
		task.CompletedAt = nil
		task.CancelledAt = nil
		task.CheckoutRunID = ""
		task.CheckedOutByAgentID = ""
		task.ExecutionLockedAt = nil
		task.LockExpiresAt = nil
	case domain.TaskStatusDone:
		task.NeedsRun = false
		task.FinishedAt = &transition.Now
		task.CompletedAt = &transition.Now
		task.CancelledAt = nil
		task.CheckoutRunID = ""
		task.CheckedOutByAgentID = ""
		task.ExecutionLockedAt = nil
		task.LockExpiresAt = nil
	case domain.TaskStatusCancelled:
		task.NeedsRun = false
		task.FinishedAt = &transition.Now
		task.CancelledAt = &transition.Now
		task.CheckoutRunID = ""
		task.CheckedOutByAgentID = ""
		task.ExecutionLockedAt = nil
		task.LockExpiresAt = nil
	}

	return task, nil
}

func (l TaskLifecycle) TransitionMatrix() map[domain.TaskStatus][]domain.TaskStatus {
	matrix := make(map[domain.TaskStatus][]domain.TaskStatus, len(taskTransitionMatrix))
	for from, allowed := range taskTransitionMatrix {
		matrix[from] = append([]domain.TaskStatus(nil), allowed...)
	}
	return matrix
}

func (l TaskLifecycle) CanTransition(from, to domain.TaskStatus) bool {
	if from == to {
		return true
	}
	allowed, ok := taskTransitionMatrix[from]
	if !ok {
		return false
	}
	for _, item := range allowed {
		if item == to {
			return true
		}
	}
	return false
}

var taskTransitionMatrix = map[domain.TaskStatus][]domain.TaskStatus{
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
	domain.TaskStatusWaitingNextCycle: {
		domain.TaskStatusTodo,
		domain.TaskStatusCancelled,
	},
	domain.TaskStatusCancelled: {
		domain.TaskStatusTodo,
	},
}

func requiresTransitionComment(status domain.TaskStatus) bool {
	return status == domain.TaskStatusBlocked || status == domain.TaskStatusDone || status == domain.TaskStatusCancelled
}
