package memory

import (
	"context"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

type runtimeRepository struct{ storage *Storage }

func (s *Storage) Runtimes() ports.RuntimeRepository { return runtimeRepository{storage: s} }

func (r runtimeRepository) GetByRuntimeID(ctx context.Context, runtimeID domain.RuntimeID) (domain.RuntimeState, error) {
	r.storage.mu.RLock()
	defer r.storage.mu.RUnlock()
	for _, state := range r.storage.runtimes {
		if state.RuntimeID == runtimeID {
			return state, nil
		}
	}
	return domain.RuntimeState{}, domain.ErrNotFound
}

func (r runtimeRepository) List(ctx context.Context) ([]domain.RuntimeState, error) {
	r.storage.mu.RLock()
	defer r.storage.mu.RUnlock()
	var res []domain.RuntimeState
	for _, state := range r.storage.runtimes {
		res = append(res, state)
	}
	return res, nil
}

func (r runtimeRepository) Save(ctx context.Context, state domain.RuntimeState) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	r.storage.runtimes[state.ID] = state
	return nil
}

func (r runtimeRepository) Delete(ctx context.Context, id string) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	delete(r.storage.runtimes, id)
	return nil
}
