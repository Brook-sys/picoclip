package ports

import (
	"context"
	"time"

	"picoclip/internal/core/domain"
)

type CompletionAuditOutcome string

const (
	CompletionAuditApprove CompletionAuditOutcome = "approve"
	CompletionAuditReject  CompletionAuditOutcome = "reject"
)

type CompletionAuditRequest struct {
	AuditID            string
	Task               domain.Task
	RequestedByAgentID string
	CompletionComment  string
	Runs               []domain.Run
	Messages           []domain.Message
	RequestedAt        time.Time
}

type CompletionAuditFinding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type CompletionAuditDecision struct {
	Outcome  CompletionAuditOutcome
	Summary  string
	Findings []CompletionAuditFinding
	ModelRef string
}

// CompletionAuditor is deliberately synchronous and does not create normal tasks or runs.
type CompletionAuditor interface {
	AuditCompletion(context.Context, CompletionAuditRequest) (CompletionAuditDecision, error)
}

type CompletionAuditRepository interface {
	Create(ctx context.Context, audit domain.CompletionAudit) error
	Update(ctx context.Context, audit domain.CompletionAudit) error
	ListByTask(ctx context.Context, taskID string) ([]domain.CompletionAudit, error)
}

type TaskPrecondition struct {
	Status        domain.TaskStatus
	UpdatedAt     time.Time
	CheckoutRunID string
}

type ConditionalTaskRepository interface {
	UpdateIfUnchanged(ctx context.Context, task domain.Task, precondition TaskPrecondition) (bool, error)
}
