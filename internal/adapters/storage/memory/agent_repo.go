package memory

import (
	"context"
	"sort"
	"sync"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

type Storage struct {
	mu               sync.RWMutex
	agents           map[string]domain.Agent
	tasks            map[string]domain.Task
	runs             map[string]domain.Run
	events           map[string]domain.Event
	runtimes         map[string]domain.RuntimeState
	messages         map[string]domain.Message
	skills           map[string]domain.Skill
	workspaces       map[string]domain.Workspace
	settings         map[string]string
	wakeups          map[string]domain.WakeupRequest
	usage            map[string]domain.UsageEvent
	budgets          map[string]domain.Budget
	webhooks         map[string]domain.WebhookSubscription
	deliveries       map[string]domain.WebhookDelivery
	completionAudits map[string]domain.CompletionAudit
}

type agentRepository struct{ storage *Storage }
type taskRepository struct{ storage *Storage }
type runRepository struct{ storage *Storage }
type eventRepository struct{ storage *Storage }
type messageRepository struct{ storage *Storage }
type skillRepository struct{ storage *Storage }
type workspaceRepository struct{ storage *Storage }
type settingsRepository struct{ storage *Storage }
type wakeupRepository struct{ storage *Storage }
type usageRepository struct{ storage *Storage }
type webhookRepository struct{ storage *Storage }
type completionAuditRepository struct{ storage *Storage }

func NewStorage() *Storage {
	return &Storage{
		agents:           make(map[string]domain.Agent),
		tasks:            make(map[string]domain.Task),
		runs:             make(map[string]domain.Run),
		events:           make(map[string]domain.Event),
		runtimes:         make(map[string]domain.RuntimeState),
		messages:         make(map[string]domain.Message),
		skills:           make(map[string]domain.Skill),
		workspaces:       make(map[string]domain.Workspace),
		settings:         make(map[string]string),
		wakeups:          make(map[string]domain.WakeupRequest),
		usage:            make(map[string]domain.UsageEvent),
		budgets:          make(map[string]domain.Budget),
		webhooks:         make(map[string]domain.WebhookSubscription),
		deliveries:       make(map[string]domain.WebhookDelivery),
		completionAudits: make(map[string]domain.CompletionAudit),
	}
}

func (s *Storage) Agents() ports.AgentRepository         { return agentRepository{storage: s} }
func (s *Storage) Tasks() ports.TaskRepository           { return taskRepository{storage: s} }
func (s *Storage) Runs() ports.RunRepository             { return runRepository{storage: s} }
func (s *Storage) Events() ports.EventRepository         { return eventRepository{storage: s} }
func (s *Storage) Messages() ports.MessageRepository     { return messageRepository{storage: s} }
func (s *Storage) Skills() ports.SkillRepository         { return skillRepository{storage: s} }
func (s *Storage) Workspaces() ports.WorkspaceRepository { return workspaceRepository{storage: s} }
func (s *Storage) Settings() ports.SettingsRepository    { return settingsRepository{storage: s} }
func (s *Storage) Wakeups() ports.WakeupRepository       { return wakeupRepository{storage: s} }
func (s *Storage) Usage() ports.UsageRepository          { return usageRepository{storage: s} }
func (s *Storage) Budgets() ports.BudgetRepository       { return budgetRepository{storage: s} }
func (s *Storage) Webhooks() ports.WebhookRepository     { return webhookRepository{storage: s} }
func (s *Storage) CompletionAudits() ports.CompletionAuditRepository {
	return completionAuditRepository{storage: s}
}

func (s *Storage) ResetAllData(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agents = make(map[string]domain.Agent)
	s.tasks = make(map[string]domain.Task)
	s.runs = make(map[string]domain.Run)
	s.events = make(map[string]domain.Event)
	s.runtimes = make(map[string]domain.RuntimeState)
	s.messages = make(map[string]domain.Message)
	s.skills = make(map[string]domain.Skill)
	s.workspaces = make(map[string]domain.Workspace)
	s.settings = make(map[string]string)
	s.wakeups = make(map[string]domain.WakeupRequest)
	s.usage = make(map[string]domain.UsageEvent)
	s.budgets = make(map[string]domain.Budget)
	s.webhooks = make(map[string]domain.WebhookSubscription)
	s.deliveries = make(map[string]domain.WebhookDelivery)
	s.completionAudits = make(map[string]domain.CompletionAudit)
	return nil
}

func (s *Storage) RestoreAllData(ctx context.Context, data ports.BackupData) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agents = make(map[string]domain.Agent)
	s.tasks = make(map[string]domain.Task)
	s.runs = make(map[string]domain.Run)
	s.events = make(map[string]domain.Event)
	s.runtimes = make(map[string]domain.RuntimeState)
	s.messages = make(map[string]domain.Message)
	s.skills = make(map[string]domain.Skill)
	s.workspaces = make(map[string]domain.Workspace)
	s.settings = make(map[string]string)
	s.wakeups = make(map[string]domain.WakeupRequest)
	s.usage = make(map[string]domain.UsageEvent)
	s.budgets = make(map[string]domain.Budget)
	s.webhooks = make(map[string]domain.WebhookSubscription)
	s.deliveries = make(map[string]domain.WebhookDelivery)
	s.completionAudits = make(map[string]domain.CompletionAudit)

	for k, v := range data.Settings {
		s.settings[k] = v
	}
	for _, x := range data.Agents {
		s.agents[x.ID] = x
	}
	for _, x := range data.Workspaces {
		s.workspaces[x.ID] = x
	}
	for _, x := range data.Skills {
		s.skills[x.ID] = x
	}
	for _, x := range data.Tasks {
		s.tasks[x.ID] = x
	}
	for _, x := range data.Runs {
		s.runs[x.ID] = x
	}
	for _, x := range data.Runtimes {
		s.runtimes[x.ID] = x
	}
	for _, x := range data.Messages {
		s.messages[x.ID] = x
	}
	for _, x := range data.Events {
		s.events[x.ID] = x
	}
	for _, x := range data.Wakeups {
		s.wakeups[x.ID] = x
	}
	for _, x := range data.Usage {
		s.usage[x.ID] = x
	}
	for _, x := range data.Budgets {
		s.budgets[x.ID] = x
	}
	for _, x := range data.Webhooks {
		s.webhooks[x.ID] = x
	}
	for _, x := range data.CompletionAudits {
		s.completionAudits[x.ID] = x
	}

	return nil
}

func (s *Storage) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}
func (r settingsRepository) Get(ctx context.Context, key string) (string, error) {
	r.storage.mu.RLock()
	defer r.storage.mu.RUnlock()
	val, ok := r.storage.settings[key]
	if !ok {
		return "", domain.ErrNotFound
	}
	return val, nil
}

func (r settingsRepository) Set(ctx context.Context, key string, value string) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	r.storage.settings[key] = value
	return nil
}

func (r settingsRepository) List(ctx context.Context) (map[string]string, error) {
	r.storage.mu.RLock()
	defer r.storage.mu.RUnlock()
	m := make(map[string]string, len(r.storage.settings))
	for k, v := range r.storage.settings {
		m[k] = v
	}
	return m, nil
}

func (r agentRepository) Create(ctx context.Context, agent domain.Agent) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	r.storage.agents[agent.ID] = agent
	return nil
}

func (r agentRepository) Get(ctx context.Context, id string) (domain.Agent, error) {
	r.storage.mu.RLock()
	defer r.storage.mu.RUnlock()
	agent, ok := r.storage.agents[id]
	if !ok {
		return domain.Agent{}, domain.ErrNotFound
	}
	return agent, nil
}

func (r agentRepository) List(ctx context.Context) ([]domain.Agent, error) {
	r.storage.mu.RLock()
	defer r.storage.mu.RUnlock()
	agents := make([]domain.Agent, 0, len(r.storage.agents))
	for _, agent := range r.storage.agents {
		agents = append(agents, agent)
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].CreatedAt.Before(agents[j].CreatedAt) })
	return agents, nil
}

func (r agentRepository) Update(ctx context.Context, agent domain.Agent) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	if _, ok := r.storage.agents[agent.ID]; !ok {
		return domain.ErrNotFound
	}
	r.storage.agents[agent.ID] = agent
	return nil
}

func (r agentRepository) Delete(ctx context.Context, id string) error {
	r.storage.mu.Lock()
	defer r.storage.mu.Unlock()
	if _, ok := r.storage.agents[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.storage.agents, id)
	return nil
}
