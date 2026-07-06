package runtimes

import (
	"bytes"
	"context"
	"encoding/json"
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

func (a *CrushAdapter) ListVersions(ctx context.Context, limit int) ([]domain.RuntimeVersion, error) {
	return listGitHubVersions(ctx, "charmbracelet", "crush", limit)
}

func (a *CrushAdapter) Install(ctx context.Context, mode domain.InstallMode, destDir string, versionAlias string) (domain.RuntimeState, error) {
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

	version, sourceURL, err := installFromGitHubRelease(ctx, "charmbracelet", "crush", "crush", "crush", versionAlias, binPath)
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
	if err := ensureCrushHookPack(state); err != nil {
		return domain.RuntimeState{}, err
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
	now := time.Now().UTC()
	health := domain.RuntimeHealth{Status: "ok", CheckedAt: now}
	if _, err := os.Stat(bin); err != nil {
		health.Status = "error"
		health.Errors = append(health.Errors, err.Error())
		health.Checks = append(health.Checks, domain.DiagnosticCheck{Name: "binary_exists", Status: "error", Message: err.Error(), CheckedAt: now})
		return health
	}
	health.Checks = append(health.Checks, domain.DiagnosticCheck{Name: "binary_exists", Status: "ok", Message: bin, CheckedAt: now})
	testCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	version, err := commandVersion(testCtx, bin, "--version")
	if err != nil {
		version, err = commandVersion(testCtx, bin, "version")
	}
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
	if err := os.WriteFile(state.ConfigPath, content, 0644); err != nil {
		return err
	}
	return ensureCrushHookPack(state)
}

func ensureCrushHookPack(state domain.RuntimeState) error {
	configPath := absPath(state.ConfigPath)
	if configPath == "" {
		return nil
	}
	hooksDir := filepath.Join(filepath.Dir(configPath), "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return err
	}
	scripts := map[string]string{
		"picoclip-guard.sh":    crushGuardHookScript,
		"picoclip-context.sh":  crushContextHookScript,
		"picoclip-rewrite.sh":  crushRewriteHookScript,
		"picoclip-log-tool.sh": crushLogToolHookScript,
	}
	for name, content := range scripts {
		path := filepath.Join(hooksDir, name)
		if err := os.WriteFile(path, []byte(content), 0755); err != nil {
			return err
		}
	}
	return mergeCrushHooks(configPath, hooksDir)
}

func absPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

func executablePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || (!strings.Contains(path, string(os.PathSeparator)) && !strings.Contains(path, "/")) {
		return path
	}
	return absPath(path)
}

func mergeCrushHooks(configPath string, hooksDir string) error {
	if configPath == "" {
		return nil
	}
	if absHooksDir, err := filepath.Abs(hooksDir); err == nil {
		hooksDir = absHooksDir
	}
	config := map[string]any{}
	if raw, err := os.ReadFile(configPath); err == nil && len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &config); err != nil {
			return err
		}
	}
	hooks, _ := config["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	preToolUse, _ := hooks["PreToolUse"].([]any)
	managedNames := map[string]bool{"picoclip-guard": true, "picoclip-rewrite": true, "picoclip-context": true, "picoclip-log-tool": true}
	kept := make([]any, 0, len(preToolUse)+4)
	for _, item := range preToolUse {
		entry, ok := item.(map[string]any)
		if !ok {
			kept = append(kept, item)
			continue
		}
		name, _ := entry["name"].(string)
		if managedNames[name] {
			continue
		}
		kept = append(kept, entry)
	}
	managed := []map[string]any{
		{"name": "picoclip-guard", "matcher": "^(bash|edit|write|multiedit)$", "command": filepath.Join(hooksDir, "picoclip-guard.sh"), "timeout": 5},
		{"name": "picoclip-rewrite", "matcher": "^bash$", "command": filepath.Join(hooksDir, "picoclip-rewrite.sh"), "timeout": 5},
		{"name": "picoclip-context", "matcher": ".*", "command": filepath.Join(hooksDir, "picoclip-context.sh"), "timeout": 5},
		{"name": "picoclip-log-tool", "matcher": ".*", "command": filepath.Join(hooksDir, "picoclip-log-tool.sh"), "timeout": 3},
	}
	for _, entry := range managed {
		kept = append(kept, entry)
	}
	hooks["PreToolUse"] = kept
	config["hooks"] = hooks
	raw, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, raw, 0644)
}

const crushGuardHookScript = `#!/usr/bin/env sh
cmd=${CRUSH_TOOL_INPUT_COMMAND:-}
file=${CRUSH_TOOL_INPUT_FILE_PATH:-}
case "$cmd" in
  *"git push"*|*"npm publish"*|*"pnpm publish"*|*"yarn publish"*|*"sudo "*) echo "PicoClip guard blocked a publishing or privileged command." >&2; exit 2 ;;
esac
case "$cmd" in
  *"rm -rf /"*|*"rm -fr /"*|*"rm -rf ~"*|*"rm -fr ~"*|*"rm -rf .git"*|*"rm -fr .git"*) echo "PicoClip guard blocked a destructive remove command." >&2; exit 2 ;;
esac
case "$file" in
  */.ssh/*|*/.gnupg/*|*/.aws/credentials|*/.config/*/credentials*) echo "PicoClip guard blocked editing a sensitive credentials path." >&2; exit 2 ;;
esac
exit 0
`

const crushContextHookScript = `#!/usr/bin/env sh
api=${PICOCLIP_API_BASE:-http://127.0.0.1:8088}
workspace=${PICOCLIP_WORKSPACE:-$CRUSH_CWD}
task=${PICOCLIP_TASK_ID:-unknown}
run=${PICOCLIP_RUN_ID:-unknown}
printf '{"context":"PicoClip context: API base URL is %s. Current task is %s and run is %s. Treat %s as the persistent workspace memory. For continuous tasks, make incremental progress, persist useful Markdown artifacts, and leave a concise status/comment before the turn ends."}\n' "$api" "$task" "$run" "$workspace"
`

const crushRewriteHookScript = `#!/usr/bin/env sh
cmd=${CRUSH_TOOL_INPUT_COMMAND:-}
api=${PICOCLIP_API_BASE:-http://127.0.0.1:8088}
if [ -z "$cmd" ]; then exit 0; fi
new=$(printf '%s' "$cmd" | sed "s#http://localhost:8080#$api#g; s#http://127.0.0.1:8080#$api#g; s#http://0.0.0.0:8080#$api#g")
if [ "$new" != "$cmd" ]; then
  esc=$(printf '%s' "$new" | sed 's/\\/\\\\/g; s/"/\\"/g')
  printf '{"context":"PicoClip rewrote an outdated local API URL to the active API base URL.","updated_input":{"command":"%s"}}\n' "$esc"
fi
`

const crushLogToolHookScript = `#!/usr/bin/env sh
log=${PICOCLIP_TOOL_LOG:-}
if [ -z "$log" ]; then
  if [ -n "$PICOCLIP_RUNTIME_LOGS" ]; then log="$PICOCLIP_RUNTIME_LOGS/tool-calls.log"; else exit 0; fi
fi
mkdir -p "$(dirname "$log")" 2>/dev/null || true
cmd=${CRUSH_TOOL_INPUT_COMMAND:-}
file=${CRUSH_TOOL_INPUT_FILE_PATH:-}
printf '%s tool=%s task=%s run=%s cwd=%s command=%s file=%s\n' "$(date -Iseconds 2>/dev/null || date)" "${CRUSH_TOOL_NAME:-unknown}" "${PICOCLIP_TASK_ID:-}" "${PICOCLIP_RUN_ID:-}" "${CRUSH_CWD:-}" "$cmd" "$file" >> "$log"
exit 0
`

func (a *CrushAdapter) Execute(ctx context.Context, state domain.RuntimeState, input ports.RuntimeExecutionInput) (ports.RuntimeExecutionResult, error) {
	_ = ensureCrushHookPack(state)
	bin := executablePath(state.BinPath)
	if input.Config["binary_path"] != "" {
		bin = executablePath(input.Config["binary_path"])
	}
	if bin == "" {
		bin = a.FallbackBinary
	}
	args := []string{"run"}
	args = append(args, input.ExtraArgs...)
	args = append(args, input.Task.Prompt)
	cmd := exec.CommandContext(ctx, bin, args...)
	if input.WorkspacePath != "" {
		cmd.Dir = input.WorkspacePath
	}
	cmd.Env = append(cmd.Environ(), envPairs(input.Env)...)
	if state.ConfigPath != "" {
		cmd.Env = append(cmd.Env, "CRUSH_GLOBAL_CONFIG="+filepath.Dir(absPath(state.ConfigPath)))
	}
	if state.DataPath != "" {
		cmd.Env = append(cmd.Env, "CRUSH_GLOBAL_DATA="+absPath(state.DataPath))
	}
	if state.LogsPath != "" {
		cmd.Env = append(cmd.Env, "PICOCLIP_RUNTIME_LOGS="+absPath(state.LogsPath))
	}
	if input.RuntimeBaseURL != "" {
		cmd.Env = append(cmd.Env, "PICOCLIP_API_BASE="+strings.TrimRight(input.RuntimeBaseURL, "/"))
	}
	if input.WorkspacePath != "" {
		cmd.Env = append(cmd.Env, "PICOCLIP_WORKSPACE="+input.WorkspacePath)
	}
	cmd.Env = append(cmd.Env, "PICOCLIP_TASK_ID="+input.Task.ID, "PICOCLIP_RUN_ID="+input.Run.ID, "PICOCLIP_AGENT_ID="+input.Agent.ID)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return ports.RuntimeExecutionResult{}, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return ports.RuntimeExecutionResult{}, err
	}

	if err := cmd.Start(); err != nil {
		return ports.RuntimeExecutionResult{}, fmt.Errorf("crush start failed: %w", err)
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

	if err != nil {
		return ports.RuntimeExecutionResult{}, fmt.Errorf("crush execution failed: %w\nstderr: %s", err, strings.TrimSpace(stderrBuf.String()))
	}
	return ports.RuntimeExecutionResult{Output: strings.TrimSpace(stdoutBuf.String())}, nil
}

func (a *CrushAdapter) Cancel(ctx context.Context, state domain.RuntimeState, run domain.Run) error {
	if run.ProcessID <= 0 {
		return nil
	}
	p, err := os.FindProcess(run.ProcessID)
	if err != nil {
		return nil
	}
	return p.Kill()
}
