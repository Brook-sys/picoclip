package web

import (
	"net/http"
	"strings"

	"picoclip/internal/core/ports"
)

type SearchResult struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title"`
	URL   string `json:"url"`
	Extra string `json:"extra,omitempty"`
}

func (s *Server) handleAPISearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := strings.ToLower(r.URL.Query().Get("q"))
	var results []SearchResult

	if query == "" {
		s.jsonResponse(w, results)
		return
	}

	// Agents
	if agents, err := s.agents.List(ctx); err == nil {
		for _, a := range agents {
			if strings.Contains(strings.ToLower(a.Name), query) || strings.Contains(strings.ToLower(a.Description), query) {
				results = append(results, SearchResult{
					ID:    a.ID,
					Type:  "Agent",
					Title: a.Name,
					URL:   "/agents/" + a.ID,
					Extra: a.Title,
				})
			}
		}
	}

	// Projects
	if projects, err := s.projects.List(ctx); err == nil {
		for _, p := range projects {
			if strings.Contains(strings.ToLower(p.Name), query) {
				results = append(results, SearchResult{
					ID:    p.ID,
					Type:  "Project",
					Title: p.Name,
					URL:   "/projects/" + p.ID,
				})
			}
		}
	}

	// Skills
	if skills, err := s.skills.List(ctx, ""); err == nil {
		for _, sk := range skills {
			if strings.Contains(strings.ToLower(sk.Name), query) || strings.Contains(strings.ToLower(sk.Slug), query) {
				results = append(results, SearchResult{
					ID:    sk.ID,
					Type:  "Skill",
					Title: sk.Name,
					URL:   "/skills/" + sk.ID,
				})
			}
		}
	}

	// Tasks
	if tasks, err := s.tasks.List(ctx, ports.TaskFilter{}); err == nil {
		for _, t := range tasks {
			if strings.Contains(strings.ToLower(t.Prompt), query) || strings.Contains(strings.ToLower(t.ID), query) || strings.Contains(strings.ToLower(t.Title), query) {
				results = append(results, SearchResult{
					ID:    t.ID,
					Type:  "Task",
					Title: taskExcerpt(t.Prompt),
					URL:   "/tasks/" + t.ID,
					Extra: string(t.Status),
				})
			}
		}
	}

	// Basic commands match
	commands := []SearchResult{
		{ID: "cmd_settings", Type: "Command", Title: "Open Settings", URL: "/settings"},
		{ID: "cmd_dashboard", Type: "Command", Title: "Open Dashboard", URL: "/"},
		{ID: "cmd_runs", Type: "Command", Title: "Open Runs", URL: "/runs"},
		{ID: "cmd_activity", Type: "Command", Title: "Open Activity", URL: "/activity"},
	}

	for _, cmd := range commands {
		if strings.Contains(strings.ToLower(cmd.Title), query) {
			results = append(results, cmd)
		}
	}

	if len(results) > 15 {
		results = results[:15]
	}

	s.jsonResponse(w, results)
}
