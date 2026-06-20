package services

import (
	"context"
	"fmt"
	"strings"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

type AgentService struct {
	storage ports.Storage
	clock   ports.Clock
	idGen   ports.IDGenerator
}

func NewAgentService(storage ports.Storage, clock ports.Clock, idGen ports.IDGenerator) *AgentService {
	return &AgentService{
		storage: storage,
		clock:   clock,
		idGen:   idGen,
	}
}

type CreateAgentInput struct {
	ProjectID       string
	Name            string
	Title           string
	ReportsToID     string
	Tags            []string
	Type            domain.AgentType
	Description     string
	SystemPrompt    string
	InstructionFile string
	SkillIDs        []string
	Config          map[string]string
	Env             map[string]string
	ExtraArgs       []string
	Capability      domain.AgentCapability
	Permissions     []domain.AgentPermission
}

func (s *AgentService) Create(ctx context.Context, name string, agentType domain.AgentType) (domain.Agent, error) {
	return s.CreateFull(ctx, CreateAgentInput{Name: name, Type: agentType})
}

func (s *AgentService) CreateWithPermissions(ctx context.Context, projectID, name string, agentType domain.AgentType, description string, capability domain.AgentCapability, permissions []domain.AgentPermission) (domain.Agent, error) {
	return s.CreateFull(ctx, CreateAgentInput{
		ProjectID:   projectID,
		Name:        name,
		Type:        agentType,
		Description: description,
		Capability:  capability,
		Permissions: permissions,
	})
}

func (s *AgentService) CreateFull(ctx context.Context, input CreateAgentInput) (domain.Agent, error) {
	if input.Name == "" || input.Type == "" {
		return domain.Agent{}, fmt.Errorf("%w: name and type are required", domain.ErrInvalidInput)
	}
	if input.Capability == "" {
		input.Capability = domain.CapabilityWorker
	}
	if len(input.Permissions) == 0 {
		input.Permissions = PermissionsForPreset("executor")
	}

	now := s.clock.Now()
	agent := domain.Agent{
		ID:              s.idGen.NewID("ag"),
		ProjectID:       input.ProjectID,
		Name:            input.Name,
		Title:           input.Title,
		ReportsToID:     input.ReportsToID,
		Tags:            normalizeTags(input.Tags),
		Type:            input.Type,
		Description:     input.Description,
		SystemPrompt:    input.SystemPrompt,
		InstructionFile: input.InstructionFile,
		Enabled:         true,
		Capability:      input.Capability,
		Permissions:     input.Permissions,
		SkillIDs:        input.SkillIDs,
		Config:          input.Config,
		Env:             input.Env,
		ExtraArgs:       input.ExtraArgs,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.storage.Agents().Create(ctx, agent); err != nil {
		return domain.Agent{}, err
	}

	return agent, nil
}

func (s *AgentService) List(ctx context.Context) ([]domain.Agent, error) {
	return s.storage.Agents().List(ctx)
}

func (s *AgentService) Get(ctx context.Context, id string) (domain.Agent, error) {
	return s.storage.Agents().Get(ctx, id)
}

func (s *AgentService) UpdatePermissions(ctx context.Context, id string, permissions []domain.AgentPermission) (domain.Agent, error) {
	agent, err := s.storage.Agents().Get(ctx, id)
	if err != nil {
		return domain.Agent{}, err
	}
	agent.Permissions = permissions
	agent.UpdatedAt = s.clock.Now()
	if err := s.storage.Agents().Update(ctx, agent); err != nil {
		return domain.Agent{}, err
	}
	return agent, nil
}

func (s *AgentService) UpdateCapability(ctx context.Context, id string, capability domain.AgentCapability) (domain.Agent, error) {
	agent, err := s.storage.Agents().Get(ctx, id)
	if err != nil {
		return domain.Agent{}, err
	}
	agent.Capability = capability
	agent.Permissions = PermissionsForCapability(capability)
	agent.UpdatedAt = s.clock.Now()
	if err := s.storage.Agents().Update(ctx, agent); err != nil {
		return domain.Agent{}, err
	}
	return agent, nil
}

func (s *AgentService) UpdateSkills(ctx context.Context, id string, skillIDs []string) (domain.Agent, error) {
	agent, err := s.storage.Agents().Get(ctx, id)
	if err != nil {
		return domain.Agent{}, err
	}
	agent.SkillIDs = skillIDs
	agent.UpdatedAt = s.clock.Now()
	if err := s.storage.Agents().Update(ctx, agent); err != nil {
		return domain.Agent{}, err
	}
	return agent, nil
}

type UpdateAgentInput struct {
	Name            *string
	Title           *string
	ReportsToID     *string
	Tags            *[]string
	Description     *string
	SystemPrompt    *string
	InstructionFile *string
	ExtraArgs       *[]string
	Enabled         *bool
}

func (s *AgentService) UpdateIdentity(ctx context.Context, id string, input UpdateAgentInput) (domain.Agent, error) {
	agent, err := s.storage.Agents().Get(ctx, id)
	if err != nil {
		return domain.Agent{}, err
	}
	if input.Name != nil {
		agent.Name = *input.Name
	}
	if input.Title != nil {
		agent.Title = *input.Title
	}
	if input.ReportsToID != nil {
		agent.ReportsToID = *input.ReportsToID
	}
	if input.Tags != nil {
		agent.Tags = normalizeTags(*input.Tags)
	}
	if input.Description != nil {
		agent.Description = *input.Description
	}
	if input.SystemPrompt != nil {
		agent.SystemPrompt = *input.SystemPrompt
	}
	if input.InstructionFile != nil {
		agent.InstructionFile = *input.InstructionFile
	}
	if input.ExtraArgs != nil {
		agent.ExtraArgs = *input.ExtraArgs
	}
	if input.Enabled != nil {
		agent.Enabled = *input.Enabled
	}
	agent.UpdatedAt = s.clock.Now()
	if err := s.storage.Agents().Update(ctx, agent); err != nil {
		return domain.Agent{}, err
	}
	return agent, nil
}

func (s *AgentService) UpdateConfig(ctx context.Context, id string, config map[string]string, env map[string]string) (domain.Agent, error) {
	agent, err := s.storage.Agents().Get(ctx, id)
	if err != nil {
		return domain.Agent{}, err
	}
	agent.Config = config
	agent.Env = env
	agent.UpdatedAt = s.clock.Now()
	if err := s.storage.Agents().Update(ctx, agent); err != nil {
		return domain.Agent{}, err
	}
	return agent, nil
}

func (s *AgentService) ListByTag(ctx context.Context, tag string) ([]domain.Agent, error) {
	agents, err := s.storage.Agents().List(ctx)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(tag) == "" {
		return agents, nil
	}
	out := make([]domain.Agent, 0)
	wanted := normalizeTag(tag)
	for _, agent := range agents {
		for _, value := range agent.Tags {
			if normalizeTag(value) == wanted {
				out = append(out, agent)
				break
			}
		}
	}
	return out, nil
}

func (s *AgentService) Delete(ctx context.Context, id string) error {
	return s.storage.Agents().Delete(ctx, id)
}

func normalizeTags(tags []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		key := normalizeTag(tag)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, tag)
	}
	return out
}

func normalizeTag(tag string) string {
	return strings.ToLower(strings.TrimSpace(tag))
}
