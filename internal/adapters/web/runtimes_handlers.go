package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"picoclip/internal/core/domain"
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
}

func (s *Server) runtimeCards(r *http.Request) []RuntimeCardView {
	states, _ := s.runtimes.States(r.Context())
	cards := make([]RuntimeCardView, 0, len(s.runtimes.Catalog()))
	for _, manifest := range s.runtimes.Catalog() {
		state, configured := states[manifest.ID]
		health := domain.RuntimeHealth{Status: "not_configured"}
		var configFiles []domain.RuntimeConfigFile
		var versions []domain.RuntimeVersion
		if configured {
			health, _ = s.runtimes.Health(r.Context(), manifest.ID)
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
		})
	}
	return cards
}

func (s *Server) handleAPIRuntimes(w http.ResponseWriter, r *http.Request) {
	s.jsonResponse(w, s.runtimeCards(r))
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
	s.handleWebSettings(w, r)
}
