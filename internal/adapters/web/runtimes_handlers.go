package web

import (
	"encoding/json"
	"net/http"
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
	Versions    []domain.RuntimeVersion
	Tested      bool
	TestedAt    string
	Functional  bool
	Checks      []domain.DiagnosticCheck
	AITested    bool
	AITestedAt  string
	AIOk        bool
	AIMessage   string
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
		aiTested, aiTestedAt, aiOK, aiMessage := runtimeAITestSummary(state)
		if configured {
			if tested {
				health = savedHealth
			}
			if adapter, ok := s.runtimes.Adapter(manifest.ID); ok {
				configFiles, _ = adapter.ReadConfig(r.Context(), state)
			}
		} else {
			if adapter, ok := s.runtimes.Adapter(manifest.ID); ok {
				versions, _ = adapter.ListVersions(r.Context(), 10)
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
			Versions:    versions,
			Tested:      tested,
			TestedAt:    testedAt,
			Functional:  functional,
			Checks:      checks,
			AITested:    aiTested,
			AITestedAt:  aiTestedAt,
			AIOk:        aiOK,
			AIMessage:   aiMessage,
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

func runtimeAITestSummary(state domain.RuntimeState) (tested bool, testedAt string, ok bool, message string) {
	if state.MetadataJSON == "" || state.MetadataJSON == "{}" {
		return false, "", false, ""
	}
	var metadata struct {
		LastAITest *services.RuntimeAITestResult `json:"last_ai_test"`
	}
	if err := json.Unmarshal([]byte(state.MetadataJSON), &metadata); err != nil || metadata.LastAITest == nil {
		return false, "", false, ""
	}
	res := metadata.LastAITest
	ok = res.Status == "ok"
	message = res.Message
	if res.Output != "" {
		message += " (Output: " + res.Output + ")"
	}
	testedAt = timeSince(res.CheckedAt)
	return true, testedAt, ok, message
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
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.handleWebSettings(w, r)
}

func (s *Server) handleWebPostRuntimeTestAI(w http.ResponseWriter, r *http.Request) {
	runtimeID := domain.RuntimeID(r.PathValue("id"))
	result, err := s.runtimes.TestAI(r.Context(), runtimeID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if result.Status != "ok" {
		http.Error(w, result.Message, http.StatusBadRequest)
		return
	}
	s.handleWebSettings(w, r)
}
