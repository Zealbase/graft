package sync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/gitx"
	"github.com/Shaik-Sirajuddin/graft/internal/store"
	"github.com/Shaik-Sirajuddin/graft/internal/transform"
)

// TestAbortCleansHaltedConflictRun drives a REAL two-provider conflict to a halt
// (leaving temp branches graft/<run>/* and a .git/graft-worktrees/ worktree),
// then aborts: the temp branches + worktrees must be gone and the run terminal
// (no longer resumable).
func TestAbortCleansHaltedConflictRun(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)

	// Same agent, divergent model -> a real git conflict that halts the run.
	writeClaudeAgentModel(t, dir, "dev", "a developer", "opus", "Shared body.")
	writeOpencodeAgent(t, dir, "dev", "a developer", "sonnet", "Shared body.")

	st, err := store.Open(filepath.Join(dir, "graft.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	eng := New(st, transform.Default(), gitx.New(dir), dir).SetHomeBase(t.TempDir())

	res, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res.Status != contract.RunConflict {
		t.Fatalf("status=%s, want conflict (changed=%v)", res.Status, res.Changed)
	}

	// Precondition: the halt left temp branches + a worktree behind.
	if out, _ := combinedGit(dir, "branch", "--list", "graft/*"); out == "" {
		t.Fatalf("expected temp graft branches after halt, found none")
	}
	wtDir := filepath.Join(dir, ".git", "graft-worktrees")
	if entries, _ := os.ReadDir(wtDir); len(entries) == 0 {
		t.Fatalf("expected a graft worktree after halt under %s", wtDir)
	}

	// Abort the halted run.
	ar, err := eng.Abort(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("abort: %v", err)
	}
	if !ar.Aborted {
		t.Fatalf("abort result Aborted=false, want true")
	}
	if ar.RunID != res.RunID {
		t.Fatalf("abort RunID=%s, want %s", ar.RunID, res.RunID)
	}
	if ar.PrunedBranches <= 0 {
		t.Fatalf("abort PrunedBranches=%d, want > 0", ar.PrunedBranches)
	}

	// Temp branches pruned.
	if out, _ := combinedGit(dir, "branch", "--list", "graft/*"); out != "" {
		t.Fatalf("temp branches survived abort: %q", out)
	}
	// Worktree staging dir removed (or empty).
	if entries, _ := os.ReadDir(wtDir); len(entries) != 0 {
		t.Fatalf("graft worktrees survived abort: %v", entries)
	}

	// Run terminal: no conflict run remains resumable.
	gctx := gitx.Resolve(dir)
	ws, _ := st.Workspace(dir, gctx.Remote, gctx.Branch, gctx.Mode)
	if again, _ := st.OpenConflictRun(ws.ID); again != nil {
		t.Fatalf("conflict run still resumable after abort: %+v", again)
	}
}

// TestAbortNoOpWhenNothingInProgress confirms aborting with no halted run is a
// clean no-op: Aborted=false, no error.
func TestAbortNoOpWhenNothingInProgress(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	writeClaudeAgent(t, dir, "x", "desc", "body")

	st, err := store.Open(filepath.Join(dir, "graft.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	eng := New(st, transform.Default(), gitx.New(dir), dir).SetHomeBase(t.TempDir())

	// No sync has run -> no workspace row, nothing to abort.
	ar, err := eng.Abort(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("abort no-op: %v", err)
	}
	if ar.Aborted {
		t.Fatalf("abort with nothing in progress reported Aborted=true: %+v", ar)
	}

	// A CLEAN sync leaves no halted run either: abort is still a no-op.
	if res, serr := eng.Run(contract.SyncOpts{}); serr != nil || res.Status != contract.RunDone {
		t.Fatalf("clean sync: res=%+v err=%v", res, serr)
	}
	ar2, err := eng.Abort(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("abort after clean sync: %v", err)
	}
	if ar2.Aborted {
		t.Fatalf("abort after clean sync reported Aborted=true: %+v", ar2)
	}
}
