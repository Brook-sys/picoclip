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

func (r *Runner) blockIfBudgetExceeded(ctx context.Context, task domain.Task, now time.Time) bool {
	stopped, budget, usage, err := NewBudgetService(r.storage, r.clock, r.idGen).IsHardStopped(ctx, task.WorkspaceID, task.AgentID)
	if err != nil {
		r.logger.Warn("runner.budget_check_failed", "task_id", task.ID, "agent_id", task.AgentID, "err", err)
		return false
	}
	if !stopped {
		return false
	}

	task.Status = domain.TaskStatusBlocked
	task.NeedsRun = false
	task.UpdatedAt = now
	_ = r.storage.Tasks().Update(ctx, task)

	message := fmt.Sprintf("Task blocked by budget %s: usage is %d tokens across %d runs.", budget.ID, usage.TotalTokens, usage.Runs)
	_ = r.storage.Messages().Create(ctx, domain.Message{ID: r.idGen.NewID("msg"), TaskID: task.ID, Role: domain.MessageRoleSystem, Body: message, CreatedAt: now})
	_ = r.bus.Publish(ctx, domain.Event{ID: r.idGen.NewID("evt"), Type: domain.EventBudgetBlocked, TaskID: task.ID, AgentID: task.AgentID, Message: message, Data: map[string]string{"budget_id": budget.ID}, CreatedAt: now})
	r.logger.Warn("runner.budget_hard_stop", "task_id", task.ID, "agent_id", task.AgentID, "budget_id", budget.ID)
	return true
}

func (r *Runner) Run(ctx context.Context, task domain.Task) {
	current, err := r.storage.Tasks().Get(ctx, task.ID)
	if err != nil || current.Status == domain.TaskStatusCancelled || current.Status == domain.TaskStatusDone {
		return
	}
	task = current
	now := r.clock.Now()
	if r.blockIfBudgetExceeded(ctx, task, now) {
		return
	}

	runID := r.idGen.NewID("run")
	lockExpiresAt := now.Add(defaultTaskLockTTL)

	task.Status = domain.TaskStatusInProgress
	task.NeedsRun = false
	task.Attempts++
	task.CheckoutRunID = runID
	task.CheckedOutByAgentID = task.AgentID
	task.StartedAt = &now
	task.ExecutionLockedAt = &now
	task.LockExpiresAt = &lockExpiresAt
	task.UpdatedAt = now
	if err := r.storage.Tasks().Update(ctx, task); err != nil {
		return
	}

	agent, err := r.storage.Agents().Get(ctx, task.AgentID)
	if err != nil {
		r.failTask(ctx, task, "agent not found")
		return
	}

	if agent.Type != "noop" {
		runtimeID := domain.RuntimeID(agent.Type)
		state, stateErr := r.runtimes.State(ctx, runtimeID)
		adapter, ok := r.runtimes.Adapter(runtimeID)
		if stateErr != nil || !state.Enabled || !ok || adapter.Resolve(ctx, state) != nil {
			r.logger.Warn("runner.runtime_unavailable", "task_id", task.ID, "agent_id", agent.ID, "type", agent.Type)
			_ = r.bus.Publish(ctx, domain.Event{ID: r.idGen.NewID("evt"), Type: domain.EventDriverMissing, TaskID: task.ID, AgentID: agent.ID, Message: "Runtime unavailable", CreatedAt: r.clock.Now()})
			r.failTask(ctx, task, "runtime unavailable")
			return
		}
	}

	run := domain.Run{
		ID:           runID,
		TaskID:       task.ID,
		AgentID:      agent.ID,
		DriverType:   string(agent.Type),
		Status:       domain.RunStatusRunning,
		Attempt:      task.Attempts,
		Input:        task.Prompt,
		StartedAt:    r.clock.Now(),
		LastOutputAt: &now,
		StallTimeout: int(r.config.TaskTimeout.Seconds()),
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

	conversation = append(conversation, r.taskProtocolContext(ctx, task, run, messages))
	for _, skill := range skills {
		conversation = append(conversation, r.skillContext(skill))
	}
	conversation = append(conversation, "User task: "+task.Prompt)
	task.Prompt = strings.Join(conversation, "\n\n")
	run.Input = task.Prompt
	run.InputTokens = estimateTokens(run.Input)
	run.TotalTokens = run.InputTokens
	_ = r.storage.Runs().Update(ctx, run)

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
		run.OutputTokens = estimateTokens(run.Error)
		run.TotalTokens = run.InputTokens + run.OutputTokens
		r.logger.Warn("runner.run_failed", "task_id", task.ID, "run_id", run.ID, "runtime", agent.Type, "err", err)
		_ = r.storage.Runs().Update(ctx, run)
		r.failTask(ctx, task, fmt.Sprintf("%v", err))
		r.recordTokenUsage(ctx, run)
		return
	}

	r.logger.Debug("runner.run_completed", "task_id", task.ID, "run_id", run.ID, "runtime", agent.Type)
	run.Status = domain.RunStatusSucceeded
	run.Output = result.Output
	run.OutputTokens = estimateTokens(run.Output)
	run.TotalTokens = run.InputTokens + run.OutputTokens
	now2 := r.clock.Now()
	run.LastOutputAt = &now2
	_ = r.storage.Runs().Update(ctx, run)
	r.recordTokenUsage(ctx, run)
	_ = r.memory.SaveRun(ctx, task, run)
	if strings.TrimSpace(result.Output) != "" {
		_ = r.storage.Messages().Create(ctx, domain.Message{ID: r.idGen.NewID("msg"), TaskID: task.ID, FromID: agent.ID, Role: domain.MessageRoleAgent, Body: result.Output, CreatedAt: now})
	}
	latest, latestErr = r.storage.Tasks().Get(ctx, task.ID)
	if latestErr == nil && latest.Status != domain.TaskStatusDone && latest.Status != domain.TaskStatusCancelled {
		if agent.Type == "noop" {
			latest.Status = domain.TaskStatusTodo
		} else {
			latest.Status = domain.TaskStatusInProgress
		}
		latest.UpdatedAt = now
		_ = r.storage.Tasks().Update(ctx, latest)
	}
	_ = r.bus.Publish(ctx, domain.Event{ID: r.idGen.NewID("evt"), Type: domain.EventRunCompleted, TaskID: task.ID, AgentID: agent.ID, RunID: run.ID, Message: "Run completed", CreatedAt: now})
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

func DefaultTaskProtocolPrompt() string {
	return strings.Join([]string{
		"PicoClip Task Protocol:",
		"Your goal is to satisfy the task title, description, and the latest user comment.",
		"1. You are running one task heartbeat.",
		"2. Before work: checkout the task using POST /agent-api/tasks/{id}/checkout if not already in_progress.",
		"3. During work: do useful work and leave a progress comment via POST /agent-api/tasks/{id}/comments.",
		"4. If satisfied/completed: PATCH /agent-api/tasks/{id} with status=done and a clear final comment.",
		"5. If blocked: PATCH /agent-api/tasks/{id} with status=blocked, explaining the blocker and owner/next action.",
		"6. If work should be split: POST /agent-api/tasks/{id}/delegate with child task title and acceptance criteria.",
		"7. Do not stay silent. Every run must leave a final comment or status update.",
	}, "\n")
}

func estimateTokens(text string) int {
	return len(strings.Fields(text)) * 4 / 3
}

func (r *Runner) recordTokenUsage(ctx context.Context, run domain.Run) {
	task, err := r.storage.Tasks().Get(ctx, run.TaskID)
	if err == nil {
		task.InputTokens += run.InputTokens
		task.OutputTokens += run.OutputTokens
		task.TotalTokens += run.TotalTokens
		_ = r.storage.Tasks().Update(ctx, task)
	}

	agent, err := r.storage.Agents().Get(ctx, run.AgentID)
	if err == nil {
		agent.InputTokens += run.InputTokens
		agent.OutputTokens += run.OutputTokens
		agent.TotalTokens += run.TotalTokens
		_ = r.storage.Agents().Update(ctx, agent)
	}
}

func (r *Runner) taskProtocolPrompt(ctx context.Context) string {
	raw, err := r.storage.Settings().Get(ctx, "general")
	if err == nil && raw != "" {
		var general struct {
			DefaultTaskProtocol string `json:"DefaultTaskProtocol"`
		}
		if jsonErr := json.Unmarshal([]byte(raw), &general); jsonErr == nil && strings.TrimSpace(general.DefaultTaskProtocol) != "" {
			return general.DefaultTaskProtocol
		}
	}
	return DefaultTaskProtocolPrompt()
}

func latestCommentByRole(messages []domain.Message, role domain.MessageRole) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == role && strings.TrimSpace(messages[i].Body) != "" {
			return messages[i].Body
		}
	}
	return ""
}

func (r *Runner) taskProtocolContext(ctx context.Context, task domain.Task, run domain.Run, messages []domain.Message) string {
	var sb strings.Builder
	sb.WriteString(r.taskProtocolPrompt(ctx))
	sb.WriteString("\n\nCompact Task Context:\n")
	sb.WriteString(fmt.Sprintf("- Task ID: %s\n", task.ID))
	sb.WriteString(fmt.Sprintf("- Title: %s\n", task.Title))
	sb.WriteString(fmt.Sprintf("- Status: %s\n", string(task.Status)))
	sb.WriteString(fmt.Sprintf("- Run ID: %s\n", run.ID))
	if task.ParentID != "" {
		sb.WriteString(fmt.Sprintf("- Parent Task ID: %s\n", task.ParentID))
	}

	latestUserComment := latestCommentByRole(messages, domain.MessageRoleUser)
	if latestUserComment != "" {
		sb.WriteString("\nLatest User Comment:\n")
		sb.WriteString(latestUserComment + "\n")
	}

	sb.WriteString("\nRecent Comments:\n")
	if len(messages) == 0 {
		sb.WriteString("(no comments)\n")
	} else {
		start := len(messages) - 8
		if start < 0 {
			start = 0
		}
		for _, m := range messages[start:] {
			sb.WriteString(fmt.Sprintf("[%s] %s: %s\n", m.CreatedAt.Format("15:04"), m.Role, m.Body))
		}
	}

	return sb.String()
}
