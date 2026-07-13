package runtimes

import (
	"bytes"
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

type PicoClawAdapter struct {
	FallbackBinary string
}

func NewPicoClawAdapter(fallbackBinary string) *PicoClawAdapter {
	if fallbackBinary == "" {
		fallbackBinary = "picoclaw"
	}
	return &PicoClawAdapter{FallbackBinary: fallbackBinary}
}

func (a *PicoClawAdapter) ID() domain.RuntimeID     { return "picoclaw" }
func (a *PicoClawAdapter) Name() string             { return "PicoClaw" }
func (a *PicoClawAdapter) Kind() domain.RuntimeKind { return domain.RuntimeKindNative }
func (a *PicoClawAdapter) SupportedInstallModes() []domain.InstallMode {
	return []domain.InstallMode{domain.InstallModeExclusive, domain.InstallModeGlobal, domain.InstallModeExisting}
}

func (a *PicoClawAdapter) ListVersions(ctx context.Context, limit int) ([]domain.RuntimeVersion, error) {
	return listGitHubVersions(ctx, "sipeed", "picoclaw", limit)
}

func (a *PicoClawAdapter) Install(ctx context.Context, mode domain.InstallMode, destDir string, versionAlias string) (domain.RuntimeState, error) {
	binName := "picoclaw"
	if runtime.GOOS == "windows" {
		binName = "picoclaw.exe"
	}
	binPath := filepath.Join(destDir, "bin", binName)
	homePath := filepath.Join(destDir, "home")
	configPath := filepath.Join(destDir, "config", "config.json")
	logsPath := filepath.Join(destDir, "logs")
	if mode == domain.InstallModeGlobal {
		binPath = filepath.Join(userBinDir(), binName)
		home, _ := os.UserHomeDir()
		homePath = filepath.Join(home, ".picoclaw")
		configPath = filepath.Join(homePath, "config.json")
		logsPath = filepath.Join(homePath, "logs")
	}

	version, sourceURL, err := installFromGitHubRelease(ctx, "sipeed", "picoclaw", "picoclaw", "picoclaw", versionAlias, binPath)
	if err != nil {
		if err := copyExistingBinary(a.FallbackBinary, binPath); err != nil {
			return domain.RuntimeState{}, fmt.Errorf("failed to download release and fallback failed: %w", err)
		}
	}

	if err := writeFileIfMissing(configPath, []byte("{\n  \"agents\": {\n    \"defaults\": {\n      \"workspace\": \""+filepath.ToSlash(filepath.Join(homePath, "workspace"))+"\",\n      \"restrict_to_workspace\": true\n    }\n  },\n  \"tools\": {\n    \"exec\": {\n      \"enabled\": true,\n      \"enable_deny_patterns\": true\n    },\n    \"mcp\": {\n      \"enabled\": false,\n      \"servers\": {}\n    }\n  }\n}\n"), 0644); err != nil {
		return domain.RuntimeState{}, err
	}
	_ = writeFileIfMissing(filepath.Join(filepath.Dir(configPath), ".security.yml"), []byte("# Sensitive PicoClaw values managed by PicoClip.\n"), 0600)
	_ = os.MkdirAll(filepath.Join(homePath, "workspace"), 0755)
	_ = os.MkdirAll(logsPath, 0755)

	state := nowState(a.ID(), mode, binPath, configPath, homePath, homePath, logsPath)
	if version != "" {
		state.Version = version
		state.SourceURL = sourceURL
		state.Source = "github_release"
	}
	return state, nil
}

func (a *PicoClawAdapter) Resolve(ctx context.Context, state domain.RuntimeState) error {
	bin := state.BinPath
	if bin == "" {
		bin = a.FallbackBinary
	}
	_, err := resolveConfiguredExecutable(bin)
	return err
}

func (a *PicoClawAdapter) Health(ctx context.Context, state domain.RuntimeState) domain.RuntimeHealth {
	bin := state.BinPath
	if bin == "" {
		bin = a.FallbackBinary
	}
	now := time.Now().UTC()
	health := domain.RuntimeHealth{Status: "ok", CheckedAt: now}
	canonical, err := resolveConfiguredExecutable(bin)
	if err != nil {
		health.Status = "error"
		health.Errors = append(health.Errors, err.Error())
		health.Checks = append(health.Checks, domain.DiagnosticCheck{Name: "binary_exists", Status: "error", Message: err.Error(), CheckedAt: now})
		return health
	}
	bin = canonical
	health.Checks = append(health.Checks, domain.DiagnosticCheck{Name: "binary_exists", Status: "ok", Message: bin, CheckedAt: now})
	testCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	version, err := commandVersion(testCtx, bin, "version")
	if err != nil {
		health.Status = "error"
		health.Errors = append(health.Errors, err.Error())
		health.Checks = append(health.Checks, domain.DiagnosticCheck{Name: "version_command", Status: "error", Message: err.Error(), CheckedAt: now})
		return health
	}
	health.Version = extractRuntimeVersion(string(a.ID()), version)
	health.Checks = append(health.Checks, domain.DiagnosticCheck{Name: "version_command", Status: "ok", Message: health.Version, CheckedAt: now})
	if state.ConfigPath != "" {
		if _, err := os.Stat(state.ConfigPath); err != nil {
			health.Checks = append(health.Checks, domain.DiagnosticCheck{Name: "config_exists", Status: "warning", Message: err.Error(), CheckedAt: now})
		} else {
			health.Checks = append(health.Checks, domain.DiagnosticCheck{Name: "config_exists", Status: "ok", Message: state.ConfigPath, CheckedAt: now})
		}
	}
	return health
}

func (a *PicoClawAdapter) ReadConfig(ctx context.Context, state domain.RuntimeState) ([]domain.RuntimeConfigFile, error) {
	if state.ConfigPath == "" {
		return nil, nil
	}
	content, err := os.ReadFile(state.ConfigPath)
	if err != nil {
		return nil, err
	}
	files := []domain.RuntimeConfigFile{{Path: state.ConfigPath, Name: "config.json", Language: "json", Content: content, Editable: true}}
	securityPath := filepath.Join(filepath.Dir(state.ConfigPath), ".security.yml")
	if security, err := os.ReadFile(securityPath); err == nil {
		files = append(files, domain.RuntimeConfigFile{Path: securityPath, Name: ".security.yml", Language: "yaml", Content: security, Editable: true, Sensitive: true})
	}
	return files, nil
}

func (a *PicoClawAdapter) WriteConfig(ctx context.Context, state domain.RuntimeState, fileName string, content []byte) error {
	if state.ConfigPath == "" {
		return fmt.Errorf("config path is not configured")
	}
	path := state.ConfigPath
	mode := os.FileMode(0644)
	if fileName == ".security.yml" {
		path = filepath.Join(filepath.Dir(state.ConfigPath), ".security.yml")
		mode = 0600
	}
	return os.WriteFile(path, content, mode)
}

func (a *PicoClawAdapter) Execute(ctx context.Context, state domain.RuntimeState, input ports.RuntimeExecutionInput) (ports.RuntimeExecutionResult, error) {
	bin := state.BinPath
	if input.Config["binary_path"] != "" {
		bin = input.Config["binary_path"]
	}
	if bin == "" {
		bin = a.FallbackBinary
	}
	canonical, err := resolveConfiguredExecutable(bin)
	if err != nil {
		return ports.RuntimeExecutionResult{}, err
	}
	bin = canonical
	args := []string{"agent", "-m", input.Task.Prompt}
	args = append(args, input.ExtraArgs...)
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(cmd.Environ(), envPairs(input.Env)...)
	if input.Task.WorkspaceID != "" {
		cmd.Env = append(cmd.Env, "PICOCLIP_WORKSPACE_ID="+input.Task.WorkspaceID)
	}
	if state.ConfigPath != "" {
		cmd.Env = append(cmd.Env, "PICOCLAW_CONFIG="+state.ConfigPath)
	}
	if state.HomePath != "" {
		cmd.Env = append(cmd.Env, "PICOCLAW_HOME="+state.HomePath)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return ports.RuntimeExecutionResult{}, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return ports.RuntimeExecutionResult{}, err
	}

	if err := startRuntimeCommand(cmd); err != nil {
		return ports.RuntimeExecutionResult{}, fmt.Errorf("picoclaw start failed: %w", err)
	}
	if input.OnStart != nil && cmd.Process != nil {
		input.OnStart(cmd.Process.Pid)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	done := make(chan bool, 2)

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stdoutPipe.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				stdoutBuf.Write(chunk)
				if input.OnOutput != nil {
					input.OnOutput(chunk, nil)
				}
			}
			if err != nil {
				break
			}
		}
		done <- true
	}()

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stderrPipe.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				stderrBuf.Write(chunk)
				if input.OnOutput != nil {
					input.OnOutput(nil, chunk)
				}
			}
			if err != nil {
				break
			}
		}
		done <- true
	}()

	<-done
	<-done

	err = cmd.Wait()

	if input.OnOutput != nil {
		input.OnOutput(stdoutBuf.Bytes(), stderrBuf.Bytes())
	}

	if err != nil {
		return ports.RuntimeExecutionResult{}, fmt.Errorf("picoclaw execution failed: %w\nstderr: %s", err, strings.TrimSpace(stderrBuf.String()))
	}
	return ports.RuntimeExecutionResult{Output: strings.TrimSpace(stdoutBuf.String())}, nil
}

func (a *PicoClawAdapter) Cancel(ctx context.Context, state domain.RuntimeState, run domain.Run) error {
	if run.ProcessID <= 0 {
		return nil
	}
	return cancelRuntimeProcess(ctx, run.ProcessID)
}
