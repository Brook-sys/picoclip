package drivers

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

type CrushDriver struct {
	binaryPath string
}

func NewCrushDriver(binaryPath string) *CrushDriver {
	if binaryPath == "" {
		binaryPath = "crush"
	}
	return &CrushDriver{binaryPath: binaryPath}
}

func (d *CrushDriver) Type() domain.AgentType {
	return "crush"
}

func envPairs(m map[string]string) []string {
	var out []string
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}

func (d *CrushDriver) Run(ctx context.Context, input ports.DriverInput) (ports.DriverResult, error) {
	binaryPath := d.binaryPath
	if input.Config["binary_path"] != "" {
		binaryPath = input.Config["binary_path"]
	}
	args := []string{"run"}
	args = append(args, input.ExtraArgs...)
	args = append(args, input.Task.Prompt)
	cmd := exec.CommandContext(ctx, binaryPath, args...)
	if len(input.Env) > 0 {
		cmd.Env = append(cmd.Environ(), envPairs(input.Env)...)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return ports.DriverResult{}, fmt.Errorf("crush execution failed: %w, output: %s", err, string(output))
	}

	return ports.DriverResult{
		Output: strings.TrimSpace(string(output)),
	}, nil
}
