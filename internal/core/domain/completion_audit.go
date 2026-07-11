package domain

import "time"

type CompletionAuditOutcome string

const (
	CompletionAuditPending    CompletionAuditOutcome = "pending"
	CompletionAuditApproved   CompletionAuditOutcome = "approved"
	CompletionAuditRejected   CompletionAuditOutcome = "rejected"
	CompletionAuditError      CompletionAuditOutcome = "error"
	CompletionAuditTimeout    CompletionAuditOutcome = "timeout"
	CompletionAuditSuperseded CompletionAuditOutcome = "superseded"
)

type CompletionAudit struct {
	ID                 string                 `json:"id"`
	TaskID             string                 `json:"task_id"`
	RequestedByAgentID string                 `json:"requested_by_agent_id,omitempty"`
	Outcome            CompletionAuditOutcome `json:"outcome"`
	Summary            string                 `json:"summary,omitempty"`
	FindingsJSON       string                 `json:"findings_json,omitempty"`
	RequestedAt        time.Time              `json:"requested_at"`
	DecidedAt          *time.Time             `json:"decided_at,omitempty"`
}

var (
	ErrCompletionAuditRejected   = completionAuditError("semantic completion audit rejected")
	ErrCompletionAuditTimeout    = completionAuditError("semantic completion audit timed out")
	ErrCompletionAuditFailed     = completionAuditError("semantic completion audit unavailable")
	ErrCompletionAuditSuperseded = completionAuditError("semantic completion audit superseded")
)

type completionAuditError string

func (e completionAuditError) Error() string { return string(e) }

// Is makes each stable audit error matchable with errors.Is.
func (e completionAuditError) Is(target error) bool { return e == target }
