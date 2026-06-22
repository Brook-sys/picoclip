package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"picoclip/internal/core/domain"
)

type AgentRepository struct {
	db *sql.DB
}

func (r *AgentRepository) Create(ctx context.Context, agent domain.Agent) error {
	tagsJSON, _ := json.Marshal(agent.Tags)
	permissionsJSON, _ := json.Marshal(agent.Permissions)
	skillIDsJSON, _ := json.Marshal(agent.SkillIDs)
	configJSON, _ := json.Marshal(agent.Config)
	envJSON, _ := json.Marshal(agent.Env)
	extraArgsJSON, _ := json.Marshal(agent.ExtraArgs)

	query := `
		INSERT INTO agents (
			id, project_id, name, title, reports_to_id, tags, type, description,
			system_prompt, instruction_file, enabled, capability, permissions,
			skill_ids, config, env, extra_args, input_tokens, output_tokens, total_tokens, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := r.db.ExecContext(ctx, query,
		agent.ID, agent.ProjectID, agent.Name, agent.Title, agent.ReportsToID, string(tagsJSON), string(agent.Type), agent.Description,
		agent.SystemPrompt, agent.InstructionFile, agent.Enabled, string(agent.Capability), string(permissionsJSON),
		string(skillIDsJSON), string(configJSON), string(envJSON), string(extraArgsJSON), agent.InputTokens, agent.OutputTokens, agent.TotalTokens, agent.CreatedAt, agent.UpdatedAt,
	)
	return err
}

func (r *AgentRepository) Get(ctx context.Context, id string) (domain.Agent, error) {
	query := `
		SELECT id, project_id, name, title, reports_to_id, tags, type, description,
			system_prompt, instruction_file, enabled, capability, permissions,
			skill_ids, config, env, extra_args, input_tokens, output_tokens, total_tokens, created_at, updated_at
		FROM agents WHERE id = ?
	`

	row := r.db.QueryRowContext(ctx, query, id)
	return scanAgent(row)
}

func (r *AgentRepository) List(ctx context.Context) ([]domain.Agent, error) {
	query := `
		SELECT id, project_id, name, title, reports_to_id, tags, type, description,
			system_prompt, instruction_file, enabled, capability, permissions,
			skill_ids, config, env, extra_args, input_tokens, output_tokens, total_tokens, created_at, updated_at
		FROM agents ORDER BY created_at ASC
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []domain.Agent
	for rows.Next() {
		agent, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, agent)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return agents, nil
}

func (r *AgentRepository) Update(ctx context.Context, agent domain.Agent) error {
	tagsJSON, _ := json.Marshal(agent.Tags)
	permissionsJSON, _ := json.Marshal(agent.Permissions)
	skillIDsJSON, _ := json.Marshal(agent.SkillIDs)
	configJSON, _ := json.Marshal(agent.Config)
	envJSON, _ := json.Marshal(agent.Env)
	extraArgsJSON, _ := json.Marshal(agent.ExtraArgs)

	query := `
		UPDATE agents SET
			project_id = ?, name = ?, title = ?, reports_to_id = ?, tags = ?, type = ?, description = ?,
			system_prompt = ?, instruction_file = ?, enabled = ?, capability = ?, permissions = ?,
			skill_ids = ?, config = ?, env = ?, extra_args = ?, input_tokens = ?, output_tokens = ?, total_tokens = ?, updated_at = ?
		WHERE id = ?
	`

	res, err := r.db.ExecContext(ctx, query,
		agent.ProjectID, agent.Name, agent.Title, agent.ReportsToID, string(tagsJSON), string(agent.Type), agent.Description,
		agent.SystemPrompt, agent.InstructionFile, agent.Enabled, string(agent.Capability), string(permissionsJSON),
		string(skillIDsJSON), string(configJSON), string(envJSON), string(extraArgsJSON), agent.InputTokens, agent.OutputTokens, agent.TotalTokens, agent.UpdatedAt, agent.ID,
	)
	if err != nil {
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return domain.ErrNotFound
	}

	return nil
}

func (r *AgentRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM agents WHERE id = ?`

	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return domain.ErrNotFound
	}

	return nil
}

func scanAgent(row scanner) (domain.Agent, error) {
	var a domain.Agent
	var tagsJSON, permissionsJSON, skillIDsJSON, configJSON, envJSON, extraArgsJSON, typeStr, capabilityStr string

	err := row.Scan(
		&a.ID, &a.ProjectID, &a.Name, &a.Title, &a.ReportsToID, &tagsJSON, &typeStr, &a.Description,
		&a.SystemPrompt, &a.InstructionFile, &a.Enabled, &capabilityStr, &permissionsJSON,
		&skillIDsJSON, &configJSON, &envJSON, &extraArgsJSON, &a.InputTokens, &a.OutputTokens, &a.TotalTokens, &a.CreatedAt, &a.UpdatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return domain.Agent{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Agent{}, err
	}

	a.Type = domain.AgentType(typeStr)
	a.Capability = domain.AgentCapability(capabilityStr)

	if tagsJSON != "" && tagsJSON != "null" {
		_ = json.Unmarshal([]byte(tagsJSON), &a.Tags)
	}
	if permissionsJSON != "" && permissionsJSON != "null" {
		_ = json.Unmarshal([]byte(permissionsJSON), &a.Permissions)
	}
	if skillIDsJSON != "" && skillIDsJSON != "null" {
		_ = json.Unmarshal([]byte(skillIDsJSON), &a.SkillIDs)
	}
	if configJSON != "" && configJSON != "null" {
		_ = json.Unmarshal([]byte(configJSON), &a.Config)
	}
	if envJSON != "" && envJSON != "null" {
		_ = json.Unmarshal([]byte(envJSON), &a.Env)
	}
	if extraArgsJSON != "" && extraArgsJSON != "null" {
		_ = json.Unmarshal([]byte(extraArgsJSON), &a.ExtraArgs)
	}

	return a, nil
}
