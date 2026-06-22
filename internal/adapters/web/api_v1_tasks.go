package web

import (
	"encoding/json"
	"net/http"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
	"picoclip/internal/core/services"
)

func (s *Server) handleAPIV1Tasks(w http.ResponseWriter, r *http.Request) {
	filter := ports.TaskFilter{AgentID: r.URL.Query().Get("agent_id"), ParentID: r.URL.Query().Get("parent_id"), WorkspaceID: r.URL.Query().Get("project_id"), Status: domain.TaskStatus(r.URL.Query().Get("status"))}
	s.apiTasksWithFilter(w, r, filter)
}

func (s *Server) handleAPIV1CreateTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProjectID string `json:"project_id"`
		AgentID   string `json:"agent_id"`
		Title     string `json:"title"`
		Prompt    string `json:"prompt"`
		Message   string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.apiError(w, domain.ErrInvalidInput)
		return
	}
	if req.Prompt == "" {
		req.Prompt = req.Message
	}
	task, err := s.tasks.CreateInWorkspace(r.Context(), req.ProjectID, req.AgentID, req.Title, req.Prompt)
	if err != nil {
		s.apiError(w, err)
		return
	}
	s.apiData(w, s.taskResponse(r, task))
}

func (s *Server) handleAPIV1Task(w http.ResponseWriter, r *http.Request) {
	task, err := s.tasks.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		s.apiError(w, err)
		return
	}
	s.apiData(w, s.taskResponse(r, task))
}

func (s *Server) handleAPIV1TaskFull(w http.ResponseWriter, r *http.Request) {
	task, err := s.tasks.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		s.apiError(w, err)
		return
	}
	messages, _ := s.tasks.GetMessages(r.Context(), task.ID)
	runs, _ := s.tasks.GetRuns(r.Context(), task.ID)
	events, _ := s.storage.Events().ListByTask(r.Context(), task.ID)
	children, _ := s.tasks.List(r.Context(), ports.TaskFilter{ParentID: task.ID})
	s.apiData(w, taskFullResponse{Task: s.taskResponse(r, task), Messages: messages, Runs: runs, Events: events, Children: s.taskResponses(r, children)})
}

func (s *Server) handleAPIV1CancelTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	task, err := s.tasks.Cancel(r.Context(), r.PathValue("id"), req.Reason)
	if err != nil {
		s.apiError(w, err)
		return
	}
	s.apiData(w, s.taskResponse(r, task))
}

func (s *Server) handleAPIV1DelegateTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FromAgentID string `json:"from_agent_id"`
		ToAgentID   string `json:"to_agent_id"`
		Title       string `json:"title"`
		Prompt      string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.apiError(w, domain.ErrInvalidInput)
		return
	}
	task, err := s.tasks.Delegate(r.Context(), r.PathValue("id"), req.FromAgentID, req.ToAgentID, req.Prompt)
	if req.Title != "" {
		task.Title = req.Title
		_ = s.storage.Tasks().Update(r.Context(), task)
	}
	if err != nil {
		s.apiError(w, err)
		return
	}
	s.apiData(w, s.taskResponse(r, task))
}

func (s *Server) handleAPIV1TaskMessages(w http.ResponseWriter, r *http.Request) {
	messages, err := s.tasks.GetMessages(r.Context(), r.PathValue("id"))
	if err != nil {
		s.apiError(w, err)
		return
	}
	s.apiList(w, messages, map[string]any{"count": len(messages)})
}

func (s *Server) handleAPIV1CreateTaskMessage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FromID string             `json:"from_id"`
		ToID   string             `json:"to_id"`
		Role   domain.MessageRole `json:"role"`
		Body   string             `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.apiError(w, domain.ErrInvalidInput)
		return
	}
	if req.Role == "" {
		req.Role = domain.MessageRoleUser
	}
	message, err := s.tasks.AddMessage(r.Context(), r.PathValue("id"), req.FromID, req.ToID, req.Role, req.Body)
	if err != nil {
		s.apiError(w, err)
		return
	}
	s.apiData(w, message)
}

func (s *Server) handleAPIV1TaskRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := s.tasks.GetRuns(r.Context(), r.PathValue("id"))
	if err != nil {
		s.apiError(w, err)
		return
	}
	s.apiList(w, runs, map[string]any{"count": len(runs)})
}

func (s *Server) handleAPIV1TaskEvents(w http.ResponseWriter, r *http.Request) {
	events, err := s.storage.Events().ListByTask(r.Context(), r.PathValue("id"))
	if err != nil {
		s.apiError(w, err)
		return
	}
	s.apiList(w, events, map[string]any{"count": len(events)})
}

func (s *Server) handleAPIV1TaskChildren(w http.ResponseWriter, r *http.Request) {
	s.apiTasksWithFilter(w, r, ports.TaskFilter{ParentID: r.PathValue("id")})
}

func (s *Server) handleAPIV1Runs(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.tasks.List(r.Context(), ports.TaskFilter{AgentID: r.URL.Query().Get("agent_id"), WorkspaceID: r.URL.Query().Get("project_id"), Status: domain.TaskStatus(r.URL.Query().Get("task_status"))})
	if err != nil {
		s.apiError(w, err)
		return
	}
	runs := make([]domain.Run, 0)
	for _, task := range tasks {
		if r.URL.Query().Get("task_id") != "" && task.ID != r.URL.Query().Get("task_id") {
			continue
		}
		taskRuns, _ := s.tasks.GetRuns(r.Context(), task.ID)
		for _, run := range taskRuns {
			if r.URL.Query().Get("status") != "" && string(run.Status) != r.URL.Query().Get("status") {
				continue
			}
			runs = append(runs, run)
		}
	}
	s.apiList(w, runs, map[string]any{"count": len(runs)})
}

func (s *Server) handleAPIV1Run(w http.ResponseWriter, r *http.Request) {
	run, err := s.storage.Runs().Get(r.Context(), r.PathValue("id"))
	if err != nil {
		s.apiError(w, err)
		return
	}
	s.apiData(w, run)
}

func (s *Server) handleAPIV1Skills(w http.ResponseWriter, r *http.Request) {
	skills, err := s.skills.List(r.Context(), r.URL.Query().Get("project_id"))
	if err != nil {
		s.apiError(w, err)
		return
	}
	s.apiList(w, skills, map[string]any{"count": len(skills)})
}

func (s *Server) handleAPIV1CreateSkill(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProjectID    string             `json:"project_id"`
		Name         string             `json:"name"`
		Description  string             `json:"description"`
		Instructions string             `json:"instructions"`
		Files        []domain.SkillFile `json:"files"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.apiError(w, domain.ErrInvalidInput)
		return
	}
	skill, err := s.skills.CreateWithFiles(r.Context(), req.ProjectID, req.Name, req.Description, req.Instructions, req.Files)
	if err != nil {
		s.apiError(w, err)
		return
	}
	s.apiData(w, skill)
}

func (s *Server) handleAPIV1Skill(w http.ResponseWriter, r *http.Request) {
	skill, err := s.skills.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		s.apiError(w, err)
		return
	}
	s.apiData(w, skill)
}

func (s *Server) handleAPIV1UpdateSkill(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string `json:"name"`
		Description  string `json:"description"`
		Instructions string `json:"instructions"`
		Enabled      bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.apiError(w, domain.ErrInvalidInput)
		return
	}
	skill, err := s.skills.Update(r.Context(), r.PathValue("id"), req.Name, req.Description, req.Instructions, req.Enabled)
	if err != nil {
		s.apiError(w, err)
		return
	}
	s.apiData(w, skill)
}

func (s *Server) handleAPIV1DeleteSkill(w http.ResponseWriter, r *http.Request) {
	if err := s.skills.Delete(r.Context(), r.PathValue("id")); err != nil {
		s.apiError(w, err)
		return
	}
	s.apiData(w, map[string]string{"status": "deleted"})
}

func (s *Server) handleAPIV1ResetSkill(w http.ResponseWriter, r *http.Request) {
	skill, err := s.skills.ResetBuiltin(r.Context(), r.PathValue("id"))
	if err != nil {
		s.apiError(w, err)
		return
	}
	s.apiData(w, skill)
}

func (s *Server) handleAPIV1SetSkillAgents(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AgentIDs           []string                 `json:"agent_ids"`
		AllowedAgentTypes  []domain.AgentType       `json:"allowed_agent_types"`
		AllowedPermissions []domain.AgentPermission `json:"allowed_permissions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.apiError(w, domain.ErrInvalidInput)
		return
	}
	skill, err := s.skills.UpdateAssignments(r.Context(), r.PathValue("id"), req.AgentIDs, req.AllowedAgentTypes, req.AllowedPermissions)
	if err != nil {
		s.apiError(w, err)
		return
	}
	s.apiData(w, skill)
}

func (s *Server) handleAPIV1Events(w http.ResponseWriter, r *http.Request) {
	limit := 100
	events, err := s.storage.Events().ListRecent(r.Context(), limit)
	if err != nil {
		s.apiError(w, err)
		return
	}
	out := make([]domain.Event, 0, len(events))
	for _, event := range events {
		if r.URL.Query().Get("task_id") != "" && event.TaskID != r.URL.Query().Get("task_id") {
			continue
		}
		if r.URL.Query().Get("agent_id") != "" && event.AgentID != r.URL.Query().Get("agent_id") {
			continue
		}
		if r.URL.Query().Get("type") != "" && string(event.Type) != r.URL.Query().Get("type") {
			continue
		}
		out = append(out, event)
	}
	s.apiList(w, out, map[string]any{"count": len(out)})
}

func (s *Server) apiTasksWithFilter(w http.ResponseWriter, r *http.Request, filter ports.TaskFilter) {
	tasks, err := s.tasks.List(r.Context(), filter)
	if err != nil {
		s.apiError(w, err)
		return
	}
	s.apiList(w, s.taskResponses(r, tasks), map[string]any{"count": len(tasks)})
}

func (s *Server) loadAPIState(w http.ResponseWriter, r *http.Request) ([]domain.Agent, []taskResponse, []domain.Workspace, []domain.Skill, bool) {
	return s.loadWebState(w, r)
}

func servicesCapabilities() any {
	return map[string]any{"permissions": services.PermissionCatalog(), "presets": services.PermissionPresets(), "legacy_presets": services.CapabilityPresets()}
}

func apiV1Paths() map[string]any {
	paths := []string{
		"GET /api/v1/health",
		"GET /api/v1/version",
		"GET /api/v1/capabilities",
		"GET /api/v1/dashboard",
		"GET,POST /api/v1/projects",
		"GET /api/v1/projects/{id}",
		"GET,POST /api/v1/agents",
		"GET,PATCH,DELETE /api/v1/agents/{id}",
		"GET,POST /api/v1/tasks",
		"GET /api/v1/tasks/{id}",
		"GET /api/v1/tasks/{id}/full",
		"POST /api/v1/tasks/{id}/cancel",
		"POST /api/v1/tasks/{id}/delegate",
		"GET,POST /api/v1/tasks/{id}/messages",
		"GET /api/v1/tasks/{id}/runs",
		"GET /api/v1/tasks/{id}/events",
		"GET /api/v1/tasks/{id}/children",
		"GET /api/v1/runs",
		"GET /api/v1/runs/{id}",
		"GET,POST /api/v1/skills",
		"GET,PATCH,DELETE /api/v1/skills/{id}",
		"GET /api/v1/events",
	}
	out := make(map[string]any, len(paths))
	for _, path := range paths {
		out[path] = map[string]any{"description": "PicoClip API v1 endpoint"}
	}
	return out
}
