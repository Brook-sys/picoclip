package memory

import (
	"context"
	"sort"

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
	for _, task := range r.storage.tasks {
		if !taskRunnable(task) {
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

func taskRunnable(task domain.Task) bool {
	if !task.NeedsRun || task.Status == domain.TaskStatusDone || task.Status == domain.TaskStatusCancelled {
		return false
	}
	if task.CheckoutRunID != "" || task.CheckedOutByAgentID != "" {
		return false
	}
	if task.MaxAttempts > 0 && task.Attempts >= task.MaxAttempts {
		return false
	}
	return true
}
