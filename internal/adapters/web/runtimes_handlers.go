package web

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/services"
)

type RuntimeCardView struct {
	ID          domain.RuntimeID
	Name        string
	Description string
	Kind        string
	Repo        string
	DocsURL     string
	State       domain.RuntimeState
	Configured  bool
	Health      domain.RuntimeHealth
	ConfigFiles []domain.RuntimeConfigFile
	BasicConfig RuntimeBasicConfig
	Versions    []domain.RuntimeVersion
	Tested      bool
	TestedAt    string
	Functional  bool
	Checks      []domain.DiagnosticCheck
	AITested    bool
	AITestedAt  string
	AIOk        bool
	AIMessage   string
	AIOutput    string
}

type RuntimeBasicConfig struct {
	ProviderID   string
	ProviderName string
	ProviderType string
	BaseURL      string
	ModelID      string
	ModelAlias   string
	APIKey       string
}

func (s *Server) runtimeCards(r *http.Request) []RuntimeCardView {
	states, _ := s.runtimes.States(r.Context())
	cards := make([]RuntimeCardView, 0, len(s.runtimes.Catalog()))
	for _, manifest := range s.runtimes.Catalog() {
		state, configured := states[manifest.ID]
		health := domain.RuntimeHealth{Status: "not_configured"}
		var configFiles []domain.RuntimeConfigFile
		var versions []domain.RuntimeVersion
		tested, testedAt, functional, checks, savedHealth := runtimeHealthSummary(state)
		aiTested, aiTestedAt, aiOK, aiMessage, aiOutput := runtimeAITestSummary(state)
		if configured {
			if tested {
				health = savedHealth
			}
			if adapter, ok := s.runtimes.Adapter(manifest.ID); ok {
				configFiles, _ = adapter.ReadConfig(r.Context(), state)
			}
		}
		cards = append(cards, RuntimeCardView{
			ID:          manifest.ID,
			Name:        manifest.Name,
			Description: manifest.Description,
			Kind:        string(manifest.Kind),
			Repo:        manifest.Repo,
			DocsURL:     manifest.DocsURL,
			State:       state,
			Configured:  configured,
			Health:      health,
			ConfigFiles: configFiles,
			BasicConfig: runtimeBasicConfigFromFiles(manifest.ID, configFiles),
			Versions:    versions,
			Tested:      tested,
			TestedAt:    testedAt,
			Functional:  functional,
			Checks:      checks,
			AITested:    aiTested,
			AITestedAt:  aiTestedAt,
			AIOk:        aiOK,
			AIMessage:   aiMessage,
			AIOutput:    aiOutput,
		})
	}
	return cards
}

func (s *Server) handleAPIRuntimes(w http.ResponseWriter, r *http.Request) {
	s.jsonResponse(w, s.runtimeCards(r))
}

func runtimeBasicConfigFromFiles(id domain.RuntimeID, files []domain.RuntimeConfigFile) RuntimeBasicConfig {
	cfg := RuntimeBasicConfig{ProviderID: "openai", ProviderName: "OpenAI", ProviderType: "openai", BaseURL: "https://api.openai.com/v1", ModelID: "gpt-4o", ModelAlias: "default", APIKey: "$OPENAI_API_KEY"}
	if id == "picoclaw" {
		cfg.ProviderType = "openai"
		cfg.ModelID = "gpt-4o"
		cfg.ModelAlias = "default"
	}
	for _, file := range files {
		if !strings.HasSuffix(file.Name, ".json") {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal(file.Content, &raw); err != nil {
			continue
		}
		if id == "crush" {
			if providers, ok := raw["providers"].(map[string]any); ok {
				for providerID, providerRaw := range providers {
					provider, ok := providerRaw.(map[string]any)
					if !ok {
						continue
					}
					cfg.ProviderID = providerID
					cfg.ProviderName, _ = provider["name"].(string)
					cfg.ProviderType, _ = provider["type"].(string)
					cfg.BaseURL, _ = provider["base_url"].(string)
					cfg.APIKey, _ = provider["api_key"].(string)
					if models, ok := provider["models"].([]any); ok && len(models) > 0 {
						if model, ok := models[0].(map[string]any); ok {
							cfg.ModelID, _ = model["id"].(string)
						}
					}
					if cfg.ProviderName == "" {
						cfg.ProviderName = cfg.ProviderID
					}
					return cfg
				}
			}
		}
		if id == "picoclaw" {
			if agents, ok := raw["agents"].(map[string]any); ok {
				if defaults, ok := agents["defaults"].(map[string]any); ok {
					cfg.ModelAlias, _ = defaults["model_name"].(string)
				}
			}
			if models, ok := raw["model_list"].([]any); ok && len(models) > 0 {
				if model, ok := models[0].(map[string]any); ok {
					cfg.ModelAlias, _ = model["model_name"].(string)
					cfg.ProviderID, _ = model["provider"].(string)
					cfg.ModelID, _ = model["model"].(string)
					if cfg.ProviderID == "" {
						cfg.ProviderID = "openai"
					}
					cfg.ProviderName = cfg.ProviderID
					cfg.ProviderType = "openai"
					cfg.ModelID = picoclawCleanModelID(cfg.ProviderID, cfg.ModelID)
					cfg.BaseURL, _ = model["api_base"].(string)
					cfg.APIKey, _ = model["api_key"].(string)
				}
			}
		}
	}
	return cfg
}

func picoclawCleanModelID(providerID, modelID string) string {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return modelID
	}
	if prefix, rest, ok := strings.Cut(modelID, "/"); ok && prefix == providerID {
		return strings.TrimSpace(rest)
	}
	if strings.HasPrefix(modelID, "cliproxyapi/") {
		return strings.TrimPrefix(modelID, "cliproxyapi/")
	}
	return modelID
}

func runtimeHealthSummary(state domain.RuntimeState) (tested bool, testedAt string, functional bool, checks []domain.DiagnosticCheck, health domain.RuntimeHealth) {
	if state.LastHealthAt == nil || state.LastHealthJSON == "" || state.LastHealthJSON == "{}" {
		return false, "", false, nil, health
	}
	if err := json.Unmarshal([]byte(state.LastHealthJSON), &health); err != nil {
		return false, "", false, nil, health
	}
	functional = health.Status == "ok"
	testedAt = timeSince(*state.LastHealthAt)
	checks = health.Checks
	return true, testedAt, functional, checks, health
}

func runtimeAITestSummary(state domain.RuntimeState) (tested bool, testedAt string, ok bool, message string, output string) {
	if state.MetadataJSON == "" || state.MetadataJSON == "{}" {
		return false, "", false, "", ""
	}
	var metadata struct {
		LastAITest *services.RuntimeAITestResult `json:"last_ai_test"`
	}
	if err := json.Unmarshal([]byte(state.MetadataJSON), &metadata); err != nil || metadata.LastAITest == nil {
		return false, "", false, "", ""
	}
	res := metadata.LastAITest
	ok = res.Status == "ok"
	message = res.Message
	output = res.Output
	testedAt = timeSince(res.CheckedAt)
	return true, testedAt, ok, message, output
}

func (s *Server) handleWebPostRuntimeExisting(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	runtimeID := domain.RuntimeID(r.PathValue("id"))
	binPath := strings.TrimSpace(r.FormValue("bin_path"))
	if binPath == "" {
		http.Error(w, "binary path required", http.StatusBadRequest)
		return
	}
	if _, err := s.runtimes.ConfigureExisting(r.Context(), runtimeID, binPath); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, _ = s.runtimes.Test(r.Context(), runtimeID)
	s.handleWebSettings(w, r)
}

func (s *Server) handleWebPostRuntimeInstall(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	runtimeID := domain.RuntimeID(r.PathValue("id"))
	mode := domain.InstallMode(r.FormValue("mode"))
	if mode == "" {
		mode = domain.InstallModeExclusive
	}
	versionAlias := strings.TrimSpace(r.FormValue("version_alias"))
	if _, err := s.runtimes.Install(r.Context(), runtimeID, mode, versionAlias); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, _ = s.runtimes.Test(r.Context(), runtimeID)
	s.handleWebSettings(w, r)
}

func (s *Server) handleWebPostRuntimeUninstall(w http.ResponseWriter, r *http.Request) {
	runtimeID := domain.RuntimeID(r.PathValue("id"))
	if err := s.runtimes.Uninstall(r.Context(), runtimeID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.handleWebSettings(w, r)
}

func (s *Server) handleWebPostRuntimeToggle(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	runtimeID := domain.RuntimeID(r.PathValue("id"))
	enabled := r.FormValue("enabled") == "true"
	if _, err := s.runtimes.SetEnabled(r.Context(), runtimeID, enabled); err != nil {
		http.Error(w, "runtime unavailable", http.StatusBadRequest)
		return
	}
	if enabled {
		w.Header().Set("HX-Trigger", `{"picoclip-toast":{"message":"Runtime enabled.","type":"success"}}`)
	} else {
		w.Header().Set("HX-Trigger", `{"picoclip-toast":{"message":"Runtime disabled.","type":"success"}}`)
	}
	s.handleWebSettings(w, r)
}

func (s *Server) handleWebPostRuntimeBasicConfig(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	runtimeID := domain.RuntimeID(r.PathValue("id"))
	state, err := s.runtimes.State(r.Context(), runtimeID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	adapter, ok := s.runtimes.Adapter(runtimeID)
	if !ok {
		http.Error(w, "runtime unavailable", http.StatusBadRequest)
		return
	}
	content, fileName, err := runtimeBasicConfigContent(runtimeID, state, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := adapter.WriteConfig(r.Context(), state, fileName, content); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, _ = s.runtimes.Test(r.Context(), runtimeID)
	if r.FormValue("test_ai") == "true" {
		result, err := s.runtimes.TestAI(r.Context(), runtimeID)
		if err != nil || result.Status != "ok" {
			w.Header().Set("HX-Trigger", `{"picoclip-toast":{"message":"Provider saved, but AI test failed.","type":"error"}}`)
		} else {
			w.Header().Set("HX-Trigger", `{"picoclip-toast":{"message":"Provider saved and AI test passed.","type":"success"}}`)
		}
		s.handleWebSettings(w, r)
		return
	}
	w.Header().Set("HX-Trigger", `{"picoclip-toast":{"message":"Runtime provider saved.","type":"success"}}`)
	s.handleWebSettings(w, r)
}

func runtimeBasicConfigContent(runtimeID domain.RuntimeID, state domain.RuntimeState, r *http.Request) ([]byte, string, error) {
	providerID := strings.TrimSpace(r.FormValue("provider_id"))
	providerName := strings.TrimSpace(r.FormValue("provider_name"))
	providerType := strings.TrimSpace(r.FormValue("provider_type"))
	baseURL := strings.TrimSpace(r.FormValue("base_url"))
	modelID := strings.TrimSpace(r.FormValue("model_id"))
	modelAlias := strings.TrimSpace(r.FormValue("model_alias"))
	apiKey := strings.TrimSpace(r.FormValue("api_key"))
	if providerID == "" || providerType == "" || modelID == "" {
		return nil, "", domain.ErrInvalidInput
	}
	if providerName == "" {
		providerName = providerID
	}
	if modelAlias == "" {
		modelAlias = modelID
	}
	if runtimeID == "picoclaw" {
		modelID = picoclawCleanModelID(providerID, modelID)
		if providerID == "cliproxyapi" {
			providerID = "openai"
		}
		config := map[string]any{}
		if state.ConfigPath != "" {
			if existing, readErr := os.ReadFile(state.ConfigPath); readErr == nil && json.Valid(existing) {
				_ = json.Unmarshal(existing, &config)
			}
		}
		agents, _ := config["agents"].(map[string]any)
		if agents == nil {
			agents = map[string]any{}
		}
		defaults, _ := agents["defaults"].(map[string]any)
		if defaults == nil {
			defaults = map[string]any{}
		}
		defaults["model_name"] = modelAlias
		defaults["workspace"] = state.HomePath + "/workspace"
		defaults["restrict_to_workspace"] = true
		agents["defaults"] = defaults
		config["agents"] = agents
		if config["tools"] == nil {
			config["tools"] = map[string]any{"exec": map[string]any{"enabled": true, "enable_deny_patterns": true}, "mcp": map[string]any{"enabled": false, "servers": map[string]any{}}}
		}
		config["model_list"] = []map[string]any{{"model_name": modelAlias, "provider": providerID, "model": modelID}}
		model := config["model_list"].([]map[string]any)[0]
		if baseURL != "" {
			model["api_base"] = baseURL
		}
		if apiKey != "" {
			model["api_key"] = apiKey
		}
		raw, err := json.MarshalIndent(config, "", "  ")
		return raw, "config.json", err
	}
	provider := map[string]any{
		"id":     providerID,
		"name":   providerName,
		"type":   providerType,
		"models": []map[string]any{{"id": modelID, "name": modelAlias}},
	}
	if baseURL != "" {
		provider["base_url"] = baseURL
	}
	if apiKey != "" {
		provider["api_key"] = apiKey
	}
	config := map[string]any{}
	if state.ConfigPath != "" {
		if existing, readErr := os.ReadFile(state.ConfigPath); readErr == nil && json.Valid(existing) {
			_ = json.Unmarshal(existing, &config)
		}
	}
	if config["$schema"] == nil {
		config["$schema"] = "https://charm.land/crush.json"
	}
	providers, _ := config["providers"].(map[string]any)
	if providers == nil {
		providers = map[string]any{}
	}
	providers[providerID] = provider
	config["providers"] = providers
	options, _ := config["options"].(map[string]any)
	if options == nil {
		options = map[string]any{}
	}
	options["disable_metrics"] = true
	config["options"] = options
	raw, err := json.MarshalIndent(config, "", "  ")
	return raw, "crush.json", err
}

func (s *Server) handleWebPostRuntimeConfig(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	runtimeID := domain.RuntimeID(r.PathValue("id"))
	fileName := r.FormValue("file_name")
	content := []byte(r.FormValue("content"))
	state, err := s.runtimes.State(r.Context(), runtimeID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	adapter, ok := s.runtimes.Adapter(runtimeID)
	if !ok {
		http.Error(w, "runtime unavailable", http.StatusBadRequest)
		return
	}
	if strings.HasSuffix(fileName, ".json") && !json.Valid(content) {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := adapter.WriteConfig(r.Context(), state, fileName, content); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, _ = s.runtimes.Test(r.Context(), runtimeID)
	s.handleWebSettings(w, r)
}

func (s *Server) handleWebPostRuntimeTest(w http.ResponseWriter, r *http.Request) {
	runtimeID := domain.RuntimeID(r.PathValue("id"))
	if _, err := s.runtimes.Test(r.Context(), runtimeID); err != nil {
		w.Header().Set("HX-Trigger", `{"picoclip-toast":{"message":"CLI check failed.","type":"error"}}`)
		s.handleWebSettings(w, r)
		return
	}
	w.Header().Set("HX-Trigger", `{"picoclip-toast":{"message":"CLI check successful.","type":"success"}}`)
	s.handleWebSettings(w, r)
}

func (s *Server) handleWebPostRuntimeTestAI(w http.ResponseWriter, r *http.Request) {
	runtimeID := domain.RuntimeID(r.PathValue("id"))
	result, err := s.runtimes.TestAI(r.Context(), runtimeID)
	if err != nil || result.Status != "ok" {
		w.Header().Set("HX-Trigger", `{"picoclip-toast":{"message":"AI test failed.","type":"error"}}`)
	} else {
		w.Header().Set("HX-Trigger", `{"picoclip-toast":{"message":"AI test successful.","type":"success"}}`)
	}
	s.handleWebSettings(w, r)
}
