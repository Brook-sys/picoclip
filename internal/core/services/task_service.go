package services

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

const defaultTaskLockTTL = 30 * time.Minute

type TaskService struct {
	storage   ports.Storage
	clock     ports.Clock
	idGen     ports.IDGenerator
	bus       ports.EventBus
	lifecycle TaskLifecycle
}

func NewTaskService(storage ports.Storage, clock ports.Clock, idGen ports.IDGenerator, bus ports.EventBus) *TaskService {
	return &TaskService{
		storage:   storage,
		clock:     clock,
		idGen:     idGen,
		bus:       bus,
		lifecycle: NewTaskLifecycle(),
	}
}

func (s *TaskService) Create(ctx context.Context, agentID, title, prompt string) (domain.Task, error) {
	return s.CreateInWorkspace(ctx, "", agentID, title, prompt)
}

func (s *TaskService) CreateInWorkspace(ctx context.Context, workspaceID, agentID, title, prompt string) (domain.Task, error) {
	return s.CreateChildInWorkspace(ctx, workspaceID, "", agentID, title, prompt)
}

func (s *TaskService) CreateChild(ctx context.Context, parentID, agentID, title, prompt string) (domain.Task, error) {
	return s.CreateChildInWorkspace(ctx, "", parentID, agentID, title, prompt)
}

func (s *TaskService) CreateChildInWorkspace(ctx context.Context, workspaceID, parentID, agentID, title, prompt string) (domain.Task, error) {
	if agentID == "" || prompt == "" {
		return domain.Task{}, fmt.Errorf("%w: agent_id and prompt are required", domain.ErrInvalidInput)
	}
	if title == "" {
		title = firstLine(prompt)
	}

	now := s.clock.Now()
	task := domain.Task{
		ID:          s.idGen.NewID("tsk"),
		ParentID:    parentID,
		WorkspaceID: workspaceID,
		AgentID:     agentID,
		Title:       title,
		Prompt:      prompt,
		Status:      domain.TaskStatusTodo,
		MaxAttempts: 1,
		NeedsRun:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.storage.Tasks().Create(ctx, task); err != nil {
		return domain.Task{}, err
	}

	event := domain.Event{
		ID:        s.idGen.NewID("evt"),
		Type:      domain.EventTaskCreated,
		TaskID:    task.ID,
		AgentID:   task.AgentID,
		Message:   "Task created",
		Data:      map[string]string{"actor": "user"},
		CreatedAt: now,
	}
	_ = s.storage.Events().Create(ctx, event)
	_ = s.bus.Publish(ctx, event)
	_, _ = NewWakeupService(s.storage, s.clock, s.idGen).Create(ctx, CreateWakeupInput{AgentID: task.AgentID, TaskID: task.ID, Reason: domain.WakeupReasonAssignment, Priority: task.Priority})

	return task, nil
}

func (s *TaskService) List(ctx context.Context, filter ports.TaskFilter) ([]domain.Task, error) {
	return s.storage.Tasks().List(ctx, filter)
}

func (s *TaskService) Get(ctx context.Context, id string) (domain.Task, error) {
	return s.storage.Tasks().Get(ctx, id)
}

func (s *TaskService) GetRuns(ctx context.Context, id string) ([]domain.Run, error) {
	return s.storage.Runs().ListByTask(ctx, id)
}

func (s *TaskService) AddMessage(ctx context.Context, taskID, fromID, toID string, role domain.MessageRole, body string) (domain.Message, error) {
	if taskID == "" || body == "" {
		return domain.Message{}, fmt.Errorf("%w: task_id and body are required", domain.ErrInvalidInput)
	}

	now := s.clock.Now()
	message := domain.Message{
		ID:        s.idGen.NewID("msg"),
		TaskID:    taskID,
		FromID:    fromID,
		ToID:      toID,
		Role:      role,
		Body:      body,
		CreatedAt: now,
	}
	if err := s.storage.Messages().Create(ctx, message); err != nil {
		return domain.Message{}, err
	}
	if task, err := s.storage.Tasks().Get(ctx, taskID); err == nil {
		if role == domain.MessageRoleUser && task.Status != domain.TaskStatusCancelled {
			if task.Status == domain.TaskStatusDone {
				subPrompt := fmt.Sprintf("Follow-up on completed task %s:\nOriginal objective: %s\nUser follow-up: %s", task.ID, task.Prompt, body)
				childTitle := "Follow-up: " + firstLine(body)
				if len(childTitle) > 100 {
					childTitle = childTitle[:100] + "..."
				}
				if child, childErr := s.CreateChildInWorkspace(ctx, task.WorkspaceID, task.ID, task.AgentID, childTitle, subPrompt); childErr == nil {
					_ = s.storage.Messages().Create(ctx, domain.Message{ID: s.idGen.NewID("msg"), TaskID: task.ID, FromID: fromID, ToID: toID, Role: domain.MessageRoleSystem, Body: "Created follow-up subtask " + child.ID + " from this comment.", CreatedAt: now})
				}
			} else {
				task.NeedsRun = true
				if task.Status == domain.TaskStatusInReview || task.Status == domain.TaskStatusBlocked {
					task.Status = domain.TaskStatusTodo
				}
				task.FinishedAt = nil
				task.CompletedAt = nil
				task.UpdatedAt = now
				_ = s.storage.Tasks().Update(ctx, task)
			}
		}
	}
	event := domain.Event{ID: s.idGen.NewID("evt"), Type: domain.EventMessageCreated, TaskID: taskID, AgentID: toID, Message: "Message created", Data: map[string]string{"role": string(role)}, CreatedAt: now}
	_ = s.storage.Events().Create(ctx, event)
	_ = s.bus.Publish(ctx, event)
	if role == domain.MessageRoleUser {
		if task, err := s.storage.Tasks().Get(ctx, taskID); err == nil {
			_, _ = NewWakeupService(s.storage, s.clock, s.idGen).Create(ctx, CreateWakeupInput{AgentID: task.AgentID, TaskID: task.ID, Reason: domain.WakeupReasonComment, Priority: task.Priority})
		}
	}
	return message, nil
}

func (s *TaskService) Delegate(ctx context.Context, parentID, fromAgentID, toAgentID, prompt string) (domain.Task, error) {
	parent, err := s.storage.Tasks().Get(ctx, parentID)
	if err != nil {
		return domain.Task{}, err
	}
	task, err := s.CreateChildInWorkspace(ctx, parent.WorkspaceID, parentID, toAgentID, "", prompt)
	if err != nil {
		return domain.Task{}, err
	}
	now := s.clock.Now()
	_ = s.storage.Messages().Create(ctx, domain.Message{ID: s.idGen.NewID("msg"), TaskID: parentID, FromID: fromAgentID, ToID: toAgentID, Role: domain.MessageRoleDelegated, Body: "Delegated task " + task.ID + ": " + prompt, CreatedAt: now})
	event := domain.Event{ID: s.idGen.NewID("evt"), Type: domain.EventTaskDelegated, TaskID: parentID, AgentID: toAgentID, Message: "Task delegated", Data: map[string]string{"child_task_id": task.ID, "from_agent_id": fromAgentID, "to_agent_id": toAgentID}, CreatedAt: now}
	_ = s.storage.Events().Create(ctx, event)
	_ = s.bus.Publish(ctx, event)
	return task, nil
}

func (s *TaskService) Cancel(ctx context.Context, id, reason string) (domain.Task, error) {
	task, err := s.storage.Tasks().Get(ctx, id)
	if err != nil {
		return domain.Task{}, err
	}
	if task.Status == domain.TaskStatusDone || task.Status == domain.TaskStatusCancelled {
		return task, nil
	}
	now := s.clock.Now()
	activeRunID := task.CheckoutRunID

	task.Status = domain.TaskStatusCancelled
	task.NeedsRun = false
	task.CheckoutRunID = ""
	task.CheckedOutByAgentID = ""
	task.ExecutionLockedAt = nil
	task.LockExpiresAt = nil
	task.CancelReason = reason
	task.FinishedAt = &now
	task.CancelledAt = &now
	task.UpdatedAt = now

	if err := s.storage.Tasks().Update(ctx, task); err != nil {
		return domain.Task{}, err
	}

	if activeRunID != "" {
		if run, runErr := s.storage.Runs().Get(ctx, activeRunID); runErr == nil && run.ProcessID > 0 {
			if p, err := os.FindProcess(run.ProcessID); err == nil {
				_ = p.Kill()
			}
		}
	}

	event := domain.Event{ID: s.idGen.NewID("evt"), Type: domain.EventTaskCanceled, TaskID: task.ID, AgentID: task.AgentID, Message: reason, CreatedAt: now}
	_ = s.storage.Events().Create(ctx, event)
	_ = s.bus.Publish(ctx, event)
	return task, nil
}

func (s *TaskService) Checkout(ctx context.Context, id, agentID, runID string, expected []domain.TaskStatus) (domain.Task, error) {
	if runID == "" {
		return domain.Task{}, fmt.Errorf("%w: run_id is required", domain.ErrInvalidInput)
	}
	task, err := s.storage.Tasks().Get(ctx, id)
	if err != nil {
		return domain.Task{}, err
	}
	if agentID == "" {
		agentID = task.AgentID
	}
	if task.CheckedOutByAgentID != "" && task.CheckedOutByAgentID != agentID {
		return domain.Task{}, fmt.Errorf("%w: task checked out by another agent", domain.ErrConflict)
	}
	if len(expected) > 0 && !statusIn(task.Status, expected) && task.CheckedOutByAgentID == "" {
		return domain.Task{}, fmt.Errorf("%w: unexpected task status", domain.ErrConflict)
	}
	now := s.clock.Now()
	task.Status = domain.TaskStatusInProgress
	task.NeedsRun = false
	task.CheckedOutByAgentID = agentID
	task.CheckoutRunID = runID
	lockExpiresAt := now.Add(defaultTaskLockTTL)
	task.StartedAt = &now
	task.ExecutionLockedAt = &now
	task.LockExpiresAt = &lockExpiresAt
	task.UpdatedAt = now
	if err := s.storage.Tasks().Update(ctx, task); err != nil {
		return domain.Task{}, err
	}
	return task, nil
}

func (s *TaskService) Release(ctx context.Context, id, agentID, comment string) (domain.Task, error) {
	task, err := s.storage.Tasks().Get(ctx, id)
	if err != nil {
		return domain.Task{}, err
	}
	if task.CheckedOutByAgentID != "" && agentID != "" && task.CheckedOutByAgentID != agentID {
		return domain.Task{}, fmt.Errorf("%w: task checked out by another agent", domain.ErrConflict)
	}
	now := s.clock.Now()
	task.CheckedOutByAgentID = ""
	task.CheckoutRunID = ""
	task.ExecutionLockedAt = nil
	task.LockExpiresAt = nil
	if task.Status == domain.TaskStatusInProgress {
		task.Status = domain.TaskStatusTodo
	}
	task.UpdatedAt = now
	if err := s.storage.Tasks().Update(ctx, task); err != nil {
		return domain.Task{}, err
	}
	if strings.TrimSpace(comment) != "" {
		_, _ = s.AddMessage(ctx, id, agentID, "", domain.MessageRoleAgent, comment)
	}
	return task, nil
}

func (s *TaskService) UpdateStatus(ctx context.Context, id string, status domain.TaskStatus, comment, agentID string) (domain.Task, error) {
	task, err := s.storage.Tasks().Get(ctx, id)
	if err != nil {
		return domain.Task{}, err
	}
	now := s.clock.Now()
	task, err = s.lifecycle.Apply(task, TaskTransition{From: task.Status, To: status, Comment: comment, Now: now, UpdatedBy: agentID})
	if err != nil {
		return domain.Task{}, err
	}
	if err := s.storage.Tasks().Update(ctx, task); err != nil {
		return domain.Task{}, err
	}
	if strings.TrimSpace(comment) != "" {
		_, _ = s.AddMessage(ctx, id, agentID, "", domain.MessageRoleAgent, comment)
	}
	return task, nil
}

func (s *TaskService) Wake(ctx context.Context, id string) (domain.Task, error) {
	task, err := s.storage.Tasks().Get(ctx, id)
	if err != nil {
		return domain.Task{}, err
	}
	if task.Status == domain.TaskStatusDone || task.Status == domain.TaskStatusCancelled {
		task.Status = domain.TaskStatusTodo
		task.CompletedAt = nil
		task.CancelledAt = nil
		task.FinishedAt = nil
	}
	task.NeedsRun = true
	task.UpdatedAt = s.clock.Now()
	return task, s.storage.Tasks().Update(ctx, task)
}

func (s *TaskService) GetMessages(ctx context.Context, id string) ([]domain.Message, error) {
	return s.storage.Messages().ListByTask(ctx, id)
}

func firstLine(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "Untitled task"
	}
	line, _, _ := strings.Cut(value, "\n")
	if len(line) > 80 {
		line = line[:80]
	}
	return line
}

func statusIn(status domain.TaskStatus, statuses []domain.TaskStatus) bool {
	for _, item := range statuses {
		if item == status {
			return true
		}
	}
	return false
}

func validTaskStatus(status domain.TaskStatus) bool {
	switch status {
	case domain.TaskStatusBacklog, domain.TaskStatusTodo, domain.TaskStatusInProgress, domain.TaskStatusInReview, domain.TaskStatusBlocked, domain.TaskStatusDone, domain.TaskStatusCancelled:
		return true
	default:
		return false
	}
}
