package drivers

import (
	"context"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

type NoopDriver struct{}

func (d *NoopDriver) Type() domain.AgentType {
	return "noop"
}

func (d *NoopDriver) Run(ctx context.Context, input ports.DriverInput) (ports.DriverResult, error) {
	return ports.DriverResult{
		Output: "noop response: " + input.Task.Prompt,
	}, nil
}
