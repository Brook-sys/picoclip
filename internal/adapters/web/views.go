package web

import (
	"context"
	"fmt"
	"html"
	"io"
	"strings"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/services"
)

func renderAgentPicker(w io.Writer, agents []domain.Agent, fieldName, selectedID, placeholder string) error {
	return AgentPicker(agents, fieldName, selectedID, placeholder).Render(context.Background(), w)
}

func renderProjectAgents(w io.Writer, project domain.Workspace, agents []domain.Agent) error {
	relatedTag := "Proj:" + project.Name
	if _, err := io.WriteString(w, `<div class="agent-related-list">`); err != nil {
		return err
	}
	for _, agent := range agents {
		related := hasTag(agent.Tags, relatedTag)
		badge := ""
		if related {
			badge = `<span class="badge">Relacionado</span>`
		}
		if _, err := fmt.Fprintf(w, `<article class="agent-mini-card">%s<strong><a href="/agents/%s">%s</a></strong><small>%s</small><div>%s</div></article>`, badge, esc(agent.ID), esc(agent.Name), esc(agent.Title), renderTagChips(agent.Tags)); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, `</div>`)
	return err
}

func hasTag(tags []string, wanted string) bool {
	wanted = strings.ToLower(strings.TrimSpace(wanted))
	for _, tag := range tags {
		if strings.ToLower(strings.TrimSpace(tag)) == wanted {
			return true
		}
	}
	return false
}

func renderCapabilityOptions(w io.Writer, selected domain.AgentCapability) error {
	for _, preset := range services.CapabilityPresets() {
		selectedAttr := ""
		if preset.ID == selected {
			selectedAttr = " selected"
		}
		if _, err := fmt.Fprintf(w, `<option value="%s"%s>%s</option>`, esc(string(preset.ID)), selectedAttr, esc(preset.Name)); err != nil {
			return err
		}
	}
	return nil
}

func renderSkillChips(w io.Writer, skills []domain.Skill, selected []string) error {
	if len(skills) == 0 {
		_, err := io.WriteString(w, `<p class="empty">Nenhuma skill.</p>`)
		return err
	}
	allowed := map[string]bool{}
	for _, id := range selected {
		allowed[id] = true
	}
	for _, skill := range skills {
		class := "chip"
		if allowed[skill.ID] || len(selected) == 0 && skill.Kind == domain.SkillKindBuiltin {
			class += " active"
		}
		if _, err := fmt.Fprintf(w, `<a class="%s" href="/skills/%s">%s</a>`, class, esc(skill.ID), esc(skill.Name)); err != nil {
			return err
		}
	}
	return nil
}

func renderPermissionPresetOptions(w io.Writer, selected []domain.AgentPermission) error {
	selectedKey := matchingPermissionPreset(selected)
	for _, preset := range services.PermissionPresets() {
		selectedAttr := ""
		if preset.ID == selectedKey {
			selectedAttr = " selected"
		}
		if _, err := fmt.Fprintf(w, `<option value="%s"%s>%s — %s</option>`, esc(preset.ID), selectedAttr, esc(preset.Name), esc(preset.Description)); err != nil {
			return err
		}
	}
	return nil
}

func renderPermissionCheckboxes(w io.Writer, selected []domain.AgentPermission) error {
	selectedMap := map[domain.AgentPermission]bool{}
	for _, permission := range selected {
		selectedMap[permission] = true
	}
	currentGroup := ""
	for _, item := range services.PermissionCatalog() {
		if item.Group != currentGroup {
			if currentGroup != "" {
				if _, err := io.WriteString(w, `</div>`); err != nil {
					return err
				}
			}
			currentGroup = item.Group
			if _, err := fmt.Fprintf(w, `<h3 class="form-group-title">%s</h3><div class="permission-grid">`, esc(currentGroup)); err != nil {
				return err
			}
		}
		checked := ""
		if selectedMap[item.ID] {
			checked = " checked"
		}
		if _, err := fmt.Fprintf(w, `<label class="permission-pill" title="%s"><input type="checkbox" name="permissions" value="%s"%s><span>%s</span></label>`, esc(item.Description), esc(string(item.ID)), checked, esc(permissionShortName(item.ID))); err != nil {
			return err
		}
	}
	if currentGroup != "" {
		_, err := io.WriteString(w, `</div>`)
		return err
	}
	return nil
}

func matchingPermissionPreset(selected []domain.AgentPermission) string {
	selectedMap := map[domain.AgentPermission]bool{}
	for _, permission := range selected {
		selectedMap[permission] = true
	}
	for _, preset := range services.PermissionPresets() {
		if len(preset.Permissions) != len(selected) {
			continue
		}
		ok := true
		for _, permission := range preset.Permissions {
			if !selectedMap[permission] {
				ok = false
				break
			}
		}
		if ok {
			return preset.ID
		}
	}
	return ""
}

func permissionShortName(permission domain.AgentPermission) string {
	parts := strings.Split(string(permission), ".")
	if len(parts) == 2 {
		return parts[0] + ": " + parts[1]
	}
	return string(permission)
}

func renderTagFilterLinks(agents []domain.Agent) string {
	tags := map[string]string{}
	for _, agent := range agents {
		for _, tag := range agent.Tags {
			tags[strings.ToLower(tag)] = tag
		}
	}
	if len(tags) == 0 {
		return `<span class="muted">Sem tags</span>`
	}
	var b strings.Builder
	b.WriteString(`<a href="/agents">Todas</a>`)
	for _, tag := range tags {
		b.WriteString(fmt.Sprintf(`<a href="/agents?tag=%s">%s</a>`, esc(tag), esc(tag)))
	}
	return b.String()
}

func renderTagChips(tags []string) string {
	if len(tags) == 0 {
		return `<span class="muted">Sem tags</span>`
	}
	var b strings.Builder
	for _, tag := range tags {
		b.WriteString(fmt.Sprintf(`<a class="chip" href="/agents?tag=%s">%s</a>`, esc(tag), esc(tag)))
	}
	return b.String()
}

func agentDisplayTags(agent domain.Agent, projects []domain.Workspace) []string {
	tags := append([]string{}, agent.Tags...)
	if agent.ProjectID != "" {
		for _, project := range projects {
			if project.ID == agent.ProjectID {
				tags = append(tags, "Proj:"+project.Name)
				break
			}
		}
	}
	return tags
}

func availableAgentTagsJSON(agents []domain.Agent, projects []domain.Workspace) string {
	seen := map[string]bool{}
	tags := make([]string, 0)
	for _, agent := range agents {
		for _, tag := range agent.Tags {
			tag = strings.TrimSpace(tag)
			if tag != "" && !seen[strings.ToLower(tag)] {
				seen[strings.ToLower(tag)] = true
				tags = append(tags, tag)
			}
		}
	}
	for _, project := range projects {
		tag := "Proj:" + project.Name
		if !seen[strings.ToLower(tag)] {
			seen[strings.ToLower(tag)] = true
			tags = append(tags, tag)
		}
	}
	var b strings.Builder
	b.WriteString("[")
	for i, tag := range tags {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(fmt.Sprintf("%q", tag))
	}
	b.WriteString("]")
	return b.String()
}

func renderProjectTagSuggestions(w io.Writer, projects []domain.Workspace) error {
	if len(projects) == 0 {
		return nil
	}
	if _, err := io.WriteString(w, `<small>Tags sugeridas: </small>`); err != nil {
		return err
	}
	for _, project := range projects {
		if _, err := fmt.Fprintf(w, `<span class="chip">Proj:%s</span>`, esc(project.Name)); err != nil {
			return err
		}
	}
	return nil
}

func renderNewAgentSkills(w io.Writer, skills []domain.Skill) error {
	for _, skill := range skills {
		if skill.Permission != "" {
			continue
		}
		if _, err := fmt.Fprintf(w, `<label class="check-row"><input type="checkbox" name="skill_ids" value="%s"><span><strong>%s</strong><small>%s</small></span></label>`, esc(skill.ID), esc(skill.Name), esc(skill.Description)); err != nil {
			return err
		}
	}
	return nil
}

func renderAgentSkillForm(w io.Writer, agentID string, skills []domain.Skill, selected []string) error {
	selectedMap := map[string]bool{}
	for _, id := range selected {
		selectedMap[id] = true
	}
	if _, err := fmt.Fprintf(w, `<form hx-post="/agents/%s/skills" hx-target="body" hx-swap="outerHTML" class="stacked permission-list">`, esc(agentID)); err != nil {
		return err
	}
	for _, skill := range skills {
		if skill.Permission != "" {
			continue
		}
		checked := ""
		if selectedMap[skill.ID] {
			checked = " checked"
		}
		if _, err := fmt.Fprintf(w, `<label class="check-row"><input type="checkbox" name="skill_ids" value="%s"%s><span><strong>%s</strong><small>%s</small></span></label>`, esc(skill.ID), checked, esc(skill.Name), esc(skill.Description)); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, `<button type="submit">Salvar skills</button></form>`)
	return err
}

func renderSkillAgentForm(w io.Writer, skill domain.Skill, agents []domain.Agent) error {
	selected := map[string]bool{}
	for _, id := range skill.AgentIDs {
		selected[id] = true
	}
	if _, err := fmt.Fprintf(w, `<form hx-post="/skills/%s/agents" data-success="Agentes da skill atualizados." hx-target="body" hx-swap="outerHTML" class="stacked permission-list">`, esc(skill.ID)); err != nil {
		return err
	}
	if len(agents) == 0 {
		if _, err := io.WriteString(w, `<p class="empty">Nenhum agente criado.</p>`); err != nil {
			return err
		}
	}
	for _, agent := range agents {
		checked := ""
		if selected[agent.ID] {
			checked = " checked"
		}
		inherited := ""
		disabled := ""
		if agentHasSkill(agent, skill.ID) {
			checked = " checked"
			inherited = " 🔒 Atribuída no agente"
			disabled = " disabled"
		} else if skillAllowedForAgent(skill, agent) && !selected[agent.ID] {
			checked = " checked"
			inherited = " 🔒 Herdada por tipo/permissão"
			disabled = " disabled"
		}
		if _, err := fmt.Fprintf(w, `<label class="check-row"><input type="checkbox" name="agent_ids" value="%s"%s%s><span><strong>%s</strong><small>%s · %d permissões%s</small></span></label>`, esc(agent.ID), checked, disabled, esc(agent.Name), esc(string(agent.Type)), len(agent.Permissions), inherited); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, `<button type="submit">Salvar agentes</button></form></div>`)
	return err
}

func agentHasSkill(agent domain.Agent, skillID string) bool {
	for _, id := range agent.SkillIDs {
		if id == skillID {
			return true
		}
	}
	return false
}

func skillAllowedForAgent(skill domain.Skill, agent domain.Agent) bool {
	if len(skill.AgentIDs) > 0 && skillHasAgent(skill, agent.ID) {
		return true
	}
	if len(skill.AllowedAgentTypes) > 0 {
		for _, agentType := range skill.AllowedAgentTypes {
			if agentType == agent.Type {
				return true
			}
		}
	}
	if len(skill.AllowedPermissions) > 0 {
		for _, required := range skill.AllowedPermissions {
			for _, permission := range agent.Permissions {
				if required == permission {
					return true
				}
			}
		}
	}
	if skill.Permission != "" {
		for _, permission := range agent.Permissions {
			if skill.Permission == permission {
				return true
			}
		}
	}
	return false
}

func projectSelectHTML(projects []domain.Workspace, emptyLabel, selected string) string {
	var b strings.Builder
	b.WriteString(`<select name="project_id"><option value="">` + esc(emptyLabel) + `</option>`)
	for _, project := range projects {
		selectedAttr := ""
		if project.ID == selected {
			selectedAttr = " selected"
		}
		b.WriteString(fmt.Sprintf(`<option value="%s"%s>%s</option>`, esc(project.ID), selectedAttr, esc(project.Name)))
	}
	b.WriteString(`</select>`)
	return b.String()
}

func agentSelectHTML(agents []domain.Agent, selected string) string {
	var b strings.Builder
	renderAgentPicker(&b, agents, "agent_id", selected, "Buscar agente...")
	return b.String()
}

func countTasks(tasks []taskResponse) (pending, running, succeeded, failed int) {
	for _, task := range tasks {
		switch task.Status {
		case domain.TaskStatusBacklog, domain.TaskStatusTodo:
			pending++
		case domain.TaskStatusInProgress, domain.TaskStatusInReview:
			running++
		case domain.TaskStatusDone:
			succeeded++
		case domain.TaskStatusBlocked, domain.TaskStatusCancelled:
			failed++
		}
	}
	return pending, running, succeeded, failed
}

func navLink(id, active, href, label string) string {
	class := ""
	if id == active {
		class = ` class="active"`
	}
	return fmt.Sprintf(`<a%s href="%s">%s</a>`, class, href, label)
}

func agentCapabilityName(capability domain.AgentCapability) string {
	for _, preset := range services.CapabilityPresets() {
		if preset.ID == capability {
			return preset.Name
		}
	}
	return "Worker"
}

func permissionsText(permissions []domain.AgentPermission) string {
	values := make([]string, 0, len(permissions))
	for _, permission := range permissions {
		values = append(values, string(permission))
	}
	return strings.Join(values, ", ")
}

func taskStatusOptions(selected domain.TaskStatus) string {
	statuses := []domain.TaskStatus{domain.TaskStatusBacklog, domain.TaskStatusTodo, domain.TaskStatusInProgress, domain.TaskStatusInReview, domain.TaskStatusBlocked, domain.TaskStatusDone, domain.TaskStatusCancelled}
	var b strings.Builder
	for _, status := range statuses {
		selectedAttr := ""
		if status == selected {
			selectedAttr = " selected"
		}
		b.WriteString(fmt.Sprintf(`<option value="%s"%s>%s</option>`, esc(string(status)), selectedAttr, esc(string(status))))
	}
	return b.String()
}

func statusClass(status domain.TaskStatus) string {
	return "status-" + strings.ReplaceAll(string(status), "_", "-")
}

func runStatusClass(status domain.RunStatus) string {
	switch status {
	case domain.RunStatusRunning:
		return "status-in-progress"
	case domain.RunStatusSucceeded:
		return "status-done"
	case domain.RunStatusFailed, domain.RunStatusTimeout, domain.RunStatusCanceled:
		return "status-blocked"
	default:
		return "status-todo"
	}
}

func taskStatusTerminal(status domain.TaskStatus) bool {
	return status == domain.TaskStatusDone || status == domain.TaskStatusCancelled
}

func esc(value string) string {
	return html.EscapeString(value)
}

func mapToLines(values map[string]string) string {
	lines := make([]string, 0, len(values))
	for key, value := range values {
		lines = append(lines, key+"="+value)
	}
	return strings.Join(lines, "\n")
}

func projectName(projects []domain.Workspace, id string) string {
	if id == "" {
		return "Global"
	}
	for _, project := range projects {
		if project.ID == id {
			return project.Name
		}
	}
	return id
}

func agentName(agents []domain.Agent, id string) string {
	for _, agent := range agents {
		if agent.ID == id {
			return agent.Name
		}
	}
	return id
}

func countAgentsForProject(agents []domain.Agent, projectID string) int {
	count := 0
	for _, agent := range agents {
		if agent.ProjectID == projectID {
			count++
		}
	}
	return count
}

func countTasksForProject(tasks []taskResponse, projectID string) int {
	count := 0
	for _, task := range tasks {
		if task.ProjectID == projectID {
			count++
		}
	}
	return count
}

func countSkillsForProject(skills []domain.Skill, projectID string) int {
	count := 0
	for _, skill := range skills {
		if skill.ProjectID == projectID {
			count++
		}
	}
	return count
}

func countTasksForAgent(tasks []taskResponse, agentID string) int {
	count := 0
	for _, task := range tasks {
		if task.AgentID == agentID {
			count++
		}
	}
	return count
}

func countAgentsUsingSkill(agents []domain.Agent, skillID string) int {
	count := 0
	for _, agent := range agents {
		for _, id := range agent.SkillIDs {
			if id == skillID {
				count++
			}
		}
	}
	return count
}

func countSkillAgents(skill domain.Skill, agents []domain.Agent) int {
	count := 0
	for _, agent := range agents {
		if agentHasSkill(agent, skill.ID) || skillHasAgent(skill, agent.ID) {
			count++
		}
	}
	return count
}

func skillHasAgent(skill domain.Skill, agentID string) bool {
	for _, id := range skill.AgentIDs {
		if id == agentID {
			return true
		}
	}
	return false
}

func limitTasks(tasks []taskResponse, limit int) []taskResponse {
	if len(tasks) <= limit {
		return tasks
	}
	return tasks[len(tasks)-limit:]
}

func limitEvents(events []domain.Event, limit int) []domain.Event {
	if len(events) <= limit {
		return events
	}
	return events[:limit]
}
