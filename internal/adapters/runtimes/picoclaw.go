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
	if err := ensurePicoClawHookPack(state); err != nil {
		return domain.RuntimeState{}, err
	}
	return state, nil
}

func (a *PicoClawAdapter) Resolve(ctx context.Context, state domain.RuntimeState) error {
	bin := state.BinPath
	if bin == "" {
		bin = a.FallbackBinary
	}
	_, err := os.Stat(bin)
	return err
}

func (a *PicoClawAdapter) Health(ctx context.Context, state domain.RuntimeState) domain.RuntimeHealth {
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
	_ = ensurePicoClawHookPack(state)
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
	if err := os.WriteFile(path, content, mode); err != nil {
		return err
	}
	return ensurePicoClawHookPack(state)
}

func ensurePicoClawHookPack(state domain.RuntimeState) error {
	configPath := absPath(state.ConfigPath)
	if configPath == "" {
		return nil
	}
	hooksDir := filepath.Join(filepath.Dir(configPath), "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return err
	}
	hookPath := filepath.Join(hooksDir, "picoclip-hook.py")
	if err := os.WriteFile(hookPath, []byte(picoclawHookScript), 0755); err != nil {
		return err
	}
	return mergePicoClawHooks(configPath, hookPath)
}

func mergePicoClawHooks(configPath string, hookPath string) error {
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
	hooks["enabled"] = true
	defaults, _ := hooks["defaults"].(map[string]any)
	if defaults == nil {
		defaults = map[string]any{}
	}
	if defaults["observer_timeout_ms"] == nil {
		defaults["observer_timeout_ms"] = 1000
	}
	if defaults["interceptor_timeout_ms"] == nil {
		defaults["interceptor_timeout_ms"] = 2000
	}
	if defaults["approval_timeout_ms"] == nil {
		defaults["approval_timeout_ms"] = 2000
	}
	hooks["defaults"] = defaults
	processes, _ := hooks["processes"].(map[string]any)
	if processes == nil {
		processes = map[string]any{}
	}
	logsDir := filepath.Join(filepath.Dir(configPath), "..", "logs")
	if abs, err := filepath.Abs(logsDir); err == nil {
		logsDir = abs
	}
	processes["picoclip"] = map[string]any{
		"enabled":   true,
		"priority":  50,
		"transport": "stdio",
		"command":   []string{"python3", absPath(hookPath)},
		"observe":   []string{"agent.turn.start", "agent.turn.end", "agent.tool.exec_start", "agent.tool.exec_end", "agent.tool.exec_skipped", "agent.error", "agent.steering.injected"},
		"intercept": []string{"before_llm", "before_tool", "after_tool", "approve_tool"},
		"env":       map[string]any{"PICOCLIP_RUNTIME_LOGS": logsDir},
	}
	hooks["processes"] = processes
	config["hooks"] = hooks
	agents, _ := config["agents"].(map[string]any)
	if agents == nil {
		agents = map[string]any{}
	}
	defaultsAgents, _ := agents["defaults"].(map[string]any)
	if defaultsAgents == nil {
		defaultsAgents = map[string]any{}
	}
	if defaultsAgents["steering_mode"] == nil {
		defaultsAgents["steering_mode"] = "all"
	}
	if defaultsAgents["context_window"] == nil {
		defaultsAgents["context_window"] = 131072
	}
	if defaultsAgents["summarize_token_percent"] == nil {
		defaultsAgents["summarize_token_percent"] = 75
	}
	if defaultsAgents["summarize_message_threshold"] == nil {
		defaultsAgents["summarize_message_threshold"] = 20
	}
	agents["defaults"] = defaultsAgents
	config["agents"] = agents
	evolution, _ := config["evolution"].(map[string]any)
	if evolution == nil {
		evolution = map[string]any{}
	}
	if evolution["enabled"] == nil {
		evolution["enabled"] = true
	}
	if evolution["mode"] == nil {
		evolution["mode"] = "observe"
	}
	config["evolution"] = evolution
	raw, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, raw, 0644)
}

const picoclawHookScript = `#!/usr/bin/env python3
from __future__ import annotations
import json
import os
import signal
import sys
from datetime import datetime, timezone

DANGEROUS_COMMAND_MARKERS = ["git push", "npm publish", "pnpm publish", "yarn publish", "sudo ", "rm -rf /", "rm -fr /", "rm -rf ~", "rm -fr ~", "rm -rf .git", "rm -fr .git"]
SENSITIVE_PATH_MARKERS = ["/.ssh/", "/.gnupg/", "/.aws/credentials", "/.config/"]


def runtime_logs() -> str:
    return os.environ.get("PICOCLIP_RUNTIME_LOGS", "").strip()


def log_file() -> str:
    base = runtime_logs()
    return os.path.join(base, "tool-calls.log") if base else ""


def append_log(entry: dict) -> None:
    path = log_file()
    if not path:
        return
    entry = {"ts": datetime.now(timezone.utc).isoformat(), "runtime": "picoclaw", "task": os.environ.get("PICOCLIP_TASK_ID", ""), "run": os.environ.get("PICOCLIP_RUN_ID", ""), **entry}
    try:
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with open(path, "a", encoding="utf-8") as handle:
            handle.write(json.dumps(entry, ensure_ascii=False, separators=(",", ":")) + "\n")
    except OSError:
        pass


def send(message_id, result=None, error=None):
    payload = {"jsonrpc": "2.0", "id": message_id}
    if error:
        payload["error"] = {"code": -32000, "message": str(error)}
    else:
        payload["result"] = result if result is not None else {}
    sys.stdout.write(json.dumps(payload, ensure_ascii=True) + "\n")
    sys.stdout.flush()


def tool_args(params: dict) -> dict:
    return params.get("arguments") or params.get("args") or {}


def arg_text(args: dict) -> str:
    for key in ("command", "cmd", "path", "file", "file_path"):
        value = args.get(key)
        if isinstance(value, str) and value:
            return value
    return json.dumps(args, ensure_ascii=False)


def deny_reason(args: dict) -> str:
    value = arg_text(args)
    low = value.lower()
    for marker in DANGEROUS_COMMAND_MARKERS:
        if marker in low:
            return "PicoClip blocked a publishing, privileged or destructive command."
    for marker in SENSITIVE_PATH_MARKERS:
        if marker in value:
            return "PicoClip blocked access to a sensitive credentials path."
    workspace = os.environ.get("PICOCLIP_WORKSPACE", "").strip()
    candidate = args.get("path") or args.get("file") or args.get("file_path")
    if workspace and isinstance(candidate, str) and candidate.startswith("/"):
        try:
            if not os.path.abspath(candidate).startswith(os.path.abspath(workspace) + os.sep):
                return "PicoClip blocked a path outside the task workspace."
        except OSError:
            pass
    return ""


def rewrite_args(args: dict) -> tuple[dict, bool]:
    api = os.environ.get("PICOCLIP_API_BASE", "http://127.0.0.1:8088").rstrip("/")
    changed = False
    out = dict(args)
    for key, value in list(out.items()):
        if not isinstance(value, str):
            continue
        new = value.replace("http://localhost:8080", api).replace("http://127.0.0.1:8080", api).replace("http://0.0.0.0:8080", api)
        if new != value:
            out[key] = new
            changed = True
    return out, changed


def handle(method: str, params: dict) -> dict:
    if method == "hook.hello":
        append_log({"stage": "hello", "tool": "hook", "command": "hello"})
        return {"ok": True, "name": "picoclip"}
    if method == "hook.runtime_event":
        payload = params.get("payload") or {}
        append_log({"stage": "event", "tool": params.get("kind", "event"), "command": arg_text(payload if isinstance(payload, dict) else {"payload": payload})})
        return {}
    if method == "hook.before_llm":
        tools = params.get("tools") or []
        tools.append({"type": "function", "function": {"name": "picoclip_runtime_context", "description": "Return PicoClip runtime context for this task and run.", "parameters": {"type": "object", "properties": {}}}})
        return {"action": "modify", "request": {"model": params.get("model"), "messages": params.get("messages", []), "tools": tools, "options": params.get("options", {})}}
    if method == "hook.before_tool":
        tool = params.get("tool") or params.get("name") or "unknown"
        args = tool_args(params)
        if tool == "picoclip_runtime_context":
            return {"action": "respond", "result": {"for_llm": f"PicoClip API: {os.environ.get('PICOCLIP_API_BASE','')}; task={os.environ.get('PICOCLIP_TASK_ID','')}; run={os.environ.get('PICOCLIP_RUN_ID','')}; workspace={os.environ.get('PICOCLIP_WORKSPACE','')}", "silent": False, "is_error": False}}
        reason = deny_reason(args)
        if reason:
            append_log({"stage": "before_tool", "tool": tool, "command": arg_text(args), "decision": "deny", "reason": reason})
            return {"action": "deny_tool", "reason": reason}
        new_args, changed = rewrite_args(args)
        append_log({"stage": "before_tool", "tool": tool, "command": arg_text(new_args), "decision": "modify" if changed else "continue"})
        if changed:
            return {"action": "modify", "call": {"tool": tool, "arguments": new_args}}
        return {"action": "continue"}
    if method == "hook.approve_tool":
        tool = params.get("tool") or params.get("name") or "unknown"
        args = tool_args(params)
        reason = deny_reason(args)
        append_log({"stage": "approve_tool", "tool": tool, "command": arg_text(args), "decision": "deny" if reason else "allow", "reason": reason})
        return {"approved": not bool(reason), "reason": reason} if reason else {"approved": True}
    if method == "hook.after_tool":
        tool = params.get("tool") or params.get("name") or "unknown"
        append_log({"stage": "after_tool", "tool": tool, "command": arg_text(tool_args(params)), "decision": "continue"})
        return {"action": "continue"}
    if method == "hook.after_llm":
        return {"action": "continue"}
    return {"action": "continue"}


def main() -> int:
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            msg = json.loads(line)
            method = str(msg.get("method") or "")
            params = msg.get("params") or {}
            if not isinstance(params, dict):
                params = {}
            message_id = msg.get("id")
            result = handle(method, params)
            if message_id:
                send(message_id, result=result)
        except Exception as exc:
            if 'message_id' in locals() and message_id:
                send(message_id, error=exc)
    return 0

def _shutdown(*_args):
    raise SystemExit(0)

if __name__ == "__main__":
    signal.signal(signal.SIGINT, _shutdown)
    signal.signal(signal.SIGTERM, _shutdown)
    raise SystemExit(main())
`

func (a *PicoClawAdapter) Execute(ctx context.Context, state domain.RuntimeState, input ports.RuntimeExecutionInput) (ports.RuntimeExecutionResult, error) {
	_ = ensurePicoClawHookPack(state)
	bin := executablePath(state.BinPath)
	if input.Config["binary_path"] != "" {
		bin = executablePath(input.Config["binary_path"])
	}
	if bin == "" {
		bin = a.FallbackBinary
	}
	args := []string{"agent", "-m", input.Task.Prompt}
	args = append(args, input.ExtraArgs...)
	cmd := exec.CommandContext(ctx, bin, args...)
	if input.WorkspacePath != "" {
		cmd.Dir = input.WorkspacePath
	}
	cmd.Env = append(cmd.Environ(), envPairs(input.Env)...)
	if state.ConfigPath != "" {
		cmd.Env = append(cmd.Env, "PICOCLAW_CONFIG="+absPath(state.ConfigPath))
	}
	if state.HomePath != "" {
		cmd.Env = append(cmd.Env, "PICOCLAW_HOME="+absPath(state.HomePath))
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

	if err != nil {
		return ports.RuntimeExecutionResult{}, fmt.Errorf("picoclaw execution failed: %w\nstderr: %s", err, strings.TrimSpace(stderrBuf.String()))
	}
	return ports.RuntimeExecutionResult{Output: strings.TrimSpace(stdoutBuf.String())}, nil
}

func (a *PicoClawAdapter) Cancel(ctx context.Context, state domain.RuntimeState, run domain.Run) error {
	if run.ProcessID <= 0 {
		return nil
	}
	p, err := os.FindProcess(run.ProcessID)
	if err != nil {
		return nil
	}
	return p.Kill()
}
