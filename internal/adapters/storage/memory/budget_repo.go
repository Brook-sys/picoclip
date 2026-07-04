package memory

import (
	"context"
	"sort"

	"picoclip/internal/core/domain"
)

type budgetRepository struct{ storage *Storage }

func (r budgetRepository) Create(ctx context.Context, budget domain.Budget) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	r.storage.budgets[budget.ID] = budget
	return nil
}

func (r budgetRepository) Get(ctx context.Context, id string) (domain.Budget, error) {
	r.storage.mu.RLock()
	defer r.storage.mu.RUnlock()
	budget, ok := r.storage.budgets[id]
	if !ok {
		return domain.Budget{}, domain.ErrNotFound
	}
	return budget, nil
}

func (r budgetRepository) List(ctx context.Context) ([]domain.Budget, error) {
	r.storage.mu.RLock()
	defer r.storage.mu.RUnlock()
	budgets := make([]domain.Budget, 0, len(r.storage.budgets))
	for _, budget := range r.storage.budgets {
		budgets = append(budgets, budget)
	}
	sort.Slice(budgets, func(i, j int) bool { return budgets[i].CreatedAt.Before(budgets[j].CreatedAt) })
	return budgets, nil
}

func (r budgetRepository) Update(ctx context.Context, budget domain.Budget) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	if _, ok := r.storage.budgets[budget.ID]; !ok {
		return domain.ErrNotFound
	}
	r.storage.budgets[budget.ID] = budget
	return nil
}

func (r budgetRepository) Delete(ctx context.Context, id string) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	delete(r.storage.budgets, id)
	return nil
}
