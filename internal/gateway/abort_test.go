package gateway_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// TestAbortSyncCleansHaltedRun drives a real two-provider conflict through the
// gateway (lock + engine), then aborts: the temp branches + worktrees are gone
// and the run is terminal (a later Sync starts fresh, no resume).
func TestAbortSyncCleansHaltedRun(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Two providers define the SAME agent "dev" with a divergent model so the
	// per-provider canonical merge conflicts on the model line.
	writeAgentFile(t, root, filepath.Join(".claude", "agents", "dev.md"),
		"---\nname: dev\ndescription: a developer\nmodel: opus\n---\nShared body.\n")
	writeAgentFile(t, root, filepath.Join(".opencode", "agents", "dev.md"),
		"---\nname: dev\ndescription: a developer\nmodel: sonnet\n---\nShared body.\n")

	res, err := g.Sync(contract.SyncOpts{Ingest: true})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res.Status != contract.RunConflict {
		t.Fatalf("status=%s, want conflict", res.Status)
	}

	// Precondition: the halt left a worktree behind.
	wtDir := filepath.Join(root, ".git", "graft-worktrees")
	if entries, _ := os.ReadDir(wtDir); len(entries) == 0 {
		t.Fatalf("expected a graft worktree after halt under %s", wtDir)
	}

	ar, err := g.AbortSync()
	if err != nil {
		t.Fatalf("AbortSync: %v", err)
	}
	if !ar.Aborted || ar.RunID != res.RunID {
		t.Fatalf("abort result = %+v, want Aborted=true RunID=%s", ar, res.RunID)
	}
	if ar.PrunedBranches <= 0 {
		t.Fatalf("abort PrunedBranches=%d, want > 0", ar.PrunedBranches)
	}

	// Worktrees gone.
	if entries, _ := os.ReadDir(wtDir); len(entries) != 0 {
		t.Fatalf("graft worktrees survived abort: %v", entries)
	}

	// Terminal: the run is no longer an OPEN conflict run, so a second abort finds
	// nothing to clean up (proves the run was marked terminated, not left
	// resumable).
	ar2, err := g.AbortSync()
	if err != nil {
		t.Fatalf("second AbortSync: %v", err)
	}
	if ar2.Aborted {
		t.Fatalf("run still resumable after abort — second abort reported Aborted=true: %+v", ar2)
	}
}

// TestAbortSyncNoOp confirms aborting with no in-progress run is a clean no-op
// (Aborted=false, no error).
func TestAbortSyncNoOp(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	ar, err := g.AbortSync()
	if err != nil {
		t.Fatalf("AbortSync no-op: %v", err)
	}
	if ar.Aborted {
		t.Fatalf("abort with nothing in progress reported Aborted=true: %+v", ar)
	}
}

// writeAgentFile writes a provider agent file under root, creating parent dirs.
func writeAgentFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
