package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

type Runner struct {
	storage  ports.Storage
	clock    ports.Clock
	idGen    ports.IDGenerator
	bus      ports.EventBus
	runtimes *RuntimeManager
	memory   ports.MemoryProvider
	logger   ports.Logger
	config   Config
}

func NewRunner(storage ports.Storage, clock ports.Clock, idGen ports.IDGenerator, bus ports.EventBus, runtimes *RuntimeManager, memory ports.MemoryProvider, logger ports.Logger, config Config) *Runner {
	return &Runner{
		storage:  storage,
		clock:    clock,
		idGen:    idGen,
		bus:      bus,
		runtimes: runtimes,
		memory:   memory,
		logger:   logger,
		config:   config,
	}
}

func (r *Runner) Run(ctx context.Context, task domain.Task) {
	current, err := r.storage.Tasks().Get(ctx, task.ID)
	if err != nil || current.Status == domain.TaskStatusCancelled || current.Status == domain.TaskStatusDone {
		return
	}
	task = current
	now := r.clock.Now()
	task.Status = domain.TaskStatusInProgress
	task.NeedsRun = false
	task.Attempts++
	task.StartedAt = &now
	task.UpdatedAt = now
	if err := r.storage.Tasks().Update(ctx, task); err != nil {
		return
	}

	agent, err := r.storage.Agents().Get(ctx, task.AgentID)
	if err != nil {
		r.failTask(ctx, task, "agent not found")
		return
	}

	if _, ok := r.runtimes.Adapter(domain.RuntimeID(agent.Type)); !ok && agent.Type != "noop" {
		r.logger.Warn("runner.runtime_unavailable", "task_id", task.ID, "agent_id", agent.ID, "type", agent.Type)
		_ = r.bus.Publish(ctx, domain.Event{ID: r.idGen.NewID("evt"), Type: domain.EventDriverMissing, TaskID: task.ID, AgentID: agent.ID, Message: "Runtime not available", CreatedAt: r.clock.Now()})
		r.failTask(ctx, task, "runtime not available")
		return
	}

	run := domain.Run{
		ID:         r.idGen.NewID("run"),
		TaskID:     task.ID,
		AgentID:    agent.ID,
		DriverType: string(agent.Type),
		Status:     domain.RunStatusRunning,
		Attempt:    task.Attempts,
		Input:      task.Prompt,
		StartedAt:  r.clock.Now(),
	}
	if err := r.storage.Runs().Create(ctx, run); err != nil {
		r.failTask(ctx, task, err.Error())
		return
	}

	messages, _ := r.storage.Messages().ListByTask(ctx, task.ID)
	skills := r.skillsForAgent(ctx, agent)
	conversation := make([]string, 0, len(messages)+len(skills)+5)
	conversation = append(conversation, r.permissionContext(agent))
	if agent.InstructionFile != "" {
		content, fileErr := os.ReadFile(agent.InstructionFile)
		if fileErr == nil {
			conversation = append(conversation, fmt.Sprintf("Agent instruction file (%s):\n%s", agent.InstructionFile, string(content)))
		} else {
			conversation = append(conversation, fmt.Sprintf("Failed to load agent instruction file %s: %v", agent.InstructionFile, fileErr))
		}
	}
	if agent.SystemPrompt != "" {
		conversation = append(conversation, "Agent custom system prompt:\n"+agent.SystemPrompt)
	}

	conversation = append(conversation, "PicoClip agent APIs: GET /agent-api/docs, GET /agent-api/me, GET /agent-api/tasks?status=todo,in_progress,in_review,blocked, GET /agent-api/tasks/{id}, POST /agent-api/tasks/{id}/checkout, POST /agent-api/tasks/{id}/comments, PATCH /agent-api/tasks/{id}, POST /agent-api/tasks/{id}/release, POST /agent-api/tasks/{id}/delegate. Always checkout before work, always leave a comment, and only mark a task done by PATCHing status=done with a clear comment.")
	for _, skill := range skills {
		conversation = append(conversation, r.skillContext(skill))
	}
	conversation = append(conversation, "User task: "+task.Prompt)
	for _, message := range messages {
		conversation = append(conversation, fmt.Sprintf("%s: %s", message.Role, message.Body))
	}
	task.Prompt = strings.Join(conversation, "\n\n")

	r.logger.Debug("runner.run_started", "task_id", task.ID, "agent_id", agent.ID, "run_id", run.ID, "runtime", agent.Type)
	_ = r.bus.Publish(ctx, domain.Event{ID: r.idGen.NewID("evt"), Type: domain.EventRunStarted, TaskID: task.ID, AgentID: agent.ID, RunID: run.ID, Message: "Run started", CreatedAt: r.clock.Now()})

	runCtx, cancel := context.WithTimeout(ctx, r.config.TaskTimeout)
	defer cancel()

	memoryContext, err := r.memory.ContextForTask(runCtx, task, agent)
	if err != nil {
		memoryContext = ""
	}

	var result ports.RuntimeExecutionResult
	if agent.Type == "noop" {
		result = ports.RuntimeExecutionResult{Output: "noop driver executed"}
	} else {
		result, err = r.runtimes.Execute(runCtx, domain.RuntimeID(agent.Type), ports.RuntimeExecutionInput{Agent: agent, Task: task, Run: run, Memory: memoryContext, Config: agent.Config, Env: agent.Env, ExtraArgs: agent.ExtraArgs})
	}
	finishedAt := r.clock.Now()
	run.FinishedAt = &finishedAt
	latest, latestErr := r.storage.Tasks().Get(ctx, task.ID)
	if latestErr == nil && latest.Status == domain.TaskStatusCancelled {
		r.logger.Warn("runner.task_canceled", "task_id", task.ID, "run_id", run.ID, "reason", latest.CancelReason)
		run.Status = domain.RunStatusCanceled
		run.Error = latest.CancelReason
		_ = r.storage.Runs().Update(ctx, run)
		return
	}

	if err != nil {
		run.Status = domain.RunStatusFailed
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			run.Status = domain.RunStatusTimeout
		}
		run.Error = err.Error()
		r.logger.Warn("runner.run_failed", "task_id", task.ID, "run_id", run.ID, "runtime", agent.Type, "err", err)
		_ = r.storage.Runs().Update(ctx, run)
		r.failTask(ctx, task, fmt.Sprintf("%v", err))
		return
	}

	r.logger.Debug("runner.run_completed", "task_id", task.ID, "run_id", run.ID, "runtime", agent.Type)
	run.Status = domain.RunStatusSucceeded
	run.Output = result.Output
	_ = r.storage.Runs().Update(ctx, run)
	_ = r.memory.SaveRun(ctx, task, run)
	if strings.TrimSpace(result.Output) != "" {
		_ = r.storage.Messages().Create(ctx, domain.Message{ID: r.idGen.NewID("msg"), TaskID: task.ID, FromID: agent.ID, Role: domain.MessageRoleAgent, Body: result.Output, CreatedAt: finishedAt})
	}
	latest, latestErr = r.storage.Tasks().Get(ctx, task.ID)
	if latestErr == nil && latest.Status != domain.TaskStatusDone && latest.Status != domain.TaskStatusCancelled {
		if agent.Type == "noop" {
			latest.Status = domain.TaskStatusTodo
		} else {
			latest.Status = domain.TaskStatusInProgress
		}
		latest.UpdatedAt = finishedAt
		_ = r.storage.Tasks().Update(ctx, latest)
	}
	_ = r.bus.Publish(ctx, domain.Event{ID: r.idGen.NewID("evt"), Type: domain.EventRunCompleted, TaskID: task.ID, AgentID: agent.ID, RunID: run.ID, Message: "Run completed", CreatedAt: finishedAt})
}

func (r *Runner) failTask(ctx context.Context, task domain.Task, message string) {
	now := r.clock.Now()
	task.Status = domain.TaskStatusBlocked
	task.NeedsRun = false
	task.FinishedAt = nil
	task.UpdatedAt = now
	_ = r.storage.Tasks().Update(ctx, task)
	_ = r.storage.Messages().Create(ctx, domain.Message{ID: r.idGen.NewID("msg"), TaskID: task.ID, FromID: task.AgentID, Role: domain.MessageRoleSystem, Body: "Run failed: " + message, CreatedAt: now})
	_ = r.bus.Publish(ctx, domain.Event{ID: r.idGen.NewID("evt"), Type: domain.EventTaskFailed, TaskID: task.ID, AgentID: task.AgentID, Message: message, CreatedAt: now})
}

func (r *Runner) skillsForAgent(ctx context.Context, agent domain.Agent) []domain.Skill {
	all, err := r.storage.Skills().List(ctx, agent.ProjectID)
	if err != nil {
		return nil
	}
	manual := make(map[string]struct{}, len(agent.SkillIDs))
	for _, id := range agent.SkillIDs {
		manual[id] = struct{}{}
	}
	permissions := make(map[domain.AgentPermission]struct{}, len(agent.Permissions))
	for _, permission := range agent.Permissions {
		permissions[permission] = struct{}{}
	}
	enabled := make([]domain.Skill, 0, len(all))
	seen := map[string]struct{}{}
	for _, skill := range all {
		if !skill.Enabled {
			continue
		}
		_, isManual := manual[skill.ID]
		_, hasPermission := permissions[skill.Permission]
		if skill.Kind == domain.SkillKindBuiltin && skill.Permission != "" && hasPermission || isManual || skillAllowedForAgent(skill, agent) {
			if _, ok := seen[skill.ID]; ok {
				continue
			}
			seen[skill.ID] = struct{}{}
			enabled = append(enabled, skill)
		}
	}
	return enabled
}

func skillAllowedForAgent(skill domain.Skill, agent domain.Agent) bool {
	if len(skill.AgentIDs) > 0 {
		for _, id := range skill.AgentIDs {
			if id == agent.ID {
				return true
			}
		}
	}
	if len(skill.AllowedAgentTypes) > 0 {
		for _, agentType := range skill.AllowedAgentTypes {
			if agentType == agent.Type {
				return true
			}
		}
	}
	if len(skill.AllowedPermissions) > 0 {
		for _, required := range skill.AllowedPermissions {
			for _, permission := range agent.Permissions {
				if required == permission {
					return true
				}
			}
		}
	}
	return false
}

func (r *Runner) permissionContext(agent domain.Agent) string {
	return fmt.Sprintf("Agent permissions:\n%s", joinPermissions(agent.Permissions))
}

func (r *Runner) skillContext(skill domain.Skill) string {
	parts := []string{fmt.Sprintf("Skill package: %s\n%s\nInstructions:\n%s", skill.Name, skill.Description, skill.Instructions)}
	for _, file := range skill.Files {
		parts = append(parts, fmt.Sprintf("Skill file %s:\n%s", file.Path, file.Content))
	}
	return strings.Join(parts, "\n\n")
}

func joinPermissions(permissions []domain.AgentPermission) string {
	values := make([]string, 0, len(permissions))
	for _, permission := range permissions {
		values = append(values, string(permission))
	}
	return strings.Join(values, ", ")
}
