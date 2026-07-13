package runtimes

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"picoclip/internal/core/domain"
)

var crushDefaultConfig = []byte("{\n  \"$schema\": \"https://charm.land/crush.json\"\n}\n")
var picoClawDefaultConfig = []byte("{\n  \"version\": 3,\n  \"agents\": {\"defaults\": {}}\n}\n")
var picoClawDefaultSecurity = []byte("# Sensitive PicoClaw values managed by PicoClip.\n")
var claurstDefaultConfig = []byte("{\n  \"version\": 1,\n  \"provider\": \"openai\",\n  \"config\": {\"provider\": \"openai\", \"model\": \"\"},\n  \"providers\": {}\n}\n")

func (a *CrushAdapter) QuickSetupSchema() domain.RuntimeQuickSetupSchema {
	return openAIQuickSetupSchema()
}
func (a *PicoClawAdapter) QuickSetupSchema() domain.RuntimeQuickSetupSchema {
	return openAIQuickSetupSchema()
}
func (a *ClaurstAdapter) QuickSetupSchema() domain.RuntimeQuickSetupSchema {
	return openAIQuickSetupSchema()
}

func (a *CrushAdapter) ResolveExistingPaths(binPath string) domain.RuntimeState {
	configDir := xdgPath("XDG_CONFIG_HOME", ".config")
	data := filepath.Join(xdgPath("XDG_DATA_HOME", filepath.Join(".local", "share")), "crush")
	return domain.RuntimeState{BinPath: binPath, ConfigPath: filepath.Join(configDir, "crush", "crush.json"), DataPath: data, LogsPath: filepath.Join(data, "logs")}
}
func (a *PicoClawAdapter) ResolveExistingPaths(binPath string) domain.RuntimeState {
	if config := strings.TrimSpace(os.Getenv("PICOCLAW_CONFIG")); config != "" {
		home := filepath.Dir(config)
		return domain.RuntimeState{BinPath: binPath, ConfigPath: config, HomePath: home, DataPath: home, LogsPath: filepath.Join(home, "logs")}
	}
	home := strings.TrimSpace(os.Getenv("PICOCLAW_HOME"))
	if home == "" {
		home = filepath.Join(existingHome(), ".picoclaw")
	}
	return domain.RuntimeState{BinPath: binPath, ConfigPath: filepath.Join(home, "config.json"), HomePath: home, DataPath: home, LogsPath: filepath.Join(home, "logs")}
}
func (a *ClaurstAdapter) ResolveExistingPaths(binPath string) domain.RuntimeState {
	home := strings.TrimSpace(os.Getenv("CLAURST_HOME"))
	if home == "" {
		legacy := filepath.Join(existingHome(), ".claurst")
		if _, err := os.Stat(legacy); err == nil {
			home = legacy
		} else {
			home = filepath.Join(xdgPath("XDG_CONFIG_HOME", ".config"), "claurst")
		}
	}
	return domain.RuntimeState{BinPath: binPath, ConfigPath: filepath.Join(home, "settings.json"), HomePath: home, DataPath: home, LogsPath: filepath.Join(home, "logs")}
}

func (a *CrushAdapter) ReadQuickSetup(ctx context.Context, state domain.RuntimeState) (domain.RuntimeQuickSetupView, error) {
	content, err := readConfigOrDefault(state.ConfigPath, crushDefaultConfig)
	if err != nil {
		return domain.RuntimeQuickSetupView{}, err
	}
	root, err := decodeJSONObject(content)
	if err != nil {
		return domain.RuntimeQuickSetupView{}, err
	}
	view := domain.RuntimeQuickSetupView{ProfileID: quickSetupProfileID, Values: map[string]string{}, Revision: configRevision(namedBytes{"crush.json", content})}
	providers, err := optionalObject(root, "providers")
	if err != nil {
		return view, err
	}
	provider, err := optionalObject(providers, "picoclip-openai")
	if err != nil || provider == nil {
		return view, err
	}
	view.Values["base_url"], err = stringValue(provider, "base_url")
	if err != nil {
		return view, err
	}
	key, err := stringValue(provider, "api_key")
	if err != nil {
		return view, err
	}
	view.SecretConfigured = key != ""
	if models, ok := provider["models"].([]any); ok && len(models) > 0 {
		if model, ok := models[0].(map[string]any); ok {
			view.Values["model"], err = stringValue(model, "id")
		}
	} else if _, exists := provider["models"]; exists {
		return view, fmt.Errorf("configuration field %q must be an array", "models")
	}
	view.Configured = view.Values["base_url"] != "" && view.Values["model"] != ""
	return view, err
}

func (a *CrushAdapter) ApplyQuickSetup(ctx context.Context, state domain.RuntimeState, input domain.RuntimeQuickSetupInput) error {
	if err := validateOpenAICompatibleInput(input.Values["base_url"], input.Values["model"]); err != nil {
		return err
	}
	content, err := readConfigOrDefault(state.ConfigPath, crushDefaultConfig)
	if err != nil {
		return err
	}
	if err := requireRevision(input, configRevision(namedBytes{"crush.json", content})); err != nil {
		return err
	}
	root, err := decodeJSONObject(content)
	if err != nil {
		return err
	}
	providers, err := object(root, "providers")
	if err != nil {
		return err
	}
	provider, err := object(providers, "picoclip-openai")
	if err != nil {
		return err
	}
	previousModel := ""
	if models, ok := provider["models"].([]any); ok {
		for _, raw := range models {
			if m, ok := raw.(map[string]any); ok {
				id, _ := stringValue(m, "id")
				if id != "" {
					previousModel = id
					break
				}
			}
		}
	}
	provider["type"] = "openai-compat"
	provider["base_url"] = strings.TrimSpace(input.Values["base_url"])
	secretUpdate(provider, "api_key", input)
	models, _ := provider["models"].([]any)
	updated := false
	for _, raw := range models {
		if m, ok := raw.(map[string]any); ok {
			id, _ := stringValue(m, "id")
			if !updated && (id == previousModel || id == input.Values["model"]) {
				m["id"] = strings.TrimSpace(input.Values["model"])
				m["name"] = strings.TrimSpace(input.Values["model"])
				updated = true
			}
		}
	}
	if !updated {
		models = append(models, map[string]any{"id": strings.TrimSpace(input.Values["model"]), "name": strings.TrimSpace(input.Values["model"])})
	}
	provider["models"] = models
	aliases, err := object(root, "models")
	if err != nil {
		return err
	}
	for _, name := range []string{"large", "small"} {
		alias, err := object(aliases, name)
		if err != nil {
			return err
		}
		alias["model"] = strings.TrimSpace(input.Values["model"])
		alias["provider"] = "picoclip-openai"
	}
	result, err := marshalJSONObject(root)
	if err != nil {
		return err
	}
	return atomicWriteFile(state.ConfigPath, result, secureConfigMode(state.ConfigPath))
}

func (a *CrushAdapter) TestQuickSetup(ctx context.Context, state domain.RuntimeState, input domain.RuntimeQuickSetupInput) (domain.RuntimeModelTestResult, error) {
	apiKey := input.APIKey
	if apiKey == "" && !input.ClearAPIKey {
		content, err := readConfigOrDefault(state.ConfigPath, crushDefaultConfig)
		if err != nil {
			return domain.RuntimeModelTestResult{}, err
		}
		root, err := decodeJSONObject(content)
		if err != nil {
			return domain.RuntimeModelTestResult{}, err
		}
		providers, _ := optionalObject(root, "providers")
		provider, _ := optionalObject(providers, "picoclip-openai")
		if provider != nil {
			apiKey, _ = stringValue(provider, "api_key")
		}
	}
	return testOpenAICompatibleModel(ctx, input.Values["base_url"], apiKey, input.Values["model"])
}

func (a *PicoClawAdapter) ReadQuickSetup(ctx context.Context, state domain.RuntimeState) (domain.RuntimeQuickSetupView, error) {
	config, err := readConfigOrDefault(state.ConfigPath, picoClawDefaultConfig)
	if err != nil {
		return domain.RuntimeQuickSetupView{}, err
	}
	securityPath := filepath.Join(filepath.Dir(state.ConfigPath), ".security.yml")
	security, err := readConfigOrDefault(securityPath, picoClawDefaultSecurity)
	if err != nil {
		return domain.RuntimeQuickSetupView{}, err
	}
	root, err := decodeJSONObject(config)
	if err != nil {
		return domain.RuntimeQuickSetupView{}, err
	}
	sec, err := decodeYAMLMap(security)
	if err != nil {
		return domain.RuntimeQuickSetupView{}, err
	}
	view := domain.RuntimeQuickSetupView{ProfileID: quickSetupProfileID, Values: map[string]string{}, Revision: configRevision(namedBytes{"config.json", config}, namedBytes{".security.yml", security})}
	if list, ok := root["model_list"].([]any); ok {
		for _, raw := range list {
			entry, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			name, _ := stringValue(entry, "model_name")
			if name == "picoclip-default" {
				model, e := stringValue(entry, "model")
				if e != nil {
					return view, e
				}
				provider, providerErr := stringValue(entry, "provider")
				if providerErr != nil {
					return view, providerErr
				}
				if provider == "" {
					// PicoClaw keeps the legacy provider/model form compatible.
					model = strings.TrimPrefix(model, "openai/")
				}
				view.Values["model"] = model
				view.Values["base_url"], err = stringValue(entry, "api_base")
				break
			}
		}
	} else if _, exists := root["model_list"]; exists {
		return view, fmt.Errorf("configuration field model_list must be an array")
	}
	if modelList, ok := sec["model_list"].(map[string]any); ok {
		credential, _ := modelList["picoclip-default:0"].(map[string]any)
		if credential == nil {
			credential, _ = modelList["picoclip-default"].(map[string]any)
		}
		if credential != nil {
			if keys, ok := credential["api_keys"].([]any); ok {
				view.SecretConfigured = len(keys) > 0
			}
		}
	}
	view.Configured = view.Values["base_url"] != "" && view.Values["model"] != ""
	return view, err
}

func (a *PicoClawAdapter) ApplyQuickSetup(ctx context.Context, state domain.RuntimeState, input domain.RuntimeQuickSetupInput) error {
	if err := validateOpenAICompatibleInput(input.Values["base_url"], input.Values["model"]); err != nil {
		return err
	}
	config, err := readConfigOrDefault(state.ConfigPath, picoClawDefaultConfig)
	if err != nil {
		return err
	}
	securityPath := filepath.Join(filepath.Dir(state.ConfigPath), ".security.yml")
	security, err := readConfigOrDefault(securityPath, picoClawDefaultSecurity)
	if err != nil {
		return err
	}
	if err = requireRevision(input, configRevision(namedBytes{"config.json", config}, namedBytes{".security.yml", security})); err != nil {
		return err
	}
	root, err := decodeJSONObject(config)
	if err != nil {
		return err
	}
	sec, err := decodeYAMLMap(security)
	if err != nil {
		return err
	}
	// PicoClaw v3 is the current schema. Do not downgrade a future version.
	if version, ok := root["version"].(float64); !ok || version < 3 {
		root["version"] = 3
	}
	var list []any
	if raw, exists := root["model_list"]; exists {
		var ok bool
		list, ok = raw.([]any)
		if !ok {
			return fmt.Errorf("configuration field model_list must be an array")
		}
	}
	var managed map[string]any
	filtered := make([]any, 0, len(list))
	for _, raw := range list {
		entry, ok := raw.(map[string]any)
		if !ok {
			filtered = append(filtered, raw)
			continue
		}
		name, _ := stringValue(entry, "model_name")
		if name == "picoclip-default" {
			if managed == nil {
				managed = entry
				filtered = append(filtered, entry)
			}
			continue
		}
		filtered = append(filtered, entry)
	}
	if managed == nil {
		managed = map[string]any{}
		filtered = append(filtered, managed)
	}
	managed["model_name"] = "picoclip-default"
	managed["provider"] = "openai"
	managed["model"] = strings.TrimSpace(input.Values["model"])
	managed["api_base"] = strings.TrimSpace(input.Values["base_url"])
	managed["enabled"] = true
	legacyKeys, _ := managed["api_keys"].([]any)
	delete(managed, "api_keys")
	root["model_list"] = filtered
	agents, err := object(root, "agents")
	if err != nil {
		return err
	}
	defaults, err := object(agents, "defaults")
	if err != nil {
		return err
	}
	defaults["model_name"] = "picoclip-default"
	modelList, err := mapObject(sec, "model_list")
	if err != nil {
		return err
	}
	credential, err := mapObject(modelList, "picoclip-default:0")
	if err != nil {
		return err
	}
	if input.ClearAPIKey {
		delete(credential, "api_keys")
	} else if input.APIKey != "" {
		credential["api_keys"] = []any{input.APIKey}
	} else if _, exists := credential["api_keys"]; !exists && len(legacyKeys) > 0 {
		credential["api_keys"] = legacyKeys
	}
	delete(modelList, "picoclip-default")
	newConfig, err := marshalJSONObject(root)
	if err != nil {
		return err
	}
	newSecurity, err := marshalYAMLMap(sec)
	if err != nil {
		return err
	}
	return atomicWritePairWithRollback(state.ConfigPath, newConfig, secureConfigMode(state.ConfigPath), securityPath, newSecurity, 0600, config)
}

func (a *PicoClawAdapter) TestQuickSetup(ctx context.Context, state domain.RuntimeState, input domain.RuntimeQuickSetupInput) (domain.RuntimeModelTestResult, error) {
	apiKey := input.APIKey
	if apiKey == "" && !input.ClearAPIKey {
		securityPath := filepath.Join(filepath.Dir(state.ConfigPath), ".security.yml")
		security, err := readConfigOrDefault(securityPath, picoClawDefaultSecurity)
		if err != nil {
			return domain.RuntimeModelTestResult{}, err
		}
		sec, err := decodeYAMLMap(security)
		if err != nil {
			return domain.RuntimeModelTestResult{}, err
		}
		if modelList, ok := sec["model_list"].(map[string]any); ok {
			credential, _ := modelList["picoclip-default:0"].(map[string]any)
			if credential == nil {
				credential, _ = modelList["picoclip-default"].(map[string]any)
			}
			if credential != nil {
				if keys, ok := credential["api_keys"].([]any); ok && len(keys) > 0 {
					apiKey, _ = keys[0].(string)
				}
			}
		}
	}
	return testOpenAICompatibleModel(ctx, input.Values["base_url"], apiKey, input.Values["model"])
}

func (a *ClaurstAdapter) ReadQuickSetup(ctx context.Context, state domain.RuntimeState) (domain.RuntimeQuickSetupView, error) {
	content, err := readConfigOrDefault(state.ConfigPath, claurstDefaultConfig)
	if err != nil {
		return domain.RuntimeQuickSetupView{}, err
	}
	root, err := decodeJSONObject(content)
	if err != nil {
		return domain.RuntimeQuickSetupView{}, err
	}
	view := domain.RuntimeQuickSetupView{ProfileID: quickSetupProfileID, Values: map[string]string{}, Revision: configRevision(namedBytes{"settings.json", content})}
	config, err := optionalObject(root, "config")
	if err != nil {
		return view, err
	}
	if config != nil {
		view.Values["model"], err = stringValue(config, "model")
		if err != nil {
			return view, err
		}
	}
	providers, err := optionalObject(root, "providers")
	if err != nil {
		return view, err
	}
	openai, err := optionalObject(providers, "openai")
	if err != nil {
		return view, err
	}
	if openai != nil {
		view.Values["base_url"], err = stringValue(openai, "api_base")
		if err != nil {
			return view, err
		}
		key, err := stringValue(openai, "api_key")
		if err != nil {
			return view, err
		}
		view.SecretConfigured = key != ""
	}
	provider, _ := stringValue(root, "provider")
	configProvider := ""
	if config != nil {
		configProvider, _ = stringValue(config, "provider")
	}
	view.Configured = (provider == "openai" || configProvider == "openai") && view.Values["base_url"] != "" && view.Values["model"] != ""
	return view, nil
}

func (a *ClaurstAdapter) ApplyQuickSetup(ctx context.Context, state domain.RuntimeState, input domain.RuntimeQuickSetupInput) error {
	if err := validateOpenAICompatibleInput(input.Values["base_url"], input.Values["model"]); err != nil {
		return err
	}
	content, err := readConfigOrDefault(state.ConfigPath, claurstDefaultConfig)
	if err != nil {
		return err
	}
	if err = requireRevision(input, configRevision(namedBytes{"settings.json", content})); err != nil {
		return err
	}
	root, err := decodeJSONObject(content)
	if err != nil {
		return err
	}
	if _, ok := root["version"]; !ok {
		root["version"] = 1
	}
	root["provider"] = "openai"
	config, err := object(root, "config")
	if err != nil {
		return err
	}
	config["provider"] = "openai"
	config["model"] = strings.TrimSpace(input.Values["model"])
	providers, err := object(root, "providers")
	if err != nil {
		return err
	}
	openai, err := object(providers, "openai")
	if err != nil {
		return err
	}
	openai["api_base"] = strings.TrimSpace(input.Values["base_url"])
	openai["enabled"] = true
	secretUpdate(openai, "api_key", input)
	result, err := marshalJSONObject(root)
	if err != nil {
		return err
	}
	return atomicWriteFile(state.ConfigPath, result, secureConfigMode(state.ConfigPath))
}

func (a *ClaurstAdapter) TestQuickSetup(ctx context.Context, state domain.RuntimeState, input domain.RuntimeQuickSetupInput) (domain.RuntimeModelTestResult, error) {
	apiKey := input.APIKey
	if apiKey == "" && !input.ClearAPIKey {
		content, err := readConfigOrDefault(state.ConfigPath, claurstDefaultConfig)
		if err != nil {
			return domain.RuntimeModelTestResult{}, err
		}
		root, err := decodeJSONObject(content)
		if err != nil {
			return domain.RuntimeModelTestResult{}, err
		}
		config, _ := optionalObject(root, "config")
		if config != nil {
			apiKey, _ = stringValue(config, "api_key")
		}
		if apiKey == "" {
			providers, _ := optionalObject(root, "providers")
			openai, _ := optionalObject(providers, "openai")
			if openai != nil {
				apiKey, _ = stringValue(openai, "api_key")
			}
		}
	}
	return testOpenAICompatibleModel(ctx, input.Values["base_url"], apiKey, input.Values["model"])
}

func optionalObject(parent map[string]any, key string) (map[string]any, error) {
	if parent == nil {
		return nil, nil
	}
	raw, ok := parent[key]
	if !ok {
		return nil, nil
	}
	value, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("configuration field %q must be an object", key)
	}
	return value, nil
}
func mapObject(parent map[string]any, key string) (map[string]any, error) {
	if raw, ok := parent[key]; ok {
		value, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("configuration field %q must be a mapping", key)
		}
		return value, nil
	}
	value := map[string]any{}
	parent[key] = value
	return value, nil
}

func ensureConfigFile(path string, content []byte, mode os.FileMode) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return atomicWriteFile(path, content, mode)
}
