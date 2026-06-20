package domain

import "time"

type SkillKind string

const (
	SkillKindBuiltin SkillKind = "builtin"
	SkillKindCustom  SkillKind = "custom"
)

type SkillFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type Skill struct {
	ID                  string            `json:"id"`
	ProjectID           string            `json:"project_id,omitempty"`
	Name                string            `json:"name"`
	Slug                string            `json:"slug,omitempty"`
	Description         string            `json:"description"`
	License             string            `json:"license,omitempty"`
	Compatibility       string            `json:"compatibility,omitempty"`
	AllowedTools        string            `json:"allowed_tools,omitempty"`
	Metadata            map[string]string `json:"metadata,omitempty"`
	Instructions        string            `json:"instructions"`
	DefaultInstructions string            `json:"default_instructions,omitempty"`
	Files               []SkillFile       `json:"files,omitempty"`
	DefaultFiles        []SkillFile       `json:"default_files,omitempty"`
	Kind                SkillKind         `json:"kind"`
	BuiltinKey          string            `json:"builtin_key,omitempty"`
	Permission          AgentPermission   `json:"permission,omitempty"`
	AgentIDs            []string          `json:"agent_ids,omitempty"`
	AllowedAgentTypes   []AgentType       `json:"allowed_agent_types,omitempty"`
	AllowedPermissions  []AgentPermission `json:"allowed_permissions,omitempty"`
	Source              string            `json:"source,omitempty"`
	Version             string            `json:"version,omitempty"`
	Enabled             bool              `json:"enabled"`
	IsModified          bool              `json:"is_modified"`
	CreatedAt           time.Time         `json:"created_at"`
	UpdatedAt           time.Time         `json:"updated_at"`
}
