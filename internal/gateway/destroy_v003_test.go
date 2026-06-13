package gateway_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// TestDestroyFSFailureLeavesDBRow verifies the fs-first ordering (review r2): if
// the .graft filesystem removal fails, Destroy must return the error WITHOUT
// having deleted the workspace db row, so a re-run can recover. We force the
// failure on the KeepStore path by making a non-agents subdir undeletable, then
// confirm RemovedRows==0; after restoring perms a re-run deletes the row.
func TestDestroyFSFailureLeavesDBRow(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("chmod-based permission denial does not apply to root")
	}
	root := newGitWorkspace(t)
	g := openGate(t, root)
	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Populate a non-agents entry under .graft that cannot be removed: a subdir
	// containing a file, with the subdir itself made read-only/no-exec so
	// RemoveAll of its child fails.
	graft := filepath.Join(root, ".graft")
	stuck := filepath.Join(graft, "runs")
	if err := os.MkdirAll(stuck, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stuck, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(stuck, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(stuck, 0o755) })

	res, err := g.Destroy(contract.DestroyOpts{KeepStore: true})
	if err == nil {
		t.Fatalf("Destroy expected fs-removal error, got nil (res=%+v)", res)
	}
	if res.RemovedRows != 0 {
		t.Fatalf("db row deleted despite fs failure (RemovedRows=%d); fs-first ordering broken", res.RemovedRows)
	}

	// Recover: restore perms and re-run. The workspace row must still be present,
	// so this run deletes it (RemovedRows==1) — proving the earlier failure left
	// the row intact.
	if err := os.Chmod(stuck, 0o755); err != nil {
		t.Fatal(err)
	}
	res2, err := g.Destroy(contract.DestroyOpts{KeepStore: true})
	if err != nil {
		t.Fatalf("recovery Destroy: %v", err)
	}
	if res2.RemovedRows != 1 {
		t.Fatalf("recovery RemovedRows=%d, want 1 (row should have survived the failed run)", res2.RemovedRows)
	}
	// agents store retained on the KeepStore path.
	if _, err := os.Stat(filepath.Join(graft, "agents")); err != nil {
		t.Fatalf("KeepStore should retain .graft/agents: %v", err)
	}
}

// TestDestroyKeepStorePartialReportsRemovedDir verifies that a successful
// KeepStore destroy removes non-agents entries and reports RemovedDir, while
// retaining the canonical store.
func TestDestroyKeepStoreRemovesNonAgents(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	graft := filepath.Join(root, ".graft")
	if err := os.MkdirAll(filepath.Join(graft, "runs"), 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := g.Destroy(contract.DestroyOpts{KeepStore: true})
	if err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if !res.RemovedDir {
		t.Fatalf("RemovedDir=false; want true (non-agents entry was removed)")
	}
	if _, err := os.Stat(filepath.Join(graft, "runs")); !os.IsNotExist(err) {
		t.Fatalf(".graft/runs should be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(graft, "agents")); err != nil {
		t.Fatalf("KeepStore should retain .graft/agents: %v", err)
	}
}
