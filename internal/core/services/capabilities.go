package services

import "picoclip/internal/core/domain"

type PermissionDefinition struct {
	ID          domain.AgentPermission `json:"id"`
	Group       string                 `json:"group"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
}

type CapabilityPreset struct {
	ID          domain.AgentCapability   `json:"id"`
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	Permissions []domain.AgentPermission `json:"permissions"`
}

type PermissionPreset struct {
	ID          string                   `json:"id"`
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	Permissions []domain.AgentPermission `json:"permissions"`
}

func PermissionCatalog() []PermissionDefinition {
	return []PermissionDefinition{
		{domain.PermissionSystemRead, "System", "Read system", "Consultar estado, documentação e capacidades do PicoClip."},
		{domain.PermissionProjectsRead, "Projects", "Read projects", "Listar e consultar workspaces/projetos."},
		{domain.PermissionProjectsWrite, "Projects", "Write projects", "Criar e alterar workspaces/projetos."},
		{domain.PermissionAgentsRead, "Agents", "Read agents", "Listar e consultar agentes."},
		{domain.PermissionAgentsCreate, "Agents", "Create agents", "Criar novos agentes."},
		{domain.PermissionAgentsUpdate, "Agents", "Update agents", "Alterar agentes, permissões, skills e configuração."},
		{domain.PermissionAgentsDelete, "Agents", "Delete agents", "Excluir agentes."},
		{domain.PermissionTasksRead, "Tasks", "Read tasks", "Listar e consultar tasks, runs, mensagens e eventos."},
		{domain.PermissionTasksCreate, "Tasks", "Create tasks", "Criar tasks para agentes."},
		{domain.PermissionTasksUpdate, "Tasks", "Update tasks", "Atualizar status, mensagens e propriedades de tasks."},
		{domain.PermissionTasksDelegate, "Tasks", "Delegate tasks", "Delegar subtarefas para outros agentes."},
		{domain.PermissionTasksCancel, "Tasks", "Cancel tasks", "Cancelar tasks pendentes ou em execução."},
		{domain.PermissionTasksRun, "Tasks", "Run tasks", "Executar ou invocar agentes para trabalhar em tasks."},
		{domain.PermissionSkillsRead, "Skills", "Read skills", "Listar e consultar skills."},
		{domain.PermissionSkillsCreate, "Skills", "Create skills", "Criar ou importar skills."},
		{domain.PermissionSkillsUpdate, "Skills", "Update skills", "Editar skills, políticas e atribuições."},
		{domain.PermissionSkillsDelete, "Skills", "Delete skills", "Excluir skills customizadas."},
		{domain.PermissionSettingsRead, "Settings", "Read settings", "Consultar prompts, env e adapters."},
		{domain.PermissionSettingsWrite, "Settings", "Write settings", "Alterar prompts, env e adapters."},
		{domain.PermissionAdaptersRead, "Adapters", "Read adapters", "Consultar configuração de adapters."},
		{domain.PermissionAdaptersWrite, "Adapters", "Write adapters", "Alterar configuração de adapters."},
	}
}

func PermissionPresets() []PermissionPreset {
	reader := []domain.AgentPermission{domain.PermissionSystemRead, domain.PermissionProjectsRead, domain.PermissionAgentsRead, domain.PermissionTasksRead, domain.PermissionSkillsRead}
	executor := append(copyPermissions(reader), domain.PermissionTasksCreate, domain.PermissionTasksUpdate, domain.PermissionTasksRun)
	coordinator := append(copyPermissions(executor), domain.PermissionTasksDelegate)
	operator := append(copyPermissions(coordinator), domain.PermissionTasksCancel, domain.PermissionAdaptersRead)
	agentManager := append(copyPermissions(coordinator), domain.PermissionAgentsCreate, domain.PermissionAgentsUpdate, domain.PermissionSkillsUpdate)
	return []PermissionPreset{
		{ID: "reader", Name: "Leitor", Description: "Consulta projetos, agentes, tasks e skills sem alterar o sistema.", Permissions: reader},
		{ID: "executor", Name: "Executor", Description: "Pode criar, atualizar e executar tasks atribuídas.", Permissions: executor},
		{ID: "coordinator", Name: "Coordenador", Description: "Divide trabalho e delega subtarefas para outros agentes.", Permissions: coordinator},
		{ID: "operator", Name: "Operador", Description: "Coordena e pode cancelar tasks problemáticas ou obsoletas.", Permissions: operator},
		{ID: "agent-manager", Name: "Gerente de agentes", Description: "Coordena trabalho e gerencia agentes/skills operacionais.", Permissions: agentManager},
		{ID: "administrator", Name: "Administrador", Description: "Controle completo sobre PicoClip.", Permissions: AllPermissions()},
	}
}

func PermissionsForPreset(id string) []domain.AgentPermission {
	for _, preset := range PermissionPresets() {
		if preset.ID == id {
			return copyPermissions(preset.Permissions)
		}
	}
	return PermissionsForPreset("executor")
}

func copyPermissions(permissions []domain.AgentPermission) []domain.AgentPermission {
	out := make([]domain.AgentPermission, len(permissions))
	copy(out, permissions)
	return out
}

func CapabilityPresets() []CapabilityPreset {
	return []CapabilityPreset{
		{ID: domain.CapabilityObserver, Name: "Legacy Observer", Description: "Preset legado mapeado para permissões de leitura.", Permissions: []domain.AgentPermission{domain.PermissionSystemRead, domain.PermissionProjectsRead, domain.PermissionAgentsRead, domain.PermissionTasksRead, domain.PermissionSkillsRead}},
		{ID: domain.CapabilityWorker, Name: "Legacy Worker", Description: "Preset legado para leitura e criação de tasks.", Permissions: []domain.AgentPermission{domain.PermissionSystemRead, domain.PermissionProjectsRead, domain.PermissionAgentsRead, domain.PermissionTasksRead, domain.PermissionTasksCreate, domain.PermissionSkillsRead}},
		{ID: domain.CapabilityCoordinator, Name: "Legacy Coordinator", Description: "Preset legado com delegação.", Permissions: []domain.AgentPermission{domain.PermissionSystemRead, domain.PermissionProjectsRead, domain.PermissionAgentsRead, domain.PermissionTasksRead, domain.PermissionTasksCreate, domain.PermissionTasksDelegate, domain.PermissionSkillsRead}},
		{ID: domain.CapabilityOperator, Name: "Legacy Operator", Description: "Preset legado com cancelamento.", Permissions: []domain.AgentPermission{domain.PermissionSystemRead, domain.PermissionProjectsRead, domain.PermissionAgentsRead, domain.PermissionTasksRead, domain.PermissionTasksCreate, domain.PermissionTasksDelegate, domain.PermissionTasksCancel, domain.PermissionSkillsRead}},
		{ID: domain.CapabilityAdministrator, Name: "Legacy Administrator", Description: "Preset legado com todas as permissões.", Permissions: AllPermissions()},
	}
}

func AllPermissions() []domain.AgentPermission {
	catalog := PermissionCatalog()
	permissions := make([]domain.AgentPermission, 0, len(catalog))
	for _, item := range catalog {
		permissions = append(permissions, item.ID)
	}
	return permissions
}

func PermissionsForCapability(capability domain.AgentCapability) []domain.AgentPermission {
	for _, preset := range CapabilityPresets() {
		if preset.ID == capability {
			permissions := make([]domain.AgentPermission, len(preset.Permissions))
			copy(permissions, preset.Permissions)
			return permissions
		}
	}
	return []domain.AgentPermission{domain.PermissionSystemRead, domain.PermissionProjectsRead, domain.PermissionAgentsRead, domain.PermissionTasksRead, domain.PermissionTasksCreate, domain.PermissionSkillsRead}
}
