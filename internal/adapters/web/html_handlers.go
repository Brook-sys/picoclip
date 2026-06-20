package web

import (
	"net/http"
	"sort"
	"strings"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
	"picoclip/internal/core/services"
)

func (s *Server) handleWebRuns(w http.ResponseWriter, r *http.Request) {
	agents, tasks, projects, _, ok := s.loadWebState(w, r)
	if !ok {
		return
	}

	var runs []domain.Run
	for _, task := range tasks {
		taskRuns, err := s.storage.Runs().ListByTask(r.Context(), task.ID)
		if err == nil {
			runs = append(runs, taskRuns...)
		}
	}

	// Sort runs descending by creation
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].StartedAt.After(runs[j].StartedAt)
	})

	agentMap := make(map[string]domain.Agent)
	for _, a := range agents {
		agentMap[a.ID] = a
	}

	taskMap := make(map[string]taskResponse)
	for _, t := range tasks {
		taskMap[t.ID] = t
	}

	_ = projects

	if err := RunsPage(runs, taskMap, agentMap).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleWebRunDetail(w http.ResponseWriter, r *http.Request) {
	run, err := s.storage.Runs().Get(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	task, err := s.tasks.Get(r.Context(), run.TaskID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	agent, err := s.agents.Get(r.Context(), run.AgentID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := RunDetailPage(run, s.taskResponse(r, task), agent).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleWebDashboard(w http.ResponseWriter, r *http.Request) {
	agents, _, projects, _, ok := s.loadWebState(w, r)
	if !ok {
		return
	}
	view, err := loadDashboardView(r.Context(), s, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := DashboardPage(view, agents, projects).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleWebProjects(w http.ResponseWriter, r *http.Request) {
	agents, tasks, projects, skills, ok := s.loadWebState(w, r)
	if !ok {
		return
	}
	if err := ProjectsPage(projects, agents, tasks, skills).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleWebProjectDetail(w http.ResponseWriter, r *http.Request) {
	project, err := s.storage.Workspaces().Get(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	agents, tasks, _, skills, ok := s.loadWebState(w, r)
	if !ok {
		return
	}
	projectAgents := agents
	projectTasks := make([]taskResponse, 0)
	for _, task := range tasks {
		if task.ProjectID == project.ID {
			projectTasks = append(projectTasks, task)
		}
	}
	projectSkills := make([]domain.Skill, 0)
	for _, skill := range skills {
		if skill.ProjectID == "" || skill.ProjectID == project.ID {
			projectSkills = append(projectSkills, skill)
		}
	}
	events, _ := s.storage.Events().ListRecent(r.Context(), 20)
	if err := ProjectDetailPage(project, projectAgents, projectTasks, projectSkills, events).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleWebAgentNew(w http.ResponseWriter, r *http.Request) {
	projects, err := s.projects.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	agents, err := s.agents.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	skills, err := s.skills.List(r.Context(), "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := AgentNewPage(projects, agents, skills).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) loadSettingsView(r *http.Request) (SettingsView, error) {
	ctx := r.Context()
	view := defaultSettingsView()
	raw, err := s.storage.Settings().List(ctx)
	if err != nil {
		return view, err
	}
	if val, ok := raw["general"]; ok {
		view.General = decodeSettingsValue(val, view.General)
	}
	if val, ok := raw["adapters"]; ok {
		view.Adapters = decodeSettingsValue(val, view.Adapters)
	}
	if val, ok := raw["environment"]; ok {
		view.Environment = decodeSettingsValue(val, view.Environment)
	}

	// Basic Stats
	if agents, err := s.agents.List(ctx); err == nil {
		view.Stats.Agents = len(agents)
	}
	if projects, err := s.projects.List(ctx); err == nil {
		view.Stats.Projects = len(projects)
	}
	if skills, err := s.skills.List(ctx, ""); err == nil {
		view.Stats.Skills = len(skills)
	}
	if tasks, err := s.tasks.List(ctx, ports.TaskFilter{}); err == nil {
		view.Stats.Tasks = len(tasks)
	}

	return view, nil
}

func (s *Server) handleWebSettings(w http.ResponseWriter, r *http.Request) {
	view, err := s.loadSettingsView(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := SettingsPage(view).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleWebPostSettingsGeneral(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	view, _ := s.loadSettingsView(r)
	view.General.Theme = r.FormValue("theme")
	view.General.LogLevel = r.FormValue("log_level")
	view.General.MaxTaskRetries = r.FormValue("max_task_retries")

	s.storage.Settings().Set(r.Context(), "general", encodeSettingsValue(view.General))
	s.handleWebSettings(w, r)
}

func (s *Server) handleWebPostSettingsAdapters(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	adapter := r.FormValue("adapter")
	if adapter == "" {
		http.Error(w, "adapter required", http.StatusBadRequest)
		return
	}
	view, _ := s.loadSettingsView(r)
	if view.Adapters[adapter] == nil {
		view.Adapters[adapter] = make(map[string]string)
	}
	for k, v := range r.Form {
		if k != "adapter" {
			view.Adapters[adapter][k] = v[0]
		}
	}
	s.storage.Settings().Set(r.Context(), "adapters", encodeSettingsValue(view.Adapters))
	s.handleWebSettings(w, r)
}

func (s *Server) handleWebPostSettingsReset(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.ToUpper(r.FormValue("confirm_text")) != "RESET" {
		w.Header().Set("HX-Retarget", "body")
		w.Header().Set("HX-Reswap", "none")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := s.storage.ResetAllData(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.skills.InstallBuiltins(r.Context())
	s.projects.EnsureDefault(r.Context())

	w.Header().Set("HX-Redirect", "/")
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleWebAgents(w http.ResponseWriter, r *http.Request) {
	agents, tasks, projects, _, ok := s.loadWebState(w, r)
	if !ok {
		return
	}
	if r.URL.Query().Get("tag") != "" {
		filtered, err := s.agents.ListByTag(r.Context(), r.URL.Query().Get("tag"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		agents = filtered
	}
	if err := AgentsPage(agents, projects, tasks).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleWebAgentDetail(w http.ResponseWriter, r *http.Request) {
	agent, err := s.agents.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	agents, tasks, projects, skills, ok := s.loadWebState(w, r)
	if !ok {
		return
	}
	agentTasks := make([]taskResponse, 0)
	runs := make([]domain.Run, 0)
	for _, task := range tasks {
		if task.AgentID == agent.ID {
			agentTasks = append(agentTasks, task)
			taskRuns, _ := s.tasks.GetRuns(r.Context(), task.ID)
			runs = append(runs, taskRuns...)
		}
	}
	if err := AgentDetailPage(agent, agents, agentTasks, projects, runs, skills).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleWebTasks(w http.ResponseWriter, r *http.Request) {
	agents, err := s.agents.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	projects, err := s.projects.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	filter := ports.TaskFilter{AgentID: r.URL.Query().Get("agent_id"), ParentID: r.URL.Query().Get("parent_id"), WorkspaceID: r.URL.Query().Get("project_id"), Status: domain.TaskStatus(r.URL.Query().Get("status"))}
	tasks, err := s.tasks.List(r.Context(), filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := TasksPage(s.taskResponses(r, tasks), agents, projects, r.URL.Query().Get("status")).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleWebTaskDetail(w http.ResponseWriter, r *http.Request) {
	task, err := s.tasks.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	agents, err := s.agents.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	projects, err := s.projects.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	messages, _ := s.tasks.GetMessages(r.Context(), task.ID)
	runs, _ := s.tasks.GetRuns(r.Context(), task.ID)
	events, _ := s.storage.Events().ListByTask(r.Context(), task.ID)
	children, _ := s.tasks.List(r.Context(), ports.TaskFilter{ParentID: task.ID})
	if err := TaskDetailPage(s.taskResponse(r, task), task, agents, projects, messages, runs, events, s.taskResponses(r, children)).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleWebSkills(w http.ResponseWriter, r *http.Request) {
	agents, _, projects, skills, ok := s.loadWebState(w, r)
	if !ok {
		return
	}
	if err := SkillsPage(skills, projects, agents).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleWebSkillDetail(w http.ResponseWriter, r *http.Request) {
	skill, err := s.skills.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	agents, err := s.agents.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	projects, err := s.projects.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := SkillDetailPage(skill, projects, agents).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleWebActivity(w http.ResponseWriter, r *http.Request) {
	events, err := s.storage.Events().ListRecent(r.Context(), 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := ActivityPage(events).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleWebPostAgent(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	permissions := services.PermissionsForPreset(r.FormValue("permission_preset"))
	agent, err := s.agents.CreateFull(r.Context(), services.CreateAgentInput{
		Name:            r.FormValue("name"),
		Title:           r.FormValue("title"),
		ReportsToID:     r.FormValue("reports_to_id"),
		Tags:            parseTags(r.FormValue("tags")),
		Type:            domain.AgentType(r.FormValue("type")),
		Description:     r.FormValue("description"),
		SystemPrompt:    r.FormValue("system_prompt"),
		InstructionFile: r.FormValue("instruction_file"),
		SkillIDs:        r.Form["skill_ids"],
		Config:          parseKeyValueLines(r.FormValue("config")),
		Env:             parseKeyValueLines(r.FormValue("env")),
		ExtraArgs:       parseLines(r.FormValue("extra_args")),
		Capability:      domain.CapabilityWorker,
		Permissions:     permissions,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/agents/"+agent.ID, http.StatusSeeOther)
}

func (s *Server) handleWebUpdateAgent(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	name := r.FormValue("name")
	title := r.FormValue("title")
	reportsToID := r.FormValue("reports_to_id")
	tags := parseTags(r.FormValue("tags"))
	description := r.FormValue("description")
	systemPrompt := r.FormValue("system_prompt")
	instructionFile := r.FormValue("instruction_file")
	extraArgs := parseLines(r.FormValue("extra_args"))
	if _, err := s.agents.UpdateIdentity(r.Context(), r.PathValue("id"), services.UpdateAgentInput{Name: &name, Title: &title, ReportsToID: &reportsToID, Tags: &tags, Description: &description, SystemPrompt: &systemPrompt, InstructionFile: &instructionFile, ExtraArgs: &extraArgs}); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, _ = s.agents.UpdateConfig(r.Context(), r.PathValue("id"), parseKeyValueLines(r.FormValue("config")), parseKeyValueLines(r.FormValue("env")))
	s.handleWebAgentDetail(w, r)
}

func (s *Server) handleWebDeleteAgent(w http.ResponseWriter, r *http.Request) {
	if err := s.agents.Delete(r.Context(), r.PathValue("id")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.handleWebAgents(w, r)
}

func (s *Server) handleWebUpdateAgentCapability(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := s.agents.UpdateCapability(r.Context(), r.PathValue("id"), domain.AgentCapability(r.FormValue("capability"))); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.handleWebAgentDetail(w, r)
}

func (s *Server) handleWebUpdateAgentPermissions(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	values := r.Form["permissions"]
	permissions := make([]domain.AgentPermission, 0, len(values))
	if r.FormValue("permission_preset") != "" {
		permissions = services.PermissionsForPreset(r.FormValue("permission_preset"))
	} else {
		for _, value := range values {
			permissions = append(permissions, domain.AgentPermission(value))
		}
	}
	if _, err := s.agents.UpdatePermissions(r.Context(), r.PathValue("id"), permissions); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.handleWebAgentDetail(w, r)
}

func (s *Server) handleWebUpdateAgentSkills(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := s.agents.UpdateSkills(r.Context(), r.PathValue("id"), r.Form["skill_ids"]); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.handleWebAgentDetail(w, r)
}

func (s *Server) handleWebPostTask(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, err := s.tasks.CreateInWorkspace(r.Context(), r.FormValue("project_id"), r.FormValue("agent_id"), r.FormValue("prompt"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.handleWebTasks(w, r)
}

func (s *Server) handleWebCancelTask(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := s.tasks.Cancel(r.Context(), r.PathValue("id"), r.FormValue("reason")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.handleWebTaskDetail(w, r)
}

func (s *Server) handleWebUpdateTaskStatus(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := s.tasks.UpdateStatus(r.Context(), r.PathValue("id"), domain.TaskStatus(r.FormValue("status")), r.FormValue("comment"), ""); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.handleWebTaskDetail(w, r)
}

func (s *Server) handleWebWakeTask(w http.ResponseWriter, r *http.Request) {
	if _, err := s.tasks.Wake(r.Context(), r.PathValue("id")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.handleWebTaskDetail(w, r)
}

func (s *Server) handleWebPostMessage(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	role := domain.MessageRole(r.FormValue("role"))
	if role == "" {
		role = domain.MessageRoleUser
	}
	if _, err := s.tasks.AddMessage(r.Context(), r.PathValue("id"), "", "", role, r.FormValue("body")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.handleWebPartialsTaskDetail(w, r)
}

func (s *Server) handleWebPostDelegate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := s.tasks.Delegate(r.Context(), r.PathValue("id"), "", r.FormValue("agent_id"), r.FormValue("prompt")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.handleWebTaskDetail(w, r)
}

func (s *Server) handleWebPostSkill(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	files := make([]domain.SkillFile, 0, 1)
	if r.FormValue("file_path") != "" || r.FormValue("file_content") != "" {
		files = append(files, domain.SkillFile{Path: r.FormValue("file_path"), Content: r.FormValue("file_content")})
	}
	if _, err := s.skills.CreateWithFiles(r.Context(), r.FormValue("project_id"), r.FormValue("name"), r.FormValue("description"), r.FormValue("instructions"), files); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.handleWebSkills(w, r)
}

func (s *Server) handleWebUpdateSkill(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	enabled := r.FormValue("enabled") == "on" || r.FormValue("enabled") == "true"
	if _, err := s.skills.Update(r.Context(), r.PathValue("id"), r.FormValue("name"), r.FormValue("description"), r.FormValue("instructions"), enabled); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.handleWebSkillDetail(w, r)
}

func (s *Server) handleWebDeleteSkill(w http.ResponseWriter, r *http.Request) {
	if err := s.skills.Delete(r.Context(), r.PathValue("id")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.handleWebSkills(w, r)
}

func (s *Server) handleWebResetSkill(w http.ResponseWriter, r *http.Request) {
	if _, err := s.skills.ResetBuiltin(r.Context(), r.PathValue("id")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.handleWebSkillDetail(w, r)
}

func (s *Server) handleWebUpdateSkillAgents(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := s.skills.UpdateAssignments(r.Context(), r.PathValue("id"), r.Form["agent_ids"], nil, nil); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.handleWebSkillDetail(w, r)
}

func (s *Server) handleWebPostProject(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := s.projects.Create(r.Context(), r.FormValue("name"), r.FormValue("description")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.handleWebProjects(w, r)
}

func (s *Server) handleWebPartialsTaskDetail(w http.ResponseWriter, r *http.Request) {
	task, err := s.tasks.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	agents, err := s.agents.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	projects, _ := s.projects.List(r.Context())
	messages, _ := s.tasks.GetMessages(r.Context(), task.ID)
	runs, _ := s.tasks.GetRuns(r.Context(), task.ID)
	events, _ := s.storage.Events().ListByTask(r.Context(), task.ID)
	children, _ := s.tasks.List(r.Context(), ports.TaskFilter{ParentID: task.ID})
	if err := TaskLive(s.taskResponse(r, task), messages, runs, events, s.taskResponses(r, children), agents, projects).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleWebPartialsTasks(w http.ResponseWriter, r *http.Request) {
	agents, err := s.agents.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	projects, _ := s.projects.List(r.Context())
	filter := ports.TaskFilter{AgentID: r.URL.Query().Get("agent_id"), ParentID: r.URL.Query().Get("parent_id"), WorkspaceID: r.URL.Query().Get("project_id"), Status: domain.TaskStatus(r.URL.Query().Get("status"))}
	tasks, err := s.tasks.List(r.Context(), filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := TaskTable(s.taskResponses(r, tasks), agents, projects).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) loadWebState(w http.ResponseWriter, r *http.Request) ([]domain.Agent, []taskResponse, []domain.Workspace, []domain.Skill, bool) {
	agents, err := s.agents.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil, nil, nil, nil, false
	}
	projects, err := s.projects.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil, nil, nil, nil, false
	}
	skills, err := s.skills.List(r.Context(), "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil, nil, nil, nil, false
	}
	tasks, err := s.tasks.List(r.Context(), ports.TaskFilter{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil, nil, nil, nil, false
	}
	return agents, s.taskResponses(r, tasks), projects, skills, true
}

func (s *Server) taskResponses(r *http.Request, tasks []domain.Task) []taskResponse {
	response := make([]taskResponse, 0, len(tasks))
	for _, task := range tasks {
		response = append(response, s.taskResponse(r, task))
	}
	return response
}

func parseKeyValueLines(value string) map[string]string {
	out := make(map[string]string)
	for _, line := range strings.Split(value, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "=", 2)
		if len(parts) == 2 && parts[0] != "" {
			out[parts[0]] = parts[1]
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseLines(value string) []string {
	out := make([]string, 0)
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func parseTags(value string) []string {
	value = strings.ReplaceAll(value, ",", "\n")
	return parseLines(value)
}
