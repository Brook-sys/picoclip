package web

import (
	"encoding/json"
	"net/http"
	"runtime"
	"strings"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
	"picoclip/internal/core/services"
)

type taskFullResponse struct {
	Task     taskResponse           `json:"task"`
	Messages []domain.Message       `json:"messages"`
	Runs     []domain.Run           `json:"runs"`
	Events   []domain.Event         `json:"events"`
	Wakeups  []domain.WakeupRequest `json:"wakeups"`
	Children []taskResponse         `json:"children"`
}

type dashboardResponse struct {
	Projects []domain.Workspace `json:"projects"`
	Agents   []domain.Agent     `json:"agents"`
	Tasks    []taskResponse     `json:"tasks"`
	Skills   []domain.Skill     `json:"skills"`
	Events   []domain.Event     `json:"events"`
	Stats    map[string]int     `json:"stats"`
}

func (s *Server) mountAPIV1(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/health", s.handleAPIV1Health)
	mux.HandleFunc("GET /api/v1/version", s.handleAPIV1Version)
	mux.HandleFunc("GET /api/v1/openapi.json", s.handleAPIV1OpenAPI)
	mux.HandleFunc("GET /api/v1/capabilities", s.handleAPIV1Capabilities)
	mux.HandleFunc("GET /api/v1/permission-presets", s.handleAPIV1PermissionPresets)
	mux.HandleFunc("GET /api/v1/diagnostics/recovery-liveness", s.handleAPIV1DiagnosticsRecoveryLiveness)
	mux.HandleFunc("GET /api/v1/dashboard", s.handleAPIV1Dashboard)
	mux.HandleFunc("GET /api/v1/projects", s.handleAPIV1Projects)
	mux.HandleFunc("POST /api/v1/projects", s.handleAPIV1CreateProject)
	mux.HandleFunc("GET /api/v1/projects/{id}", s.handleAPIV1Project)
	mux.HandleFunc("GET /api/v1/projects/{id}/agents", s.handleAPIV1ProjectAgents)
	mux.HandleFunc("GET /api/v1/projects/{id}/tasks", s.handleAPIV1ProjectTasks)
	mux.HandleFunc("GET /api/v1/projects/{id}/skills", s.handleAPIV1ProjectSkills)
	mux.HandleFunc("GET /api/v1/tags", s.handleAPIV1Tags)
	mux.HandleFunc("GET /api/v1/tags/{tag}/agents", s.handleAPIV1TagAgents)
	mux.HandleFunc("GET /api/v1/agents", s.handleAPIV1Agents)
	mux.HandleFunc("POST /api/v1/agents", s.handleAPIV1CreateAgent)
	mux.HandleFunc("GET /api/v1/agents/{id}", s.handleAPIV1Agent)
	mux.HandleFunc("PATCH /api/v1/agents/{id}", s.handleAPIV1UpdateAgent)
	mux.HandleFunc("DELETE /api/v1/agents/{id}", s.handleAPIV1DeleteAgent)
	mux.HandleFunc("GET /api/v1/agents/{id}/tasks", s.handleAPIV1AgentTasks)
	mux.HandleFunc("GET /api/v1/agents/{id}/runs", s.handleAPIV1AgentRuns)
	mux.HandleFunc("PUT /api/v1/agents/{id}/skills", s.handleAPIV1SetAgentSkills)
	mux.HandleFunc("GET /api/v1/tasks", s.handleAPIV1Tasks)
	mux.HandleFunc("POST /api/v1/tasks", s.handleAPIV1CreateTask)
	mux.HandleFunc("GET /api/v1/tasks/{id}", s.handleAPIV1Task)
	mux.HandleFunc("GET /api/v1/tasks/{id}/full", s.handleAPIV1TaskFull)
	mux.HandleFunc("POST /api/v1/tasks/{id}/cancel", s.handleAPIV1CancelTask)
	mux.HandleFunc("POST /api/v1/tasks/{id}/pause", s.handleAPIV1PauseContinuousTask)
	mux.HandleFunc("POST /api/v1/tasks/{id}/resume", s.handleAPIV1ResumeContinuousTask)
	mux.HandleFunc("POST /api/v1/tasks/{id}/run-now", s.handleAPIV1RunContinuousTaskNow)
	mux.HandleFunc("POST /api/v1/tasks/{id}/delegate", s.handleAPIV1DelegateTask)
	mux.HandleFunc("GET /api/v1/tasks/{id}/messages", s.handleAPIV1TaskMessages)
	mux.HandleFunc("POST /api/v1/tasks/{id}/messages", s.handleAPIV1CreateTaskMessage)
	mux.HandleFunc("GET /api/v1/tasks/{id}/runs", s.handleAPIV1TaskRuns)
	mux.HandleFunc("GET /api/v1/tasks/{id}/events", s.handleAPIV1TaskEvents)
	mux.HandleFunc("GET /api/v1/tasks/{id}/wakeups", s.handleAPIV1TaskWakeups)
	mux.HandleFunc("GET /api/v1/tasks/{id}/children", s.handleAPIV1TaskChildren)
	mux.HandleFunc("GET /api/v1/runs", s.handleAPIV1Runs)
	mux.HandleFunc("GET /api/v1/runs/{id}", s.handleAPIV1Run)
	mux.HandleFunc("GET /api/v1/skills", s.handleAPIV1Skills)
	mux.HandleFunc("POST /api/v1/skills", s.handleAPIV1CreateSkill)
	mux.HandleFunc("POST /api/v1/skills/import", s.handleAPIV1ImportSkill)
	mux.HandleFunc("GET /api/v1/skills/{id}", s.handleAPIV1Skill)
	mux.HandleFunc("PATCH /api/v1/skills/{id}", s.handleAPIV1UpdateSkill)
	mux.HandleFunc("DELETE /api/v1/skills/{id}", s.handleAPIV1DeleteSkill)
	mux.HandleFunc("POST /api/v1/skills/{id}/reset", s.handleAPIV1ResetSkill)
	mux.HandleFunc("PUT /api/v1/skills/{id}/agents", s.handleAPIV1SetSkillAgents)
	mux.HandleFunc("GET /api/v1/webhooks", s.handleAPIV1Webhooks)
	mux.HandleFunc("POST /api/v1/webhooks", s.handleAPIV1CreateWebhook)
	mux.HandleFunc("GET /api/v1/webhooks/{id}/deliveries", s.handleAPIV1WebhookDeliveries)
	mux.HandleFunc("GET /api/v1/usage", s.handleAPIV1Usage)
	mux.HandleFunc("GET /api/v1/events", s.handleAPIV1Events)
	mux.HandleFunc("GET /api/v1/activity", s.handleAPIV1Events)
}

func (s *Server) handleAPIV1Health(w http.ResponseWriter, r *http.Request) {
	s.apiData(w, map[string]any{"status": "ok"})
}

func (s *Server) handleAPIV1Version(w http.ResponseWriter, r *http.Request) {
	s.apiData(w, map[string]any{"name": "picoclip", "api_version": "v1", "go": runtime.Version()})
}

func (s *Server) handleAPIV1OpenAPI(w http.ResponseWriter, r *http.Request) {
	s.apiData(w, map[string]any{"openapi": "3.1.0", "info": map[string]string{"title": "PicoClip API", "version": "v1"}, "paths": apiV1Paths()})
}

func (s *Server) handleAPIV1Capabilities(w http.ResponseWriter, r *http.Request) {
	s.apiData(w, servicesCapabilities())
}

func (s *Server) handleAPIV1PermissionPresets(w http.ResponseWriter, r *http.Request) {
	s.apiData(w, services.PermissionPresets())
}

func (s *Server) handleAPIV1DiagnosticsRecoveryLiveness(w http.ResponseWriter, r *http.Request) {
	limit, err := parseAPILimit(r, 20, 100)
	if err != nil {
		s.apiError(w, err)
		return
	}
	report, err := s.diagnostics.RecoveryLiveness(r.Context(), limit)
	if err != nil {
		s.apiError(w, err)
		return
	}
	s.apiList(w, report, map[string]any{"limit": limit, "count": len(report.Items)})
}

func (s *Server) handleAPIV1Dashboard(w http.ResponseWriter, r *http.Request) {
	agents, tasks, projects, skills, ok := s.loadAPIState(w, r)
	if !ok {
		return
	}
	events, _ := s.storage.Events().ListRecent(r.Context(), 20)
	pending, running, succeeded, failed := countTasks(tasks)
	s.apiData(w, dashboardResponse{Projects: projects, Agents: agents, Tasks: tasks, Skills: skills, Events: events, Stats: map[string]int{"projects": len(projects), "agents": len(agents), "tasks": len(tasks), "skills": len(skills), "pending": pending, "running": running, "succeeded": succeeded, "attention": failed}})
}

func (s *Server) handleAPIV1Projects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.projects.List(r.Context())
	if err != nil {
		s.apiError(w, err)
		return
	}
	s.apiList(w, projects, map[string]any{"count": len(projects)})
}

func (s *Server) handleAPIV1CreateProject(w http.ResponseWriter, r *http.Request) {
	var req struct{ Name, Description string }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.apiError(w, domain.ErrInvalidInput)
		return
	}
	project, err := s.projects.Create(r.Context(), req.Name, req.Description)
	if err != nil {
		s.apiError(w, err)
		return
	}
	s.apiData(w, project)
}

func (s *Server) handleAPIV1Project(w http.ResponseWriter, r *http.Request) {
	project, err := s.storage.Workspaces().Get(r.Context(), r.PathValue("id"))
	if err != nil {
		s.apiError(w, err)
		return
	}
	s.apiData(w, project)
}

func (s *Server) handleAPIV1ProjectAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := s.agents.List(r.Context())
	if err != nil {
		s.apiError(w, err)
		return
	}
	projectID := r.PathValue("id")
	out := make([]domain.Agent, 0)
	for _, agent := range agents {
		if agent.ProjectID == projectID {
			out = append(out, agent)
		}
	}
	s.apiList(w, out, map[string]any{"count": len(out)})
}

func (s *Server) handleAPIV1ProjectTasks(w http.ResponseWriter, r *http.Request) {
	s.apiTasksWithFilter(w, r, ports.TaskFilter{WorkspaceID: r.PathValue("id")})
}

func (s *Server) handleAPIV1ProjectSkills(w http.ResponseWriter, r *http.Request) {
	skills, err := s.skills.List(r.Context(), r.PathValue("id"))
	if err != nil {
		s.apiError(w, err)
		return
	}
	s.apiList(w, skills, map[string]any{"count": len(skills)})
}

func (s *Server) handleAPIV1Tags(w http.ResponseWriter, r *http.Request) {
	agents, err := s.agents.List(r.Context())
	if err != nil {
		s.apiError(w, err)
		return
	}
	tags := map[string]string{}
	for _, agent := range agents {
		for _, tag := range agent.Tags {
			tags[strings.ToLower(tag)] = tag
		}
	}
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		out = append(out, tag)
	}
	s.apiList(w, out, map[string]any{"count": len(out)})
}

func (s *Server) handleAPIV1TagAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := s.agents.ListByTag(r.Context(), r.PathValue("tag"))
	if err != nil {
		s.apiError(w, err)
		return
	}
	s.apiList(w, agents, map[string]any{"count": len(agents)})
}

func (s *Server) handleAPIV1Agents(w http.ResponseWriter, r *http.Request) {
	var agents []domain.Agent
	var err error
	if r.URL.Query().Get("tag") != "" {
		agents, err = s.agents.ListByTag(r.Context(), r.URL.Query().Get("tag"))
	} else {
		agents, err = s.agents.List(r.Context())
	}
	if err != nil {
		s.apiError(w, err)
		return
	}
	s.apiList(w, agents, map[string]any{"count": len(agents)})
}

func (s *Server) handleAPIV1CreateAgent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProjectID        string                   `json:"project_id"`
		Name             string                   `json:"name"`
		Title            string                   `json:"title"`
		ReportsToID      string                   `json:"reports_to_id"`
		Tags             []string                 `json:"tags"`
		Type             string                   `json:"type"`
		Description      string                   `json:"description"`
		SystemPrompt     string                   `json:"system_prompt"`
		InstructionFile  string                   `json:"instruction_file"`
		SkillIDs         []string                 `json:"skill_ids"`
		Config           map[string]string        `json:"config"`
		Env              map[string]string        `json:"env"`
		ExtraArgs        []string                 `json:"extra_args"`
		Capability       domain.AgentCapability   `json:"capability"`
		PermissionPreset string                   `json:"permission_preset"`
		Permissions      []domain.AgentPermission `json:"permissions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.apiError(w, domain.ErrInvalidInput)
		return
	}
	permissions := req.Permissions
	if len(permissions) == 0 {
		permissions = services.PermissionsForPreset(req.PermissionPreset)
	}
	agentType := domain.AgentType(req.Type)
	if err := s.validateAgentRuntime(r.Context(), agentType); err != nil {
		s.apiError(w, err)
		return
	}
	agent, err := s.agents.CreateFull(r.Context(), services.CreateAgentInput{
		ProjectID:       req.ProjectID,
		Name:            req.Name,
		Title:           req.Title,
		ReportsToID:     req.ReportsToID,
		Tags:            req.Tags,
		Type:            agentType,
		Description:     req.Description,
		SystemPrompt:    req.SystemPrompt,
		InstructionFile: req.InstructionFile,
		SkillIDs:        req.SkillIDs,
		Config:          req.Config,
		Env:             req.Env,
		ExtraArgs:       req.ExtraArgs,
		Capability:      req.Capability,
		Permissions:     permissions,
	})
	if err != nil {
		s.apiError(w, err)
		return
	}
	s.apiData(w, agent)
}

func (s *Server) handleAPIV1Agent(w http.ResponseWriter, r *http.Request) {
	agent, err := s.agents.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		s.apiError(w, err)
		return
	}
	s.apiData(w, agent)
}

func (s *Server) handleAPIV1UpdateAgent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name            *string                  `json:"name"`
		Title           *string                  `json:"title"`
		ReportsToID     *string                  `json:"reports_to_id"`
		Tags            *[]string                `json:"tags"`
		Description     *string                  `json:"description"`
		SystemPrompt    *string                  `json:"system_prompt"`
		InstructionFile *string                  `json:"instruction_file"`
		ExtraArgs       *[]string                `json:"extra_args"`
		Config          map[string]string        `json:"config"`
		Env             map[string]string        `json:"env"`
		Capability      domain.AgentCapability   `json:"capability"`
		Permissions     []domain.AgentPermission `json:"permissions"`
		SkillIDs        []string                 `json:"skill_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.apiError(w, domain.ErrInvalidInput)
		return
	}
	agent, err := s.agents.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		s.apiError(w, err)
		return
	}
	if req.Name != nil || req.Title != nil || req.ReportsToID != nil || req.Tags != nil || req.Description != nil || req.SystemPrompt != nil || req.InstructionFile != nil || req.ExtraArgs != nil {
		agent, err = s.agents.UpdateIdentity(r.Context(), agent.ID, services.UpdateAgentInput{
			Name:            req.Name,
			Title:           req.Title,
			ReportsToID:     req.ReportsToID,
			Tags:            req.Tags,
			Description:     req.Description,
			SystemPrompt:    req.SystemPrompt,
			InstructionFile: req.InstructionFile,
			ExtraArgs:       req.ExtraArgs,
		})
		if err != nil {
			s.apiError(w, err)
			return
		}
	}
	if req.Config != nil || req.Env != nil {
		agent, err = s.agents.UpdateConfig(r.Context(), agent.ID, req.Config, req.Env)
		if err != nil {
			s.apiError(w, err)
			return
		}
	}
	if req.Capability != "" {
		agent, err = s.agents.UpdateCapability(r.Context(), agent.ID, req.Capability)
		if err != nil {
			s.apiError(w, err)
			return
		}
	}
	if req.Permissions != nil {
		agent, err = s.agents.UpdatePermissions(r.Context(), agent.ID, req.Permissions)
		if err != nil {
			s.apiError(w, err)
			return
		}
	}
	if req.SkillIDs != nil {
		agent, err = s.agents.UpdateSkills(r.Context(), agent.ID, req.SkillIDs)
		if err != nil {
			s.apiError(w, err)
			return
		}
	}
	s.apiData(w, agent)
}

func (s *Server) handleAPIV1DeleteAgent(w http.ResponseWriter, r *http.Request) {
	if err := s.agents.Delete(r.Context(), r.PathValue("id")); err != nil {
		s.apiError(w, err)
		return
	}
	s.apiData(w, map[string]string{"status": "deleted"})
}

func (s *Server) handleAPIV1AgentTasks(w http.ResponseWriter, r *http.Request) {
	s.apiTasksWithFilter(w, r, ports.TaskFilter{AgentID: r.PathValue("id")})
}

func (s *Server) handleAPIV1AgentRuns(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.tasks.List(r.Context(), ports.TaskFilter{AgentID: r.PathValue("id")})
	if err != nil {
		s.apiError(w, err)
		return
	}
	runs := make([]domain.Run, 0)
	for _, task := range tasks {
		taskRuns, _ := s.tasks.GetRuns(r.Context(), task.ID)
		runs = append(runs, taskRuns...)
	}
	s.apiList(w, runs, map[string]any{"count": len(runs)})
}

func (s *Server) handleAPIV1SetAgentSkills(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SkillIDs []string `json:"skill_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.apiError(w, domain.ErrInvalidInput)
		return
	}
	agent, err := s.agents.UpdateSkills(r.Context(), r.PathValue("id"), req.SkillIDs)
	if err != nil {
		s.apiError(w, err)
		return
	}
	s.apiData(w, agent)
}
