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
const schemaVersion = 1

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
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_version (version, applied_at) VALUES (?, ?)`, v, nowStr()); err != nil {
			return err
		}
	}
	return tx.Commit()
}
