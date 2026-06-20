package domain

import "time"

type EventType string

const (
	EventAgentCreated   EventType = "agent.created"
	EventTaskCreated    EventType = "task.created"
	EventTaskQueued     EventType = "task.queued"
	EventTaskStarted    EventType = "task.started"
	EventTaskCompleted  EventType = "task.completed"
	EventTaskFailed     EventType = "task.failed"
	EventTaskCanceled   EventType = "task.canceled"
	EventRunStarted     EventType = "run.started"
	EventRunOutput      EventType = "run.output"
	EventRunCompleted   EventType = "run.completed"
	EventRunFailed      EventType = "run.failed"
	EventDriverMissing  EventType = "driver.missing"
	EventTaskDelegated  EventType = "task.delegated"
	EventMessageCreated EventType = "message.created"
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
