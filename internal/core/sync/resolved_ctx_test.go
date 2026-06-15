package sync

// Lock-path TOCTOU fix (v0.0.5 Risk A): the engine accepts a pre-resolved git
// context via WithResolvedContext so the caller (gateway) derives BOTH the
// workspace lock path and the engine's workspace key from a SINGLE gitx.Resolve.
// These tests assert the seam: Run keys the workspace row on the INJECTED branch
// (not a fresh resolve), and the injected context is CONSUMED (cleared) after one
// Run so a reused engine never carries a stale branch forward.

import (
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/gitx"
)

// TestWithResolvedContext_KeysWorkspaceOnInjectedBranch asserts that when a
// context is injected, the workspace row Run creates is keyed on the INJECTED
// branch rather than the branch a fresh gitx.Resolve would report. We create a
// SECOND real branch (so git ref ops succeed), stay checked out on the first, and
// inject the second — proving Run keys on the injected branch, not the checkout.
func TestWithResolvedContext_KeysWorkspaceOnInjectedBranch(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	writeClaudeAgent(t, dir, "reviewer", "reviews code", "You review code.")

	// Real checked-out branch, plus a second branch pointing at the same commit.
	real := gitx.Resolve(dir)
	mustGit(t, dir, "branch", "other-branch")
	if real.Branch == "other-branch" {
		t.Fatalf("setup: real branch unexpectedly named other-branch")
	}

	eng, st := newEngine(t, dir)
	defer st.Close()

	injected := gitx.Context{Mode: real.Mode, Remote: real.Remote, Branch: "other-branch"}
	eng.WithResolvedContext(injected)

	res, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res.Status != contract.RunDone {
		t.Fatalf("status=%s, want done (conflicts=%v)", res.Status, res.Conflicts)
	}

	// The workspace must be findable under the INJECTED branch identity, and NOT
	// under the real checked-out branch.
	ws, err := st.FindWorkspace(dir, injected.Remote, injected.Branch)
	if err != nil {
		t.Fatalf("find workspace (injected branch): %v", err)
	}
	if ws == nil {
		t.Fatalf("no workspace keyed on injected branch %q — Run did not use the injected context (Risk A regression)", injected.Branch)
	}
	if other, _ := st.FindWorkspace(dir, real.Remote, real.Branch); other != nil {
		t.Fatalf("workspace was keyed on the real checked-out branch %q, not the injected %q — Run re-resolved instead of using the injection", real.Branch, injected.Branch)
	}
}

// TestWithResolvedContext_ConsumedAfterRun asserts the injected context is a
// ONE-SHOT: a second Run with no fresh injection falls back to a self-resolve, so
// a reused engine cannot carry a stale branch into a later run.
func TestWithResolvedContext_ConsumedAfterRun(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	writeClaudeAgent(t, dir, "reviewer", "reviews code", "You review code.")

	eng, st := newEngine(t, dir)
	defer st.Close()

	real := gitx.Resolve(dir)
	mustGit(t, dir, "branch", "stale-branch")
	eng.WithResolvedContext(gitx.Context{Mode: real.Mode, Remote: real.Remote, Branch: "stale-branch"})

	if _, err := eng.Run(contract.SyncOpts{}); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// Internal invariant: the injected context was consumed (cleared) by Run so a
	// reused engine never carries a stale branch into a later run.
	if eng.resolvedCtx != nil {
		t.Fatalf("resolvedCtx not consumed after Run — a reused engine would carry a stale branch (Risk A)")
	}

	// The first run was keyed on the injected (stale) branch.
	staleWS, err := st.FindWorkspace(dir, real.Remote, "stale-branch")
	if err != nil {
		t.Fatalf("find workspace (stale branch): %v", err)
	}
	if staleWS == nil {
		t.Fatalf("first Run did not key the workspace on the injected branch")
	}
}
