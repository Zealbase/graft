package gateway

// TestWorkspaceLockPath_BranchMismatch guards Risk A (TOCTOU between Open and
// Sync): the lock path must be keyed on the SAME identity as the engine's
// workspace key so that concurrent syncs for different branches on the same
// root do NOT share a lock (which would incorrectly serialize unrelated branches)
// and so that a single sync correctly serializes itself against other instances
// on the same root+branch.
//
// We test the static property: wsHash(root, remote, branch) is stable and
// distinct for different (root, remote, branch) triples. The TOCTOU risk is
// that workspaceLockPath re-resolves git state at call time; if the branch
// changes between Open and Sync the lock file path changes too. We assert the
// contract via wsHash's behavior.

import (
	"path/filepath"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/gitx"
	"github.com/adrg/xdg"
)

// recordingStore is a fake contract.Store that records the (remote, branch)
// FindWorkspace was queried with. Every other Store method is unimplemented —
// embedding the interface gives a nil-method skeleton, and conflictRunOpen only
// touches FindWorkspace (and, when a workspace is found, OpenConflictRun).
type recordingStore struct {
	contract.Store // nil embedded interface: only the overridden methods are safe to call

	gotRemote, gotBranch string
	ws                   *contract.Workspace // returned from FindWorkspace
}

func (s *recordingStore) FindWorkspace(root, remote, branch string) (*contract.Workspace, error) {
	s.gotRemote, s.gotBranch = remote, branch
	return s.ws, nil
}

// TestWsHash_DistinctIdentities verifies that wsHash produces different values
// for different (root, remote, branch) triples. Two workspaces that differ in
// ANY identity component must produce different lock file paths.
func TestWsHash_DistinctIdentities(t *testing.T) {
	cases := []struct {
		name    string
		a, b    [3]string // [root, remote, branch]
		wantSame bool
	}{
		{
			name:     "same identity → same hash",
			a:        [3]string{"/workspace/foo", "origin", "main"},
			b:        [3]string{"/workspace/foo", "origin", "main"},
			wantSame: true,
		},
		{
			name:     "different root → different hash",
			a:        [3]string{"/workspace/foo", "origin", "main"},
			b:        [3]string{"/workspace/bar", "origin", "main"},
			wantSame: false,
		},
		{
			name:     "different branch → different hash",
			a:        [3]string{"/workspace/foo", "origin", "main"},
			b:        [3]string{"/workspace/foo", "origin", "feature"},
			wantSame: false,
		},
		{
			name:     "different remote → different hash",
			a:        [3]string{"/workspace/foo", "origin", "main"},
			b:        [3]string{"/workspace/foo", "upstream", "main"},
			wantSame: false,
		},
		{
			name:     "empty remote (internal mode) vs origin → different hash",
			a:        [3]string{"/workspace/foo", "", "main"},
			b:        [3]string{"/workspace/foo", "origin", "main"},
			wantSame: false,
		},
		{
			name:     "empty branch vs main → different hash",
			a:        [3]string{"/workspace/foo", "origin", ""},
			b:        [3]string{"/workspace/foo", "origin", "main"},
			wantSame: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ha := wsHash(tc.a[0], tc.a[1], tc.a[2])
			hb := wsHash(tc.b[0], tc.b[1], tc.b[2])
			if tc.wantSame && ha != hb {
				t.Errorf("wsHash(%v) = %q, wsHash(%v) = %q — expected equal", tc.a, ha, tc.b, hb)
			}
			if !tc.wantSame && ha == hb {
				t.Errorf("wsHash(%v) = wsHash(%v) = %q — expected distinct (hash collision or identity not separated)", tc.a, tc.b, ha)
			}
		})
	}
}

// TestWsHash_Stability verifies that wsHash produces the same output across
// multiple calls for the same inputs (no randomness/nonce).
func TestWsHash_Stability(t *testing.T) {
	root, remote, branch := "/my/workspace", "https://github.com/org/repo.git", "main"
	h1 := wsHash(root, remote, branch)
	h2 := wsHash(root, remote, branch)
	if h1 != h2 {
		t.Fatalf("wsHash is not stable: %q != %q", h1, h2)
	}
	if h1 == "" {
		t.Fatalf("wsHash produced empty string")
	}
}

// TestLockPathFor_DerivedFromSingleContext is the POSITIVE assertion for the
// Risk A fix (v0.0.5): the lock path and the engine's workspace identity must be
// derived from the SAME single gitx.Resolve, so a concurrent `git checkout` can
// no longer mismatch the lock against the workspace key.
//
// gate.lockPathFor now takes the already-resolved context as a PARAMETER (it does
// NOT re-resolve), and Sync passes that very same context to the engine via
// WithResolvedContext. We assert the binding directly:
//
//   - lockPathFor(ctx) == globalLockPath(root, ctx.Remote, ctx.Branch) for the
//     EXACT context passed — proving the lock path is keyed on the passed branch,
//     not on a fresh independent resolve.
//   - Two distinct contexts (different branch) yield distinct lock paths through
//     lockPathFor, confirming the branch component is load-bearing.
//
// Because Sync derives BOTH the lock path (lockPathFor(gctx)) and the engine key
// (engine.WithResolvedContext(gctx)) from one gctx value, the branch they see is
// identical by construction — the TOCTOU window is closed.
func TestLockPathFor_DerivedFromSingleContext(t *testing.T) {
	root := t.TempDir()
	g := &gate{root: root}

	remote := "https://github.com/example/repo.git"

	ctxMain := gitx.Context{Remote: remote, Branch: "main"}
	ctxFeature := gitx.Context{Remote: remote, Branch: "feature"}

	// 1. The lock path is keyed on the PASSED context's branch (no re-resolve):
	//    lockPathFor(ctx) must equal globalLockPath(root, ctx.Remote, ctx.Branch).
	for _, ctx := range []gitx.Context{ctxMain, ctxFeature} {
		got, err := g.lockPathFor(ctx)
		if err != nil {
			t.Fatalf("lockPathFor(%+v): %v", ctx, err)
		}
		want, err := globalLockPath(root, ctx.Remote, ctx.Branch)
		if err != nil {
			t.Fatalf("globalLockPath(%+v): %v", ctx, err)
		}
		if got != want {
			t.Fatalf("lockPathFor(%+v)=%q, want %q — lock path is NOT derived from the "+
				"passed context's branch (re-resolve regression, Risk A)", ctx, got, want)
		}
	}

	// 2. The branch component is load-bearing: different branches -> different
	//    lock paths through the SAME entry point used by Sync.
	lockMain, err := g.lockPathFor(ctxMain)
	if err != nil {
		t.Fatalf("lockPathFor(main): %v", err)
	}
	lockFeature, err := g.lockPathFor(ctxFeature)
	if err != nil {
		t.Fatalf("lockPathFor(feature): %v", err)
	}
	if lockMain == lockFeature {
		t.Fatalf("lockPathFor produced the SAME path for main and feature — branch identity "+
			"is not part of the lock key (would mis-serialize unrelated branches)")
	}

	// 3. Structural invariants: under XDG data home and a .lock file.
	xdgData := xdg.DataHome
	if xdgData != "" {
		for _, p := range []string{lockMain, lockFeature} {
			if !isUnderDir(p, xdgData) {
				t.Errorf("lock path %q is not under XDG data home %q", p, xdgData)
			}
		}
	}
	for _, p := range []string{lockMain, lockFeature} {
		if filepath.Ext(p) != ".lock" {
			t.Errorf("lock path %q does not end in .lock", p)
		}
	}
}

// TestConflictRunOpen_KeysOffPassedContext is the POSITIVE assertion for the
// residual conflict-run TOCTOU fix (v0.0.5 review): conflictRunOpen must probe
// the workspace identity carried by the PASSED gctx — the same single
// gitx.Resolve Sync used for the lock path and engine key — rather than making
// its own independent re-resolve. A concurrent `git checkout` between Sync's
// resolve and this probe can therefore no longer make it inspect a different
// branch's workspace and reach the wrong skip-gate decision.
//
// We pass a context with a distinctive branch and assert FindWorkspace was
// queried with EXACTLY that branch/remote. ws==nil makes conflictRunOpen return
// false without needing OpenConflictRun.
func TestConflictRunOpen_KeysOffPassedContext(t *testing.T) {
	rs := &recordingStore{ws: nil}
	g := &gate{root: t.TempDir(), store: rs}

	gctx := gitx.Context{Remote: "https://github.com/example/repo.git", Branch: "feature-x"}

	if got := g.conflictRunOpen(gctx); got {
		t.Fatalf("conflictRunOpen returned true for an absent workspace; want false")
	}
	if rs.gotBranch != gctx.Branch {
		t.Fatalf("conflictRunOpen queried FindWorkspace with branch %q, want %q "+
			"(must key off the PASSED gctx, not a re-resolve)", rs.gotBranch, gctx.Branch)
	}
	if rs.gotRemote != gctx.Remote {
		t.Fatalf("conflictRunOpen queried FindWorkspace with remote %q, want %q",
			rs.gotRemote, gctx.Remote)
	}
}

// isUnderDir reports whether child is rooted under parent (or equal to parent).
func isUnderDir(child, parent string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	// filepath.Rel returns ".." for paths not under parent.
	return len(rel) < 2 || rel[:2] != ".."
}
