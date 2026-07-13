package services

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

const maxRemoteSkillYAMLBytes = 1 << 20

type remoteSkillLookupFunc func(context.Context, string, string) ([]netip.Addr, error)
type remoteSkillDialFunc func(context.Context, string, string) (net.Conn, error)

func newRemoteSkillHTTPClient(lookup remoteSkillLookupFunc, dial remoteSkillDialFunc) *http.Client {
	return &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			Proxy: nil,
			DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				return dialPublicRemoteSkillHostWith(ctx, network, address, lookup, dial)
			},
		},
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("remote skill fetch exceeded redirect limit")
			}
			return validateRemoteSkillURL(request.URL)
		},
	}
}

var remoteSkillHTTPClient = newRemoteSkillHTTPClient(
	net.DefaultResolver.LookupNetIP,
	(&net.Dialer{}).DialContext,
)

type remoteSkillYAML struct {
	Name         string             `yaml:"name"`
	Description  string             `yaml:"description"`
	Instructions string             `yaml:"instructions"`
	Files        []domain.SkillFile `yaml:"files"`
	Version      string             `yaml:"version"`
}

type SkillService struct {
	storage ports.Storage
	clock   ports.Clock
	idGen   ports.IDGenerator
}

func NewSkillService(storage ports.Storage, clock ports.Clock, idGen ports.IDGenerator) *SkillService {
	return &SkillService{storage: storage, clock: clock, idGen: idGen}
}

func (s *SkillService) InstallBuiltins(ctx context.Context) error {
	existing, err := s.storage.Skills().List(ctx, "")
	if err != nil {
		return err
	}
	existingByKey := make(map[string]domain.Skill, len(existing))
	for _, skill := range existing {
		if skill.Kind == domain.SkillKindBuiltin && skill.BuiltinKey != "" {
			existingByKey[skill.BuiltinKey] = skill
		}
	}

	for _, builtin := range builtinSkills() {
		if current, ok := existingByKey[builtin.slug]; ok {
			current.Name = builtin.name
			current.Slug = builtin.slug
			current.Description = builtin.description
			current.DefaultInstructions = builtin.instructions
			current.DefaultFiles = builtin.files
			current.Permission = builtin.permission
			current.Metadata = builtin.metadata
			current.Source = "builtin"
			current.Version = "1.0.0"
			current.UpdatedAt = s.clock.Now()
			if !current.IsModified {
				current.Instructions = builtin.instructions
				current.Files = builtin.files
			}
			if err := s.storage.Skills().Update(ctx, current); err != nil {
				return err
			}
			continue
		}

		now := s.clock.Now()
		skill := domain.Skill{
			ID:                  s.idGen.NewID("sk"),
			Name:                builtin.name,
			Slug:                builtin.slug,
			Description:         builtin.description,
			Instructions:        builtin.instructions,
			DefaultInstructions: builtin.instructions,
			Files:               builtin.files,
			DefaultFiles:        builtin.files,
			Kind:                domain.SkillKindBuiltin,
			BuiltinKey:          builtin.slug,
			Permission:          builtin.permission,
			Metadata:            builtin.metadata,
			Source:              "builtin",
			Version:             "1.0.0",
			Enabled:             true,
			CreatedAt:           now,
			UpdatedAt:           now,
		}
		if err := s.storage.Skills().Create(ctx, skill); err != nil {
			return err
		}
	}
	return nil
}

func (s *SkillService) Create(ctx context.Context, projectID, name, description, instructions string) (domain.Skill, error) {
	return s.CreateWithFiles(ctx, projectID, name, description, instructions, nil)
}

func (s *SkillService) CreateWithFiles(ctx context.Context, projectID, name, description, instructions string, files []domain.SkillFile) (domain.Skill, error) {
	if name == "" || instructions == "" {
		return domain.Skill{}, fmt.Errorf("%w: name and instructions are required", domain.ErrInvalidInput)
	}
	slug := skillSlug(name)
	if err := validateSkillSlug(slug); err != nil {
		return domain.Skill{}, err
	}
	now := s.clock.Now()
	skill := domain.Skill{ID: s.idGen.NewID("sk"), ProjectID: projectID, Name: name, Slug: slug, Description: description, Instructions: instructions, Files: files, Kind: domain.SkillKindCustom, Source: "local", Enabled: true, CreatedAt: now, UpdatedAt: now}
	if err := s.storage.Skills().Create(ctx, skill); err != nil {
		return domain.Skill{}, err
	}
	return skill, nil
}

func (s *SkillService) ImportRemoteYAML(ctx context.Context, projectID, sourceURL string) (domain.Skill, error) {
	parsedURL, err := url.ParseRequestURI(sourceURL)
	if err != nil || validateRemoteSkillURL(parsedURL) != nil {
		return domain.Skill{}, fmt.Errorf("%w: source_url must be an absolute http or https URL without credentials and use port 80 or 443", domain.ErrInvalidInput)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return domain.Skill{}, fmt.Errorf("%w: invalid source_url", domain.ErrInvalidInput)
	}
	response, err := remoteSkillHTTPClient.Do(request)
	if err != nil {
		return domain.Skill{}, fmt.Errorf("%w: fetch remote skill YAML: %v", domain.ErrInvalidInput, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return domain.Skill{}, fmt.Errorf("%w: fetch remote skill YAML: unexpected HTTP status %d", domain.ErrInvalidInput, response.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxRemoteSkillYAMLBytes+1))
	if err != nil {
		return domain.Skill{}, fmt.Errorf("read remote skill YAML: %w", err)
	}
	if len(body) > maxRemoteSkillYAMLBytes {
		return domain.Skill{}, fmt.Errorf("%w: remote skill YAML exceeds %d bytes", domain.ErrInvalidInput, maxRemoteSkillYAMLBytes)
	}

	var document remoteSkillYAML
	decoder := yaml.NewDecoder(bytes.NewReader(body))
	decoder.KnownFields(true)
	if err := decoder.Decode(&document); err != nil {
		return domain.Skill{}, fmt.Errorf("%w: invalid remote skill YAML: %v", domain.ErrInvalidInput, err)
	}

	if strings.TrimSpace(document.Name) == "" || strings.TrimSpace(document.Instructions) == "" {
		return domain.Skill{}, fmt.Errorf("%w: name and instructions are required", domain.ErrInvalidInput)
	}
	for _, file := range document.Files {
		if strings.TrimSpace(file.Path) == "" {
			return domain.Skill{}, fmt.Errorf("%w: skill file path is required", domain.ErrInvalidInput)
		}
	}

	slug := skillSlug(document.Name)
	if err := validateSkillSlug(slug); err != nil {
		return domain.Skill{}, err
	}

	skill := domain.Skill{
		ID:           s.idGen.NewID("sk"),
		ProjectID:    projectID,
		Name:         document.Name,
		Slug:         slug,
		Description:  document.Description,
		Instructions: document.Instructions,
		Files:        document.Files,
		Kind:         domain.SkillKindCustom,
		Enabled:      true,
		Source:       sourceURL,
		Version:      document.Version,
		CreatedAt:    s.clock.Now(),
		UpdatedAt:    s.clock.Now(),
	}

	if err := s.storage.Skills().Create(ctx, skill); err != nil {
		return domain.Skill{}, err
	}
	return skill, nil
}

func validateRemoteSkillURL(remoteURL *url.URL) error {
	if remoteURL == nil || (remoteURL.Scheme != "http" && remoteURL.Scheme != "https") || remoteURL.Host == "" || remoteURL.User != nil {
		return fmt.Errorf("remote skill URL must be an absolute http or https URL without credentials")
	}
	if port := remoteURL.Port(); port != "" && !((remoteURL.Scheme == "http" && port == "80") || (remoteURL.Scheme == "https" && port == "443")) {
		return fmt.Errorf("remote skill URL must use the default port for its scheme")
	}
	return nil
}

func dialPublicRemoteSkillHostWith(ctx context.Context, network, address string, lookup remoteSkillLookupFunc, dial remoteSkillDialFunc) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	addresses, err := lookup(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	for _, resolved := range addresses {
		if !isPublicRemoteSkillIP(resolved) {
			return nil, fmt.Errorf("%w: remote skill URL resolves to a non-public address", domain.ErrInvalidInput)
		}
		connection, err := dial(ctx, network, net.JoinHostPort(resolved.String(), port))
		if err == nil {
			return connection, nil
		}
	}
	return nil, fmt.Errorf("connect to remote skill URL: no public address accepted the connection")
}

var blockedRemoteSkillPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001:db8::/32"),
}

func isPublicRemoteSkillIP(address netip.Addr) bool {
	address = address.Unmap()
	if !address.IsValid() || !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsMulticast() || address.IsUnspecified() {
		return false
	}
	for _, blocked := range blockedRemoteSkillPrefixes {
		if blocked.Contains(address) {
			return false
		}
	}
	return true
}

func (s *SkillService) List(ctx context.Context, projectID string) ([]domain.Skill, error) {
	return s.storage.Skills().List(ctx, projectID)
}

func (s *SkillService) Get(ctx context.Context, id string) (domain.Skill, error) {
	return s.storage.Skills().Get(ctx, id)
}

func (s *SkillService) Update(ctx context.Context, id, name, description, instructions string, enabled bool) (domain.Skill, error) {
	skill, err := s.storage.Skills().Get(ctx, id)
	if err != nil {
		return domain.Skill{}, err
	}
	if name == "" || instructions == "" {
		return domain.Skill{}, fmt.Errorf("%w: name and instructions are required", domain.ErrInvalidInput)
	}
	skill.Name = name
	skill.Description = description
	skill.Instructions = instructions
	skill.Enabled = enabled
	skill.IsModified = skill.Kind == domain.SkillKindBuiltin && (skill.Instructions != skill.DefaultInstructions || !sameSkillFiles(skill.Files, skill.DefaultFiles))
	skill.UpdatedAt = s.clock.Now()
	if err := s.storage.Skills().Update(ctx, skill); err != nil {
		return domain.Skill{}, err
	}
	return skill, nil
}

func (s *SkillService) UpdateAssignments(ctx context.Context, id string, agentIDs []string, allowedTypes []domain.AgentType, allowedPermissions []domain.AgentPermission) (domain.Skill, error) {
	skill, err := s.storage.Skills().Get(ctx, id)
	if err != nil {
		return domain.Skill{}, err
	}
	skill.AgentIDs = agentIDs
	skill.AllowedAgentTypes = allowedTypes
	skill.AllowedPermissions = allowedPermissions
	skill.UpdatedAt = s.clock.Now()
	if err := s.storage.Skills().Update(ctx, skill); err != nil {
		return domain.Skill{}, err
	}
	return skill, nil
}

func (s *SkillService) ResetBuiltin(ctx context.Context, id string) (domain.Skill, error) {
	skill, err := s.storage.Skills().Get(ctx, id)
	if err != nil {
		return domain.Skill{}, err
	}
	if skill.Kind != domain.SkillKindBuiltin {
		return domain.Skill{}, fmt.Errorf("%w: only builtin skills can be reset", domain.ErrInvalidInput)
	}
	skill.Instructions = skill.DefaultInstructions
	skill.Files = skill.DefaultFiles
	skill.IsModified = false
	skill.UpdatedAt = s.clock.Now()
	if err := s.storage.Skills().Update(ctx, skill); err != nil {
		return domain.Skill{}, err
	}
	return skill, nil
}

func (s *SkillService) Delete(ctx context.Context, id string) error {
	skill, err := s.storage.Skills().Get(ctx, id)
	if err != nil {
		return err
	}
	if skill.Kind == domain.SkillKindBuiltin {
		return fmt.Errorf("%w: builtin skills cannot be deleted", domain.ErrInvalidInput)
	}
	return s.storage.Skills().Delete(ctx, id)
}

type builtinSkillDefinition struct {
	name         string
	slug         string
	description  string
	instructions string
	permission   domain.AgentPermission
	files        []domain.SkillFile
	metadata     map[string]string
}

func builtinSkills() []builtinSkillDefinition {
	base := []builtinSkillDefinition{
		permissionSkill(domain.PermissionSystemRead, "system-read", "System read", "Allows an agent to inspect PicoClip system state and API documentation.", "Use GET /agent-api/docs to discover available endpoints. Use read-only endpoints to understand projects, agents, tasks, skills and current system state before acting."),
		permissionSkill(domain.PermissionProjectsRead, "projects-read", "Projects read", "Allows an agent to list and inspect projects/workspaces.", "Use GET /agent-api/projects to inspect workspaces. Prefer project-scoped context when creating or delegating work."),
		permissionSkill(domain.PermissionProjectsWrite, "projects-write", "Projects write", "Allows an agent to create or update projects/workspaces.", "Create or update projects only when the requested work requires a separate workspace context. Include a concise name and useful description."),
		permissionSkill(domain.PermissionAgentsRead, "agents-read", "Agents read", "Allows an agent to inspect available agents and their responsibilities.", "Use GET /agent-api/agents before delegation so tasks are assigned to the most appropriate agent."),
		permissionSkill(domain.PermissionAgentsCreate, "agents-create", "Agents create", "Allows an agent to create new agents when work requires a new role.", "Create agents only when existing agents are insufficient. Give the new agent explicit permissions and a narrow responsibility."),
		permissionSkill(domain.PermissionAgentsUpdate, "agents-update", "Agents update", "Allows an agent to update agent permissions, skills and configuration.", "Update agents conservatively. Prefer adding the minimum permission or skill needed for the current workflow."),
		permissionSkill(domain.PermissionAgentsDelete, "agents-delete", "Agents delete", "Allows an agent to delete agents.", "Delete agents only when explicitly requested or clearly obsolete. Avoid deleting agents that own running tasks."),
		permissionSkill(domain.PermissionTasksRead, "tasks-read", "Tasks read", "Allows an agent to inspect tasks, messages, runs and events.", "Use GET /agent-api/tasks and task detail endpoints to understand assignments, dependencies and current status."),
		permissionSkill(domain.PermissionTasksCreate, "tasks-create", "Tasks create", "Allows an agent to create new tasks.", "Create tasks with clear prompt, project_id when applicable, assignee agent_id and acceptance criteria."),
		permissionSkill(domain.PermissionTasksUpdate, "tasks-update", "Tasks update", "Allows an agent to add messages and update task state.", "Use task messages for incremental context, questions, decisions and status notes. Keep messages concise and actionable."),
		permissionSkill(domain.PermissionTasksDelegate, "tasks-delegate", "Tasks delegate", "Allows an agent to delegate subtasks to other agents.", "Delegate when work should be split or another agent is better suited. Use POST /agent-api/tasks/{id}/delegate with to_agent_id and a prompt containing context, expected output and acceptance criteria."),
		permissionSkill(domain.PermissionTasksCancel, "tasks-cancel", "Tasks cancel", "Allows an agent to cancel problematic tasks.", "Cancel only when a task is obsolete, unsafe, duplicate or stuck. Include a clear cancellation reason."),
		permissionSkill(domain.PermissionTasksRun, "tasks-run", "Tasks run", "Allows an agent to invoke or run assigned work.", "When running work, inspect task context, execute the minimal necessary action, and report final output or failure reason."),
		permissionSkill(domain.PermissionSkillsRead, "skills-read", "Skills read", "Allows an agent to inspect available skills.", "Use skills metadata to decide whether extra instructions apply before performing specialized work."),
		permissionSkill(domain.PermissionSkillsCreate, "skills-create", "Skills create", "Allows an agent to create Agent Skills compatible skills.", "Create skills as directories with SKILL.md frontmatter and optional scripts, references and assets. Follow the Agent Skills specification."),
		permissionSkill(domain.PermissionSkillsUpdate, "skills-update", "Skills update", "Allows an agent to update skills and skill assignments.", "Update skills carefully. Keep SKILL.md concise and move detailed material to references, scripts or assets."),
		permissionSkill(domain.PermissionSkillsDelete, "skills-delete", "Skills delete", "Allows an agent to delete custom skills.", "Delete only custom skills and only when no agent depends on them."),
		permissionSkill(domain.PermissionSettingsRead, "settings-read", "Settings read", "Allows an agent to inspect prompts, environment and adapter settings.", "Read settings to understand runtime constraints before suggesting configuration changes."),
		permissionSkill(domain.PermissionSettingsWrite, "settings-write", "Settings write", "Allows an agent to update prompts, environment and adapter settings.", "Change settings only with explicit intent. Prefer scoped settings over global changes."),
		permissionSkill(domain.PermissionAdaptersRead, "adapters-read", "Adapters read", "Allows an agent to inspect adapter configuration.", "Read adapter settings before diagnosing driver/runtime failures."),
		permissionSkill(domain.PermissionAdaptersWrite, "adapters-write", "Adapters write", "Allows an agent to change adapter configuration.", "Update adapter settings such as binary path, default args, cwd strategy, timeout and environment carefully."),
	}
	return base
}

func permissionSkill(permission domain.AgentPermission, slug, name, description, body string) builtinSkillDefinition {
	instructions := fmt.Sprintf("---\nname: %s\ndescription: %s\nmetadata:\n  picoclip.permission: %s\n  picoclip.builtin: \"true\"\n---\n\n%s", slug, description, permission, body)
	return builtinSkillDefinition{name: name, slug: slug, description: description, instructions: instructions, permission: permission, metadata: map[string]string{"picoclip.permission": string(permission), "picoclip.builtin": "true"}}
}

func skillSlug(name string) string {
	value := strings.ToLower(strings.TrimSpace(name))
	value = strings.ReplaceAll(value, " ", "-")
	value = regexp.MustCompile(`[^a-z0-9-]+`).ReplaceAllString(value, "-")
	value = regexp.MustCompile(`-+`).ReplaceAllString(value, "-")
	return strings.Trim(value, "-")
}

func validateSkillSlug(slug string) error {
	if slug == "" || len(slug) > 64 || !regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`).MatchString(slug) {
		return fmt.Errorf("%w: skill name must follow Agent Skills kebab-case naming", domain.ErrInvalidInput)
	}
	return nil
}

func sameSkillFiles(a, b []domain.SkillFile) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
