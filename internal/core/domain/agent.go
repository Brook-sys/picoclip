package domain

import "time"

type AgentType string

type AgentCapability string

type AgentPermission string

const (
	CapabilityObserver      AgentCapability = "observer"
	CapabilityWorker        AgentCapability = "worker"
	CapabilityCoordinator   AgentCapability = "coordinator"
	CapabilityOperator      AgentCapability = "operator"
	CapabilityAdministrator AgentCapability = "administrator"
)

const (
	PermissionSystemRead    AgentPermission = "system.read"
	PermissionProjectsRead  AgentPermission = "projects.read"
	PermissionProjectsWrite AgentPermission = "projects.write"
	PermissionAgentsRead    AgentPermission = "agents.read"
	PermissionAgentsCreate  AgentPermission = "agents.create"
	PermissionAgentsUpdate  AgentPermission = "agents.update"
	PermissionAgentsDelete  AgentPermission = "agents.delete"
	PermissionTasksRead     AgentPermission = "tasks.read"
	PermissionTasksCreate   AgentPermission = "tasks.create"
	PermissionTasksUpdate   AgentPermission = "tasks.update"
	PermissionTasksDelegate AgentPermission = "tasks.delegate"
	PermissionTasksCancel   AgentPermission = "tasks.cancel"
	PermissionTasksRun      AgentPermission = "tasks.run"
	PermissionSkillsRead    AgentPermission = "skills.read"
	PermissionSkillsCreate  AgentPermission = "skills.create"
	PermissionSkillsUpdate  AgentPermission = "skills.update"
	PermissionSkillsDelete  AgentPermission = "skills.delete"
	PermissionSettingsRead  AgentPermission = "settings.read"
	PermissionSettingsWrite AgentPermission = "settings.write"
	PermissionAdaptersRead  AgentPermission = "adapters.read"
	PermissionAdaptersWrite AgentPermission = "adapters.write"
)

const (
	PermissionCreateAgents = PermissionAgentsCreate
	PermissionDeleteAgents = PermissionAgentsDelete
	PermissionCreateTasks  = PermissionTasksCreate
	PermissionDelegate     = PermissionTasksDelegate
	PermissionCancelTasks  = PermissionTasksCancel
	PermissionManageSkills = PermissionSkillsUpdate
	PermissionViewSystem   = PermissionSystemRead
)

type Agent struct {
	ID              string            `json:"id"`
	ProjectID       string            `json:"project_id,omitempty"`
	Name            string            `json:"name"`
	Title           string            `json:"title,omitempty"`
	ReportsToID     string            `json:"reports_to_id,omitempty"`
	Tags            []string          `json:"tags,omitempty"`
	Type            AgentType         `json:"type"`
	Description     string            `json:"description"`
	SystemPrompt    string            `json:"system_prompt,omitempty"`
	InstructionFile string            `json:"instruction_file,omitempty"`
	Enabled         bool              `json:"enabled"`
	Capability      AgentCapability   `json:"capability,omitempty"`
	Permissions     []AgentPermission `json:"permissions,omitempty"`
	SkillIDs        []string          `json:"skill_ids,omitempty"`
	Config          map[string]string `json:"config,omitempty"`
	Env             map[string]string `json:"env,omitempty"`
	ExtraArgs       []string          `json:"extra_args,omitempty"`
	InputTokens     int               `json:"input_tokens"`
	OutputTokens    int               `json:"output_tokens"`
	TotalTokens     int               `json:"total_tokens"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}
