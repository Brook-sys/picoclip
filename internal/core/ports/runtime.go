package ports

import (
	"context"

	"picoclip/internal/core/domain"
)

type RuntimeExecutionInput struct {
	Agent     domain.Agent
	Task      domain.Task
	Run       domain.Run
	Memory    string
	Config    map[string]string
	Env       map[string]string
	ExtraArgs []string
}

type RuntimeExecutionResult struct {
	Output string
	Logs   []string
	Data   map[string]string
}

type RuntimeAdapter interface {
	ID() domain.RuntimeID
	Name() string
	Kind() domain.RuntimeKind
	SupportedInstallModes() []domain.InstallMode
	Install(ctx context.Context, mode domain.InstallMode, destDir string) (domain.RuntimeState, error)
	Resolve(ctx context.Context, state domain.RuntimeState) error
	Health(ctx context.Context, state domain.RuntimeState) domain.RuntimeHealth
	ReadConfig(ctx context.Context, state domain.RuntimeState) ([]domain.RuntimeConfigFile, error)
	WriteConfig(ctx context.Context, state domain.RuntimeState, fileName string, content []byte) error
	Execute(ctx context.Context, state domain.RuntimeState, input RuntimeExecutionInput) (RuntimeExecutionResult, error)
}

type RuntimeRepository interface {
	GetByRuntimeID(ctx context.Context, runtimeID domain.RuntimeID) (domain.RuntimeState, error)
	List(ctx context.Context) ([]domain.RuntimeState, error)
	Save(ctx context.Context, state domain.RuntimeState) error
	Delete(ctx context.Context, id string) error
}
