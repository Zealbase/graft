package sync

// Reproduction guard for the suspected post-sync-validation resume loop.
//
// The worry (build prompt): after a *PostSyncValidationError the run might be
// left in a non-terminal status so a subsequent `graft sync` re-enters the SAME
// run forever (resume spin), or the errors.As branch in Run mishandles the
// status. This test exercises a SECOND Engine.Run after a corruption-triggered
// post-sync failure and asserts:
//   - the first run is persisted with a TERMINAL status (done), not conflict;
//   - there is NO open conflict run to resume (OpenConflictRun returns nil);
//   - a second Run terminates (the -timeout on the package run is the loop
//     tripwire) and does NOT resume the prior run.

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/gitx"
	"github.com/Shaik-Sirajuddin/graft/internal/store"
	"github.com/Shaik-Sirajuddin/graft/internal/transform"
)

func TestPostSyncValidationTerminatesNoResumeLoop(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	writeClaudeAgent(t, dir, "reviewer", "reviews code", "You review code.")

	st, err := store.Open(filepath.Join(dir, "graft.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	badYAML := "name: reviewer\ndescription: \"\"\nmodel: sonnet\n"
	cg := &corruptingGit{inner: gitx.New(dir), root: dir, agent: "reviewer", yaml: badYAML}
	eng := New(st, transform.Default(), cg, dir).SetHomeBase(t.TempDir())

	res, err := eng.Run(contract.SyncOpts{})
	var pe *PostSyncValidationError
	if err == nil || !errors.As(err, &pe) {
		t.Fatalf("want *PostSyncValidationError, got status=%s err=%v", res.Status, err)
	}
	if res.Status != contract.RunDone {
		t.Fatalf("result status = %s, want done", res.Status)
	}

	// The DB run row must be terminal so resume cannot pick it up.
	gctx := gitx.Resolve(dir)
	ws, err := st.Workspace(dir, gctx.Remote, gctx.Branch, gctx.Mode)
	if err != nil {
		t.Fatal(err)
	}
	cr, err := st.OpenConflictRun(ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cr != nil {
		t.Fatalf("post-validation-failure left a resumable conflict run: %+v", cr)
	}

	// A second Run must NOT resume the prior run and must terminate. (Loop would
	// surface as the package -timeout firing.) With the corrupt canonical on disk
	// it re-detects no provider drift, so it should reach a terminal status again.
	res2, err2 := eng.Run(contract.SyncOpts{})
	var pe2 *PostSyncValidationError
	if err2 != nil && !errors.As(err2, &pe2) {
		t.Fatalf("second run hard error: %v", err2)
	}
	if res2.Status == contract.RunConflict {
		t.Fatalf("second run resumed into a conflict spin: %+v", res2)
	}
}
