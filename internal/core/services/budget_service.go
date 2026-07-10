package services

import (
	"context"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

type BudgetService struct {
	storage ports.Storage
	clock   ports.Clock
	idGen   ports.IDGenerator
}

func NewBudgetService(storage ports.Storage, clock ports.Clock, idGen ports.IDGenerator) *BudgetService {
	return &BudgetService{storage: storage, clock: clock, idGen: idGen}
}

func (s *BudgetService) Create(ctx context.Context, budget domain.Budget) (domain.Budget, error) {
	now := s.clock.Now()
	if budget.ID == "" {
		budget.ID = s.idGen.NewID("budget")
	}
	if budget.Scope == "" {
		budget.Scope = domain.BudgetScopeGlobal
	}
	budget.CreatedAt = now
	budget.UpdatedAt = now
	budget.Enabled = true
	if !budget.HardStop {
		budget.HardStop = true
	}
	if err := s.storage.Budgets().Create(ctx, budget); err != nil {
		return domain.Budget{}, err
	}
	return budget, nil
}

func (s *BudgetService) List(ctx context.Context) ([]domain.Budget, error) {
	return s.storage.Budgets().List(ctx)
}

func (s *BudgetService) UsageForAgent(ctx context.Context, agentID string) (domain.BudgetUsage, error) {
	input, output, cached, costMicros, err := s.storage.Usage().SumByAgent(ctx, agentID)
	if err != nil {
		return domain.BudgetUsage{}, err
	}
	return domain.BudgetUsage{InputTokens: input, OutputTokens: output, CachedTokens: cached, TotalTokens: input + output + cached, CostMicros: costMicros}, nil
}

func (s *BudgetService) UsageForWorkspace(ctx context.Context, workspaceID string) (domain.BudgetUsage, error) {
	return s.usageForScope(ctx, workspaceID, "", true)
}

func (s *BudgetService) IsHardStopped(ctx context.Context, workspaceID string, agentID string) (bool, domain.Budget, domain.BudgetUsage, error) {
	budgets, err := s.storage.Budgets().List(ctx)
	if err != nil {
		return false, domain.Budget{}, domain.BudgetUsage{}, err
	}
	for _, budget := range budgets {
		if !budget.HardStop || !budget.AppliesTo(workspaceID, agentID) {
			continue
		}
		usage, err := s.usageForBudget(ctx, budget)
		if err != nil {
			return false, domain.Budget{}, domain.BudgetUsage{}, err
		}
		if budget.Exceeded(usage) {
			return true, budget, usage, nil
		}
	}
	usage, err := s.usageForScope(ctx, workspaceID, agentID, false)
	if err != nil {
		return false, domain.Budget{}, domain.BudgetUsage{}, err
	}
	return false, domain.Budget{}, usage, nil
}

func (s *BudgetService) usageForBudget(ctx context.Context, budget domain.Budget) (domain.BudgetUsage, error) {
	switch budget.Scope {
	case domain.BudgetScopeGlobal:
		return s.usageForScope(ctx, "", "", false)
	case domain.BudgetScopeWorkspace:
		return s.usageForScope(ctx, budget.WorkspaceID, "", false)
	case domain.BudgetScopeAgent:
		return s.usageForScope(ctx, "", budget.AgentID, false)
	default:
		return domain.BudgetUsage{}, nil
	}
}

func (s *BudgetService) usageForScope(ctx context.Context, workspaceID string, agentID string, strictWorkspaceLookup bool) (domain.BudgetUsage, error) {
	events, err := s.storage.Usage().List(ctx)
	if err != nil {
		return domain.BudgetUsage{}, err
	}
	usage := domain.BudgetUsage{}
	for _, event := range events {
		if agentID != "" && event.AgentID != agentID {
			continue
		}
		if workspaceID != "" {
			task, err := s.storage.Tasks().Get(ctx, event.TaskID)
			if err != nil {
				if strictWorkspaceLookup {
					return domain.BudgetUsage{}, err
				}
				continue
			}
			if task.WorkspaceID != workspaceID {
				continue
			}
		}
		usage.InputTokens += event.InputTokens
		usage.OutputTokens += event.OutputTokens
		usage.CachedTokens += event.CachedTokens
		usage.TotalTokens += event.InputTokens + event.OutputTokens + event.CachedTokens
		usage.CostMicros += event.CostMicros
		usage.Runs++
	}
	return usage, nil
}
