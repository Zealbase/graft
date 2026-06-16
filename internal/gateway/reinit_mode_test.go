package gateway_test

import (
	"os/exec"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/gitx"
	"github.com/Shaik-Sirajuddin/graft/internal/store"
)

// TestReInitStaysInternal guards a LOW-sev re-init consistency bug. The first
// `graft init` of a no-git workspace resolves GitInternal and seeds graft's own
// .git. A SECOND init then sees that seeded .git, so gitx.Resolve reports
// GitTracked. The store UPSERT already refuses to downgrade git_mode
// internal->tracked, so the DB row stays "internal"; this test pins the matching
// behavior for the REPORTED InitResult.GitMode, which must ALSO stay internal so
// the report and the persisted row never disagree.
func TestReInitStaysInternal(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dataHome := isolateXDG(t)
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir() // NO git in root -> first init resolves GitInternal.
	seedAgentFile(t, root)

	// Helper: read the persisted workspace row from the global store using the
	// SAME identity the gateway keys internal-mode workspaces by (remote="",
	// branch=gitx.InternalBranch).
	readRowMode := func() contract.GitMode {
		t.Helper()
		st, err := store.Open(globalDB(dataHome))
		if err != nil {
			t.Fatalf("store.Open: %v", err)
		}
		defer st.Close()
		ws, err := st.FindWorkspace(root, "", gitx.InternalBranch)
		if err != nil {
			t.Fatalf("FindWorkspace: %v", err)
		}
		if ws == nil {
			t.Fatal("FindWorkspace returned nil, expected a workspace row")
		}
		return ws.GitMode
	}

	// First init: existing==nil, gctx.Mode==GitInternal -> seeds, reports
	// internal, Created==true. (First-internal-init behavior is unchanged.)
	g := openGate(t, root)
	res1, err := g.Init()
	if err != nil {
		t.Fatalf("first Init: %v", err)
	}
	if res1.GitMode != contract.GitInternal {
		t.Fatalf("first Init GitMode = %q, want %q", res1.GitMode, contract.GitInternal)
	}
	if !res1.Created {
		t.Fatalf("first Init Created = false, want true")
	}
	if got := readRowMode(); got != contract.GitInternal {
		t.Fatalf("after first Init, persisted git_mode = %q, want %q", got, contract.GitInternal)
	}

	// Second init: the seeded .git now makes gitx.Resolve report GitTracked, so
	// gctx.Mode == GitTracked. The existing row is internal, so the reported mode
	// must be pinned back to internal and Created must be false.
	res2, err := g.Init()
	if err != nil {
		t.Fatalf("second Init: %v", err)
	}
	if res2.GitMode != contract.GitInternal {
		t.Fatalf("second Init GitMode = %q, want %q (must stay internal on re-init)", res2.GitMode, contract.GitInternal)
	}
	if res2.Created {
		t.Fatalf("second Init Created = true, want false (workspace already existed)")
	}
	if got := readRowMode(); got != contract.GitInternal {
		t.Fatalf("after second Init, persisted git_mode = %q, want %q", got, contract.GitInternal)
	}
}
