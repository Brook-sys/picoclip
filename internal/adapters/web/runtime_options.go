package web

import (
	"context"

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
		state, ok := states[manifest.ID]
		if !ok || !state.Enabled {
			continue
		}
		adapter, ok := s.runtimes.Adapter(manifest.ID)
		if !ok || adapter.Resolve(ctx, state) != nil {
			continue
		}
		options = append(options, RuntimeOption{ID: manifest.ID, Label: manifest.Name})
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
		return domain.ErrRuntimeUnavailable
	}
	if agentType == "crush" || agentType == "picoclaw" {
		runtimeID := domain.RuntimeID(agentType)
		state, err := s.runtimes.State(ctx, runtimeID)
		if err != nil || !state.Enabled {
			return domain.ErrRuntimeUnavailable
		}
		adapter, ok := s.runtimes.Adapter(runtimeID)
		if !ok || adapter.Resolve(ctx, state) != nil {
			return domain.ErrRuntimeUnavailable
		}
	}
	return nil
}
