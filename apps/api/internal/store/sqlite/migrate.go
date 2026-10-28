package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
)

//go:embed schema.sql
var schemaSQL string

// schemaVersion is the version schema.sql describes. Bump it, and add a step to
// migrate(), when the shape changes; the file itself always describes the
// CURRENT shape (a fresh database applies it once and lands at the latest
// version), and the steps are only for databases that already exist.
const schemaVersion = 2

// Version history:
//
//	1  the local-first baseline (schema.sql as first shipped)
//	2  actions.claimed_by_agent_id (TDM-16): claims keyed by agent id

// migrate brings the database to schemaVersion. Idempotent: reopening an
// up-to-date file runs nothing. Every step runs inside one transaction so a
// half-applied upgrade never persists.
func migrate(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_version (
		version    INTEGER NOT NULL,
		applied_at TEXT    NOT NULL
	)`); err != nil {
		return fmt.Errorf("schema_version: %w", err)
	}
	var current int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&current); err != nil {
		return err
	}
	if current > schemaVersion {
		return fmt.Errorf("database schema version %d is newer than this binary supports (%d)", current, schemaVersion)
	}
	for v := current + 1; v <= schemaVersion; v++ {
		switch v {
		case 1:
			if _, err := tx.ExecContext(ctx, schemaSQL); err != nil {
				return fmt.Errorf("apply schema v1: %w", err)
			}
		case 2:
			// A fresh database already has the column (step 1 applied the
			// CURRENT schema.sql), so the ALTER is conditional — every step has
			// to be a no-op on a database step 1 just created.
			if err := addColumnIfMissing(ctx, tx, "actions", "claimed_by_agent_id", "TEXT"); err != nil {
				return fmt.Errorf("migrate v2: %w", err)
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_version (version, applied_at) VALUES (?, ?)`, v, nowStr()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// addColumnIfMissing adds table.column unless it is already there. Steps that add
// a column use it because schema.sql always describes the latest shape: on a
// brand-new database the column arrived with step 1, and a bare ALTER would
// fail with "duplicate column name".
func addColumnIfMissing(ctx context.Context, tx *sql.Tx, table, column, decl string) error {
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	_, err := tx.ExecContext(ctx, `ALTER TABLE `+table+` ADD COLUMN `+column+` `+decl)
	return err
}
