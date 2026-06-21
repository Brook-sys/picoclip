package sqlite

import (
	"context"
	"database/sql"
	"time"

	"picoclip/internal/core/domain"
)

type RuntimeRepository struct {
	db *sql.DB
}

func (r *RuntimeRepository) GetByRuntimeID(ctx context.Context, runtimeID domain.RuntimeID) (domain.RuntimeState, error) {
	var state domain.RuntimeState
	var lastHealthAt sql.NullTime
	err := r.db.QueryRowContext(ctx, `
		SELECT id, runtime_id, mode, enabled, version, bin_path, config_path, home_path, data_path, logs_path, source, source_url, checksum, installed_at, updated_at, last_health_at, last_health_json, settings_json, metadata_json
		FROM runtime_states WHERE runtime_id = ?
	`, runtimeID).Scan(&state.ID, &state.RuntimeID, &state.Mode, &state.Enabled, &state.Version, &state.BinPath, &state.ConfigPath, &state.HomePath, &state.DataPath, &state.LogsPath, &state.Source, &state.SourceURL, &state.Checksum, &state.InstalledAt, &state.UpdatedAt, &lastHealthAt, &state.LastHealthJSON, &state.SettingsJSON, &state.MetadataJSON)
	if err == sql.ErrNoRows {
		return domain.RuntimeState{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.RuntimeState{}, err
	}
	if lastHealthAt.Valid {
		state.LastHealthAt = &lastHealthAt.Time
	}
	return state, nil
}

func (r *RuntimeRepository) List(ctx context.Context) ([]domain.RuntimeState, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, runtime_id, mode, enabled, version, bin_path, config_path, home_path, data_path, logs_path, source, source_url, checksum, installed_at, updated_at, last_health_at, last_health_json, settings_json, metadata_json
		FROM runtime_states
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []domain.RuntimeState
	for rows.Next() {
		var state domain.RuntimeState
		var lastHealthAt sql.NullTime
		err := rows.Scan(&state.ID, &state.RuntimeID, &state.Mode, &state.Enabled, &state.Version, &state.BinPath, &state.ConfigPath, &state.HomePath, &state.DataPath, &state.LogsPath, &state.Source, &state.SourceURL, &state.Checksum, &state.InstalledAt, &state.UpdatedAt, &lastHealthAt, &state.LastHealthJSON, &state.SettingsJSON, &state.MetadataJSON)
		if err != nil {
			return nil, err
		}
		if lastHealthAt.Valid {
			state.LastHealthAt = &lastHealthAt.Time
		}
		res = append(res, state)
	}
	return res, rows.Err()
}

func (r *RuntimeRepository) Save(ctx context.Context, state domain.RuntimeState) error {
	if state.SettingsJSON == "" {
		state.SettingsJSON = "{}"
	}
	if state.MetadataJSON == "" {
		state.MetadataJSON = "{}"
	}
	if state.LastHealthJSON == "" {
		state.LastHealthJSON = "{}"
	}
	if state.InstalledAt.IsZero() {
		state.InstalledAt = time.Now().UTC()
	}
	state.UpdatedAt = time.Now().UTC()
	var lastHealthAt interface{}
	if state.LastHealthAt != nil {
		lastHealthAt = *state.LastHealthAt
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO runtime_states (id, runtime_id, mode, enabled, version, bin_path, config_path, home_path, data_path, logs_path, source, source_url, checksum, installed_at, updated_at, last_health_at, last_health_json, settings_json, metadata_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET runtime_id = excluded.runtime_id, mode = excluded.mode, enabled = excluded.enabled, version = excluded.version, bin_path = excluded.bin_path, config_path = excluded.config_path, home_path = excluded.home_path, data_path = excluded.data_path, logs_path = excluded.logs_path, source = excluded.source, source_url = excluded.source_url, checksum = excluded.checksum, updated_at = excluded.updated_at, last_health_at = excluded.last_health_at, last_health_json = excluded.last_health_json, settings_json = excluded.settings_json, metadata_json = excluded.metadata_json
	`, state.ID, state.RuntimeID, state.Mode, state.Enabled, state.Version, state.BinPath, state.ConfigPath, state.HomePath, state.DataPath, state.LogsPath, state.Source, state.SourceURL, state.Checksum, state.InstalledAt, state.UpdatedAt, lastHealthAt, state.LastHealthJSON, state.SettingsJSON, state.MetadataJSON)
	return err
}

func (r *RuntimeRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM runtime_states WHERE id = ?`, id)
	return err
}
