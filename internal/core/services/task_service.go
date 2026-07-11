package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

const defaultTaskLockTTL = 30 * time.Minute
const completionAuditSettingsKey = "completion_audit.v1"

type completionAuditConfig struct {
	Mode           string `json:"mode"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

type RunCanceler interface {
	CancelRun(ctx context.Context, run domain.Run) error
}

type TaskService struct {
	storage           ports.Storage
	clock             ports.Clock
	idGen             ports.IDGenerator
	bus               ports.EventBus
	lifecycle         TaskLifecycle
	canceler          RunCanceler
	completionAuditor ports.CompletionAuditor
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

func (s *TaskService) SetCanceler(canceler RunCanceler) {
	s.canceler = canceler
}

// SetCompletionAuditor injects the direct, non-recursive audit port. A nil value is
// valid only when the explicit settings mode remains disabled.
func (s *TaskService) SetCompletionAuditor(auditor ports.CompletionAuditor) {
	s.completionAuditor = auditor
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

type CreateTaskInput struct {
	WorkspaceID      string
	ParentID         string
	AgentID          string
	Title            string
	Prompt           string
	Mode             domain.TaskMode
	LoopDelaySeconds int
}

func (s *TaskService) CreateWithOptions(ctx context.Context, input CreateTaskInput) (domain.Task, error) {
	if input.AgentID == "" || input.Prompt == "" {
		return domain.Task{}, fmt.Errorf("%w: agent_id and prompt are required", domain.ErrInvalidInput)
	}
	if input.Title == "" {
		input.Title = firstLine(input.Prompt)
	}

	now := s.clock.Now()
	task := domain.Task{
		ID:          s.idGen.NewID("tsk"),
		ParentID:    input.ParentID,
		WorkspaceID: input.WorkspaceID,
		AgentID:     input.AgentID,
		Title:       input.Title,
		Prompt:      input.Prompt,
		Status:      domain.TaskStatusTodo,
		Mode:        domain.TaskModeOnce,
		MaxAttempts: 1,
		NeedsRun:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if input.Mode == domain.TaskModeContinuous {
		if input.LoopDelaySeconds < 1 {
			input.LoopDelaySeconds = 60
		}
		task.Mode = domain.TaskModeContinuous
		task.MaxAttempts = 0
		task.LoopDelaySeconds = input.LoopDelaySeconds
	}

	err := s.storage.RunInTx(ctx, func(txCtx context.Context) error {
		if err := s.storage.Tasks().Create(txCtx, task); err != nil {
			return err
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
		if err := s.storage.Events().Create(txCtx, event); err != nil {
			return err
		}
		if err := s.storage.Events().CreateOutbox(txCtx, event); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return domain.Task{}, err
	}

	_, _ = NewWakeupService(s.storage, s.clock, s.idGen).Create(ctx, CreateWakeupInput{AgentID: task.AgentID, TaskID: task.ID, Reason: domain.WakeupReasonAssignment, Priority: task.Priority})

	return task, nil
}

func (s *TaskService) CreateChildInWorkspace(ctx context.Context, workspaceID, parentID, agentID, title, prompt string) (domain.Task, error) {
	return s.CreateWithOptions(ctx, CreateTaskInput{
		WorkspaceID: workspaceID,
		ParentID:    parentID,
		AgentID:     agentID,
		Title:       title,
		Prompt:      prompt,
	})
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
	err := s.storage.RunInTx(ctx, func(txCtx context.Context) error {
		if err := s.storage.Messages().Create(txCtx, message); err != nil {
			return err
		}
		if task, err := s.storage.Tasks().Get(txCtx, taskID); err == nil {
			if role == domain.MessageRoleUser && task.Status != domain.TaskStatusCancelled {
				if task.Status == domain.TaskStatusDone {
					subPrompt := fmt.Sprintf("Follow-up on completed task %s:\nOriginal objective: %s\nUser follow-up: %s", task.ID, task.Prompt, body)
					childTitle := "Follow-up: " + firstLine(body)
					if len(childTitle) > 100 {
						childTitle = childTitle[:100] + "..."
					}
					if child, childErr := s.CreateChildInWorkspace(txCtx, task.WorkspaceID, task.ID, task.AgentID, childTitle, subPrompt); childErr == nil {
						_ = s.storage.Messages().Create(txCtx, domain.Message{ID: s.idGen.NewID("msg"), TaskID: task.ID, FromID: fromID, ToID: toID, Role: domain.MessageRoleSystem, Body: "Created follow-up subtask " + child.ID + " from this comment.", CreatedAt: now})
					}
				} else {
					task.NeedsRun = true
					if task.Status == domain.TaskStatusInReview || task.Status == domain.TaskStatusBlocked {
						task.Status = domain.TaskStatusTodo
					}
					task.FinishedAt = nil
					task.CompletedAt = nil
					task.UpdatedAt = now
					if err := s.storage.Tasks().Update(txCtx, task); err != nil {
						return err
					}
				}
			}
		}
		event := domain.Event{ID: s.idGen.NewID("evt"), Type: domain.EventMessageCreated, TaskID: taskID, AgentID: toID, Message: "Message created", Data: map[string]string{"role": string(role)}, CreatedAt: now}
		if err := s.storage.Events().Create(txCtx, event); err != nil {
			return err
		}
		return s.storage.Events().CreateOutbox(txCtx, event)
	})
	if err != nil {
		return domain.Message{}, err
	}
	if role == domain.MessageRoleUser {
		_ = s.scheduleCommentWakeup(ctx, taskID, message)
	}
	return message, nil
}

func (s *TaskService) scheduleCommentWakeup(ctx context.Context, taskID string, message domain.Message) error {
	task, err := s.storage.Tasks().Get(ctx, taskID)
	if err != nil {
		return err
	}
	if task.Status == domain.TaskStatusDone || task.Status == domain.TaskStatusCancelled {
		return nil
	}
	payload := map[string]string{"message_id": message.ID}
	if message.FromID != "" {
		payload["from_id"] = message.FromID
	}
	if message.ToID != "" {
		payload["to_id"] = message.ToID
	}
	wakeups, err := s.storage.Wakeups().ListByTask(ctx, task.ID)
	if err != nil {
		return err
	}
	for _, wakeup := range wakeups {
		if wakeup.AgentID == task.AgentID && wakeup.Reason == domain.WakeupReasonComment && wakeup.Status == domain.WakeupStatusPending {
			wakeup.Payload = payload
			wakeup.Priority = task.Priority
			wakeup.DueAt = s.clock.Now()
			wakeup.UpdatedAt = s.clock.Now()
			return s.storage.Wakeups().Update(ctx, wakeup)
		}
	}
	_, err = NewWakeupService(s.storage, s.clock, s.idGen).Create(ctx, CreateWakeupInput{AgentID: task.AgentID, TaskID: task.ID, Reason: domain.WakeupReasonComment, Priority: task.Priority, Payload: payload})
	return err
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

	err = s.storage.RunInTx(ctx, func(txCtx context.Context) error {
		_ = s.storage.Messages().Create(txCtx, domain.Message{ID: s.idGen.NewID("msg"), TaskID: parentID, FromID: fromAgentID, ToID: toAgentID, Role: domain.MessageRoleDelegated, Body: "Delegated task " + task.ID + ": " + prompt, CreatedAt: now})
		event := domain.Event{ID: s.idGen.NewID("evt"), Type: domain.EventTaskDelegated, TaskID: parentID, AgentID: toAgentID, Message: "Task delegated", Data: map[string]string{"child_task_id": task.ID, "from_agent_id": fromAgentID, "to_agent_id": toAgentID}, CreatedAt: now}
		if err := s.storage.Events().Create(txCtx, event); err != nil {
			return err
		}
		return s.storage.Events().CreateOutbox(txCtx, event)
	})
	return task, err
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

	task, err = s.lifecycle.Apply(task, TaskTransition{From: task.Status, To: domain.TaskStatusCancelled, Comment: reason, Now: now})
	if err != nil {
		return domain.Task{}, err
	}
	task.CancelReason = reason

	var activeRun domain.Run
	hasActiveRun := false
	if activeRunID != "" {
		if run, runErr := s.storage.Runs().Get(ctx, activeRunID); runErr == nil {
			activeRun = run
			hasActiveRun = true
		}
	}

	err = s.storage.RunInTx(ctx, func(txCtx context.Context) error {
		if err := s.storage.Tasks().Update(txCtx, task); err != nil {
			return err
		}
		if hasActiveRun {
			activeRun.Status = domain.RunStatusCanceled
			activeRun.Error = reason
			activeRun.FinishedAt = &now
			if err := s.storage.Runs().Update(txCtx, activeRun); err != nil {
				return err
			}
		}
		event := domain.Event{ID: s.idGen.NewID("evt"), Type: domain.EventTaskCanceled, TaskID: task.ID, AgentID: task.AgentID, RunID: activeRunID, Message: reason, CreatedAt: now}
		if err := s.storage.Events().Create(txCtx, event); err != nil {
			return err
		}
		return s.storage.Events().CreateOutbox(txCtx, event)
	})
	if err != nil {
		return domain.Task{}, err
	}

	if hasActiveRun {
		if s.canceler != nil {
			if cancelErr := s.canceler.CancelRun(ctx, activeRun); cancelErr != nil && !errors.Is(cancelErr, domain.ErrDriverUnavailable) {
				return task, cancelErr
			}
		} else if activeRun.ProcessID > 0 {
			if p, err := os.FindProcess(activeRun.ProcessID); err == nil {
				_ = p.Kill()
			}
		}
	}

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
	if status == domain.TaskStatusDone && task.Status != domain.TaskStatusDone && (task.Status == domain.TaskStatusInProgress || task.Status == domain.TaskStatusInReview) {
		return s.completeWithAudit(ctx, task, comment, agentID)
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

func (s *TaskService) completionAuditConfig(ctx context.Context) (completionAuditConfig, error) {
	raw, err := s.storage.Settings().Get(ctx, completionAuditSettingsKey)
	if errors.Is(err, domain.ErrNotFound) {
		return completionAuditConfig{Mode: "disabled"}, nil
	}
	if err != nil {
		return completionAuditConfig{}, err
	}
	var config completionAuditConfig
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return completionAuditConfig{}, fmt.Errorf("%w: invalid completion audit configuration", domain.ErrCompletionAuditFailed)
	}
	if config.Mode == "" {
		config.Mode = "disabled"
	}
	return config, nil
}

func (s *TaskService) completeWithAudit(ctx context.Context, snapshot domain.Task, comment, agentID string) (domain.Task, error) {
	config, err := s.completionAuditConfig(ctx)
	if err != nil {
		return domain.Task{}, err
	}
	if config.Mode == "disabled" {
		return s.applyApprovedCompletion(ctx, snapshot, comment, agentID)
	}
	if config.Mode != "enforce" || s.completionAuditor == nil {
		return s.persistAuditFailure(ctx, snapshot, agentID, domain.CompletionAuditError, "auditor unavailable", domain.EventCompletionAuditError, domain.ErrCompletionAuditFailed)
	}
	if config.TimeoutSeconds < 1 {
		config.TimeoutSeconds = 30
	}
	now := s.clock.Now()
	audit := domain.CompletionAudit{ID: s.idGen.NewID("audit"), TaskID: snapshot.ID, RequestedByAgentID: agentID, Outcome: domain.CompletionAuditPending, RequestedAt: now}
	if err := s.storage.RunInTx(ctx, func(txCtx context.Context) error {
		return s.persistAuditEvent(txCtx, audit, domain.EventCompletionAuditRequested, "completion audit requested")
	}); err != nil {
		return domain.Task{}, err
	}
	runs, err := s.storage.Runs().ListByTask(ctx, snapshot.ID)
	if err != nil {
		return domain.Task{}, err
	}
	messages, err := s.storage.Messages().ListByTask(ctx, snapshot.ID)
	if err != nil {
		return domain.Task{}, err
	}
	auditCtx, cancel := context.WithTimeout(ctx, time.Duration(config.TimeoutSeconds)*time.Second)
	decision, auditErr := s.completionAuditor.AuditCompletion(auditCtx, ports.CompletionAuditRequest{AuditID: audit.ID, Task: snapshot, RequestedByAgentID: agentID, CompletionComment: comment, Runs: runs, Messages: messages, RequestedAt: now})
	cancel()
	if auditErr != nil {
		if errors.Is(auditErr, context.DeadlineExceeded) {
			return s.finishAuditFailure(ctx, audit, domain.CompletionAuditTimeout, "completion audit timed out", domain.EventCompletionAuditTimeout, domain.ErrCompletionAuditTimeout)
		}
		return s.finishAuditFailure(ctx, audit, domain.CompletionAuditError, "completion audit unavailable", domain.EventCompletionAuditError, domain.ErrCompletionAuditFailed)
	}
	if decision.Outcome == ports.CompletionAuditApprove {
		return s.applyAuditedApproval(ctx, snapshot, comment, agentID, audit, decision)
	}
	if decision.Outcome == ports.CompletionAuditReject {
		return s.applyAuditRejection(ctx, snapshot, agentID, audit, decision)
	}
	return s.finishAuditFailure(ctx, audit, domain.CompletionAuditError, "completion audit returned an invalid decision", domain.EventCompletionAuditError, domain.ErrCompletionAuditFailed)
}

func (s *TaskService) persistAuditEvent(ctx context.Context, audit domain.CompletionAudit, eventType domain.EventType, message string) error {
	if err := s.storage.CompletionAudits().Create(ctx, audit); err != nil {
		return err
	}
	event := domain.Event{ID: s.idGen.NewID("evt"), Type: eventType, TaskID: audit.TaskID, AgentID: audit.RequestedByAgentID, Message: message, Data: map[string]string{"audit_id": audit.ID, "outcome": string(audit.Outcome)}, CreatedAt: s.clock.Now()}
	if err := s.storage.Events().Create(ctx, event); err != nil {
		return err
	}
	return s.storage.Events().CreateOutbox(ctx, event)
}

func (s *TaskService) persistAuditFailure(ctx context.Context, task domain.Task, agentID string, outcome domain.CompletionAuditOutcome, summary string, eventType domain.EventType, result error) (domain.Task, error) {
	now := s.clock.Now()
	audit := domain.CompletionAudit{ID: s.idGen.NewID("audit"), TaskID: task.ID, RequestedByAgentID: agentID, Outcome: outcome, Summary: summary, RequestedAt: now, DecidedAt: &now}
	err := s.storage.RunInTx(ctx, func(txCtx context.Context) error { return s.persistAuditEvent(txCtx, audit, eventType, summary) })
	if err != nil {
		return domain.Task{}, err
	}
	return domain.Task{}, result
}

func (s *TaskService) finishAuditFailure(ctx context.Context, audit domain.CompletionAudit, outcome domain.CompletionAuditOutcome, summary string, eventType domain.EventType, result error) (domain.Task, error) {
	now := s.clock.Now()
	audit.Outcome, audit.Summary, audit.DecidedAt = outcome, summary, &now
	err := s.storage.RunInTx(ctx, func(txCtx context.Context) error {
		if err := s.storage.CompletionAudits().Update(txCtx, audit); err != nil {
			return err
		}
		event := domain.Event{ID: s.idGen.NewID("evt"), Type: eventType, TaskID: audit.TaskID, AgentID: audit.RequestedByAgentID, Message: summary, Data: map[string]string{"audit_id": audit.ID, "outcome": string(outcome)}, CreatedAt: now}
		if err := s.storage.Events().Create(txCtx, event); err != nil {
			return err
		}
		return s.storage.Events().CreateOutbox(txCtx, event)
	})
	if err != nil {
		return domain.Task{}, err
	}
	return domain.Task{}, result
}

func (s *TaskService) applyApprovedCompletion(ctx context.Context, snapshot domain.Task, comment, agentID string) (domain.Task, error) {
	now := s.clock.Now()
	updated, err := s.lifecycle.Apply(snapshot, TaskTransition{From: snapshot.Status, To: domain.TaskStatusDone, Comment: comment, Now: now, UpdatedBy: agentID})
	if err != nil {
		return domain.Task{}, err
	}
	if err := s.storage.Tasks().Update(ctx, updated); err != nil {
		return domain.Task{}, err
	}
	if strings.TrimSpace(comment) != "" {
		_, _ = s.AddMessage(ctx, snapshot.ID, agentID, "", domain.MessageRoleAgent, comment)
	}
	return updated, nil
}

func (s *TaskService) applyAuditedApproval(ctx context.Context, snapshot domain.Task, comment, agentID string, audit domain.CompletionAudit, decision ports.CompletionAuditDecision) (domain.Task, error) {
	now := s.clock.Now()
	updated, err := s.lifecycle.Apply(snapshot, TaskTransition{From: snapshot.Status, To: domain.TaskStatusDone, Comment: comment, Now: now, UpdatedBy: agentID})
	if err != nil {
		return domain.Task{}, err
	}
	audit.Outcome, audit.Summary, audit.DecidedAt = domain.CompletionAuditApproved, decision.Summary, &now
	err = s.storage.RunInTx(ctx, func(txCtx context.Context) error {
		ok, err := s.storage.Tasks().UpdateIfUnchanged(txCtx, updated, ports.TaskPrecondition{Status: snapshot.Status, UpdatedAt: snapshot.UpdatedAt, CheckoutRunID: snapshot.CheckoutRunID})
		if err != nil {
			return err
		}
		if !ok {
			return domain.ErrConflict
		}
		if err := s.storage.CompletionAudits().Update(txCtx, audit); err != nil {
			return err
		}
		if strings.TrimSpace(comment) != "" {
			if err := s.storage.Messages().Create(txCtx, domain.Message{ID: s.idGen.NewID("msg"), TaskID: snapshot.ID, FromID: agentID, Role: domain.MessageRoleAgent, Body: comment, CreatedAt: now}); err != nil {
				return err
			}
		}
		return s.createAuditEvents(txCtx, snapshot, audit, domain.EventCompletionAuditApproved, "completion audit approved", domain.EventTaskCompleted, "Task completed")
	})
	if errors.Is(err, domain.ErrConflict) {
		if _, markErr := s.finishAuditFailure(ctx, audit, domain.CompletionAuditSuperseded, "task changed while completion audit ran", domain.EventCompletionAuditSuperseded, domain.ErrCompletionAuditSuperseded); markErr != nil && !errors.Is(markErr, domain.ErrCompletionAuditSuperseded) {
			return domain.Task{}, markErr
		}
		return domain.Task{}, domain.ErrCompletionAuditSuperseded
	}
	if err != nil {
		return domain.Task{}, err
	}
	return updated, nil
}

func (s *TaskService) applyAuditRejection(ctx context.Context, snapshot domain.Task, agentID string, audit domain.CompletionAudit, decision ports.CompletionAuditDecision) (domain.Task, error) {
	now := s.clock.Now()
	rejected := snapshot
	// Preserve an active checkout so the dispatcher cannot start duplicate work.
	// Without checkout ownership, rejection deliberately makes the rework runnable.
	rejected.Status = domain.TaskStatusInProgress
	rejected.NeedsRun = snapshot.CheckoutRunID == "" && snapshot.CheckedOutByAgentID == ""
	rejected.FinishedAt, rejected.CompletedAt, rejected.UpdatedAt = nil, nil, now
	findings, _ := json.Marshal(decision.Findings)
	audit.Outcome, audit.Summary, audit.FindingsJSON, audit.DecidedAt = domain.CompletionAuditRejected, decision.Summary, string(findings), &now
	err := s.storage.RunInTx(ctx, func(txCtx context.Context) error {
		ok, err := s.storage.Tasks().UpdateIfUnchanged(txCtx, rejected, ports.TaskPrecondition{Status: snapshot.Status, UpdatedAt: snapshot.UpdatedAt, CheckoutRunID: snapshot.CheckoutRunID})
		if err != nil {
			return err
		}
		if !ok {
			return domain.ErrConflict
		}
		if err := s.storage.CompletionAudits().Update(txCtx, audit); err != nil {
			return err
		}
		feedback := "Semantic completion audit rejected: " + decision.Summary
		if err := s.storage.Messages().Create(txCtx, domain.Message{ID: s.idGen.NewID("msg"), TaskID: snapshot.ID, Role: domain.MessageRoleSystem, Body: feedback, CreatedAt: now}); err != nil {
			return err
		}
		return s.createAuditEvents(txCtx, snapshot, audit, domain.EventCompletionAuditRejected, "completion audit rejected")
	})
	if errors.Is(err, domain.ErrConflict) {
		if _, markErr := s.finishAuditFailure(ctx, audit, domain.CompletionAuditSuperseded, "task changed while completion audit ran", domain.EventCompletionAuditSuperseded, domain.ErrCompletionAuditSuperseded); markErr != nil && !errors.Is(markErr, domain.ErrCompletionAuditSuperseded) {
			return domain.Task{}, markErr
		}
		return domain.Task{}, domain.ErrCompletionAuditSuperseded
	}
	if err != nil {
		return domain.Task{}, err
	}
	return domain.Task{}, domain.ErrCompletionAuditRejected
}

func (s *TaskService) createAuditEvents(ctx context.Context, task domain.Task, audit domain.CompletionAudit, entries ...interface{}) error {
	for i := 0; i < len(entries); i += 2 {
		event := domain.Event{ID: s.idGen.NewID("evt"), Type: entries[i].(domain.EventType), TaskID: task.ID, AgentID: task.AgentID, Message: entries[i+1].(string), Data: map[string]string{"audit_id": audit.ID, "outcome": string(audit.Outcome)}, CreatedAt: s.clock.Now()}
		if err := s.storage.Events().Create(ctx, event); err != nil {
			return err
		}
		if err := s.storage.Events().CreateOutbox(ctx, event); err != nil {
			return err
		}
	}
	return nil
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

func (s *TaskService) PauseContinuous(ctx context.Context, id string) (domain.Task, error) {
	task, err := s.storage.Tasks().Get(ctx, id)
	if err != nil {
		return domain.Task{}, err
	}
	if task.Mode != domain.TaskModeContinuous {
		return domain.Task{}, fmt.Errorf("%w: task is not continuous", domain.ErrInvalidInput)
	}
	now := s.clock.Now()
	task.LoopPausedAt = &now
	task.LoopNextRunAt = nil
	task.NeedsRun = false
	if task.CheckoutRunID == "" {
		task.Status = domain.TaskStatusWaitingNextCycle
	}
	task.UpdatedAt = now
	return task, s.storage.Tasks().Update(ctx, task)
}

func (s *TaskService) ResumeContinuous(ctx context.Context, id string) (domain.Task, error) {
	task, err := s.storage.Tasks().Get(ctx, id)
	if err != nil {
		return domain.Task{}, err
	}
	if task.Mode != domain.TaskModeContinuous {
		return domain.Task{}, fmt.Errorf("%w: task is not continuous", domain.ErrInvalidInput)
	}
	now := s.clock.Now()
	delay := task.LoopDelaySeconds
	if delay < 1 {
		delay = 60
		task.LoopDelaySeconds = delay
	}
	nextRunAt := now.Add(time.Duration(delay) * time.Second)
	task.LoopPausedAt = nil
	if task.CheckoutRunID == "" {
		task.Status = domain.TaskStatusWaitingNextCycle
		task.NeedsRun = false
		task.LoopNextRunAt = &nextRunAt
	}
	task.UpdatedAt = now
	return task, s.storage.Tasks().Update(ctx, task)
}

func (s *TaskService) RunContinuousNow(ctx context.Context, id string) (domain.Task, error) {
	task, err := s.storage.Tasks().Get(ctx, id)
	if err != nil {
		return domain.Task{}, err
	}
	if task.Mode != domain.TaskModeContinuous {
		return domain.Task{}, fmt.Errorf("%w: task is not continuous", domain.ErrInvalidInput)
	}
	now := s.clock.Now()
	task.LoopPausedAt = nil
	task.LoopNextRunAt = nil
	if task.CheckoutRunID == "" {
		task.Status = domain.TaskStatusTodo
		task.NeedsRun = true
		task.FinishedAt = nil
		task.CompletedAt = nil
	}
	task.UpdatedAt = now
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
	case domain.TaskStatusBacklog, domain.TaskStatusTodo, domain.TaskStatusInProgress, domain.TaskStatusWaitingNextCycle, domain.TaskStatusInReview, domain.TaskStatusBlocked, domain.TaskStatusDone, domain.TaskStatusCancelled:
		return true
	default:
		return false
	}
}
