package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestMigrateAddsBudgetAccountingSchemaToExistingDatabase(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "budget-migrations.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.ExecContext(ctx, `
		CREATE TABLE schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMP NOT NULL
		);
	`); err != nil {
		t.Fatal(err)
	}
	for version := 1; version <= 16; version++ {
		if _, err := db.ExecContext(ctx, `INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`, version, fmt.Sprintf("migration_%d", version), time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
	}

	storage := NewStorage(db)
	if err := storage.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	var name string
	if err := db.QueryRowContext(ctx, `SELECT name FROM schema_migrations WHERE version = 17`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "create_budget_accounting_tables" {
		t.Fatalf("migration name = %q, want %q", name, "create_budget_accounting_tables")
	}

	for _, table := range []string{"budget_policies", "budget_accounts", "budget_reservations", "pricing_catalog"} {
		var found string
		if err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&found); err != nil {
			t.Fatalf("table %q was not created: %v", table, err)
		}
	}
	for _, index := range []string{
		"idx_budget_policies_scope_period",
		"idx_budget_accounts_status",
		"idx_budget_reservations_status_deadline",
		"idx_budget_reservations_scope_created",
		"idx_pricing_catalog_lookup",
	} {
		var found string
		if err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?`, index).Scan(&found); err != nil {
			t.Fatalf("index %q was not created: %v", index, err)
		}
	}
}

func TestBudgetAccountingMigrationEnforcesSchemaConstraints(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "budget-constraints.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	storage := NewStorage(db)
	if err := storage.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO budget_policies (id, scope_type, scope_id, period_kind, period_start, currency, token_limit, cost_limit_micros, enforcement, enabled, version, created_at, updated_at)
		VALUES ('invalid', 'invalid_scope', '', 'lifetime', ?, 'USD', 100, 0, 'hard', 1, 1, ?, ?)
	`, now, now, now); err == nil {
		t.Fatal("expected invalid budget policy scope to violate CHECK constraint")
	}

	if _, err := db.ExecContext(ctx, `
		INSERT INTO budget_policies (id, scope_type, scope_id, period_kind, period_start, currency, token_limit, cost_limit_micros, enforcement, enabled, version, created_at, updated_at)
		VALUES ('policy-1', 'workspace', 'workspace-1', 'lifetime', ?, 'USD', 100, 500, 'hard', 1, 1, ?, ?)
	`, now, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO budget_policies (id, scope_type, scope_id, period_kind, period_start, currency, token_limit, cost_limit_micros, enforcement, enabled, version, created_at, updated_at)
		VALUES ('policy-2', 'workspace', 'workspace-1', 'lifetime', ?, 'USD', 100, 500, 'hard', 1, 1, ?, ?)
	`, now, now, now); err == nil {
		t.Fatal("expected duplicate active hard policy to violate unique index")
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO budget_accounts (policy_id, settled_tokens, reserved_tokens, settled_cost_micros, reserved_cost_micros, status, suspension_reason, version, created_at, updated_at)
		VALUES ('missing-policy', 0, 0, 0, 0, 'active', '', 1, ?, ?)
	`, now, now); err == nil {
		t.Fatal("expected missing policy to violate foreign key constraint")
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO budget_accounts (policy_id, settled_tokens, reserved_tokens, settled_cost_micros, reserved_cost_micros, status, suspension_reason, version, created_at, updated_at)
		VALUES ('policy-1', -1, 0, 0, 0, 'active', '', 1, ?, ?)
	`, now, now); err == nil {
		t.Fatal("expected negative account balance to violate CHECK constraint")
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO budget_reservations (id, request_id, task_id, run_id, agent_id, workspace_id, provider, model, pricing_version, currency, reserved_tokens, reserved_cost_micros, settled_tokens, settled_cost_micros, status, created_at, updated_at)
		VALUES ('reservation-1', 'request-1', 'task-1', 'run-1', 'agent-1', 'workspace-1', 'provider', 'model', 'v1', 'USD', 10, 1, 0, 0, 'unknown', ?, ?)
	`, now, now); err == nil {
		t.Fatal("expected invalid reservation status to violate CHECK constraint")
	}
}
