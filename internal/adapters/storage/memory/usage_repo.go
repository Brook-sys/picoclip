package memory

import (
	"context"

	"picoclip/internal/core/domain"
)

func (r usageRepository) Create(ctx context.Context, event domain.UsageEvent) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	if event.RunID != "" {
		for _, existing := range r.storage.usage {
			if existing.RunID == event.RunID {
				return nil
			}
		}
	}
	r.storage.usage[event.ID] = event
	return nil
}

func (r usageRepository) List(ctx context.Context) ([]domain.UsageEvent, error) {
	r.storage.mu.RLock()
	defer r.storage.mu.RUnlock()
	events := make([]domain.UsageEvent, 0, len(r.storage.usage))
	for _, ev := range r.storage.usage {
		events = append(events, ev)
	}
	return events, nil
}

func (r usageRepository) ListByTask(ctx context.Context, taskID string) ([]domain.UsageEvent, error) {
	r.storage.mu.RLock()
	defer r.storage.mu.RUnlock()
	events := make([]domain.UsageEvent, 0)
	for _, ev := range r.storage.usage {
		if ev.TaskID == taskID {
			events = append(events, ev)
		}
	}
	return events, nil
}

func (r usageRepository) SumByAgent(ctx context.Context, agentID string) (input, output, cached int, costMicros int64, err error) {
	r.storage.mu.RLock()
	defer r.storage.mu.RUnlock()
	for _, ev := range r.storage.usage {
		if ev.AgentID == agentID {
			input += ev.InputTokens
			output += ev.OutputTokens
			cached += ev.CachedTokens
			costMicros += ev.CostMicros
		}
	}
	return
}

func (r usageRepository) DeleteByTask(ctx context.Context, taskID string) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	for id, event := range r.storage.usage {
		if event.TaskID == taskID {
			delete(r.storage.usage, id)
		}
	}
	return nil
}

func (r usageRepository) DeleteHistory(ctx context.Context) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	r.storage.usage = make(map[string]domain.UsageEvent)
	return nil
}
