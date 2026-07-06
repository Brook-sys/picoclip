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

func (r *Runner) emitEvent(ctx context.Context, ev domain.Event) {
	_ = r.storage.Events().Create(ctx, ev)
	_ = r.storage.Events().CreateOutbox(ctx, ev)
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

	message := fmt.Sprintf("Task blocked by budget %s: usage is %d tokens across %d runs.", budget.ID, usage.TotalTokens, usage.Runs)
	_ = r.storage.RunInTx(ctx, func(txCtx context.Context) error {
		if err := r.storage.Tasks().Update(txCtx, task); err != nil {
			return err
		}
		if err := r.storage.Messages().Create(txCtx, domain.Message{ID: r.idGen.NewID("msg"), TaskID: task.ID, Role: domain.MessageRoleSystem, Body: message, CreatedAt: now}); err != nil {
			return err
		}
		ev := domain.Event{ID: r.idGen.NewID("evt"), Type: domain.EventBudgetBlocked, TaskID: task.ID, AgentID: task.AgentID, Message: message, Data: map[string]string{"budget_id": budget.ID}, CreatedAt: now}
		if err := r.storage.Events().Create(txCtx, ev); err != nil {
			return err
		}
		return r.storage.Events().CreateOutbox(txCtx, ev)
	})
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
	if r.maxAttemptsExceeded(task) {
		r.failTask(ctx, task, "maximum attempts reached")
		return
	}
	if r.blockIfBudgetExceeded(ctx, task, now) {
		return
	}

	agent, err := r.storage.Agents().Get(ctx, task.AgentID)
	if err != nil {
		r.failTask(ctx, task, "agent not found")
		return
	}

	var run domain.Run

	if task.CheckoutRunID != "" {
		// Task já veio com lease do ClaimNextRunnable
		run, err = r.storage.Runs().Get(ctx, task.CheckoutRunID)
		if err != nil {
			r.failTask(ctx, task, "run not found for lease")
			return
		}
	} else {
		// Caminho legacy (ClaimNextPending)
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

		run = domain.Run{
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
	}

	if agent.Type != "noop" {
		runtimeID := domain.RuntimeID(agent.Type)
		state, stateErr := r.runtimes.State(ctx, runtimeID)
		adapter, ok := r.runtimes.Adapter(runtimeID)
		if stateErr != nil || !state.Enabled || !ok || adapter.Resolve(ctx, state) != nil {
			r.logger.Warn("runner.runtime_unavailable", "task_id", task.ID, "agent_id", agent.ID, "type", agent.Type)
			r.emitEvent(ctx, domain.Event{ID: r.idGen.NewID("evt"), Type: domain.EventDriverMissing, TaskID: task.ID, AgentID: agent.ID, Message: "Runtime unavailable", CreatedAt: r.clock.Now()})
			r.failTask(ctx, task, "runtime unavailable")
			return
		}
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
	if catalog := r.skillCatalogContext(skills); catalog != "" {
		conversation = append(conversation, catalog)
	}
	manualSkills := r.manualSkillIDs(agent)
	for _, skill := range skills {
		if _, ok := manualSkills[skill.ID]; ok {
			conversation = append(conversation, r.skillContext(skill))
		}
	}
	conversation = append(conversation, "User task: "+promptSnippet(task.Prompt, 2500))
	task.Prompt = strings.Join(conversation, "\n\n")
	run.Input = task.Prompt
	run.InputTokens = estimateTokens(run.Input)
	run.TotalTokens = run.InputTokens
	_ = r.storage.Runs().Update(ctx, run)

	r.logger.Debug("runner.run_started", "task_id", task.ID, "agent_id", agent.ID, "run_id", run.ID, "runtime", agent.Type)
	r.emitEvent(ctx, domain.Event{ID: r.idGen.NewID("evt"), Type: domain.EventRunStarted, TaskID: task.ID, AgentID: agent.ID, RunID: run.ID, Message: "Run started", CreatedAt: r.clock.Now()})

	runCtx, cancel := context.WithTimeout(ctx, r.config.TaskTimeout)
	defer cancel()

	memoryContext, err := r.memory.ContextForTask(runCtx, task, agent)
	if err != nil {
		memoryContext = ""
	}

	workspacePath := ""
	if task.WorkspaceID != "" {
		if workspace, workspaceErr := r.storage.Workspaces().Get(runCtx, task.WorkspaceID); workspaceErr == nil {
			workspacePath = workspace.RootPath
		}
	}

	var result ports.RuntimeExecutionResult
	if agent.Type == "noop" {
		result = ports.RuntimeExecutionResult{Output: "noop driver executed"}
	} else {
		lastRunStreamPersist := time.Time{}
		pendingRunStreamBytes := 0
		result, err = r.runtimes.Execute(runCtx, domain.RuntimeID(agent.Type), ports.RuntimeExecutionInput{
			Agent:          agent,
			Task:           task,
			Run:            run,
			Memory:         memoryContext,
			Config:         agent.Config,
			Env:            agent.Env,
			ExtraArgs:      agent.ExtraArgs,
			WorkspacePath:  workspacePath,
			RuntimeBaseURL: r.config.RuntimeBaseURL,
			OnStart: func(pid int) {
				run.ProcessID = pid
				_ = r.storage.Runs().Update(ctx, run)
			},
			OnOutput: func(stdout, stderr []byte) {
				now := r.clock.Now()
				run.Output = appendRunStream(run.Output, stdout)
				run.Error = appendRunStream(run.Error, stderr)
				run.LastOutputAt = &now
				pendingRunStreamBytes += len(stdout) + len(stderr)
				if lastRunStreamPersist.IsZero() || now.Sub(lastRunStreamPersist) >= 500*time.Millisecond || pendingRunStreamBytes >= 16*1024 {
					_ = r.storage.Runs().Update(ctx, run)
					lastRunStreamPersist = now
					pendingRunStreamBytes = 0
				}
				_ = r.bus.Publish(ctx, domain.Event{
					ID:        r.idGen.NewID("evt"),
					Type:      domain.EventRunOutput,
					TaskID:    task.ID,
					AgentID:   agent.ID,
					RunID:     run.ID,
					Message:   "Run output",
					Data:      map[string]string{"stdout": string(stdout), "stderr": string(stderr)},
					CreatedAt: now,
				})
			},
		})
	}
	finishedAt := r.clock.Now()
	run.FinishedAt = &finishedAt
	latest, latestErr := r.storage.Tasks().Get(ctx, task.ID)
	if latestErr == nil && latest.Status == domain.TaskStatusCancelled {
		r.logger.Warn("runner.task_canceled", "task_id", task.ID, "run_id", run.ID, "reason", latest.CancelReason)
		run.Status = domain.RunStatusCanceled
		run.Error = latest.CancelReason
		_ = r.storage.Runs().Update(ctx, run)
		r.emitEvent(ctx, domain.Event{ID: r.idGen.NewID("evt"), Type: domain.EventRunCanceled, TaskID: task.ID, AgentID: agent.ID, RunID: run.ID, Message: "Run canceled", CreatedAt: finishedAt})
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
		r.emitEvent(ctx, domain.Event{ID: r.idGen.NewID("evt"), Type: domain.EventRunFailed, TaskID: task.ID, AgentID: agent.ID, RunID: run.ID, Message: err.Error(), CreatedAt: finishedAt})
		if task.Mode == domain.TaskModeContinuous {
			if isRateLimitError(err.Error()) {
				r.scheduleContinuousRateLimitBackoff(ctx, task, run, finishedAt, err.Error())
			} else if isTransientProviderError(err.Error()) {
				r.scheduleContinuousTransientProviderBackoff(ctx, task, run, finishedAt, err.Error())
			} else if shouldPauseContinuousAfterRuntimeError(err.Error()) {
				r.pauseContinuousAfterRuntimeError(ctx, task, run, finishedAt, err.Error())
			} else {
				r.completeContinuousCycle(ctx, task, run, finishedAt)
			}
		} else {
			r.failTask(ctx, task, fmt.Sprintf("%v", err))
		}
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
		if latest.Mode == domain.TaskModeContinuous {
			r.completeContinuousCycle(ctx, latest, run, now)
		} else {
			if agent.Type == "noop" {
				latest.Status = domain.TaskStatusTodo
			} else {
				latest.Status = domain.TaskStatusInProgress
			}
			if latest.CheckoutRunID == run.ID {
				latest.CheckoutRunID = ""
				latest.CheckedOutByAgentID = ""
				latest.ExecutionLockedAt = nil
				latest.LockExpiresAt = nil
			}
			latest.UpdatedAt = now
			_ = r.storage.Tasks().Update(ctx, latest)
		}
	}
	r.emitEvent(ctx, domain.Event{ID: r.idGen.NewID("evt"), Type: domain.EventRunCompleted, TaskID: task.ID, AgentID: agent.ID, RunID: run.ID, Message: "Run completed", CreatedAt: now})
}

func (r *Runner) completeContinuousCycle(ctx context.Context, task domain.Task, run domain.Run, finishedAt time.Time) {
	latest, err := r.storage.Tasks().Get(ctx, task.ID)
	if err == nil {
		task = latest
	}

	if task.CheckoutRunID == run.ID || run.ID == "" {
		task.CheckoutRunID = ""
		task.CheckedOutByAgentID = ""
		task.ExecutionLockedAt = nil
		task.LockExpiresAt = nil
	}
	task.FinishedAt = &finishedAt
	task.UpdatedAt = finishedAt

	if task.Status == domain.TaskStatusCancelled || task.Status == domain.TaskStatusDone || task.Mode != domain.TaskModeContinuous {
		_ = r.storage.Tasks().Update(ctx, task)
		return
	}

	if task.LoopPausedAt != nil {
		task.Status = domain.TaskStatusWaitingNextCycle
		task.NeedsRun = false
		_ = r.storage.Tasks().Update(ctx, task)
		return
	}

	delay := task.LoopDelaySeconds
	if delay < 1 {
		delay = 60
		task.LoopDelaySeconds = delay
	}
	nextRunAt := finishedAt.Add(time.Duration(delay) * time.Second)
	task.Status = domain.TaskStatusWaitingNextCycle
	task.NeedsRun = false
	task.LoopRunCount++
	task.LoopNextRunAt = &nextRunAt

	_ = r.storage.Tasks().Update(ctx, task)
}

func isRateLimitError(message string) bool {
	message = strings.ToLower(message)
	markers := []string{
		"status\":429",
		"status 429",
		"429",
		"too many requests",
		"rate limit",
		"rate_limit",
	}
	for _, marker := range markers {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func rateLimitBackoffDuration(consecutiveFailures int) time.Duration {
	if consecutiveFailures < 1 {
		consecutiveFailures = 1
	}
	seconds := 6
	for i := 1; i < consecutiveFailures; i++ {
		seconds *= 3
		if seconds >= 7200 {
			return 2 * time.Hour
		}
	}
	return time.Duration(seconds) * time.Second
}

func isTransientProviderError(message string) bool {
	message = strings.ToLower(message)
	markers := []string{
		"internal_server_error",
		"internal server error",
		"status\":500",
		"status 500",
		"code\":500",
		"code 500",
		"bad gateway",
		"status\":502",
		"status 502",
		"service unavailable",
		"status\":503",
		"status 503",
		"gateway timeout",
		"status\":504",
		"status 504",
	}
	for _, marker := range markers {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func transientProviderBackoffDuration(consecutiveFailures int) time.Duration {
	if consecutiveFailures < 1 {
		consecutiveFailures = 1
	}
	minutes := 2
	for i := 1; i < consecutiveFailures; i++ {
		minutes *= 2
		if minutes >= 60 {
			return time.Hour
		}
	}
	return time.Duration(minutes) * time.Minute
}

func (r *Runner) consecutiveRateLimitFailures(ctx context.Context, taskID string) int {
	return r.consecutiveFailures(ctx, taskID, isRateLimitError)
}

func (r *Runner) consecutiveTransientProviderFailures(ctx context.Context, taskID string) int {
	return r.consecutiveFailures(ctx, taskID, isTransientProviderError)
}

func (r *Runner) consecutiveFailures(ctx context.Context, taskID string, match func(string) bool) int {
	runs, err := r.storage.Runs().ListByTask(ctx, taskID)
	if err != nil {
		return 1
	}
	count := 0
	for i := len(runs) - 1; i >= 0; i-- {
		run := runs[i]
		if run.Status == domain.RunStatusRunning {
			continue
		}
		if match(run.Error) {
			count++
			continue
		}
		break
	}
	if count < 1 {
		return 1
	}
	return count
}

func (r *Runner) scheduleContinuousRateLimitBackoff(ctx context.Context, task domain.Task, run domain.Run, finishedAt time.Time, message string) {
	r.scheduleContinuousBackoff(ctx, task, run, finishedAt, rateLimitBackoffDuration(r.consecutiveRateLimitFailures(ctx, task.ID)), "rate limit/429", message)
}

func (r *Runner) scheduleContinuousTransientProviderBackoff(ctx context.Context, task domain.Task, run domain.Run, finishedAt time.Time, message string) {
	failures := r.consecutiveTransientProviderFailures(ctx, task.ID)
	if failures >= 5 {
		r.pauseContinuousAfterRuntimeError(ctx, task, run, finishedAt, message)
		return
	}
	r.scheduleContinuousBackoff(ctx, task, run, finishedAt, transientProviderBackoffDuration(failures), "transient provider error", message)
}

func (r *Runner) scheduleContinuousBackoff(ctx context.Context, task domain.Task, run domain.Run, finishedAt time.Time, backoff time.Duration, reason, message string) {
	latest, err := r.storage.Tasks().Get(ctx, task.ID)
	if err == nil {
		task = latest
	}
	if task.CheckoutRunID == run.ID || run.ID == "" {
		task.CheckoutRunID = ""
		task.CheckedOutByAgentID = ""
		task.ExecutionLockedAt = nil
		task.LockExpiresAt = nil
	}
	nextRunAt := finishedAt.Add(backoff)
	task.Status = domain.TaskStatusWaitingNextCycle
	task.NeedsRun = false
	task.LoopNextRunAt = &nextRunAt
	task.FinishedAt = &finishedAt
	task.UpdatedAt = finishedAt
	_ = r.storage.Tasks().Update(ctx, task)
	body := fmt.Sprintf("Continuous task delayed after %s. Next attempt in %s. Error: %s", reason, backoff.String(), message)
	_ = r.storage.Messages().Create(ctx, domain.Message{ID: r.idGen.NewID("msg"), TaskID: task.ID, Role: domain.MessageRoleSystem, Body: body, CreatedAt: finishedAt})
	r.emitEvent(ctx, domain.Event{ID: r.idGen.NewID("evt"), Type: domain.EventTaskReleased, TaskID: task.ID, AgentID: task.AgentID, RunID: run.ID, Message: body, CreatedAt: finishedAt})
}

func shouldPauseContinuousAfterRuntimeError(message string) bool {
	message = strings.ToLower(message)
	markers := []string{
		"auth_unavailable",
		"no auth available",
		"unauthorized",
		"invalid api key",
		"quota",
		"insufficient_quota",
		"no such file or directory",
		"runtime unavailable",
		"start failed",
		"fork/exec",
	}
	for _, marker := range markers {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func (r *Runner) pauseContinuousAfterRuntimeError(ctx context.Context, task domain.Task, run domain.Run, finishedAt time.Time, message string) {
	latest, err := r.storage.Tasks().Get(ctx, task.ID)
	if err == nil {
		task = latest
	}
	if task.CheckoutRunID == run.ID || run.ID == "" {
		task.CheckoutRunID = ""
		task.CheckedOutByAgentID = ""
		task.ExecutionLockedAt = nil
		task.LockExpiresAt = nil
	}
	task.Status = domain.TaskStatusWaitingNextCycle
	task.NeedsRun = false
	task.LoopNextRunAt = nil
	task.LoopPausedAt = &finishedAt
	task.FinishedAt = &finishedAt
	task.UpdatedAt = finishedAt
	_ = r.storage.Tasks().Update(ctx, task)
	body := "Continuous task paused after runtime/provider error: " + message
	_ = r.storage.Messages().Create(ctx, domain.Message{ID: r.idGen.NewID("msg"), TaskID: task.ID, Role: domain.MessageRoleSystem, Body: body, CreatedAt: finishedAt})
	r.emitEvent(ctx, domain.Event{ID: r.idGen.NewID("evt"), Type: domain.EventTaskReleased, TaskID: task.ID, AgentID: task.AgentID, RunID: run.ID, Message: body, CreatedAt: finishedAt})
}

func (r *Runner) failTask(ctx context.Context, task domain.Task, message string) {
	now := r.clock.Now()
	task.Status = domain.TaskStatusBlocked
	task.NeedsRun = false
	task.CheckoutRunID = ""
	task.CheckedOutByAgentID = ""
	task.ExecutionLockedAt = nil
	task.LockExpiresAt = nil
	task.FinishedAt = nil
	task.UpdatedAt = now
	_ = r.storage.RunInTx(ctx, func(txCtx context.Context) error {
		if err := r.storage.Tasks().Update(txCtx, task); err != nil {
			return err
		}
		if err := r.storage.Messages().Create(txCtx, domain.Message{ID: r.idGen.NewID("msg"), TaskID: task.ID, FromID: task.AgentID, Role: domain.MessageRoleSystem, Body: "Run failed: " + message, CreatedAt: now}); err != nil {
			return err
		}
		ev := domain.Event{ID: r.idGen.NewID("evt"), Type: domain.EventTaskFailed, TaskID: task.ID, AgentID: task.AgentID, Message: message, CreatedAt: now}
		if err := r.storage.Events().Create(txCtx, ev); err != nil {
			return err
		}
		return r.storage.Events().CreateOutbox(txCtx, ev)
	})
}

func (r *Runner) maxAttemptsExceeded(task domain.Task) bool {
	limit := task.MaxAttempts
	if limit <= 0 && task.Mode == domain.TaskModeContinuous {
		return false
	}
	if limit <= 0 {
		limit = r.config.MaxAttempts
	}
	return limit > 0 && task.Attempts >= limit
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
	return fmt.Sprintf("Agent permissions: %s", joinPermissions(agent.Permissions))
}

func (r *Runner) manualSkillIDs(agent domain.Agent) map[string]struct{} {
	ids := make(map[string]struct{}, len(agent.SkillIDs))
	for _, id := range agent.SkillIDs {
		ids[id] = struct{}{}
	}
	return ids
}

func (r *Runner) skillCatalogContext(skills []domain.Skill) string {
	if len(skills) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("Available skills are not injected by default to save context. If you need full instructions, call GET /agent-api/skills and use the relevant skill only. Catalog:\n")
	limit := len(skills)
	if limit > 12 {
		limit = 12
	}
	for i := 0; i < limit; i++ {
		skill := skills[i]
		sb.WriteString(fmt.Sprintf("- %s (%s): %s\n", skill.Name, skill.ID, promptSnippet(skill.Description, 140)))
	}
	if len(skills) > limit {
		sb.WriteString(fmt.Sprintf("- ...and %d more available via GET /agent-api/skills\n", len(skills)-limit))
	}
	return strings.TrimSpace(sb.String())
}

func (r *Runner) skillContext(skill domain.Skill) string {
	parts := []string{fmt.Sprintf("Skill package: %s\n%s\nInstructions:\n%s", skill.Name, promptSnippet(skill.Description, 240), promptSnippet(strings.TrimSpace(skill.Instructions), 1200))}
	for _, file := range skill.Files {
		parts = append(parts, fmt.Sprintf("Skill file %s:\n%s", file.Path, promptSnippet(file.Content, 1200)))
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
		"Work on the task title, prompt, and latest user comment.",
		"Use /agent-api to checkout, comment, update status, delegate, or cancel when needed.",
		"Every run must leave a concise progress/final comment.",
		"For continuous tasks: make incremental progress, do not mark done unless the loop should permanently stop, and avoid duplicate delegations.",
	}, "\n")
}

const maxRunStreamBytes = 256 * 1024

func estimateTokens(text string) int {
	return len(strings.Fields(text)) * 4 / 3
}

func appendRunStream(current string, chunk []byte) string {
	if len(chunk) == 0 {
		return current
	}
	combined := current + string(chunk)
	if len(combined) <= maxRunStreamBytes {
		return combined
	}
	return combined[len(combined)-maxRunStreamBytes:]
}

func (r *Runner) recordTokenUsage(ctx context.Context, run domain.Run) {
	usage, err := r.storage.Usage().ListByTask(ctx, run.TaskID)
	if err == nil {
		for _, event := range usage {
			if event.RunID == run.ID {
				return
			}
		}
	}

	createdAt := r.clock.Now()
	if run.FinishedAt != nil {
		createdAt = *run.FinishedAt
	}
	usageID := r.idGen.NewID("usage")
	if run.ID != "" {
		usageID = "usage_" + run.ID
	}
	_ = r.storage.Usage().Create(ctx, domain.UsageEvent{
		ID:           usageID,
		RunID:        run.ID,
		TaskID:       run.TaskID,
		AgentID:      run.AgentID,
		Provider:     run.DriverType,
		InputTokens:  run.InputTokens,
		OutputTokens: run.OutputTokens,
		CostMicros:   0,
		CreatedAt:    createdAt,
	})

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

func explicitAgentQuestion(body string) string {
	body = strings.TrimSpace(body)
	markers := []string{"Pergunta para você:", "Pergunta ao usuário:", "User question:", "Question for user:"}
	for _, marker := range markers {
		idx := strings.Index(strings.ToLower(body), strings.ToLower(marker))
		if idx < 0 {
			continue
		}
		question := strings.TrimSpace(body[idx+len(marker):])
		if question == "" || !strings.Contains(question, "?") {
			return ""
		}
		return question
	}
	return ""
}

func userQuestions(messages []domain.Message) []domain.Message {
	questions := make([]domain.Message, 0)
	for _, message := range messages {
		if message.Role != domain.MessageRoleAgent && message.Role != domain.MessageRoleSystem {
			continue
		}
		if explicitAgentQuestion(message.Body) == "" {
			continue
		}
		questions = append(questions, message)
	}
	return questions
}

func formatTaskSummary(task domain.Task) string {
	parts := []string{fmt.Sprintf("%s (%s)", task.ID, task.Status)}
	if strings.TrimSpace(task.Title) != "" {
		parts = append(parts, task.Title)
	}
	if task.CheckoutRunID != "" {
		parts = append(parts, "running")
	}
	return strings.Join(parts, " · ")
}

func promptSnippet(value string, limit int) string {
	value = strings.TrimSpace(strings.Join(strings.Fields(value), " "))
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}

func latestMessageSnippet(messages []domain.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		body := promptSnippet(messages[i].Body, 220)
		if body != "" {
			return fmt.Sprintf("%s: %s", messages[i].Role, body)
		}
	}
	return ""
}

func (r *Runner) formatChildTaskForPrompt(ctx context.Context, child domain.Task) string {
	parts := []string{formatTaskSummary(child)}
	if child.Prompt != "" {
		parts = append(parts, "prompt: "+promptSnippet(child.Prompt, 180))
	}
	if runs, err := r.storage.Runs().ListByTask(ctx, child.ID); err == nil && len(runs) > 0 {
		latest := runs[len(runs)-1]
		runSummary := fmt.Sprintf("latest run: %s", latest.Status)
		if latest.Output != "" {
			runSummary += " output: " + promptSnippet(latest.Output, 220)
		} else if latest.Error != "" {
			runSummary += " error: " + promptSnippet(latest.Error, 220)
		}
		parts = append(parts, runSummary)
	}
	if messages, err := r.storage.Messages().ListByTask(ctx, child.ID); err == nil {
		if latest := latestMessageSnippet(messages); latest != "" {
			parts = append(parts, "latest message: "+latest)
		}
	}
	return strings.Join(parts, " | ")
}

func (r *Runner) taskProtocolContext(ctx context.Context, task domain.Task, run domain.Run, messages []domain.Message) string {
	var sb strings.Builder
	sb.WriteString(r.taskProtocolPrompt(ctx))
	sb.WriteString("\n\nCompact Task Context:\n")
	sb.WriteString(fmt.Sprintf("- Task ID: %s\n", task.ID))
	sb.WriteString(fmt.Sprintf("- Title: %s\n", task.Title))
	sb.WriteString(fmt.Sprintf("- Status: %s\n", string(task.Status)))
	sb.WriteString(fmt.Sprintf("- Run ID: %s\n", run.ID))
	if strings.TrimSpace(r.config.RuntimeBaseURL) != "" {
		sb.WriteString(fmt.Sprintf("- PicoClip API Base URL: %s\n", strings.TrimRight(r.config.RuntimeBaseURL, "/")))
	}
	if task.ParentID != "" {
		sb.WriteString(fmt.Sprintf("- Parent Task ID: %s\n", task.ParentID))
	}
	if task.Mode == domain.TaskModeContinuous {
		sb.WriteString("\nContinuous Task Rules:\n")
		sb.WriteString(fmt.Sprintf("- Cycle %d; next delay %ds.\n", task.LoopRunCount+1, task.LoopDelaySeconds))
		sb.WriteString("- Make incremental progress, report what changed, and do not mark done unless stopping permanently.\n")
		sb.WriteString("- If asking the user, use Portuguese marker 'Pergunta para você:' with 2-4 options and a safe default; otherwise keep working with safe assumptions.\n")
		sb.WriteString("- Check existing child tasks before delegating duplicates.\n")
	}

	children, _ := r.storage.Tasks().List(ctx, ports.TaskFilter{ParentID: task.ID})
	if len(children) > 0 {
		sb.WriteString("\nChild Tasks To Supervise:\n")
		sb.WriteString("Before delegating, compare the requested work with these child tasks. Update, wait for, or summarize existing children instead of creating duplicates.\n")
		limit := len(children) - 8
		if limit < 0 {
			limit = 0
		}
		for _, child := range children[limit:] {
			sb.WriteString("- " + r.formatChildTaskForPrompt(ctx, child) + "\n")
		}
	}

	questions := userQuestions(messages)
	if task.Mode == domain.TaskModeContinuous && len(questions) > 0 {
		sb.WriteString("\nOpen Questions Raised For User (non-blocking):\n")
		start := len(questions) - 5
		if start < 0 {
			start = 0
		}
		for _, question := range questions[start:] {
			sb.WriteString(fmt.Sprintf("[%s] %s\n", question.CreatedAt.Format("15:04"), explicitAgentQuestion(question.Body)))
		}
	}

	latestUserComment := latestCommentByRole(messages, domain.MessageRoleUser)
	if latestUserComment != "" {
		sb.WriteString("\nLatest User Comment:\n")
		sb.WriteString(promptSnippet(latestUserComment, 1200) + "\n")
	}

	sb.WriteString("\nRecent Comments Summary:\n")
	if len(messages) == 0 {
		sb.WriteString("(no comments)\n")
	} else {
		start := len(messages) - 5
		if start < 0 {
			start = 0
		}
		for _, m := range messages[start:] {
			sb.WriteString(fmt.Sprintf("[%s] %s: %s\n", m.CreatedAt.Format("15:04"), m.Role, promptSnippet(m.Body, 420)))
		}
	}

	return sb.String()
}
