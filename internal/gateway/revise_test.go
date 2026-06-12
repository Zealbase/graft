package gateway_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/gateway"
	"github.com/Shaik-Sirajuddin/graft/internal/store"
)

// TestGlobalDBPath: the sqlite db is created under XDG_DATA_HOME, not in the repo.
func TestGlobalDBPath(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	data := os.Getenv("XDG_DATA_HOME")
	if _, err := os.Stat(globalDB(data)); err != nil {
		t.Fatalf("global db missing at %s: %v", globalDB(data), err)
	}
	if _, err := os.Stat(filepath.Join(root, ".graft", "graft.db")); !os.IsNotExist(err) {
		t.Fatalf("in-repo db should not exist: %v", err)
	}
}

// TestCreatedDerivedFromFindWorkspace: Created is true only on the first init for
// a given identity, derived from the store (no .initialized sentinel).
func TestCreatedDerivedFromFindWorkspace(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)

	res1, err := g.Init()
	if err != nil {
		t.Fatalf("Init 1: %v", err)
	}
	if !res1.Created {
		t.Fatalf("first init Created=false, want true")
	}
	// No sentinel file is written anymore.
	if _, err := os.Stat(filepath.Join(root, ".graft", ".initialized")); !os.IsNotExist(err) {
		t.Fatalf(".initialized sentinel should not be written: %v", err)
	}

	res2, err := g.Init()
	if err != nil {
		t.Fatalf("Init 2: %v", err)
	}
	if res2.Created {
		t.Fatalf("second init Created=true, want false")
	}
}

// TestGraftGitignoreWritten: Open writes a .graft/.gitignore that ignores stray
// local bits while keeping agents/ committed.
func TestGraftGitignoreWritten(t *testing.T) {
	root := newGitWorkspace(t)
	_ = openGate(t, root)

	data, err := os.ReadFile(filepath.Join(root, ".graft", ".gitignore"))
	if err != nil {
		t.Fatalf("read .graft/.gitignore: %v", err)
	}
	s := string(data)
	for _, want := range []string{"*", "!.gitignore", "!agents/"} {
		if !contains(s, want) {
			t.Fatalf(".gitignore missing %q:\n%s", want, s)
		}
	}
}

// TestLockAtGlobalPath: the workspace lock file is created under XDG_DATA_HOME
// (locks dir), not inside the repo's .graft/.
func TestLockAtGlobalPath(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// Sync acquires the lock; after it returns the lock file should exist under
	// the global locks dir and NOT in the repo.
	if _, err := g.Sync(contract.SyncOpts{}); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".graft", "lock")); !os.IsNotExist(err) {
		t.Fatalf("in-repo lock should not exist: %v", err)
	}
	locksDir := filepath.Join(os.Getenv("XDG_DATA_HOME"), "graft", "locks")
	entries, err := os.ReadDir(locksDir)
	if err != nil {
		t.Fatalf("global locks dir missing: %v", err)
	}
	found := false
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".lock" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no .lock file under global locks dir %s", locksDir)
	}
}

// TestLegacyDBMigration: an old-layout repo with an in-repo .graft/graft.db is
// migrated into the global db on Open, and the old runtime bits are removed
// while the portable agents/ + .meta.json survive.
func TestLegacyDBMigration(t *testing.T) {
	root := newGitWorkspace(t)
	data := os.Getenv("XDG_DATA_HOME")

	// Build an OLD-layout in-repo db with a workspace row + a couple of legacy
	// runtime artifacts, plus a portable agent dir that must survive.
	oldDB := filepath.Join(root, ".graft", "graft.db")
	if err := os.MkdirAll(filepath.Dir(oldDB), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy, err := store.Open(oldDB)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	ws, err := legacy.Workspace(root, "git@example.com:me/x.git", "main", contract.GitTracked)
	if err != nil {
		t.Fatalf("seed legacy workspace: %v", err)
	}
	legacy.Close()

	// Legacy runtime sentinels + a portable agent that must NOT be deleted.
	mustWrite(t, filepath.Join(root, ".graft", "lock"), "")
	mustWrite(t, filepath.Join(root, ".graft", ".initialized"), "tracked\n")
	agentMeta := filepath.Join(root, ".graft", "agents", "keep", ".meta.json")
	mustWrite(t, agentMeta, `{"canonicalHash":"x"}`)
	mustWrite(t, filepath.Join(root, ".graft", "agents", "keep", "agent.yaml"), "name: keep\n")

	// Open the gateway -> triggers migration + cleanup.
	g := openGate(t, root)
	defer g.Close()

	// Old db + runtime artifacts removed.
	for _, p := range []string{oldDB, filepath.Join(root, ".graft", "lock"), filepath.Join(root, ".graft", ".initialized")} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("legacy artifact not cleaned: %s (err=%v)", p, err)
		}
	}
	// Portable store survived.
	if _, err := os.Stat(agentMeta); err != nil {
		t.Fatalf("portable .meta.json must survive migration: %v", err)
	}

	// The workspace row landed in the GLOBAL db.
	gstore, err := store.Open(globalDB(data))
	if err != nil {
		t.Fatalf("open global db: %v", err)
	}
	defer gstore.Close()
	found, err := gstore.FindWorkspace(ws.Root, ws.Remote, ws.Branch)
	if err != nil {
		t.Fatalf("FindWorkspace in global db: %v", err)
	}
	if found == nil {
		t.Fatalf("migrated workspace row not found in global db")
	}
}

// TestLegacyMigrationNoOpWhenNoOldDB: a fresh repo (no old db) opens cleanly and
// leaves no in-repo db; migration is a no-op.
func TestLegacyMigrationNoOpWhenNoOldDB(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".graft", "graft.db")); !os.IsNotExist(err) {
		t.Fatalf("no in-repo db expected: %v", err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// contains is a tiny substring helper (avoids importing strings in two files).
func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// ensure gateway import is used even if a subset of tests is built.
var _ = gateway.Open
