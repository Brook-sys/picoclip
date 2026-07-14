package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
	"picoclip/internal/core/services"
)

type RuntimeCardView struct {
	ID                  domain.RuntimeID
	Name                string
	Description         string
	Kind                string
	Repo                string
	DocsURL             string
	State               domain.RuntimeState
	Configured          bool
	Health              domain.RuntimeHealth
	ConfigFiles         []domain.RuntimeConfigFile
	Versions            []domain.RuntimeVersion
	Tested              bool
	TestedAt            string
	Functional          bool
	Checks              []domain.DiagnosticCheck
	AITested            bool
	AITestedAt          string
	AIOk                bool
	AIMessage           string
	AIOutput            string
	QuickSetupSupported bool
	QuickSetupSchema    domain.RuntimeQuickSetupSchema
	QuickSetup          domain.RuntimeQuickSetupView
	QuickSetupError     string
}

func (s *Server) runtimeCards(r *http.Request) []RuntimeCardView {
	states, _ := s.runtimes.States(r.Context())
	cards := make([]RuntimeCardView, 0, len(s.runtimes.Catalog()))
	for _, manifest := range s.runtimes.Catalog() {
		state, configured := states[manifest.ID]
		health := domain.RuntimeHealth{Status: "not_configured"}
		var configFiles []domain.RuntimeConfigFile
		var versions []domain.RuntimeVersion
		var quickSchema domain.RuntimeQuickSetupSchema
		var quickSetup domain.RuntimeQuickSetupView
		var quickSetupError string
		quickSupported := false
		tested, testedAt, functional, checks, savedHealth := runtimeHealthSummary(state)
		aiTested, aiTestedAt, aiOK, aiMessage, aiOutput := runtimeAITestSummary(state)
		if configured {
			if freshHealth, err := s.runtimes.Test(r.Context(), manifest.ID); err == nil {
				health = freshHealth
				tested = true
				testedAt = "just now"
				functional = freshHealth.Status == "ok"
				checks = freshHealth.Checks
			} else if tested {
				health = savedHealth
			}
			if adapter, ok := s.runtimes.Adapter(manifest.ID); ok {
				configFiles, _ = adapter.ReadConfig(r.Context(), state)
				for i := range configFiles {
					configFiles[i].Revision = runtimeConfigRevision(configFiles[i].Content)
					configFiles[i].Content = redactRuntimeConfig(configFiles[i])
				}
				if _, ok := adapter.(ports.RuntimeQuickConfigurator); ok && functional {
					quickSupported = true
					var err error
					quickSchema, quickSetup, err = s.runtimes.QuickSetup(r.Context(), manifest.ID)
					if err != nil {
						quickSetupError = err.Error()
					}
				}
			}
		} else {
			if adapter, ok := s.runtimes.Adapter(manifest.ID); ok {
				versions, _ = adapter.ListVersions(r.Context(), 10)
			}
		}
		cards = append(cards, RuntimeCardView{
			ID:                  manifest.ID,
			Name:                manifest.Name,
			Description:         manifest.Description,
			Kind:                string(manifest.Kind),
			Repo:                manifest.Repo,
			DocsURL:             manifest.DocsURL,
			State:               state,
			Configured:          configured,
			Health:              health,
			ConfigFiles:         configFiles,
			Versions:            versions,
			Tested:              tested,
			TestedAt:            testedAt,
			Functional:          functional,
			Checks:              checks,
			AITested:            aiTested,
			AITestedAt:          aiTestedAt,
			AIOk:                aiOK,
			AIMessage:           aiMessage,
			AIOutput:            aiOutput,
			QuickSetupSupported: quickSupported,
			QuickSetupSchema:    quickSchema,
			QuickSetup:          quickSetup,
			QuickSetupError:     quickSetupError,
		})
	}
	return cards
}

func (s *Server) handleAPIRuntimes(w http.ResponseWriter, r *http.Request) {
	s.jsonResponse(w, s.runtimeCards(r))
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
		message := "Runtime install failed: " + err.Error()
		trigger, _ := json.Marshal(map[string]any{"picoclip-toast": map[string]string{"message": message, "type": "error"}})
		w.Header().Set("HX-Trigger", string(trigger))
		s.handleWebSettings(w, r)
		return
	}
	_, _ = s.runtimes.Test(r.Context(), runtimeID)
	w.Header().Set("HX-Trigger", `{"picoclip-toast":{"message":"Runtime installed and verified.","type":"success"}}`)
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

func (s *Server) handleWebPostRuntimeConfig(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	runtimeID := domain.RuntimeID(r.PathValue("id"))
	fileName := strings.TrimSpace(r.FormValue("file_name"))
	submitted := []byte(r.FormValue("content"))
	revision := strings.TrimSpace(r.FormValue("revision"))
	if filepath.Base(fileName) != fileName {
		http.Error(w, "unknown runtime config file", http.StatusBadRequest)
		return
	}
	err := s.runtimes.UpdateConfig(r.Context(), runtimeID, fileName, revision, func(original domain.RuntimeConfigFile) ([]byte, error) {
		if strings.HasSuffix(fileName, ".json") && !json.Valid(submitted) {
			return nil, errors.New("invalid json")
		}
		if strings.HasSuffix(fileName, ".yml") || strings.HasSuffix(fileName, ".yaml") {
			var parsed any
			if err := yaml.Unmarshal(submitted, &parsed); err != nil {
				return nil, errors.New("invalid yaml")
			}
		}
		return restoreRedactedRuntimeConfig(original, submitted)
	})
	if err != nil {
		if errors.Is(err, domain.ErrConfigurationChanged) {
			w.Header().Set("HX-Refresh", "true")
			http.Error(w, "runtime configuration changed; reload before saving", http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, _ = s.runtimes.Test(r.Context(), runtimeID)
	s.handleWebSettings(w, r)
}

func (s *Server) handleWebPostRuntimeQuickSetup(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, err := s.runtimes.ApplyQuickSetup(r.Context(), domain.RuntimeID(r.PathValue("id")), runtimeQuickSetupInput(r))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, domain.ErrConfigurationChanged) {
			status = http.StatusConflict
			w.Header().Set("HX-Refresh", "true")
		}
		http.Error(w, err.Error(), status)
		return
	}
	w.Header().Set("HX-Trigger", `{"picoclip-toast":{"message":"Provider configuration saved.","type":"success"}}`)
	s.handleWebSettings(w, r)
}

func runtimeQuickSetupInput(r *http.Request) domain.RuntimeQuickSetupInput {
	return domain.RuntimeQuickSetupInput{
		ProfileID: strings.TrimSpace(r.FormValue("profile_id")),
		Values: map[string]string{
			"base_url": strings.TrimSpace(r.FormValue("base_url")),
			"model":    strings.TrimSpace(r.FormValue("model")),
		},
		APIKey:      r.FormValue("api_key"),
		ClearAPIKey: r.FormValue("clear_api_key") == "true",
		Revision:    r.FormValue("revision"),
	}
}

func (s *Server) handleWebPostRuntimeTestModel(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	result, err := s.runtimes.TestQuickSetup(r.Context(), domain.RuntimeID(r.PathValue("id")), runtimeQuickSetupInput(r))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	latency := result.Latency.Round(time.Millisecond).String()
	if result.Status == "ok" {
		_, _ = fmt.Fprintf(w, `<div class="runtime-ai-result runtime-ai-success"><strong>%s</strong><p>%s</p><small>%s</small></div>`, html.EscapeString(result.Message), html.EscapeString(result.Output), html.EscapeString(latency))
		return
	}
	_, _ = fmt.Fprintf(w, `<div class="runtime-ai-result runtime-ai-error"><strong>%s</strong><small>%s</small></div>`, html.EscapeString(result.Message), html.EscapeString(latency))
}

func (s *Server) handleWebPostRuntimeTest(w http.ResponseWriter, r *http.Request) {
	runtimeID := domain.RuntimeID(r.PathValue("id"))
	health, err := s.runtimes.Test(r.Context(), runtimeID)
	if err != nil || health.Status != "ok" {
		message := "CLI check failed."
		if len(health.Errors) > 0 && strings.TrimSpace(health.Errors[0]) != "" {
			message += " " + health.Errors[0]
		}
		trigger, _ := json.Marshal(map[string]any{"picoclip-toast": map[string]string{"message": message, "type": "error"}})
		w.Header().Set("HX-Trigger", string(trigger))
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
