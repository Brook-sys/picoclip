package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"picoclip/internal/core/domain"
)

type WorkspaceRepository struct {
	db *sql.DB
}

func (r *WorkspaceRepository) Create(ctx context.Context, workspace domain.Workspace) error {
	query := `
		INSERT INTO workspaces (id, name, description, root_path, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.ExecContext(ctx, query,
		workspace.ID, workspace.Name, workspace.Description, workspace.RootPath, workspace.CreatedAt, workspace.UpdatedAt,
	)
	return err
}

func (r *WorkspaceRepository) Get(ctx context.Context, id string) (domain.Workspace, error) {
	query := `SELECT id, name, description, root_path, created_at, updated_at FROM workspaces WHERE id = ?`
	row := r.db.QueryRowContext(ctx, query, id)
	return scanWorkspace(row)
}

func (r *WorkspaceRepository) List(ctx context.Context) ([]domain.Workspace, error) {
	query := `SELECT id, name, description, root_path, created_at, updated_at FROM workspaces ORDER BY created_at ASC`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workspaces []domain.Workspace
	for rows.Next() {
		workspace, err := scanWorkspace(rows)
		if err != nil {
			return nil, err
		}
		workspaces = append(workspaces, workspace)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return workspaces, nil
}

func (r *WorkspaceRepository) Update(ctx context.Context, workspace domain.Workspace) error {
	query := `
		UPDATE workspaces SET name = ?, description = ?, root_path = ?, updated_at = ?
		WHERE id = ?
	`
	res, err := r.db.ExecContext(ctx, query,
		workspace.Name, workspace.Description, workspace.RootPath, workspace.UpdatedAt, workspace.ID,
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

func (r *WorkspaceRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM workspaces WHERE id = ?`
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

func scanWorkspace(row scanner) (domain.Workspace, error) {
	var w domain.Workspace
	err := row.Scan(&w.ID, &w.Name, &w.Description, &w.RootPath, &w.CreatedAt, &w.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Workspace{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Workspace{}, err
	}
	return w, nil
}
