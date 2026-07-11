package domain

import "time"

type EventType string

const (
	EventAgentCreated              EventType = "agent.created"
	EventTaskCreated               EventType = "task.created"
	EventTaskQueued                EventType = "task.queued"
	EventTaskStarted               EventType = "task.started"
	EventTaskCompleted             EventType = "task.completed"
	EventTaskFailed                EventType = "task.failed"
	EventTaskCanceled              EventType = "task.canceled"
	EventTaskReleased              EventType = "task.released"
	EventRunStarted                EventType = "run.started"
	EventRunOutput                 EventType = "run.output"
	EventRunCompleted              EventType = "run.completed"
	EventRunFailed                 EventType = "run.failed"
	EventRunCanceled               EventType = "run.canceled"
	EventRunTimeout                EventType = "run.timeout"
	EventRunRecovered              EventType = "run.recovered"
	EventRuntimeStarted            EventType = "runtime.started"
	EventRuntimeProcessStarted     EventType = "runtime.process_started"
	EventRuntimeHeartbeat          EventType = "runtime.heartbeat"
	EventRuntimeCompleted          EventType = "runtime.completed"
	EventRuntimeTimeout            EventType = "runtime.timeout"
	EventRuntimeStalled            EventType = "runtime.stalled"
	EventRuntimeCancelRequested    EventType = "runtime.cancel_requested"
	EventRuntimeCancelSucceeded    EventType = "runtime.cancel_succeeded"
	EventRuntimeCancelFailed       EventType = "runtime.cancel_failed"
	EventReconcilerFailed          EventType = "reconciler.failed"
	EventRetryScheduled            EventType = "retry.scheduled"
	EventAgentHeartbeatWakeup      EventType = "agent.heartbeat_wakeup"
	EventDriverMissing             EventType = "driver.missing"
	EventTaskDelegated             EventType = "task.delegated"
	EventMessageCreated            EventType = "message.created"
	EventBudgetBlocked             EventType = "budget.blocked"
	EventCompletionAuditRequested  EventType = "completion_audit.requested"
	EventCompletionAuditApproved   EventType = "completion_audit.approved"
	EventCompletionAuditRejected   EventType = "completion_audit.rejected"
	EventCompletionAuditError      EventType = "completion_audit.error"
	EventCompletionAuditTimeout    EventType = "completion_audit.timeout"
	EventCompletionAuditSuperseded EventType = "completion_audit.superseded"
)

type Event struct {
	ID        string            `json:"id"`
	Type      EventType         `json:"type"`
	TaskID    string            `json:"task_id,omitempty"`
	AgentID   string            `json:"agent_id,omitempty"`
	RunID     string            `json:"run_id,omitempty"`
	Message   string            `json:"message"`
	Data      map[string]string `json:"data,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}
