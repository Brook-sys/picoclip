package memory

import (
	"context"
	"sort"

	"picoclip/internal/core/domain"
)

func (r workspaceRepository) Create(ctx context.Context, workspace domain.Workspace) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	r.storage.workspaces[workspace.ID] = workspace
	return nil
}

func (r workspaceRepository) Get(ctx context.Context, id string) (domain.Workspace, error) {
	r.storage.mu.RLock()
	defer r.storage.mu.RUnlock()
	workspace, ok := r.storage.workspaces[id]
	if !ok {
		return domain.Workspace{}, domain.ErrNotFound
	}
	return workspace, nil
}

func (r workspaceRepository) List(ctx context.Context) ([]domain.Workspace, error) {
	r.storage.mu.RLock()
	defer r.storage.mu.RUnlock()
	workspaces := make([]domain.Workspace, 0, len(r.storage.workspaces))
	for _, workspace := range r.storage.workspaces {
		workspaces = append(workspaces, workspace)
	}
	sort.Slice(workspaces, func(i, j int) bool { return workspaces[i].CreatedAt.Before(workspaces[j].CreatedAt) })
	return workspaces, nil
}

func (r workspaceRepository) Update(ctx context.Context, workspace domain.Workspace) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	if _, ok := r.storage.workspaces[workspace.ID]; !ok {
		return domain.ErrNotFound
	}
	r.storage.workspaces[workspace.ID] = workspace
	return nil
}

func (r workspaceRepository) Delete(ctx context.Context, id string) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	if _, ok := r.storage.workspaces[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.storage.workspaces, id)
	return nil
}
