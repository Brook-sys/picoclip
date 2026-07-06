package memory

import (
	"context"
	"sort"
	"time"

	"picoclip/internal/core/domain"
)

func (r eventRepository) Create(ctx context.Context, event domain.Event) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	r.storage.events[event.ID] = event
	return nil
}

func (r eventRepository) CreateOutbox(ctx context.Context, event domain.Event) error {
	return nil
}

func (r eventRepository) ListOutbox(ctx context.Context, limit int) ([]domain.Event, error) {
	return nil, nil
}

func (r eventRepository) DeleteOutbox(ctx context.Context, id string) error {
	return nil
}

func (r eventRepository) MarkOutboxFailed(ctx context.Context, id string, message string, nextAttemptAt time.Time) error {
	return nil
}

func (r eventRepository) ListByTask(ctx context.Context, taskID string) ([]domain.Event, error) {
	r.storage.mu.RLock()
	defer r.storage.mu.RUnlock()
	events := make([]domain.Event, 0)
	for _, event := range r.storage.events {
		if event.TaskID == taskID {
			events = append(events, event)
		}
	}
	sort.Slice(events, func(i, j int) bool { return events[i].CreatedAt.Before(events[j].CreatedAt) })
	return events, nil
}

func (r eventRepository) ListRecent(ctx context.Context, limit int) ([]domain.Event, error) {
	r.storage.mu.RLock()
	defer r.storage.mu.RUnlock()
	events := make([]domain.Event, 0, len(r.storage.events))
	for _, event := range r.storage.events {
		events = append(events, event)
	}
	sort.Slice(events, func(i, j int) bool { return events[i].CreatedAt.After(events[j].CreatedAt) })
	if len(events) > limit {
		events = events[:limit]
	}
	return events, nil
}

func (r eventRepository) Delete(ctx context.Context, id string) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	if _, ok := r.storage.events[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.storage.events, id)
	return nil
}

func (r eventRepository) DeleteAll(ctx context.Context) (int, error) {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	count := len(r.storage.events)
	r.storage.events = make(map[string]domain.Event)
	return count, nil
}

func (r eventRepository) DeleteFinished(ctx context.Context) (int, error) {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	count := 0
	for id, event := range r.storage.events {
		if event.RunID != "" {
			if run, ok := r.storage.runs[event.RunID]; ok && run.Status == domain.RunStatusRunning {
				continue
			}
		}
		if event.TaskID != "" {
			if task, ok := r.storage.tasks[event.TaskID]; ok && task.Status == domain.TaskStatusInProgress {
				continue
			}
		}
		delete(r.storage.events, id)
		count++
	}
	return count, nil
}
