package domain

import "time"

type RuntimeID string

type RuntimeKind string

const (
	RuntimeKindNative   RuntimeKind = "native"
	RuntimeKindExternal RuntimeKind = "external"
	RuntimeKindSandbox  RuntimeKind = "sandbox"
)

type InstallMode string

const (
	InstallModeExclusive InstallMode = "exclusive"
	InstallModeGlobal    InstallMode = "global"
	InstallModeExisting  InstallMode = "existing"
	InstallModeRemote    InstallMode = "remote"
)

type RuntimeState struct {
	ID             string      `json:"id"`
	RuntimeID      RuntimeID   `json:"runtime_id"`
	Mode           InstallMode `json:"mode"`
	Enabled        bool        `json:"enabled"`
	Version        string      `json:"version"`
	BinPath        string      `json:"bin_path"`
	ConfigPath     string      `json:"config_path"`
	HomePath       string      `json:"home_path"`
	DataPath       string      `json:"data_path"`
	LogsPath       string      `json:"logs_path"`
	Source         string      `json:"source"`
	SourceURL      string      `json:"source_url"`
	Checksum       string      `json:"checksum"`
	InstalledAt    time.Time   `json:"installed_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
	LastHealthAt   *time.Time  `json:"last_health_at,omitempty"`
	LastHealthJSON string      `json:"last_health_json,omitempty"`
	SettingsJSON   string      `json:"settings_json"`
	MetadataJSON   string      `json:"metadata_json"`
}

type RuntimeConfigFile struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Language  string `json:"language"`
	Content   []byte `json:"content"`
	Editable  bool   `json:"editable"`
	Sensitive bool   `json:"sensitive"`
}

type DiagnosticCheck struct {
	Name      string    `json:"name"`
	Status    string    `json:"status"` // ok, error, warning
	Message   string    `json:"message"`
	CheckedAt time.Time `json:"checked_at"`
}

type RuntimeHealth struct {
	Status    string            `json:"status"`
	Version   string            `json:"version"`
	Warnings  []string          `json:"warnings"`
	Errors    []string          `json:"errors"`
	Checks    []DiagnosticCheck `json:"checks"`
	CheckedAt time.Time         `json:"checked_at"`
}

// RuntimeVersion holds release info from an upstream source like GitHub.
type RuntimeVersion struct {
	Tag        string    `json:"tag"`
	Label      string    `json:"label"`
	Latest     bool      `json:"latest"`
	Nightly    bool      `json:"nightly"`
	Prerelease bool      `json:"prerelease"`
	CreatedAt  time.Time `json:"created_at"`
}
