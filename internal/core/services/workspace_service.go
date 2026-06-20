package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

type WorkspaceService struct {
	storage ports.Storage
	clock   ports.Clock
	idGen   ports.IDGenerator
	baseDir string
}

func NewWorkspaceService(storage ports.Storage, clock ports.Clock, idGen ports.IDGenerator, baseDir string) *WorkspaceService {
	return &WorkspaceService{storage: storage, clock: clock, idGen: idGen, baseDir: baseDir}
}

func (s *WorkspaceService) Create(ctx context.Context, name, description string) (domain.Workspace, error) {
	if name == "" {
		return domain.Workspace{}, fmt.Errorf("%w: name is required", domain.ErrInvalidInput)
	}
	now := s.clock.Now()
	id := s.idGen.NewID("prj")
	rootPath := filepath.Join(s.baseDir, id)
	if err := os.MkdirAll(rootPath, 0o755); err != nil {
		return domain.Workspace{}, err
	}
	workspace := domain.Workspace{ID: id, Name: name, Description: description, RootPath: rootPath, CreatedAt: now, UpdatedAt: now}
	if err := s.storage.Workspaces().Create(ctx, workspace); err != nil {
		return domain.Workspace{}, err
	}
	return workspace, nil
}

func (s *WorkspaceService) EnsureDefault(ctx context.Context) (domain.Workspace, error) {
	workspaces, err := s.List(ctx)
	if err != nil {
		return domain.Workspace{}, err
	}
	if len(workspaces) > 0 {
		return workspaces[0], nil
	}
	return s.Create(ctx, "Default", "Workspace padrão para agentes e tarefas locais")
}

func (s *WorkspaceService) List(ctx context.Context) ([]domain.Workspace, error) {
	return s.storage.Workspaces().List(ctx)
}

func (s *WorkspaceService) Get(ctx context.Context, id string) (domain.Workspace, error) {
	return s.storage.Workspaces().Get(ctx, id)
}

func (s *WorkspaceService) Delete(ctx context.Context, id string) error {
	return s.storage.Workspaces().Delete(ctx, id)
}
