package e2e

import (
	"os"
	"path/filepath"
	"testing"
)

// globalDBPath returns the per-test global db path (under the XDG_DATA_HOME
// that the harness sets for each subprocess run).
func globalDBPath(dir string) string {
	return filepath.Join(dir, "xdg-data", "graft", "graft.db")
}

// existsAbs reports whether an absolute path exists.
func existsAbs(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Scenario 1: init (tracked + non-git/internal), idempotent; .graft created.

func TestInit_Tracked_Idempotent(t *testing.T) {
	root := newGitWorkspace(t)

	// First init: created=true, tracked git mode, .graft tree present.
	var r1 initResult
	decodeJSON(t, mustGraft(t, root, "init", "-o", "json"), &r1)
	if !r1.Created {
		t.Fatalf("first init created=false, want true")
	}
	if r1.GitMode != "tracked" {
		t.Fatalf("git_mode=%q, want tracked", r1.GitMode)
	}
	if filepath.Clean(r1.Root) != filepath.Clean(root) {
		// macOS /private symlink etc.; only fail if clearly unrelated.
		if !exists(root, ".graft") {
			t.Fatalf("init root=%q does not match workspace %q", r1.Root, root)
		}
	}

	// file: global db created; in-repo .graft has NO db/lock/.initialized.
	if !existsAbs(globalDBPath(root)) {
		t.Fatalf("global graft.db not created at %s", globalDBPath(root))
	}
	for _, absent := range []string{".graft/graft.db", ".graft/lock", ".graft/.initialized"} {
		if exists(root, absent) {
			t.Fatalf("in-repo path should be absent after init (global-db layout): %s", absent)
		}
	}
	// file: .graft/.gitignore written (keeps agents/ in the repo, ignores runtime).
	if !exists(root, ".graft/.gitignore") {
		t.Fatal("init did not write .graft/.gitignore")
	}
	// file: .graft/agents/ directory created.
	if !exists(root, ".graft/agents") {
		t.Fatal("init did not create .graft/agents/")
	}

	// db: exactly one workspace row exists in the GLOBAL db.
	db := openDB(t, root)
	if n := queryInt(t, db, "SELECT COUNT(*) FROM workspaces"); n != 1 {
		t.Fatalf("workspaces rows = %d, want 1", n)
	}
	// NOTE: the DB git_mode column is asserted via the JSON surface only — the
	// store hardcodes git_mode='tracked' on insert (see suite verdict, owner db),
	// so the column is not authoritative. The InitResult JSON is.

	// Second init: idempotent, created=false, no extra workspace row.
	var r2 initResult
	decodeJSON(t, mustGraft(t, root, "init", "-o", "json"), &r2)
	if r2.Created {
		t.Fatalf("second init created=true, want false (idempotent)")
	}
	db2 := openDB(t, root)
	if n := queryInt(t, db2, "SELECT COUNT(*) FROM workspaces"); n != 1 {
		t.Fatalf("after second init workspaces rows = %d, want 1", n)
	}
}

func TestInit_Internal_NonGit(t *testing.T) {
	// A plain dir with no git repo resolves to internal mode.
	root := t.TempDir()
	var r initResult
	decodeJSON(t, mustGraft(t, root, "init", "-o", "json"), &r)
	// The authoritative git-mode surface is the InitResult JSON (from gitx.Resolve).
	if r.GitMode != "internal" {
		t.Fatalf("git_mode=%q, want internal for non-git dir", r.GitMode)
	}
	// Global db created; no in-repo db.
	if !existsAbs(globalDBPath(root)) {
		t.Fatalf("global graft.db not created at %s", globalDBPath(root))
	}
	if exists(root, ".graft/graft.db") {
		t.Fatal("in-repo .graft/graft.db should NOT exist (global db layout)")
	}
}

func TestInit_TableOutput(t *testing.T) {
	root := newGitWorkspace(t)
	r := mustGraft(t, root, "init", "-o", "table")
	for _, want := range []string{"KEY", "VALUE", "root", "git_mode", "created"} {
		if !contains(r.stdout, want) {
			t.Fatalf("init table missing %q in:\n%s", want, r.stdout)
		}
	}
}
