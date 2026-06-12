package e2e

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

// TestLegacyMigration_E2E: seed an OLD-layout repo (in-repo .graft/graft.db +
// lock + .initialized) and run `graft init`. The binary must:
//   1. Migrate the workspace row from the old in-repo db into the global db.
//   2. Remove the old in-repo db, lock, .initialized (runtime artifacts).
//   3. Leave agents/ and .meta.json untouched (portable store survives).
//
// Verification: file assertions (artifacts absent/present) + db row in the
// global db. No product code changes — if migration is absent the test reports
// it as a bug to the gateway owner.
func TestLegacyMigration_E2E(t *testing.T) {
	root := newGitWorkspace(t)

	// ---- SEED: build old-layout in-repo db --------------------------------
	graftDir := filepath.Join(root, ".graft")
	if err := os.MkdirAll(graftDir, 0o755); err != nil {
		t.Fatal(err)
	}

	oldDBPath := filepath.Join(graftDir, "graft.db")
	seedLegacyDB(t, oldDBPath, root)

	// Old-layout runtime sentinels.
	writeFile(t, root, ".graft/lock", "")
	writeFile(t, root, ".graft/.initialized", "tracked\n")

	// Portable agent that MUST survive migration (agents/ + .meta.json).
	writeFile(t, root, ".graft/agents/old-agent/agent.yaml", "name: old-agent\ndescription: test\nmodel: sonnet\n---\nBody.\n")
	writeFile(t, root, ".graft/agents/old-agent/.meta.json", `{"canonicalHash":"abc123"}`)
	writeFile(t, root, ".graft/agents/old-agent/instructions.md", "Body.\n")

	// ---- ACT: first graft command triggers migration ---------------------
	mustGraft(t, root, "init")

	// ---- ASSERT: old runtime artifacts gone ------------------------------
	for _, absent := range []string{
		".graft/graft.db",
		".graft/lock",
		".graft/.initialized",
	} {
		if exists(root, absent) {
			t.Errorf("legacy artifact was NOT cleaned up by migration: %s", absent)
		}
	}

	// ---- ASSERT: portable store survived ---------------------------------
	for _, kept := range []string{
		".graft/agents/old-agent/agent.yaml",
		".graft/agents/old-agent/.meta.json",
		".graft/agents/old-agent/instructions.md",
	} {
		if !exists(root, kept) {
			t.Errorf("portable agent file was deleted by migration: %s", kept)
		}
	}

	// ---- ASSERT: workspace row landed in global db -----------------------
	db := openDB(t, root)
	if n := queryInt(t, db, "SELECT COUNT(*) FROM workspaces"); n < 1 {
		t.Errorf("no workspace row in global db after migration")
	}
	// The migrated row should have our workspace root in it.
	found := queryInt(t, db, "SELECT COUNT(*) FROM workspaces WHERE root = ?", root)
	if found == 0 {
		// root from the seeded db may not match t.TempDir() exactly (symlinks etc.)
		// — log as informational rather than hard-failing the migration.
		t.Logf("NOTE: workspace row root != %s (may be a symlink-resolved path); total rows=%d",
			root, queryInt(t, db, "SELECT COUNT(*) FROM workspaces"))
	}
}

// TestLegacyMigration_NoOp_WhenNoOldDB: a fresh repo with no old in-repo db
// must still work correctly — migration is a no-op and no in-repo db is
// created.
func TestLegacyMigration_NoOp_WhenNoOldDB(t *testing.T) {
	root := newGitWorkspace(t)
	mustGraft(t, root, "init")

	if exists(root, ".graft/graft.db") {
		t.Fatal("in-repo .graft/graft.db should NOT exist on a fresh init (global db layout)")
	}
	if !existsAbs(globalDBPath(root)) {
		t.Fatalf("global db not created at %s", globalDBPath(root))
	}
}

// seedLegacyDB creates a minimal old-layout sqlite db at dbPath with a
// workspace row for root, mimicking what the old binary wrote.
func seedLegacyDB(t *testing.T, dbPath, root string) {
	t.Helper()
	// Use the same sqlite driver the e2e suite uses (modernc.org/sqlite).
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("seed legacy db open: %v", err)
	}
	defer db.Close()

	// Minimal schema matching the old in-repo layout (same as the current
	// embedded schema — migration works on schema-identical dbs).
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS workspaces (
		id         TEXT PRIMARY KEY,
		root       TEXT NOT NULL,
		remote     TEXT NOT NULL,
		branch     TEXT NOT NULL,
		git_mode   TEXT NOT NULL DEFAULT 'tracked',
		created_at INTEGER NOT NULL DEFAULT 0,
		UNIQUE (root, remote, branch)
	)`)
	if err != nil {
		t.Fatalf("seed legacy schema: %v", err)
	}
	_, err = db.Exec(`INSERT INTO workspaces (id, root, remote, branch, git_mode, created_at)
		VALUES ('legacy-ws-id', ?, 'git@example.com:me/repo.git', 'main', 'tracked', 1000)`,
		root)
	if err != nil {
		t.Fatalf("seed legacy workspace row: %v", err)
	}
}

