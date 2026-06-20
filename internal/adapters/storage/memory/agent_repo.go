package memory

import (
	"context"
	"sort"
	"sync"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

type Storage struct {
	mu         sync.RWMutex
	agents     map[string]domain.Agent
	tasks      map[string]domain.Task
	runs       map[string]domain.Run
	events     map[string]domain.Event
	messages   map[string]domain.Message
	skills     map[string]domain.Skill
	workspaces map[string]domain.Workspace
}

type agentRepository struct{ storage *Storage }
type taskRepository struct{ storage *Storage }
type runRepository struct{ storage *Storage }
type eventRepository struct{ storage *Storage }
type messageRepository struct{ storage *Storage }
type skillRepository struct{ storage *Storage }
type workspaceRepository struct{ storage *Storage }

func NewStorage() *Storage {
	return &Storage{
		agents:     make(map[string]domain.Agent),
		tasks:      make(map[string]domain.Task),
		runs:       make(map[string]domain.Run),
		events:     make(map[string]domain.Event),
		messages:   make(map[string]domain.Message),
		skills:     make(map[string]domain.Skill),
		workspaces: make(map[string]domain.Workspace),
	}
}

func (s *Storage) Agents() ports.AgentRepository         { return agentRepository{storage: s} }
func (s *Storage) Tasks() ports.TaskRepository           { return taskRepository{storage: s} }
func (s *Storage) Runs() ports.RunRepository             { return runRepository{storage: s} }
func (s *Storage) Events() ports.EventRepository         { return eventRepository{storage: s} }
func (s *Storage) Messages() ports.MessageRepository     { return messageRepository{storage: s} }
func (s *Storage) Skills() ports.SkillRepository         { return skillRepository{storage: s} }
func (s *Storage) Workspaces() ports.WorkspaceRepository { return workspaceRepository{storage: s} }

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
