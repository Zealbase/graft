package database

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
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

func TestMigrateSrcMissingColumnSkipped(t *testing.T) {
	// Per spec, "no such column" is a schema-absent condition: a src that
	// pre-dates a column is skipped (not an error). This confirms that path and
	// that the rows from the partial table are gracefully omitted rather than
	// causing a hard failure.
	dir := t.TempDir()
	src := filepath.Join(dir, "partial.db")
	dst := filepath.Join(dir, "global.db")

	// Build a src with workspaces but WITHOUT the git_mode column.
	raw, err := sql.Open("sqlite", "file:"+src+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	if _, err := raw.Exec(`CREATE TABLE workspaces (id TEXT PRIMARY KEY, root TEXT, remote TEXT, branch TEXT, created_at INTEGER)`); err != nil {
		raw.Close()
		t.Fatalf("create partial table: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO workspaces VALUES ('w1', '/r', 'o', 'main', 1)`); err != nil {
		raw.Close()
		t.Fatalf("insert: %v", err)
	}
	raw.Close()

	// Must not error (schema-absent -> skip).
	if err := Migrate(dst, src); err != nil {
		t.Fatalf("expected no error for missing column (schema-absent skip), got: %v", err)
	}
	// The partial table is skipped; dst workspaces row count is 0.
	db, err := Open(dst)
	if err != nil {
		t.Fatalf("open dst: %v", err)
	}
	defer db.Close()
	if got := countRows(t, db, "workspaces"); got != 0 {
		t.Fatalf("workspaces = %d, want 0 (partial table skipped)", got)
	}
}

func TestCopyTableRealErrorSurfaced(t *testing.T) {
	// A genuine (non-schema-absent) query error must be surfaced, not swallowed.
	// We exercise copyTable directly (same package) with a closed src DB, which
	// produces an "sql: database is closed" error — not a schema error.
	dir := t.TempDir()
	dst := filepath.Join(dir, "global.db")

	dstDB, err := Open(dst)
	if err != nil {
		t.Fatalf("open dst: %v", err)
	}
	defer dstDB.Close()
	tx, err := dstDB.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback()

	// A closed DB produces a real I/O-class error (not "no such table/column").
	closedSrc, _ := sql.Open("sqlite", "file:"+filepath.Join(dir, "closed.db")+"?_pragma=journal_mode(WAL)")
	closedSrc.Close() // close immediately — any Query will fail with ErrConnDone

	err = copyTable(closedSrc, tx, "workspaces", "id, root, remote, branch, git_mode, created_at")
	if err == nil {
		t.Fatalf("expected error from closed src DB, got nil (data-loss path)")
	}
	// Error must not be mistaken for a schema-absent skip.
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "no such table") || strings.Contains(msg, "no such column") {
		t.Fatalf("error incorrectly looks schema-absent: %v", err)
	}
}

func TestMigrateSrcAbsentTableSkipped(t *testing.T) {
	// A src DB that pre-dates a table entirely (no such table) must NOT cause an
	// error — the table is silently skipped. This confirms the schema-absent
	// exception is still in place after the MED fix.
	dir := t.TempDir()
	src := filepath.Join(dir, "old.db")
	dst := filepath.Join(dir, "global.db")

	// Build a src with ONLY workspaces (no agents, sync_runs, etc.).
	raw, err := sql.Open("sqlite", "file:"+src+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	if _, err := raw.Exec(`CREATE TABLE workspaces (id TEXT PRIMARY KEY, root TEXT NOT NULL, remote TEXT NOT NULL, branch TEXT NOT NULL, git_mode TEXT NOT NULL DEFAULT 'tracked', created_at INTEGER NOT NULL DEFAULT 0)`); err != nil {
		raw.Close()
		t.Fatalf("create table: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO workspaces VALUES ('ws1', '/r', 'origin', 'main', 'tracked', 1)`); err != nil {
		raw.Close()
		t.Fatalf("insert: %v", err)
	}
	raw.Close()

	if err := Migrate(dst, src); err != nil {
		t.Fatalf("absent tables should be skipped, got: %v", err)
	}

	db, err := Open(dst)
	if err != nil {
		t.Fatalf("open dst: %v", err)
	}
	defer db.Close()
	if got := countRows(t, db, "workspaces"); got != 1 {
		t.Fatalf("workspaces = %d, want 1", got)
	}
	// Other tables were absent in src — they must be empty in dst (not errored).
	if got := countRows(t, db, "agents"); got != 0 {
		t.Fatalf("agents = %d, want 0", got)
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
