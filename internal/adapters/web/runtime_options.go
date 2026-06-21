package web

import (
	"context"
	"fmt"

	"picoclip/internal/core/domain"
)

type RuntimeOption struct {
	ID    domain.RuntimeID
	Label string
}

func (s *Server) agentRuntimeOptions(ctx context.Context) []RuntimeOption {
	states, _ := s.runtimes.States(ctx)
	options := make([]RuntimeOption, 0, len(states)+1)
	for _, manifest := range s.runtimes.Catalog() {
		if _, ok := states[manifest.ID]; ok {
			options = append(options, RuntimeOption{ID: manifest.ID, Label: manifest.Name})
		}
	}
	if s.debugMode {
		options = append(options, RuntimeOption{ID: "noop", Label: "Noop (debug)"})
	}
	return options
}

func (s *Server) validateAgentRuntime(ctx context.Context, agentType domain.AgentType) error {
	if agentType == "noop" {
		if s.debugMode {
			return nil
		}
		return fmt.Errorf("runtime not available: noop")
	}
	if agentType == "crush" || agentType == "picoclaw" {
		if _, err := s.runtimes.State(ctx, domain.RuntimeID(agentType)); err != nil {
			return fmt.Errorf("runtime not installed: %s", agentType)
		}
	}
	return nil
}
