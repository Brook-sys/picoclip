package ports

import (
	"context"

	"picoclip/internal/core/domain"
)

type BackupData struct {
	Settings   map[string]string  `json:"settings"`
	Agents     []domain.Agent     `json:"agents"`
	Workspaces []domain.Workspace `json:"projects"`
	Skills     []domain.Skill     `json:"skills"`
	Tasks      []domain.Task      `json:"tasks"`
	Runs       []domain.Run       `json:"runs"`
	Messages   []domain.Message   `json:"messages"`
	Events     []domain.Event     `json:"events"`
}

type Storage interface {
	Agents() AgentRepository
	Tasks() TaskRepository
	Runs() RunRepository
	Events() EventRepository
	Messages() MessageRepository
	Skills() SkillRepository
	Workspaces() WorkspaceRepository
	Settings() SettingsRepository
	ResetAllData(ctx context.Context) error
	RestoreAllData(ctx context.Context, data BackupData) error
}

type SettingsRepository interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string) error
	List(ctx context.Context) (map[string]string, error)
}

type AgentRepository interface {
	Create(ctx context.Context, agent domain.Agent) error
	Get(ctx context.Context, id string) (domain.Agent, error)
	List(ctx context.Context) ([]domain.Agent, error)
	Update(ctx context.Context, agent domain.Agent) error
	Delete(ctx context.Context, id string) error
}

type SkillRepository interface {
	Create(ctx context.Context, skill domain.Skill) error
	Get(ctx context.Context, id string) (domain.Skill, error)
	List(ctx context.Context, projectID string) ([]domain.Skill, error)
	Update(ctx context.Context, skill domain.Skill) error
	Delete(ctx context.Context, id string) error
}

type WorkspaceRepository interface {
	Create(ctx context.Context, workspace domain.Workspace) error
	Get(ctx context.Context, id string) (domain.Workspace, error)
	List(ctx context.Context) ([]domain.Workspace, error)
	Update(ctx context.Context, workspace domain.Workspace) error
	Delete(ctx context.Context, id string) error
}

type TaskFilter struct {
	AgentID     string
	ParentID    string
	WorkspaceID string
	Status      domain.TaskStatus
	Statuses    []domain.TaskStatus
}

type TaskRepository interface {
	Create(ctx context.Context, task domain.Task) error
	Get(ctx context.Context, id string) (domain.Task, error)
	List(ctx context.Context, filter TaskFilter) ([]domain.Task, error)
	Update(ctx context.Context, task domain.Task) error
	ClaimNextPending(ctx context.Context) (domain.Task, error)
}

type RunRepository interface {
	Create(ctx context.Context, run domain.Run) error
	Get(ctx context.Context, id string) (domain.Run, error)
	ListByTask(ctx context.Context, taskID string) ([]domain.Run, error)
	Update(ctx context.Context, run domain.Run) error
}

type EventRepository interface {
	Create(ctx context.Context, event domain.Event) error
	ListByTask(ctx context.Context, taskID string) ([]domain.Event, error)
	ListRecent(ctx context.Context, limit int) ([]domain.Event, error)
}

type MessageRepository interface {
	Create(ctx context.Context, message domain.Message) error
	ListByTask(ctx context.Context, taskID string) ([]domain.Message, error)
}
