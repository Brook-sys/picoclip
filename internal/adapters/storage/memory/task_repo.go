package memory

import (
	"context"
	"sort"
	"time"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

func (r taskRepository) Create(ctx context.Context, task domain.Task) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	r.storage.tasks[task.ID] = task
	return nil
}

func (r taskRepository) Get(ctx context.Context, id string) (domain.Task, error) {
	r.storage.mu.RLock()
	defer r.storage.mu.RUnlock()
	task, ok := r.storage.tasks[id]
	if !ok {
		return domain.Task{}, domain.ErrNotFound
	}
	return task, nil
}

func (r taskRepository) List(ctx context.Context, filter ports.TaskFilter) ([]domain.Task, error) {
	r.storage.mu.RLock()
	defer r.storage.mu.RUnlock()
	tasks := make([]domain.Task, 0)
	for _, task := range r.storage.tasks {
		if filter.AgentID != "" && task.AgentID != filter.AgentID {
			continue
		}
		if filter.ParentID != "" && task.ParentID != filter.ParentID {
			continue
		}
		if filter.WorkspaceID != "" && task.WorkspaceID != filter.WorkspaceID {
			continue
		}
		if filter.Status != "" && task.Status != filter.Status {
			continue
		}
		if len(filter.Statuses) > 0 && !taskStatusIn(task.Status, filter.Statuses) {
			continue
		}
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].CreatedAt.Before(tasks[j].CreatedAt) })
	return tasks, nil
}

func (r taskRepository) Update(ctx context.Context, task domain.Task) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	if _, ok := r.storage.tasks[task.ID]; !ok {
		return domain.ErrNotFound
	}
	r.storage.tasks[task.ID] = task
	return nil
}

func (r taskRepository) Delete(ctx context.Context, id string) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	if _, ok := r.storage.tasks[id]; !ok {
		return domain.ErrNotFound
	}
	deleteTaskData(r.storage, id)
	return nil
}

func (r taskRepository) DeleteFinished(ctx context.Context) (int, error) {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	ids := make([]string, 0)
	for _, task := range r.storage.tasks {
		if task.Status == domain.TaskStatusDone || task.Status == domain.TaskStatusCancelled {
			ids = append(ids, task.ID)
		}
	}
	for _, id := range ids {
		deleteTaskData(r.storage, id)
	}
	return len(ids), nil
}

func deleteTaskData(storage *Storage, id string) {
	delete(storage.tasks, id)
	for runID, run := range storage.runs {
		if run.TaskID == id {
			delete(storage.runs, runID)
		}
	}
	for messageID, message := range storage.messages {
		if message.TaskID == id {
			delete(storage.messages, messageID)
		}
	}
	for eventID, event := range storage.events {
		if event.TaskID == id {
			delete(storage.events, eventID)
		}
	}
	for wakeupID, wakeup := range storage.wakeups {
		if wakeup.TaskID == id {
			delete(storage.wakeups, wakeupID)
		}
	}
	for usageID, usage := range storage.usage {
		if usage.TaskID == id {
			delete(storage.usage, usageID)
		}
	}
}

func taskStatusIn(status domain.TaskStatus, statuses []domain.TaskStatus) bool {
	for _, candidate := range statuses {
		if candidate == status {
			return true
		}
	}
	return false
}

func (r taskRepository) ClaimNextPending(ctx context.Context) (domain.Task, error) {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	var selected *domain.Task
	now := time.Now()
	for _, task := range r.storage.tasks {
		if !taskRunnable(task, now) {
			continue
		}
		if selected == nil || task.Priority > selected.Priority || task.Priority == selected.Priority && task.CreatedAt.Before(selected.CreatedAt) {
			t := task
			selected = &t
		}
	}
	if selected == nil {
		return domain.Task{}, domain.ErrNoPendingTasks
	}
	task := *selected
	task.NeedsRun = false
	r.storage.tasks[task.ID] = task
	return task, nil
}

func (r taskRepository) ClaimNextRunnable(ctx context.Context, now time.Time, lockTTL time.Duration) (domain.Task, domain.Run, error) {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	var selected *domain.Task
	for _, task := range r.storage.tasks {
		if !taskRunnable(task, now) {
			continue
		}
		if selected == nil || task.Priority > selected.Priority || task.Priority == selected.Priority && task.CreatedAt.Before(selected.CreatedAt) {
			t := task
			selected = &t
		}
	}
	if selected == nil {
		return domain.Task{}, domain.Run{}, domain.ErrNoPendingTasks
	}
	task := *selected
	task.NeedsRun = false
	task.Status = domain.TaskStatusInProgress
	task.Attempts++
	runID := "run_" + task.ID + "_" + now.Format("20060102150405")
	task.CheckoutRunID = runID
	task.CheckedOutByAgentID = task.AgentID
	task.StartedAt = &now
	task.ExecutionLockedAt = &now
	expires := now.Add(lockTTL)
	task.LockExpiresAt = &expires
	task.UpdatedAt = now
	r.storage.tasks[task.ID] = task

	run := domain.Run{
		ID:           runID,
		TaskID:       task.ID,
		AgentID:      task.AgentID,
		DriverType:   "",
		Status:       domain.RunStatusRunning,
		Attempt:      task.Attempts,
		StartedAt:    now,
		LastOutputAt: &now,
		StallTimeout: int(lockTTL.Seconds()),
	}
	r.storage.runs[run.ID] = run
	return task, run, nil
}

func taskRunnable(task domain.Task, now time.Time) bool {
	if !task.NeedsRun || task.Status == domain.TaskStatusDone || task.Status == domain.TaskStatusCancelled {
		return false
	}
	if task.Mode == domain.TaskModeContinuous {
		if task.LoopPausedAt != nil {
			return false
		}
		if task.Status == domain.TaskStatusWaitingNextCycle && (task.LoopNextRunAt == nil || task.LoopNextRunAt.After(now)) {
			return false
		}
	}
	if task.CheckoutRunID != "" || task.CheckedOutByAgentID != "" {
		return false
	}
	if task.MaxAttempts > 0 && task.Attempts >= task.MaxAttempts {
		return false
	}
	return true
}
