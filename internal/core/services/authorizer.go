package services

import (
	"context"
	"fmt"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

type Authorizer struct {
	storage ports.Storage
}

func NewAuthorizer(storage ports.Storage) *Authorizer {
	return &Authorizer{storage: storage}
}

func (a *Authorizer) RequireAgentPermission(ctx context.Context, agentID string, permission domain.AgentPermission) error {
	if agentID == "" {
		return fmt.Errorf("%w: agent_id is required", domain.ErrInvalidInput)
	}
	agent, err := a.storage.Agents().Get(ctx, agentID)
	if err != nil {
		return err
	}
	for _, item := range agent.Permissions {
		if item == permission {
			return nil
		}
	}
	return fmt.Errorf("%w: permission %s required", domain.ErrForbidden, permission)
}
