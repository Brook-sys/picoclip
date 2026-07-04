package domain

import "time"

type WakeupReason string

type WakeupStatus string

const (
	WakeupReasonAssignment WakeupReason = "assignment"
	WakeupReasonComment    WakeupReason = "comment"
	WakeupReasonManual     WakeupReason = "manual"
	WakeupReasonRetry      WakeupReason = "retry"
	WakeupReasonSchedule   WakeupReason = "schedule"
	WakeupReasonRecovery   WakeupReason = "recovery"
)

const (
	WakeupStatusPending   WakeupStatus = "pending"
	WakeupStatusClaimed   WakeupStatus = "claimed"
	WakeupStatusCompleted WakeupStatus = "completed"
	WakeupStatusCancelled WakeupStatus = "cancelled"
)

type WakeupRequest struct {
	ID        string            `json:"id"`
	AgentID   string            `json:"agent_id"`
	TaskID    string            `json:"task_id,omitempty"`
	Reason    WakeupReason      `json:"reason"`
	Status    WakeupStatus      `json:"status"`
	Priority  int               `json:"priority"`
	DueAt     time.Time         `json:"due_at"`
	ClaimedAt *time.Time        `json:"claimed_at,omitempty"`
	Payload   map[string]string `json:"payload,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}
