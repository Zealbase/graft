package database

import (
	"database/sql"
	"fmt"
	"os"
)

// migrationTables lists every store table in foreign-key dependency order
// (parents before children) so that a row's referenced rows are always copied
// first. Each entry is a table name plus its column list (in a stable order).
var migrationTables = []struct {
	name string
	cols string
}{
	{"workspaces", "id, root, remote, branch, git_mode, created_at"},
	{"agents", "id, ws_id, name, canonical_hash"},
	{"sync_runs", "run_id, ws_id, base_branch, base_start_hash, beta_branch, phase, status, started_at, ended_at"},
	{"branches", "id, run_id, name, kind, head_hash, state"},
	{"provider_links", "id, agent_id, provider, file_path, content_hash, commit_hash"},
	{"agent_states", "id, run_id, agent_id, in_sync, reason"},
	{"conflicts", "id, run_id, agent_name, path, status"},
}

// Migrate imports an OLD per-repo store database (srcDBPath, the legacy
// <root>/.graft/graft.db) into the destination global database at dstPath.
//
// It is:
//   - a no-op when srcDBPath does not exist (returns nil), so callers can invoke
//     it unconditionally on first open under the new layout;
//   - idempotent: rows already present in the destination (by primary key) are
//     skipped via INSERT OR IGNORE, so a repeat import imports nothing new;
//   - FK-safe: tables are copied parents-first and inside a single destination
//     transaction.
//
// The destination is opened via Open (so its schema + pragmas, incl.
// foreign_keys=on, are applied); the source is opened read-only.
func Migrate(dstPath, srcDBPath string) error {
	if _, err := os.Stat(srcDBPath); err != nil {
		if os.IsNotExist(err) {
			return nil // nothing to migrate from
		}
		return fmt.Errorf("stat source db: %w", err)
	}

	dst, err := Open(dstPath)
	if err != nil {
		return fmt.Errorf("open destination db: %w", err)
	}
	defer dst.Close()

	src, err := OpenReadOnly(srcDBPath)
	if err != nil {
		return fmt.Errorf("open source db: %w", err)
	}
	defer src.Close()

	tx, err := dst.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, t := range migrationTables {
		if err := copyTable(src, tx, t.name, t.cols); err != nil {
			return fmt.Errorf("migrate table %s: %w", t.name, err)
		}
	}
	return tx.Commit()
}

// copyTable reads every row of one table from src and inserts it into the
// destination transaction with INSERT OR IGNORE (idempotent on primary key).
// A source table that does not exist (older partial layout) is skipped.
func copyTable(src *sql.DB, tx *sql.Tx, table, cols string) error {
	rows, err := src.Query(fmt.Sprintf(`SELECT %s FROM %s`, cols, table))
	if err != nil {
		// Source predates this table — nothing to import for it.
		return nil
	}
	defer rows.Close()

	colTypes, err := rows.Columns()
	if err != nil {
		return err
	}
	n := len(colTypes)
	placeholders := ""
	for i := 0; i < n; i++ {
		if i > 0 {
			placeholders += ", "
		}
		placeholders += "?"
	}
	insert := fmt.Sprintf(`INSERT OR IGNORE INTO %s (%s) VALUES (%s)`, table, cols, placeholders)

	for rows.Next() {
		vals := make([]any, n)
		ptrs := make([]any, n)
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return err
		}
		if _, err := tx.Exec(insert, vals...); err != nil {
			return err
		}
	}
	return rows.Err()
}
