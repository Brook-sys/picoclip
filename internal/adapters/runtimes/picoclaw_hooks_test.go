package runtimes

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"picoclip/internal/core/domain"
)

func TestPicoClawHookScriptProtocol(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not available")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "picoclip-hook.py")
	if err := os.WriteFile(script, []byte(picoclawHookScript), 0755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(python, script)
	cmd.Env = append(os.Environ(), "PICOCLIP_RUNTIME_LOGS="+dir, "PICOCLIP_RUN_ID=run_test", "PICOCLIP_TASK_ID=task_test")
	cmd.Stdin = strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"hook.hello","params":{}}
{"jsonrpc":"2.0","id":2,"method":"hook.approve_tool","params":{"tool":"bash","arguments":{"command":"git push origin main"}}}
`)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two responses, got %q", out)
	}
	var hello map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &hello); err != nil {
		t.Fatal(err)
	}
	result := hello["result"].(map[string]any)
	if result["name"] != "picoclip" {
		t.Fatalf("unexpected hello result: %#v", result)
	}
	var approval map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &approval); err != nil {
		t.Fatal(err)
	}
	approvalResult := approval["result"].(map[string]any)
	if approvalResult["approved"] != false {
		t.Fatalf("expected git push to be denied: %#v", approvalResult)
	}
}

func TestEnsurePicoClawHookPackWritesScriptAndMergesConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		t.Fatal(err)
	}
	existing := []byte(`{
  "model_list": [{"model_name":"topmodel","provider":"openai","model":"topmodel"}],
  "hooks": {"processes": {"custom": {"enabled": true}}},
  "agents": {"defaults": {"context_window": 4096}}
}`)
	if err := os.WriteFile(configPath, existing, 0644); err != nil {
		t.Fatal(err)
	}
	state := domain.RuntimeState{RuntimeID: "picoclaw", ConfigPath: configPath}
	if err := ensurePicoClawHookPack(state); err != nil {
		t.Fatalf("ensure hook pack: %v", err)
	}
	hookPath := filepath.Join(filepath.Dir(configPath), "hooks", "picoclip-hook.py")
	info, err := os.Stat(hookPath)
	if err != nil {
		t.Fatalf("missing hook script: %v", err)
	}
	if info.Mode().Perm()&0100 == 0 {
		t.Fatalf("hook script is not executable: %v", info.Mode())
	}
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if _, ok := config["model_list"].([]any); !ok {
		t.Fatalf("model_list was not preserved")
	}
	hooks := config["hooks"].(map[string]any)
	processes := hooks["processes"].(map[string]any)
	if _, ok := processes["custom"]; !ok {
		t.Fatalf("custom process hook was not preserved")
	}
	picoclip := processes["picoclip"].(map[string]any)
	command := picoclip["command"].([]any)
	if len(command) != 2 || !filepath.IsAbs(command[1].(string)) {
		t.Fatalf("managed hook command is not absolute: %#v", command)
	}
	agents := config["agents"].(map[string]any)
	defaults := agents["defaults"].(map[string]any)
	if defaults["context_window"].(float64) != 4096 {
		t.Fatalf("existing context_window was overwritten: %#v", defaults["context_window"])
	}
	if defaults["steering_mode"] != "all" {
		t.Fatalf("steering_mode not configured: %#v", defaults["steering_mode"])
	}
	evolution := config["evolution"].(map[string]any)
	if evolution["mode"] != "observe" {
		t.Fatalf("evolution mode = %#v", evolution["mode"])
	}
}
