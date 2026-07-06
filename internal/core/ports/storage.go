package ports

import (
	"context"
	"time"

	"picoclip/internal/core/domain"
)

type BackupData struct {
	Settings   map[string]string            `json:"settings"`
	Agents     []domain.Agent               `json:"agents"`
	Workspaces []domain.Workspace           `json:"projects"`
	Skills     []domain.Skill               `json:"skills"`
	Tasks      []domain.Task                `json:"tasks"`
	Runs       []domain.Run                 `json:"runs"`
	Runtimes   []domain.RuntimeState        `json:"runtimes"`
	Messages   []domain.Message             `json:"messages"`
	Events     []domain.Event               `json:"events"`
	Wakeups    []domain.WakeupRequest       `json:"wakeups"`
	Usage      []domain.UsageEvent          `json:"usage"`
	Budgets    []domain.Budget              `json:"budgets"`
	Webhooks   []domain.WebhookSubscription `json:"webhooks"`
}

type Storage interface {
	Agents() AgentRepository
	Tasks() TaskRepository
	Runs() RunRepository
	Runtimes() RuntimeRepository
	Events() EventRepository
	Messages() MessageRepository
	Skills() SkillRepository
	Workspaces() WorkspaceRepository
	Settings() SettingsRepository
	Wakeups() WakeupRepository
	Usage() UsageRepository
	Budgets() BudgetRepository
	Webhooks() WebhookRepository
	ResetAllData(ctx context.Context) error
	RestoreAllData(ctx context.Context, data BackupData) error
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
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
	Delete(ctx context.Context, id string) error
	DeleteFinished(ctx context.Context) (int, error)
	ClaimNextPending(ctx context.Context) (domain.Task, error)
	ClaimNextRunnable(ctx context.Context, now time.Time, lockTTL time.Duration) (domain.Task, domain.Run, error)
}

type RunRepository interface {
	Create(ctx context.Context, run domain.Run) error
	Get(ctx context.Context, id string) (domain.Run, error)
	ListByTask(ctx context.Context, taskID string) ([]domain.Run, error)
	ListRunning(ctx context.Context) ([]domain.Run, error)
	Update(ctx context.Context, run domain.Run) error
	Delete(ctx context.Context, id string) error
	DeleteFinished(ctx context.Context) (int, error)
}

type WakeupRepository interface {
	Create(ctx context.Context, wakeup domain.WakeupRequest) error
	Get(ctx context.Context, id string) (domain.WakeupRequest, error)
	ListPending(ctx context.Context, now time.Time, limit int) ([]domain.WakeupRequest, error)
	ListByTask(ctx context.Context, taskID string) ([]domain.WakeupRequest, error)
	Update(ctx context.Context, wakeup domain.WakeupRequest) error
}

type EventRepository interface {
	Create(ctx context.Context, event domain.Event) error
	CreateOutbox(ctx context.Context, event domain.Event) error
	ListOutbox(ctx context.Context, limit int) ([]domain.Event, error)
	DeleteOutbox(ctx context.Context, id string) error
	MarkOutboxFailed(ctx context.Context, id string, message string, nextAttemptAt time.Time) error
	ListByTask(ctx context.Context, taskID string) ([]domain.Event, error)
	ListRecent(ctx context.Context, limit int) ([]domain.Event, error)
	Delete(ctx context.Context, id string) error
	DeleteFinished(ctx context.Context) (int, error)
	DeleteAll(ctx context.Context) (int, error)
}

type MessageRepository interface {
	Create(ctx context.Context, message domain.Message) error
	ListByTask(ctx context.Context, taskID string) ([]domain.Message, error)
}

type UsageRepository interface {
	Create(ctx context.Context, event domain.UsageEvent) error
	List(ctx context.Context) ([]domain.UsageEvent, error)
	ListByTask(ctx context.Context, taskID string) ([]domain.UsageEvent, error)
	SumByAgent(ctx context.Context, agentID string) (input, output, cached int, costMicros int64, err error)
}

type BudgetRepository interface {
	Create(ctx context.Context, budget domain.Budget) error
	Get(ctx context.Context, id string) (domain.Budget, error)
	List(ctx context.Context) ([]domain.Budget, error)
	Update(ctx context.Context, budget domain.Budget) error
	Delete(ctx context.Context, id string) error
}

type WebhookRepository interface {
	CreateSubscription(ctx context.Context, subscription domain.WebhookSubscription) error
	GetSubscription(ctx context.Context, id string) (domain.WebhookSubscription, error)
	ListSubscriptions(ctx context.Context) ([]domain.WebhookSubscription, error)
	UpdateSubscription(ctx context.Context, subscription domain.WebhookSubscription) error
	DeleteSubscription(ctx context.Context, id string) error
	CreateDelivery(ctx context.Context, delivery domain.WebhookDelivery) error
	GetDelivery(ctx context.Context, id string) (domain.WebhookDelivery, error)
	ListDueDeliveries(ctx context.Context, now time.Time, limit int) ([]domain.WebhookDelivery, error)
	UpdateDelivery(ctx context.Context, delivery domain.WebhookDelivery) error
	ListDeliveries(ctx context.Context, subscriptionID string, limit int) ([]domain.WebhookDelivery, error)
}
