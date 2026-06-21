package runtimes

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

type CrushAdapter struct {
	FallbackBinary string
}

func NewCrushAdapter(fallbackBinary string) *CrushAdapter {
	if fallbackBinary == "" {
		fallbackBinary = "crush"
	}
	return &CrushAdapter{FallbackBinary: fallbackBinary}
}

func (a *CrushAdapter) ID() domain.RuntimeID     { return "crush" }
func (a *CrushAdapter) Name() string             { return "Crush" }
func (a *CrushAdapter) Kind() domain.RuntimeKind { return domain.RuntimeKindNative }
func (a *CrushAdapter) SupportedInstallModes() []domain.InstallMode {
	return []domain.InstallMode{domain.InstallModeExclusive, domain.InstallModeGlobal, domain.InstallModeExisting}
}

func (a *CrushAdapter) Install(ctx context.Context, mode domain.InstallMode, destDir string) (domain.RuntimeState, error) {
	binName := "crush"
	if runtime.GOOS == "windows" {
		binName = "crush.exe"
	}
	binPath := filepath.Join(destDir, "bin", binName)
	configPath := filepath.Join(destDir, "config", "crush.json")
	dataPath := filepath.Join(destDir, "data")
	logsPath := filepath.Join(destDir, "logs")
	if mode == domain.InstallModeGlobal {
		binPath = filepath.Join(userBinDir(), binName)
		home, _ := os.UserHomeDir()
		configPath = filepath.Join(home, ".config", "crush", "crush.json")
		dataPath = filepath.Join(home, ".local", "share", "crush")
		logsPath = filepath.Join(dataPath, "logs")
	}

	version, sourceURL, err := installFromGitHubRelease(ctx, "charmbracelet", "crush", "crush", "crush", binPath)
	if err != nil {
		if err := copyExistingBinary(a.FallbackBinary, binPath); err != nil {
			return domain.RuntimeState{}, fmt.Errorf("failed to download release and fallback failed: %w", err)
		}
	}

	if err := writeFileIfMissing(configPath, []byte("{\n  \"$schema\": \"https://charm.land/crush.json\",\n  \"options\": {\n    \"disable_metrics\": true\n  }\n}\n"), 0644); err != nil {
		return domain.RuntimeState{}, err
	}
	_ = os.MkdirAll(dataPath, 0755)
	_ = os.MkdirAll(logsPath, 0755)

	state := nowState(a.ID(), mode, binPath, configPath, "", dataPath, logsPath)
	if version != "" {
		state.Version = version
		state.SourceURL = sourceURL
		state.Source = "github_release"
	}
	return state, nil
}

func (a *CrushAdapter) Resolve(ctx context.Context, state domain.RuntimeState) error {
	bin := state.BinPath
	if bin == "" {
		bin = a.FallbackBinary
	}
	_, err := os.Stat(bin)
	return err
}

func (a *CrushAdapter) Health(ctx context.Context, state domain.RuntimeState) domain.RuntimeHealth {
	bin := state.BinPath
	if bin == "" {
		bin = a.FallbackBinary
	}
	health := domain.RuntimeHealth{Status: "ok", CheckedAt: time.Now().UTC()}
	version, err := commandVersion(ctx, bin, "--version")
	if err != nil {
		version, err = commandVersion(ctx, bin, "version")
	}
	if err != nil {
		health.Status = "error"
		health.Errors = append(health.Errors, err.Error())
		return health
	}
	health.Version = version
	return health
}

func (a *CrushAdapter) ReadConfig(ctx context.Context, state domain.RuntimeState) ([]domain.RuntimeConfigFile, error) {
	if state.ConfigPath == "" {
		return nil, nil
	}
	content, err := os.ReadFile(state.ConfigPath)
	if err != nil {
		return nil, err
	}
	return []domain.RuntimeConfigFile{{Path: state.ConfigPath, Name: "crush.json", Language: "json", Content: content, Editable: true}}, nil
}

func (a *CrushAdapter) WriteConfig(ctx context.Context, state domain.RuntimeState, fileName string, content []byte) error {
	if state.ConfigPath == "" {
		return fmt.Errorf("config path is not configured")
	}
	return os.WriteFile(state.ConfigPath, content, 0644)
}

func (a *CrushAdapter) Execute(ctx context.Context, state domain.RuntimeState, input ports.RuntimeExecutionInput) (ports.RuntimeExecutionResult, error) {
	bin := state.BinPath
	if input.Config["binary_path"] != "" {
		bin = input.Config["binary_path"]
	}
	if bin == "" {
		bin = a.FallbackBinary
	}
	args := []string{"run"}
	args = append(args, input.ExtraArgs...)
	args = append(args, input.Task.Prompt)
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(cmd.Environ(), envPairs(input.Env)...)
	if state.ConfigPath != "" {
		cmd.Env = append(cmd.Env, "CRUSH_GLOBAL_CONFIG="+state.ConfigPath)
	}
	if state.DataPath != "" {
		cmd.Env = append(cmd.Env, "CRUSH_GLOBAL_DATA="+state.DataPath)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ports.RuntimeExecutionResult{}, fmt.Errorf("crush execution failed: %w, output: %s", err, string(output))
	}
	return ports.RuntimeExecutionResult{Output: strings.TrimSpace(string(output))}, nil
}
