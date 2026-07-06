package runtimes

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"picoclip/internal/core/domain"
)

func TestEnsureCrushHookPackWritesScriptsAndMergesConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config", "crush.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		t.Fatal(err)
	}
	existing := []byte(`{
  "$schema": "https://charm.land/crush.json",
  "providers": {"test": {"type": "openai-compat"}},
  "hooks": {"PreToolUse": [{"name":"custom-hook","matcher":"^bash$","command":"/tmp/custom.sh"}]}
}`)
	if err := os.WriteFile(configPath, existing, 0644); err != nil {
		t.Fatal(err)
	}

	state := domain.RuntimeState{RuntimeID: "crush", ConfigPath: configPath}
	if err := ensureCrushHookPack(state); err != nil {
		t.Fatalf("ensure hook pack: %v", err)
	}

	for _, name := range []string{"picoclip-guard.sh", "picoclip-context.sh", "picoclip-rewrite.sh", "picoclip-log-tool.sh"} {
		info, err := os.Stat(filepath.Join(filepath.Dir(configPath), "hooks", name))
		if err != nil {
			t.Fatalf("missing hook %s: %v", name, err)
		}
		if info.Mode().Perm()&0100 == 0 {
			t.Fatalf("hook %s is not executable: %v", name, info.Mode())
		}
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if _, ok := config["providers"].(map[string]any)["test"]; !ok {
		t.Fatalf("existing provider was not preserved")
	}
	hooks := config["hooks"].(map[string]any)["PreToolUse"].([]any)
	seen := map[string]bool{}
	for _, item := range hooks {
		entry := item.(map[string]any)
		name, _ := entry["name"].(string)
		seen[name] = true
		if name != "custom-hook" {
			command, _ := entry["command"].(string)
			if !filepath.IsAbs(command) {
				t.Fatalf("managed hook command is not absolute: %s", command)
			}
		}
	}
	for _, name := range []string{"custom-hook", "picoclip-guard", "picoclip-rewrite", "picoclip-context", "picoclip-log-tool"} {
		if !seen[name] {
			t.Fatalf("missing hook entry %s", name)
		}
	}
}
