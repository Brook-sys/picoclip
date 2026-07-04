package memory

import (
	"context"
	"sort"
	"time"

	"picoclip/internal/core/domain"
)

func (r wakeupRepository) Create(ctx context.Context, wakeup domain.WakeupRequest) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	r.storage.wakeups[wakeup.ID] = wakeup
	return nil
}

func (r wakeupRepository) Get(ctx context.Context, id string) (domain.WakeupRequest, error) {
	r.storage.mu.RLock()
	defer r.storage.mu.RUnlock()
	wakeup, ok := r.storage.wakeups[id]
	if !ok {
		return domain.WakeupRequest{}, domain.ErrNotFound
	}
	return wakeup, nil
}

func (r wakeupRepository) ListPending(ctx context.Context, now time.Time, limit int) ([]domain.WakeupRequest, error) {
	r.storage.mu.RLock()
	defer r.storage.mu.RUnlock()
	wakeups := make([]domain.WakeupRequest, 0)
	for _, wakeup := range r.storage.wakeups {
		if wakeup.Status != domain.WakeupStatusPending || wakeup.DueAt.After(now) {
			continue
		}
		wakeups = append(wakeups, wakeup)
	}
	sort.Slice(wakeups, func(i, j int) bool {
		if wakeups[i].Priority == wakeups[j].Priority {
			return wakeups[i].DueAt.Before(wakeups[j].DueAt)
		}
		return wakeups[i].Priority > wakeups[j].Priority
	})
	if limit > 0 && len(wakeups) > limit {
		wakeups = wakeups[:limit]
	}
	return wakeups, nil
}

func (r wakeupRepository) ListByTask(ctx context.Context, taskID string) ([]domain.WakeupRequest, error) {
	r.storage.mu.RLock()
	defer r.storage.mu.RUnlock()
	wakeups := make([]domain.WakeupRequest, 0)
	for _, wakeup := range r.storage.wakeups {
		if wakeup.TaskID == taskID {
			wakeups = append(wakeups, wakeup)
		}
	}
	sort.Slice(wakeups, func(i, j int) bool { return wakeups[i].CreatedAt.After(wakeups[j].CreatedAt) })
	return wakeups, nil
}

func (r wakeupRepository) Update(ctx context.Context, wakeup domain.WakeupRequest) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	if _, ok := r.storage.wakeups[wakeup.ID]; !ok {
		return domain.ErrNotFound
	}
	r.storage.wakeups[wakeup.ID] = wakeup
	return nil
}
