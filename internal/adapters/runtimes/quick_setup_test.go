package runtimes

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	"picoclip/internal/core/domain"
)

func TestValidateOpenAICompatibleInput(t *testing.T) {
	for _, tc := range []struct {
		base, model string
		ok          bool
	}{
		{"http://127.0.0.1:11434/v1", "qwen/model", true},
		{"https://provider.example/v1", "model", true},
		{"ftp://provider.example", "model", false},
		{"provider.example", "model", false},
		{"https://provider.example", " ", false},
	} {
		err := validateOpenAICompatibleInput(tc.base, tc.model)
		if (err == nil) != tc.ok {
			t.Fatalf("base=%q model=%q err=%v", tc.base, tc.model, err)
		}
	}
}

func TestCrushQuickSetupPreservesUnrelatedAndSecretSemantics(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "crush.json")
	original := `{"$schema":"x","mcp":{"server":true},"providers":{"other":{"type":"x"},"picoclip-openai":{"base_url":"https://old/v1","api_key":"old-secret","models":[{"id":"old","name":"old"},{"id":"keep","name":"Keep"}],"custom":true}},"models":{"large":{"model":"old","provider":"picoclip-openai","temperature":0.2},"other":{"model":"x"}}}`
	mustWrite(t, path, original, 0600)
	a := NewCrushAdapter("")
	state := domain.RuntimeState{ConfigPath: path}
	view, err := a.ReadQuickSetup(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if view.Values["base_url"] != "https://old/v1" || view.Values["model"] != "old" || !view.SecretConfigured || strings.Contains(mustJSON(t, view), "old-secret") {
		t.Fatalf("bad redacted view: %#v", view)
	}
	if err := a.ApplyQuickSetup(context.Background(), state, quickInput(view.Revision, "https://new/v1", "new/model", "", false)); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	readJSON(t, path, &got)
	if got["mcp"].(map[string]any)["server"] != true {
		t.Fatal("lost unrelated field")
	}
	provider := got["providers"].(map[string]any)["picoclip-openai"].(map[string]any)
	if provider["api_key"] != "old-secret" || provider["base_url"] != "https://new/v1" || provider["custom"] != true {
		t.Fatalf("provider=%#v", provider)
	}
	if got["models"].(map[string]any)["large"].(map[string]any)["temperature"] != 0.2 {
		t.Fatal("lost tuning")
	}
	view, _ = a.ReadQuickSetup(context.Background(), state)
	if err := a.ApplyQuickSetup(context.Background(), state, quickInput(view.Revision, "https://new/v1", "next", "replacement", false)); err != nil {
		t.Fatal(err)
	}
	view, _ = a.ReadQuickSetup(context.Background(), state)
	if err := a.ApplyQuickSetup(context.Background(), state, quickInput(view.Revision, "https://new/v1", "next", "", true)); err != nil {
		t.Fatal(err)
	}
	readJSON(t, path, &got)
	provider = got["providers"].(map[string]any)["picoclip-openai"].(map[string]any)
	if _, ok := provider["api_key"]; ok {
		t.Fatal("key not cleared")
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0600 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
	before, _ := os.ReadFile(path)
	err = a.ApplyQuickSetup(context.Background(), state, quickInput("stale", "https://x/v1", "m", "", false))
	if !errors.Is(err, domain.ErrConfigurationChanged) {
		t.Fatalf("err=%v", err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("stale write changed file")
	}
}

func TestPicoClawQuickSetupKeepsSecretOnlyInSecurityAndDeduplicates(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config.json")
	security := filepath.Join(dir, ".security.yml")
	mustWrite(t, config, `{"agents":{"defaults":{"workspace":"/tmp/w","restrict_to_workspace":true}},"tools":{"mcp":{"enabled":false}},"model_list":[{"model_name":"other","model":"x"},{"model_name":"picoclip-default","model":"openai/old","api_base":"https://old/v1","api_keys":["json-secret"],"custom":1},{"model_name":"picoclip-default","model":"duplicate"}]}`, 0600)
	mustWrite(t, security, "other: keep\nmodel_list:\n  picoclip-default:0:\n    api_keys:\n      - old-secret\n  other:0:\n    api_keys: [other-secret]\n", 0600)
	a := NewPicoClawAdapter("")
	state := domain.RuntimeState{ConfigPath: config}
	view, err := a.ReadQuickSetup(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if !view.SecretConfigured || strings.Contains(mustJSON(t, view), "old-secret") {
		t.Fatal("secret leaked")
	}
	if err := a.ApplyQuickSetup(context.Background(), state, quickInput(view.Revision, "http://localhost:4000/v1", "org/model", "new-secret", false)); err != nil {
		t.Fatal(err)
	}
	bytes, _ := os.ReadFile(config)
	if strings.Contains(string(bytes), "new-secret") {
		t.Fatal("secret in json")
	}
	if strings.Contains(string(bytes), "json-secret") || strings.Contains(string(bytes), "api_keys") {
		t.Fatal("legacy managed secret remained in config.json")
	}
	var got map[string]any
	readJSON(t, config, &got)
	if got["version"].(float64) != 3 {
		t.Fatal("missing v3")
	}
	if got["tools"].(map[string]any)["mcp"] == nil {
		t.Fatal("lost tools")
	}
	count := 0
	for _, raw := range got["model_list"].([]any) {
		if raw.(map[string]any)["model_name"] == "picoclip-default" {
			count++
			if raw.(map[string]any)["provider"] != "openai" || raw.(map[string]any)["model"] != "org/model" {
				t.Fatalf("managed model must use current explicit PicoClaw provider format: %#v", raw)
			}
			if raw.(map[string]any)["custom"] != float64(1) {
				t.Fatal("lost optional")
			}
		}
	}
	if count != 1 {
		t.Fatalf("managed count=%d", count)
	}
	sec, _ := os.ReadFile(security)
	if !strings.Contains(string(sec), "new-secret") || !strings.Contains(string(sec), "other-secret") {
		t.Fatalf("security=%s", sec)
	}
	if info, _ := os.Stat(security); info.Mode().Perm() != 0600 {
		t.Fatalf("security mode=%o", info.Mode().Perm())
	}
	if info, _ := os.Stat(config); info.Mode().Perm() != 0600 {
		t.Fatalf("config mode=%o", info.Mode().Perm())
	}
	view, _ = a.ReadQuickSetup(context.Background(), state)
	if err := a.ApplyQuickSetup(context.Background(), state, quickInput(view.Revision, "http://localhost:4000/v1", "org/model", "", true)); err != nil {
		t.Fatal(err)
	}
	sec, _ = os.ReadFile(security)
	var node any
	if err := yaml.Unmarshal(sec, &node); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sec), "new-secret") {
		t.Fatal("secret not cleared")
	}
}

func TestPicoClawQuickSetupRecognizesBaseSecurityCredential(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config.json")
	security := filepath.Join(dir, ".security.yml")
	mustWrite(t, config, `{"version":3,"model_list":[{"model_name":"picoclip-default","provider":"openai","model":"m","api_base":"https://x/v1"}]}`, 0600)
	mustWrite(t, security, "model_list:\n  picoclip-default:\n    api_keys: [base-secret]\n", 0600)
	view, err := NewPicoClawAdapter("").ReadQuickSetup(context.Background(), domain.RuntimeState{ConfigPath: config})
	if err != nil || !view.SecretConfigured {
		t.Fatalf("view=%#v err=%v", view, err)
	}
}

func TestExistingPathsHonorRuntimeOverrides(t *testing.T) {
	t.Setenv("PICOCLAW_CONFIG", "/custom/picoclaw.json")
	t.Setenv("CLAURST_HOME", "/custom/claurst")
	if got := NewPicoClawAdapter("").ResolveExistingPaths("/bin/picoclaw"); got.ConfigPath != "/custom/picoclaw.json" {
		t.Fatalf("picoclaw=%#v", got)
	}
	if got := NewClaurstAdapter("").ResolveExistingPaths("/bin/claurst"); got.ConfigPath != "/custom/claurst/settings.json" || got.HomePath != "/custom/claurst" {
		t.Fatalf("claurst=%#v", got)
	}
}

func TestClaurstQuickSetupPreservesAdvancedSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	mustWrite(t, path, `{"version":3,"theme":"dark","config":{"other":true},"providers":{"openai":{"api_key":"old","options":{"reasoning":true}},"other":{"enabled":true}},"projects":["x"]}`, 0644)
	a := NewClaurstAdapter("")
	state := domain.RuntimeState{ConfigPath: path}
	view, err := a.ReadQuickSetup(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.ApplyQuickSetup(context.Background(), state, quickInput(view.Revision, "https://api.example/v1", "gpt-x", "", false)); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	readJSON(t, path, &got)
	if got["version"].(float64) != 3 || got["theme"] != "dark" || got["provider"] != "openai" {
		t.Fatalf("got=%#v", got)
	}
	openai := got["providers"].(map[string]any)["openai"].(map[string]any)
	if openai["api_key"] != "old" || openai["api_base"] != "https://api.example/v1" || openai["options"] == nil {
		t.Fatalf("openai=%#v", openai)
	}
	if got["config"].(map[string]any)["model"] != "gpt-x" || got["config"].(map[string]any)["other"] != true {
		t.Fatal("config merge failed")
	}
}

func quickInput(rev, base, model, key string, clear bool) domain.RuntimeQuickSetupInput {
	return domain.RuntimeQuickSetupInput{ProfileID: "openai-compatible", Values: map[string]string{"base_url": base, "model": model}, APIKey: key, ClearAPIKey: clear, Revision: rev}
}
func mustWrite(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}
func readJSON(t *testing.T, path string, dst any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, dst); err != nil {
		t.Fatal(err)
	}
}
func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
