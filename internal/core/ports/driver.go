package ports

import (
	"context"

	"picoclip/internal/core/domain"
)

type Driver interface {
	Type() domain.AgentType
	Run(ctx context.Context, input DriverInput) (DriverResult, error)
}

type DriverInput struct {
	Agent     domain.Agent
	Task      domain.Task
	Run       domain.Run
	Memory    string
	Config    map[string]string
	Env       map[string]string
	ExtraArgs []string
}

type DriverResult struct {
	Output string
	Logs   []string
	Data   map[string]string
}
