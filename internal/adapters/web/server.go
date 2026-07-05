package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
	"picoclip/internal/core/services"
)

type Server struct {
	agents          *services.AgentService
	tasks           *services.TaskService
	skills          *services.SkillService
	projects        *services.WorkspaceService
	runtimes        *services.RuntimeManager
	diagnostics     *services.DiagnosticsService
	storage         ports.Storage
	bus             ports.EventBus
	debugMode       bool
	adapterSettings map[string]map[string]string
	auth            *services.Authorizer
}

func NewServer(agents *services.AgentService, tasks *services.TaskService, skills *services.SkillService, projects *services.WorkspaceService, runtimes *services.RuntimeManager, diagnostics *services.DiagnosticsService, storage ports.Storage, bus ports.EventBus, debugMode bool) *Server {
	return &Server{agents: agents, tasks: tasks, skills: skills, projects: projects, runtimes: runtimes, diagnostics: diagnostics, storage: storage, bus: bus, debugMode: debugMode, adapterSettings: map[string]map[string]string{"noop": {"timeout": "1m"}}, auth: services.NewAuthorizer(storage)}
}

func (s *Server) Mount(mux *http.ServeMux) {
	s.mountAPIV1(mux)

	mux.HandleFunc("GET /api/agents", s.handleGetAgents)
	mux.HandleFunc("POST /api/agents", s.handleCreateAgent)
	mux.HandleFunc("DELETE /api/agents/{id}", s.handleDeleteAgent)
	mux.HandleFunc("POST /api/agents/{id}/permissions", s.handleUpdateAgentPermissions)
	mux.HandleFunc("POST /api/agents/{id}/skills", s.handleUpdateAgentSkills)
	mux.HandleFunc("GET /api/tasks", s.handleGetTasks)
	mux.HandleFunc("POST /api/tasks", s.handleCreateTask)
	mux.HandleFunc("POST /api/tasks/{id}/cancel", s.handleCancelTask)
	mux.HandleFunc("POST /api/tasks/{id}/messages", s.handleCreateMessage)
	mux.HandleFunc("POST /api/tasks/{id}/delegate", s.handleDelegateTask)
	mux.HandleFunc("GET /api/skills", s.handleGetSkills)
	mux.HandleFunc("POST /api/skills", s.handleCreateSkill)
	mux.HandleFunc("PUT /api/skills/{id}", s.handleUpdateSkill)
	mux.HandleFunc("DELETE /api/skills/{id}", s.handleDeleteSkill)
	mux.HandleFunc("GET /api/projects", s.handleGetProjects)
	mux.HandleFunc("POST /api/projects", s.handleCreateProject)
	mux.HandleFunc("GET /api/capabilities", s.handleGetCapabilities)
	mux.HandleFunc("GET /api/search", s.handleAPISearch)
	mux.HandleFunc("GET /api/runtimes", s.handleAPIRuntimes)
	mux.HandleFunc("GET /api/diagnostics", s.handleAPIDiagnostics)
	mux.HandleFunc("GET /agent-api/docs", s.handleAgentDocs)
	mux.HandleFunc("GET /agent-api/me", s.handleAgentMe)
	mux.HandleFunc("GET /agent-api/agents/me/inbox-lite", s.handleAgentInboxLite)
	mux.HandleFunc("GET /agent-api/agents", s.handleGetAgents)
	mux.HandleFunc("GET /agent-api/tasks", s.handleGetTasks)
	mux.HandleFunc("GET /agent-api/issues", s.handleGetTasks)
	mux.HandleFunc("GET /agent-api/tasks/{id}", s.handleAgentTaskDetail)
	mux.HandleFunc("GET /agent-api/issues/{id}", s.handleAgentTaskDetail)
	mux.HandleFunc("GET /agent-api/tasks/{id}/heartbeat-context", s.handleAgentHeartbeatContext)
	mux.HandleFunc("GET /agent-api/issues/{id}/heartbeat-context", s.handleAgentHeartbeatContext)
	mux.HandleFunc("GET /agent-api/tasks/{id}/comments", s.handleAgentTaskComments)
	mux.HandleFunc("GET /agent-api/issues/{id}/comments", s.handleAgentTaskComments)
	mux.HandleFunc("GET /agent-api/projects", s.handleGetProjects)
	mux.HandleFunc("GET /agent-api/skills", s.handleGetSkills)
	mux.HandleFunc("POST /agent-api/tasks", s.handleCreateTask)
	mux.HandleFunc("POST /agent-api/issues", s.handleCreateTask)
	mux.HandleFunc("POST /agent-api/tasks/{id}/comments", s.handleAgentCreateComment)
	mux.HandleFunc("POST /agent-api/issues/{id}/comments", s.handleAgentCreateComment)
	mux.HandleFunc("POST /agent-api/tasks/{id}/messages", s.handleCreateMessage)
	mux.HandleFunc("POST /agent-api/tasks/{id}/checkout", s.handleAgentCheckoutTask)
	mux.HandleFunc("POST /agent-api/issues/{id}/checkout", s.handleAgentCheckoutTask)
	mux.HandleFunc("POST /agent-api/tasks/{id}/release", s.handleAgentReleaseTask)
	mux.HandleFunc("POST /agent-api/issues/{id}/release", s.handleAgentReleaseTask)
	mux.HandleFunc("PATCH /agent-api/tasks/{id}", s.handleAgentUpdateTask)
	mux.HandleFunc("PATCH /agent-api/issues/{id}", s.handleAgentUpdateTask)
	mux.HandleFunc("POST /agent-api/tasks/{id}/wake", s.handleAgentWakeTask)
	mux.HandleFunc("POST /agent-api/issues/{id}/wake", s.handleAgentWakeTask)
	mux.HandleFunc("POST /agent-api/tasks/{id}/delegate", s.handleDelegateTask)
	mux.HandleFunc("POST /agent-api/tasks/{id}/cancel", s.handleCancelTask)

	mux.HandleFunc("GET /", s.handleWebDashboard)
	mux.HandleFunc("GET /projects", s.handleWebProjects)
	mux.HandleFunc("GET /projects/{id}", s.handleWebProjectDetail)
	mux.HandleFunc("GET /agents", s.handleWebAgents)
	mux.HandleFunc("GET /agents/new", s.handleWebAgentNew)
	mux.HandleFunc("GET /agents/{id}", s.handleWebAgentDetail)
	mux.HandleFunc("GET /tasks", s.handleWebTasks)
	mux.HandleFunc("GET /tasks/{id}", s.handleWebTaskDetail)
	mux.HandleFunc("GET /runs", s.handleWebRuns)
	mux.HandleFunc("GET /runs/{id}", s.handleWebRunDetail)
	mux.HandleFunc("GET /partials/runs/{id}", s.handleWebPartialsRunDetail)
	mux.HandleFunc("GET /skills", s.handleWebSkills)
	mux.HandleFunc("GET /skills/{id}", s.handleWebSkillDetail)
	mux.HandleFunc("GET /activity", s.handleWebActivity)
	mux.HandleFunc("GET /sse/activity", s.handleSSEActivity)
	mux.HandleFunc("GET /sse/runs/{id}/logs", s.handleSSERunLogs)
	mux.HandleFunc("GET /settings", s.handleWebSettings)
	mux.HandleFunc("GET /settings/adapters", s.handleWebSettings)
	mux.HandleFunc("POST /settings/general", s.handleWebPostSettingsGeneral)
	mux.HandleFunc("POST /settings/adapters", s.handleWebPostSettingsAdapters)
	mux.HandleFunc("POST /settings/adapters/test", s.handleWebPostSettingsAdaptersTest)
	mux.HandleFunc("POST /runtimes/{id}/install", s.handleWebPostRuntimeInstall)
	mux.HandleFunc("POST /runtimes/{id}/existing", s.handleWebPostRuntimeExisting)
	mux.HandleFunc("POST /runtimes/{id}/test", s.handleWebPostRuntimeTest)
	mux.HandleFunc("POST /runtimes/{id}/test-ai", s.handleWebPostRuntimeTestAI)
	mux.HandleFunc("POST /runtimes/{id}/uninstall", s.handleWebPostRuntimeUninstall)
	mux.HandleFunc("POST /runtimes/{id}/config", s.handleWebPostRuntimeConfig)
	mux.HandleFunc("POST /runtimes/{id}/toggle", s.handleWebPostRuntimeToggle)
	mux.HandleFunc("POST /settings/environment", s.handleWebPostSettingsEnvironment)
	mux.HandleFunc("POST /settings/budgets", s.handleWebPostSettingsBudgets)
	mux.HandleFunc("POST /settings/budgets/{id}/toggle", s.handleWebToggleBudget)
	mux.HandleFunc("POST /settings/budgets/{id}/delete", s.handleWebDeleteBudget)
	mux.HandleFunc("POST /settings/webhooks", s.handleWebCreateWebhook)
	mux.HandleFunc("POST /settings/webhooks/{id}/edit", s.handleWebUpdateWebhook)
	mux.HandleFunc("POST /settings/webhooks/{id}/toggle", s.handleWebToggleWebhook)
	mux.HandleFunc("POST /settings/webhooks/{id}/test", s.handleWebTestWebhook)
	mux.HandleFunc("POST /settings/webhooks/{id}/delete", s.handleWebDeleteWebhook)
	mux.HandleFunc("POST /settings/webhook-deliveries/{id}/retry", s.handleWebRetryWebhookDelivery)
	mux.HandleFunc("GET /settings/webhooks/{id}", s.handleWebWebhookDetail)
	mux.HandleFunc("GET /settings/export", s.handleWebSettingsExport)
	mux.HandleFunc("POST /settings/import", s.handleWebSettingsImport)
	mux.HandleFunc("POST /settings/reset", s.handleWebPostSettingsReset)
	mux.HandleFunc("POST /agents", s.handleWebPostAgent)
	mux.HandleFunc("POST /agents/{id}/edit", s.handleWebUpdateAgent)
	mux.HandleFunc("POST /agents/{id}/delete", s.handleWebDeleteAgent)
	mux.HandleFunc("POST /agents/{id}/capability", s.handleWebUpdateAgentCapability)
	mux.HandleFunc("POST /agents/{id}/permissions", s.handleWebUpdateAgentPermissions)
	mux.HandleFunc("POST /agents/{id}/skills", s.handleWebUpdateAgentSkills)
	mux.HandleFunc("POST /tasks", s.handleWebPostTask)
	mux.HandleFunc("POST /tasks/{id}/cancel", s.handleWebCancelTask)
	mux.HandleFunc("POST /tasks/{id}/status", s.handleWebUpdateTaskStatus)
	mux.HandleFunc("POST /tasks/{id}/wake", s.handleWebWakeTask)
	mux.HandleFunc("POST /tasks/{id}/pause", s.handleWebPauseContinuousTask)
	mux.HandleFunc("POST /tasks/{id}/resume", s.handleWebResumeContinuousTask)
	mux.HandleFunc("POST /tasks/{id}/run-now", s.handleWebRunContinuousTaskNow)
	mux.HandleFunc("POST /tasks/{id}/edit-inline", s.handleWebPostTaskInlineEdit)
	mux.HandleFunc("POST /tasks/{id}/messages", s.handleWebPostMessage)
	mux.HandleFunc("POST /tasks/{id}/delegate", s.handleWebPostDelegate)
	mux.HandleFunc("POST /agents/{id}/edit-inline", s.handleWebPostAgentInlineEdit)
	mux.HandleFunc("POST /skills", s.handleWebPostSkill)
	mux.HandleFunc("POST /skills/{id}/edit", s.handleWebUpdateSkill)
	mux.HandleFunc("POST /skills/{id}/delete", s.handleWebDeleteSkill)
	mux.HandleFunc("POST /skills/{id}/reset", s.handleWebResetSkill)
	mux.HandleFunc("POST /skills/{id}/agents", s.handleWebUpdateSkillAgents)
	mux.HandleFunc("POST /skills/{id}/files/{index}", s.handleWebUpdateSkillFile)
	mux.HandleFunc("POST /projects", s.handleWebPostProject)
	mux.HandleFunc("GET /partials/tasks", s.handleWebPartialsTasks)
	mux.HandleFunc("GET /partials/tasks/{id}", s.handleWebPartialsTaskDetail)
	mux.Handle("GET /assets/", s.handleAssets())
}

func (s *Server) handleGetAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := s.agents.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.jsonResponse(w, agents)
}

func (s *Server) handleCreateAgent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProjectID   string                 `json:"project_id"`
		Name        string                 `json:"name"`
		Type        string                 `json:"type"`
		Description string                 `json:"description"`
		Capability  domain.AgentCapability `json:"capability"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	agentType := domain.AgentType(req.Type)
	if err := s.validateAgentRuntime(r.Context(), agentType); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	agent, err := s.agents.CreateFull(r.Context(), services.CreateAgentInput{
		ProjectID:   req.ProjectID,
		Name:        req.Name,
		Type:        agentType,
		Description: req.Description,
		Capability:  req.Capability,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.jsonResponse(w, agent)
}

func (s *Server) handleGetTasks(w http.ResponseWriter, r *http.Request) {
	status, statuses := parseTaskStatusFilter(r.URL.Query().Get("status"))
	filter := ports.TaskFilter{AgentID: r.URL.Query().Get("agent_id"), ParentID: r.URL.Query().Get("parent_id"), WorkspaceID: r.URL.Query().Get("project_id"), Status: status, Statuses: statuses}
	tasks, err := s.tasks.List(r.Context(), filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := make([]taskResponse, 0, len(tasks))
	for _, task := range tasks {
		response = append(response, s.taskResponse(r, task))
	}
	s.jsonResponse(w, response)
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProjectID       string `json:"project_id"`
		ParentID        string `json:"parent_id"`
		AgentID         string `json:"agent_id"`
		AssigneeAgentID string `json:"assignee_agent_id"`
		FromAgentID     string `json:"from_agent_id"`
		Title           string `json:"title"`
		Message         string `json:"message"`
		Prompt          string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Prompt == "" {
		req.Prompt = req.Message
	}

	if req.AgentID == "" {
		req.AgentID = req.AssigneeAgentID
	}
	if isAgentAPIRequest(r) {
		if req.FromAgentID == "" {
			req.FromAgentID = req.AgentID
		}
		if err := s.auth.RequireAgentPermission(r.Context(), req.FromAgentID, domain.PermissionTasksCreate); err != nil {
			writeTaskError(w, err)
			return
		}
	}
	task, err := s.tasks.CreateChildInWorkspace(r.Context(), req.ProjectID, req.ParentID, req.AgentID, req.Title, req.Prompt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.jsonResponse(w, s.taskResponse(r, task))
}

func (s *Server) handleCreateMessage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FromID string             `json:"from_id"`
		ToID   string             `json:"to_id"`
		Role   domain.MessageRole `json:"role"`
		Body   string             `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Role == "" {
		req.Role = domain.MessageRoleUser
	}
	if isAgentAPIRequest(r) {
		if err := s.auth.RequireAgentPermission(r.Context(), req.FromID, domain.PermissionTasksUpdate); err != nil {
			writeTaskError(w, err)
			return
		}
	}
	message, err := s.tasks.AddMessage(r.Context(), r.PathValue("id"), req.FromID, req.ToID, req.Role, req.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.jsonResponse(w, message)
}

func (s *Server) handleAgentMe(w http.ResponseWriter, r *http.Request) {
	agents, err := s.agents.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	agentID := r.URL.Query().Get("agent_id")
	for _, agent := range agents {
		if agentID == "" || agent.ID == agentID {
			tasks, _ := s.tasks.List(r.Context(), ports.TaskFilter{AgentID: agent.ID})
			s.jsonResponse(w, map[string]any{"agent": agent, "tasks": s.taskResponses(r, tasks)})
			return
		}
	}
	http.Error(w, domain.ErrNotFound.Error(), http.StatusNotFound)
}

func (s *Server) handleAgentInboxLite(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("agent_id")
	if agentID == "" {
		http.Error(w, "agent_id required", http.StatusBadRequest)
		return
	}
	tasks, err := s.tasks.List(r.Context(), ports.TaskFilter{AgentID: agentID})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		if task.Status == domain.TaskStatusDone || task.Status == domain.TaskStatusCancelled {
			continue
		}
		items = append(items, map[string]any{
			"task_id": task.ID,
			"title":   task.Title,
			"status":  task.Status,
			"reason":  s.latestWakeReason(r, task.ID),
		})
	}
	s.jsonResponse(w, map[string]any{"agent_id": agentID, "inbox": items})
}

func (s *Server) handleAgentHeartbeatContext(w http.ResponseWriter, r *http.Request) {
	task, err := s.tasks.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	messages, _ := s.tasks.GetMessages(r.Context(), task.ID)
	lastUser := ""
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == domain.MessageRoleUser {
			lastUser = messages[i].Body
			break
		}
	}
	ctx := map[string]any{
		"task_id":           task.ID,
		"title":             task.Title,
		"prompt":            task.Prompt,
		"last_user_comment": lastUser,
		"status":            task.Status,
		"checkout_run_id":   task.CheckoutRunID,
		"wake_reason":       s.latestWakeReason(r, task.ID),
		"skills":            s.compactSkills(r, task),
		"apis":              []string{"/agent-api/tasks", "/agent-api/projects", "/agent-api/skills"},
	}
	s.jsonResponse(w, ctx)
}

func (s *Server) latestWakeReason(r *http.Request, taskID string) string {
	wakeups, err := s.storage.Wakeups().ListByTask(r.Context(), taskID)
	if err != nil || len(wakeups) == 0 {
		return "assignment"
	}
	return string(wakeups[0].Reason)
}

func (s *Server) compactSkills(r *http.Request, task domain.Task) []map[string]string {
	agent, err := s.agents.Get(r.Context(), task.AgentID)
	if err != nil {
		return []map[string]string{}
	}
	skills, err := s.skills.List(r.Context(), task.WorkspaceID)
	if err != nil {
		return []map[string]string{}
	}
	manual := map[string]struct{}{}
	for _, id := range agent.SkillIDs {
		manual[id] = struct{}{}
	}
	out := make([]map[string]string, 0)
	for _, skill := range skills {
		if !skill.Enabled {
			continue
		}
		if _, ok := manual[skill.ID]; ok || compactSkillAllowedForAgent(skill, agent) || skill.Kind == domain.SkillKindBuiltin && skill.Permission != "" && agentHasPermission(agent, skill.Permission) {
			out = append(out, map[string]string{"id": skill.ID, "name": skill.Name, "description": skill.Description})
		}
	}
	return out
}

func compactSkillAllowedForAgent(skill domain.Skill, agent domain.Agent) bool {
	for _, id := range skill.AgentIDs {
		if id == agent.ID {
			return true
		}
	}
	for _, agentType := range skill.AllowedAgentTypes {
		if agentType == agent.Type {
			return true
		}
	}
	for _, required := range skill.AllowedPermissions {
		if agentHasPermission(agent, required) {
			return true
		}
	}
	return false
}

func agentHasPermission(agent domain.Agent, required domain.AgentPermission) bool {
	for _, permission := range agent.Permissions {
		if permission == required {
			return true
		}
	}
	return false
}

func (s *Server) handleAgentTaskDetail(w http.ResponseWriter, r *http.Request) {
	task, err := s.tasks.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	messages, _ := s.tasks.GetMessages(r.Context(), task.ID)
	runs, _ := s.tasks.GetRuns(r.Context(), task.ID)
	events, _ := s.storage.Events().ListByTask(r.Context(), task.ID)
	children, _ := s.tasks.List(r.Context(), ports.TaskFilter{ParentID: task.ID})
	s.jsonResponse(w, taskFullResponse{Task: s.taskResponse(r, task), Messages: messages, Runs: runs, Events: events, Children: s.taskResponses(r, children)})
}

func (s *Server) handleAgentTaskComments(w http.ResponseWriter, r *http.Request) {
	messages, err := s.tasks.GetMessages(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.jsonResponse(w, messages)
}

func (s *Server) handleAgentCreateComment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FromID string             `json:"from_id"`
		ToID   string             `json:"to_id"`
		Role   domain.MessageRole `json:"role"`
		Body   string             `json:"body"`
		Reopen bool               `json:"reopen"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Role == "" {
		req.Role = domain.MessageRoleUser
	}
	if req.FromID == "" {
		if task, err := s.tasks.Get(r.Context(), r.PathValue("id")); err == nil {
			req.FromID = task.AgentID
		}
	}
	if err := s.auth.RequireAgentPermission(r.Context(), req.FromID, domain.PermissionTasksUpdate); err != nil {
		writeTaskError(w, err)
		return
	}
	message, err := s.tasks.AddMessage(r.Context(), r.PathValue("id"), req.FromID, req.ToID, req.Role, req.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Reopen {
		_, _ = s.tasks.Wake(r.Context(), r.PathValue("id"))
	}
	s.jsonResponse(w, message)
}

func (s *Server) handleAgentCheckoutTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AgentID          string   `json:"agent_id"`
		RunID            string   `json:"run_id"`
		ExpectedStatuses []string `json:"expected_statuses"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.auth.RequireAgentPermission(r.Context(), req.AgentID, domain.PermissionTasksRun); err != nil {
		writeTaskError(w, err)
		return
	}
	if req.RunID == "" {
		req.RunID = "run_" + r.PathValue("id") + "_auto"
	}
	task, err := s.tasks.Checkout(r.Context(), r.PathValue("id"), req.AgentID, req.RunID, parseTaskStatuses(req.ExpectedStatuses))
	if err != nil {
		writeTaskError(w, err)
		return
	}
	s.jsonResponse(w, s.taskResponse(r, task))
}

func (s *Server) handleAgentReleaseTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AgentID string `json:"agent_id"`
		Comment string `json:"comment"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if err := s.auth.RequireAgentPermission(r.Context(), req.AgentID, domain.PermissionTasksUpdate); err != nil {
		writeTaskError(w, err)
		return
	}
	task, err := s.tasks.Release(r.Context(), r.PathValue("id"), req.AgentID, req.Comment)
	if err != nil {
		writeTaskError(w, err)
		return
	}
	s.jsonResponse(w, s.taskResponse(r, task))
}

func (s *Server) handleAgentUpdateTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Status  domain.TaskStatus `json:"status"`
		Comment string            `json:"comment"`
		AgentID string            `json:"agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.auth.RequireAgentPermission(r.Context(), req.AgentID, domain.PermissionTasksUpdate); err != nil {
		writeTaskError(w, err)
		return
	}
	task, err := s.tasks.UpdateStatus(r.Context(), r.PathValue("id"), req.Status, req.Comment, req.AgentID)
	if err != nil {
		writeTaskError(w, err)
		return
	}
	s.jsonResponse(w, s.taskResponse(r, task))
}

func (s *Server) handleAgentWakeTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AgentID string `json:"agent_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if err := s.auth.RequireAgentPermission(r.Context(), req.AgentID, domain.PermissionTasksUpdate); err != nil {
		writeTaskError(w, err)
		return
	}
	task, err := s.tasks.Wake(r.Context(), r.PathValue("id"))
	if err != nil {
		writeTaskError(w, err)
		return
	}
	s.jsonResponse(w, s.taskResponse(r, task))
}

func parseTaskStatusFilter(value string) (domain.TaskStatus, []domain.TaskStatus) {
	if strings.Contains(value, ",") {
		return "", parseTaskStatuses(strings.Split(value, ","))
	}
	return domain.TaskStatus(value), nil
}

func parseTaskStatuses(values []string) []domain.TaskStatus {
	statuses := make([]domain.TaskStatus, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			statuses = append(statuses, domain.TaskStatus(value))
		}
	}
	return statuses
}

func isAgentAPIRequest(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/agent-api/")
}

func writeTaskError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, domain.ErrConflict) {
		status = http.StatusConflict
	}
	if errors.Is(err, domain.ErrNotFound) {
		status = http.StatusNotFound
	}
	if errors.Is(err, domain.ErrForbidden) {
		status = http.StatusForbidden
	}
	http.Error(w, err.Error(), status)
}

func (s *Server) handleDelegateTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FromAgentID string `json:"from_agent_id"`
		ToAgentID   string `json:"to_agent_id"`
		Title       string `json:"title"`
		Prompt      string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.auth.RequireAgentPermission(r.Context(), req.FromAgentID, domain.PermissionDelegate); err != nil {
		writeTaskError(w, err)
		return
	}
	if req.ToAgentID == "" {
		writeTaskError(w, domain.ErrInvalidInput)
		return
	}
	if _, err := s.agents.Get(r.Context(), req.ToAgentID); err != nil {
		writeTaskError(w, err)
		return
	}

	task, err := s.tasks.Delegate(r.Context(), r.PathValue("id"), req.FromAgentID, req.ToAgentID, req.Prompt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Title != "" {
		task.Title = req.Title
		_ = s.storage.Tasks().Update(r.Context(), task)
	}
	s.jsonResponse(w, s.taskResponse(r, task))
}

type taskResponse struct {
	ID               string            `json:"id"`
	ParentID         string            `json:"parent_id,omitempty"`
	ProjectID        string            `json:"project_id,omitempty"`
	AgentID          string            `json:"agent_id"`
	Title            string            `json:"title"`
	Status           domain.TaskStatus `json:"status"`
	Message          string            `json:"message"`
	Prompt           string            `json:"prompt"`
	Response         string            `json:"response"`
	CancelReason     string            `json:"cancel_reason,omitempty"`
	CheckoutRunID    string            `json:"checkout_run_id,omitempty"`
	Mode             domain.TaskMode   `json:"mode"`
	LoopDelaySeconds int               `json:"loop_delay_seconds"`
	LoopRunCount     int               `json:"loop_run_count"`
	LoopNextRunAt    string            `json:"loop_next_run_at,omitempty"`
	LoopPausedAt     string            `json:"loop_paused_at,omitempty"`
	InputTokens      int               `json:"input_tokens"`
	OutputTokens     int               `json:"output_tokens"`
	TotalTokens      int               `json:"total_tokens"`
	CreatedAt        string            `json:"created_at"`
	UpdatedAt        string            `json:"updated_at"`
}

func (s *Server) taskResponse(r *http.Request, task domain.Task) taskResponse {
	loopNextRunAt := ""
	if task.LoopNextRunAt != nil {
		loopNextRunAt = task.LoopNextRunAt.Format("2006-01-02T15:04:05Z07:00")
	}
	loopPausedAt := ""
	if task.LoopPausedAt != nil {
		loopPausedAt = task.LoopPausedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	runs, _ := s.tasks.GetRuns(r.Context(), task.ID)
	response := ""
	if len(runs) > 0 {
		last := runs[len(runs)-1]
		response = last.Output
		if response == "" {
			response = last.Error
		}
	}
	return taskResponse{
		ID:               task.ID,
		ParentID:         task.ParentID,
		ProjectID:        task.WorkspaceID,
		AgentID:          task.AgentID,
		Title:            task.Title,
		Message:          task.Prompt,
		Prompt:           task.Prompt,
		Response:         response,
		Status:           task.Status,
		CancelReason:     task.CancelReason,
		CheckoutRunID:    task.CheckoutRunID,
		Mode:             task.Mode,
		LoopDelaySeconds: task.LoopDelaySeconds,
		LoopRunCount:     task.LoopRunCount,
		LoopNextRunAt:    loopNextRunAt,
		LoopPausedAt:     loopPausedAt,
		InputTokens:      task.InputTokens,
		OutputTokens:     task.OutputTokens,
		TotalTokens:      task.TotalTokens,
		CreatedAt:        task.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:        task.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func (s *Server) jsonResponse(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}
