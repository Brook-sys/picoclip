package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"picoclip/internal/core/domain"
)

type SkillRepository struct {
	db *sql.DB
}

func (r *SkillRepository) Create(ctx context.Context, skill domain.Skill) error {
	metadataJSON, _ := json.Marshal(skill.Metadata)
	filesJSON, _ := json.Marshal(skill.Files)
	defaultFilesJSON, _ := json.Marshal(skill.DefaultFiles)
	agentIDsJSON, _ := json.Marshal(skill.AgentIDs)
	allowedAgentTypesJSON, _ := json.Marshal(skill.AllowedAgentTypes)
	allowedPermissionsJSON, _ := json.Marshal(skill.AllowedPermissions)

	query := `
		INSERT INTO skills (
			id, project_id, name, slug, description, license, compatibility, allowed_tools,
			metadata, instructions, default_instructions, files, default_files, kind, builtin_key,
			permission, agent_ids, allowed_agent_types, allowed_permissions, source, version,
			enabled, is_modified, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.ExecContext(ctx, query,
		skill.ID, skill.ProjectID, skill.Name, skill.Slug, skill.Description, skill.License, skill.Compatibility, skill.AllowedTools,
		string(metadataJSON), skill.Instructions, skill.DefaultInstructions, string(filesJSON), string(defaultFilesJSON), string(skill.Kind), skill.BuiltinKey,
		string(skill.Permission), string(agentIDsJSON), string(allowedAgentTypesJSON), string(allowedPermissionsJSON), skill.Source, skill.Version,
		skill.Enabled, skill.IsModified, skill.CreatedAt, skill.UpdatedAt,
	)
	return err
}

func (r *SkillRepository) Get(ctx context.Context, id string) (domain.Skill, error) {
	query := `
		SELECT id, project_id, name, slug, description, license, compatibility, allowed_tools,
			metadata, instructions, default_instructions, files, default_files, kind, builtin_key,
			permission, agent_ids, allowed_agent_types, allowed_permissions, source, version,
			enabled, is_modified, created_at, updated_at
		FROM skills WHERE id = ?
	`
	row := r.db.QueryRowContext(ctx, query, id)
	return scanSkill(row)
}

func (r *SkillRepository) List(ctx context.Context, projectID string) ([]domain.Skill, error) {
	query := `
		SELECT id, project_id, name, slug, description, license, compatibility, allowed_tools,
			metadata, instructions, default_instructions, files, default_files, kind, builtin_key,
			permission, agent_ids, allowed_agent_types, allowed_permissions, source, version,
			enabled, is_modified, created_at, updated_at
		FROM skills 
	`
	var args []any
	if projectID != "" {
		query += "WHERE project_id = ? OR project_id = '' ORDER BY created_at ASC"
		args = append(args, projectID)
	} else {
		query += "ORDER BY created_at ASC"
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var skills []domain.Skill
	for rows.Next() {
		skill, err := scanSkill(rows)
		if err != nil {
			return nil, err
		}
		skills = append(skills, skill)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return skills, nil
}

func (r *SkillRepository) Update(ctx context.Context, skill domain.Skill) error {
	metadataJSON, _ := json.Marshal(skill.Metadata)
	filesJSON, _ := json.Marshal(skill.Files)
	defaultFilesJSON, _ := json.Marshal(skill.DefaultFiles)
	agentIDsJSON, _ := json.Marshal(skill.AgentIDs)
	allowedAgentTypesJSON, _ := json.Marshal(skill.AllowedAgentTypes)
	allowedPermissionsJSON, _ := json.Marshal(skill.AllowedPermissions)

	query := `
		UPDATE skills SET
			project_id = ?, name = ?, slug = ?, description = ?, license = ?, compatibility = ?, allowed_tools = ?,
			metadata = ?, instructions = ?, default_instructions = ?, files = ?, default_files = ?, kind = ?, builtin_key = ?,
			permission = ?, agent_ids = ?, allowed_agent_types = ?, allowed_permissions = ?, source = ?, version = ?,
			enabled = ?, is_modified = ?, updated_at = ?
		WHERE id = ?
	`
	res, err := r.db.ExecContext(ctx, query,
		skill.ProjectID, skill.Name, skill.Slug, skill.Description, skill.License, skill.Compatibility, skill.AllowedTools,
		string(metadataJSON), skill.Instructions, skill.DefaultInstructions, string(filesJSON), string(defaultFilesJSON), string(skill.Kind), skill.BuiltinKey,
		string(skill.Permission), string(agentIDsJSON), string(allowedAgentTypesJSON), string(allowedPermissionsJSON), skill.Source, skill.Version,
		skill.Enabled, skill.IsModified, skill.UpdatedAt,
		skill.ID,
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

func (r *SkillRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM skills WHERE id = ?`
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

func scanSkill(row scanner) (domain.Skill, error) {
	var s domain.Skill
	var metadataJSON, filesJSON, defaultFilesJSON, kindStr, builtinKey, permissionStr, agentIDsJSON, allowedAgentTypesJSON, allowedPermissionsJSON string

	err := row.Scan(
		&s.ID, &s.ProjectID, &s.Name, &s.Slug, &s.Description, &s.License, &s.Compatibility, &s.AllowedTools,
		&metadataJSON, &s.Instructions, &s.DefaultInstructions, &filesJSON, &defaultFilesJSON, &kindStr, &builtinKey,
		&permissionStr, &agentIDsJSON, &allowedAgentTypesJSON, &allowedPermissionsJSON, &s.Source, &s.Version,
		&s.Enabled, &s.IsModified, &s.CreatedAt, &s.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Skill{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Skill{}, err
	}

	s.Kind = domain.SkillKind(kindStr)
	s.BuiltinKey = builtinKey
	s.Permission = domain.AgentPermission(permissionStr)

	if metadataJSON != "" && metadataJSON != "null" {
		_ = json.Unmarshal([]byte(metadataJSON), &s.Metadata)
	}
	if filesJSON != "" && filesJSON != "null" {
		_ = json.Unmarshal([]byte(filesJSON), &s.Files)
	}
	if defaultFilesJSON != "" && defaultFilesJSON != "null" {
		_ = json.Unmarshal([]byte(defaultFilesJSON), &s.DefaultFiles)
	}
	if agentIDsJSON != "" && agentIDsJSON != "null" {
		_ = json.Unmarshal([]byte(agentIDsJSON), &s.AgentIDs)
	}
	if allowedAgentTypesJSON != "" && allowedAgentTypesJSON != "null" {
		_ = json.Unmarshal([]byte(allowedAgentTypesJSON), &s.AllowedAgentTypes)
	}
	if allowedPermissionsJSON != "" && allowedPermissionsJSON != "null" {
		_ = json.Unmarshal([]byte(allowedPermissionsJSON), &s.AllowedPermissions)
	}
	return s, nil
}
