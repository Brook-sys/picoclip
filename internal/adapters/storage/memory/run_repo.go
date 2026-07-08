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

func (r runRepository) DeleteByTask(ctx context.Context, taskID string) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	for id, run := range r.storage.runs {
		if run.TaskID == taskID {
			delete(r.storage.runs, id)
		}
	}
	return nil
}

func (r runRepository) DeleteHistory(ctx context.Context) (int, error) {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	deleted := 0
	for id, run := range r.storage.runs {
		if run.Status == domain.RunStatusRunning {
			continue
		}
		delete(r.storage.runs, id)
		deleted++
	}
	return deleted, nil
}
