package database

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// seedFullDB creates a store DB at path and inserts one row into every table
// (FK-consistent) so Migrate has a complete graph to copy.
func seedFullDB(t *testing.T, path string) {
	t.Helper()
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open seed db: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`INSERT INTO workspaces (id, root, remote, branch, git_mode, created_at)
		   VALUES ('ws1', '/repo', 'origin', 'main', 'tracked', 100)`,
		`INSERT INTO agents (id, ws_id, name, canonical_hash)
		   VALUES ('ag1', 'ws1', 'foo', 'CANON')`,
		`INSERT INTO sync_runs (run_id, ws_id, base_branch, base_start_hash, beta_branch, phase, status, started_at, ended_at)
		   VALUES ('run1', 'ws1', 'main', 'h0', '', 'merge', 'conflict', 1, 0)`,
		`INSERT INTO branches (id, run_id, name, kind, head_hash, state)
		   VALUES ('br1', 'run1', 'graft/run1/beta/1', 'beta', 'h1', 'active')`,
		`INSERT INTO provider_links (id, agent_id, provider, file_path, content_hash, commit_hash)
		   VALUES ('pl1', 'ag1', 'claudecode', 'p', 'CANON', 'c1')`,
		`INSERT INTO agent_states (id, run_id, agent_id, in_sync, reason)
		   VALUES ('as1', 'run1', 'ag1', 1, '')`,
		`INSERT INTO conflicts (id, run_id, agent_name, path, status)
		   VALUES ('cf1', 'run1', 'foo', 'a.md', 'open')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed exec: %v\n%s", err, s)
		}
	}
}

func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func TestMigrateImportsAllTables(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "old.graft.db")
	dst := filepath.Join(dir, "global.graft.db")
	seedFullDB(t, src)

	if err := Migrate(dst, src); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	db, err := Open(dst)
	if err != nil {
		t.Fatalf("open dst: %v", err)
	}
	defer db.Close()

	for _, table := range []string{
		"workspaces", "agents", "sync_runs", "branches",
		"provider_links", "agent_states", "conflicts",
	} {
		if got := countRows(t, db, table); got != 1 {
			t.Fatalf("table %s: got %d rows, want 1", table, got)
		}
	}

	// Spot-check a copied value survived intact.
	var status string
	if err := db.QueryRow(`SELECT status FROM sync_runs WHERE run_id='run1'`).Scan(&status); err != nil {
		t.Fatalf("read migrated run: %v", err)
	}
	if status != "conflict" {
		t.Fatalf("migrated run status = %q, want conflict", status)
	}
}

func TestMigrateIdempotent(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "old.graft.db")
	dst := filepath.Join(dir, "global.graft.db")
	seedFullDB(t, src)

	if err := Migrate(dst, src); err != nil {
		t.Fatalf("Migrate first: %v", err)
	}
	// Second import of the same source must add nothing.
	if err := Migrate(dst, src); err != nil {
		t.Fatalf("Migrate second: %v", err)
	}

	db, err := Open(dst)
	if err != nil {
		t.Fatalf("open dst: %v", err)
	}
	defer db.Close()

	for _, table := range []string{
		"workspaces", "agents", "sync_runs", "branches",
		"provider_links", "agent_states", "conflicts",
	} {
		if got := countRows(t, db, table); got != 1 {
			t.Fatalf("after re-import, table %s: got %d rows, want 1", table, got)
		}
	}
}

func TestMigratePreservesExistingDestRows(t *testing.T) {
	// A destination that already holds a DIFFERENT workspace keeps it, and the
	// source rows are added alongside (idempotent merge, not overwrite).
	dir := t.TempDir()
	src := filepath.Join(dir, "old.graft.db")
	dst := filepath.Join(dir, "global.graft.db")
	seedFullDB(t, src)

	pre, err := Open(dst)
	if err != nil {
		t.Fatalf("open dst pre: %v", err)
	}
	if _, err := pre.Exec(
		`INSERT INTO workspaces (id, root, remote, branch, git_mode, created_at)
		   VALUES ('wsOther', '/other', 'origin', 'main', 'tracked', 5)`,
	); err != nil {
		t.Fatalf("seed dst: %v", err)
	}
	pre.Close()

	if err := Migrate(dst, src); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	db, err := Open(dst)
	if err != nil {
		t.Fatalf("open dst: %v", err)
	}
	defer db.Close()
	if got := countRows(t, db, "workspaces"); got != 2 {
		t.Fatalf("workspaces = %d, want 2 (existing + imported)", got)
	}
}

func TestMigrateMissingSourceNoOp(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "global.graft.db")
	missing := filepath.Join(dir, "does-not-exist.db")

	if err := Migrate(dst, missing); err != nil {
		t.Fatalf("Migrate missing src should be no-op, got: %v", err)
	}
	// Calling against a missing source must not even require the dst to pre-exist;
	// nothing should have been created/populated.
	db, err := Open(dst)
	if err != nil {
		t.Fatalf("open dst: %v", err)
	}
	defer db.Close()
	if got := countRows(t, db, "workspaces"); got != 0 {
		t.Fatalf("workspaces = %d after no-op migrate, want 0", got)
	}
}
