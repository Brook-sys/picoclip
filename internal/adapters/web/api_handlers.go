package web

import (
	"encoding/json"
	"net/http"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/services"
)

func (s *Server) handleDeleteAgent(w http.ResponseWriter, r *http.Request) {
	if err := s.agents.Delete(r.Context(), r.PathValue("id")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.jsonResponse(w, map[string]string{"status": "deleted"})
}

func (s *Server) handleUpdateAgentPermissions(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Permissions []domain.AgentPermission `json:"permissions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	agent, err := s.agents.UpdatePermissions(r.Context(), r.PathValue("id"), req.Permissions)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.jsonResponse(w, agent)
}

func (s *Server) handleUpdateAgentSkills(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SkillIDs []string `json:"skill_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	agent, err := s.agents.UpdateSkills(r.Context(), r.PathValue("id"), req.SkillIDs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.jsonResponse(w, agent)
}

func (s *Server) handleCancelTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AgentID string `json:"agent_id"`
		Reason  string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if isAgentAPIRequest(r) || req.AgentID != "" {
		if err := s.auth.RequireAgentPermission(r.Context(), req.AgentID, domain.PermissionTasksCancel); err != nil {
			writeTaskError(w, err)
			return
		}
	}
	task, err := s.tasks.Cancel(r.Context(), r.PathValue("id"), req.Reason)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.jsonResponse(w, s.taskResponse(r, task))
}

func (s *Server) handleGetSkills(w http.ResponseWriter, r *http.Request) {
	skills, err := s.skills.List(r.Context(), r.URL.Query().Get("project_id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.jsonResponse(w, skills)
}

func (s *Server) handleCreateSkill(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AgentID      string `json:"agent_id"`
		ProjectID    string `json:"project_id"`
		Name         string `json:"name"`
		Description  string `json:"description"`
		Instructions string `json:"instructions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.AgentID != "" {
		if err := s.auth.RequireAgentPermission(r.Context(), req.AgentID, domain.PermissionSkillsCreate); err != nil {
			writeTaskError(w, err)
			return
		}
	}
	skill, err := s.skills.Create(r.Context(), req.ProjectID, req.Name, req.Description, req.Instructions)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.jsonResponse(w, skill)
}

func (s *Server) handleUpdateSkill(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AgentID      string `json:"agent_id"`
		Name         string `json:"name"`
		Description  string `json:"description"`
		Instructions string `json:"instructions"`
		Enabled      bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.AgentID != "" {
		if err := s.auth.RequireAgentPermission(r.Context(), req.AgentID, domain.PermissionSkillsUpdate); err != nil {
			writeTaskError(w, err)
			return
		}
	}
	skill, err := s.skills.Update(r.Context(), r.PathValue("id"), req.Name, req.Description, req.Instructions, req.Enabled)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.jsonResponse(w, skill)
}

func (s *Server) handleDeleteSkill(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("agent_id")
	if agentID != "" {
		if err := s.auth.RequireAgentPermission(r.Context(), agentID, domain.PermissionSkillsDelete); err != nil {
			writeTaskError(w, err)
			return
		}
	}
	if err := s.skills.Delete(r.Context(), r.PathValue("id")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.jsonResponse(w, map[string]string{"status": "deleted"})
}

func (s *Server) handleGetProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.projects.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.jsonResponse(w, projects)
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	project, err := s.projects.Create(r.Context(), req.Name, req.Description)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.jsonResponse(w, project)
}

func (s *Server) handleGetCapabilities(w http.ResponseWriter, r *http.Request) {
	s.jsonResponse(w, map[string]any{"permissions": services.PermissionCatalog(), "presets": services.PermissionPresets(), "legacy_presets": services.CapabilityPresets()})
}

func (s *Server) handleAgentDocs(w http.ResponseWriter, r *http.Request) {
	s.jsonResponse(w, map[string]any{
		"purpose":     "APIs locais para agentes entenderem o sistema, consultarem contexto e operarem quando tiverem permissão.",
		"permissions": []string{"agents.create", "agents.delete", "tasks.create", "tasks.delegate", "tasks.cancel", "skills.manage", "system.view"},
		"endpoints": []map[string]string{
			{"method": "GET", "path": "/agent-api/docs", "description": "Documentação das APIs disponíveis para agentes."},
			{"method": "GET", "path": "/agent-api/me?agent_id=...", "description": "Identidade do agente, permissões e inbox."},
			{"method": "GET", "path": "/agent-api/agents", "description": "Lista agentes, tipos, descrições, permissões e skills atribuídas."},
			{"method": "GET", "path": "/agent-api/tasks?status=todo,in_progress", "description": "Lista tarefas, com filtros status, agent_id, parent_id e project_id."},
			{"method": "GET", "path": "/agent-api/tasks/{id}", "description": "Detalhe da task com comentários, runs, eventos e subtasks."},
			{"method": "POST", "path": "/agent-api/tasks/{id}/checkout", "description": "Checkout atômico antes de trabalhar."},
			{"method": "POST", "path": "/agent-api/tasks/{id}/comments", "description": "Adiciona comentário durável à conversa."},
			{"method": "PATCH", "path": "/agent-api/tasks/{id}", "description": "Atualiza status. Use done/blocked com comentário obrigatório."},
			{"method": "POST", "path": "/agent-api/tasks/{id}/release", "description": "Libera checkout com comentário opcional."},
			{"method": "POST", "path": "/agent-api/tasks/{id}/wake", "description": "Acorda/reabre task para novo heartbeat."},
			{"method": "POST", "path": "/agent-api/tasks/{id}/delegate", "description": "Cria subtarefa delegada para outro agente."},
		},
	})
}
