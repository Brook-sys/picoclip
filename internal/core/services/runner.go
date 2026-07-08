package services

import (
	"context"
	"errors"
	"fmt"
	"strconv"
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

func (r *Runner) emitRuntimeEvent(ctx context.Context, eventType domain.EventType, task domain.Task, agent domain.Agent, run domain.Run, message string, data map[string]string, now time.Time) {
	if data == nil {
		data = map[string]string{}
	}
	if _, ok := data["runtime_id"]; !ok {
		data["runtime_id"] = string(agent.Type)
	}
	r.emitEvent(ctx, domain.Event{ID: r.idGen.NewID("evt"), Type: eventType, TaskID: task.ID, AgentID: agent.ID, RunID: run.ID, Message: message, Data: data, CreatedAt: now})
}

func retryClassificationData(classification, retryable, reason string) map[string]string {
	return map[string]string{"classification": classification, "retryable": retryable, "reason": reason}
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
			classification := retryClassificationData("non_retryable", "false", "runtime_unavailable")
			r.emitEvent(ctx, domain.Event{ID: r.idGen.NewID("evt"), Type: domain.EventDriverMissing, TaskID: task.ID, AgentID: agent.ID, Message: "Runtime unavailable", Data: classification, CreatedAt: r.clock.Now()})
			r.failTaskWithData(ctx, task, "runtime unavailable", classification)
			return
		}
	}

	prompt, err := NewPromptBuilder(r.storage).Build(ctx, PromptBuildInput{Agent: agent, Task: task, Run: run})
	if err != nil {
		r.failTask(ctx, task, "prompt build failed: "+err.Error())
		return
	}
	task.Prompt = prompt
	run.Input = task.Prompt
	run.InputTokens = estimateTokens(run.Input)
	run.TotalTokens = run.InputTokens
	_ = r.storage.Runs().Update(ctx, run)

	r.logger.Debug("runner.run_started", "task_id", task.ID, "agent_id", agent.ID, "run_id", run.ID, "runtime", agent.Type)
	r.emitEvent(ctx, domain.Event{ID: r.idGen.NewID("evt"), Type: domain.EventRunStarted, TaskID: task.ID, AgentID: agent.ID, RunID: run.ID, Message: "Run started", CreatedAt: r.clock.Now()})
	r.emitRuntimeEvent(ctx, domain.EventRuntimeStarted, task, agent, run, "Runtime execution started", map[string]string{"phase": "started"}, r.clock.Now())

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
		result, err = r.runtimes.Execute(runCtx, domain.RuntimeID(agent.Type), ports.RuntimeExecutionInput{
			Agent:     agent,
			Task:      task,
			Run:       run,
			Memory:    memoryContext,
			Config:    agent.Config,
			Env:       agent.Env,
			ExtraArgs: agent.ExtraArgs,
			OnStart: func(pid int) {
				run.ProcessID = pid
				_ = r.storage.Runs().Update(ctx, run)
				r.emitRuntimeEvent(ctx, domain.EventRuntimeProcessStarted, task, agent, run, "Runtime process started", map[string]string{"phase": "process_started", "pid": strconv.Itoa(pid)}, r.clock.Now())
			},
			OnOutput: func(stdout, stderr []byte) {
				now := r.clock.Now()
				run.Output = appendRunStream(run.Output, stdout)
				run.Error = appendRunStream(run.Error, stderr)
				run.LastOutputAt = &now
				_ = r.storage.Runs().Update(ctx, run)
				r.emitRuntimeEvent(ctx, domain.EventRuntimeHeartbeat, task, agent, run, "Runtime output heartbeat", map[string]string{"phase": "output_heartbeat", "stdout_bytes": strconv.Itoa(len(stdout)), "stderr_bytes": strconv.Itoa(len(stderr))}, now)
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
		classification := retryClassificationData("unknown", "unknown", "runtime_error")
		if run.Status == domain.RunStatusTimeout {
			classification = retryClassificationData("retryable", "true", "runtime_timeout")
		}
		r.emitEvent(ctx, domain.Event{ID: r.idGen.NewID("evt"), Type: domain.EventRunFailed, TaskID: task.ID, AgentID: agent.ID, RunID: run.ID, Message: err.Error(), Data: classification, CreatedAt: finishedAt})
		if run.Status == domain.RunStatusTimeout {
			data := retryClassificationData("retryable", "true", "runtime_timeout")
			data["phase"] = "timeout_handled"
			data["status"] = string(run.Status)
			r.emitRuntimeEvent(ctx, domain.EventRuntimeTimeout, task, agent, run, "Runtime execution timed out", data, finishedAt)
		} else {
			data := retryClassificationData("unknown", "unknown", "runtime_error")
			data["phase"] = "completed"
			data["status"] = string(run.Status)
			r.emitRuntimeEvent(ctx, domain.EventRuntimeCompleted, task, agent, run, "Runtime execution failed", data, finishedAt)
		}
		if task.Mode == domain.TaskModeContinuous {
			r.completeContinuousCycle(ctx, task, run, finishedAt)
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
	r.emitRuntimeEvent(ctx, domain.EventRuntimeCompleted, task, agent, run, "Runtime execution completed", map[string]string{"phase": "completed", "status": string(run.Status)}, now)
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

func (r *Runner) failTask(ctx context.Context, task domain.Task, message string) {
	r.failTaskWithData(ctx, task, message, nil)
}

func (r *Runner) failTaskWithData(ctx context.Context, task domain.Task, message string, data map[string]string) {
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
		ev := domain.Event{ID: r.idGen.NewID("evt"), Type: domain.EventTaskFailed, TaskID: task.ID, AgentID: task.AgentID, Message: message, Data: data, CreatedAt: now}
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
		"7. For continuous tasks, do not mark done just because one heartbeat completed; report progress and let PicoClip schedule the next cycle.",
		"8. Do not stay silent. Every run must leave a final comment or status update.",
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

func latestCommentByRole(messages []domain.Message, role domain.MessageRole) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == role && strings.TrimSpace(messages[i].Body) != "" {
			return messages[i].Body
		}
	}
	return ""
}

func userQuestions(messages []domain.Message) []domain.Message {
	questions := make([]domain.Message, 0)
	for _, message := range messages {
		if message.Role != domain.MessageRoleAgent && message.Role != domain.MessageRoleSystem {
			continue
		}
		body := strings.TrimSpace(message.Body)
		if body == "" || !strings.Contains(body, "?") {
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
