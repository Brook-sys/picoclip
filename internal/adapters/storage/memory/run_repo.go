package memory

import (
	"context"
	"sort"

	"picoclip/internal/core/domain"
)

func (r runRepository) Create(ctx context.Context, run domain.Run) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	r.storage.runs[run.ID] = run
	return nil
}

func (r runRepository) Get(ctx context.Context, id string) (domain.Run, error) {
	r.storage.mu.RLock()
	defer r.storage.mu.RUnlock()
	run, ok := r.storage.runs[id]
	if !ok {
		return domain.Run{}, domain.ErrNotFound
	}
	return run, nil
}

func (r runRepository) ListRunning(ctx context.Context) ([]domain.Run, error) {
	r.storage.mu.RLock()
	defer r.storage.mu.RUnlock()
	runs := make([]domain.Run, 0)
	for _, run := range r.storage.runs {
		if run.Status == domain.RunStatusRunning {
			runs = append(runs, run)
		}
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].StartedAt.Before(runs[j].StartedAt) })
	return runs, nil
}

func (r runRepository) ListByTask(ctx context.Context, taskID string) ([]domain.Run, error) {
	r.storage.mu.RLock()
	defer r.storage.mu.RUnlock()
	runs := make([]domain.Run, 0)
	for _, run := range r.storage.runs {
		if run.TaskID == taskID {
			runs = append(runs, run)
		}
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].StartedAt.Before(runs[j].StartedAt) })
	return runs, nil
}

func (r runRepository) Update(ctx context.Context, run domain.Run) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	if _, ok := r.storage.runs[run.ID]; !ok {
		return domain.ErrNotFound
	}
	r.storage.runs[run.ID] = run
	return nil
}

func (r runRepository) Delete(ctx context.Context, id string) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	if _, ok := r.storage.runs[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.storage.runs, id)
	for eventID, event := range r.storage.events {
		if event.RunID == id {
			delete(r.storage.events, eventID)
		}
	}
	for usageID, usage := range r.storage.usage {
		if usage.RunID == id {
			delete(r.storage.usage, usageID)
		}
	}
	return nil
}

func (r runRepository) DeleteFinished(ctx context.Context) (int, error) {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	count := 0
	for id, run := range r.storage.runs {
		if run.Status == domain.RunStatusRunning {
			continue
		}
		delete(r.storage.runs, id)
		for eventID, event := range r.storage.events {
			if event.RunID == id {
				delete(r.storage.events, eventID)
			}
		}
		for usageID, usage := range r.storage.usage {
			if usage.RunID == id {
				delete(r.storage.usage, usageID)
			}
		}
		count++
	}
	return count, nil
}
