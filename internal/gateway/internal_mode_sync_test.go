package gateway_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// assertNoRawGitLeak fails the test if err carries the raw git rev-parse failure
// that this fix is designed to eliminate (v0.0.6 #3). A non-nil err is always a
// failure here; the substring checks make a regression obvious.
func assertNoRawGitLeak(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	msg := err.Error()
	for _, raw := range []string{"head hash", "rev-parse", "Needed a single revision", "git rev-parse"} {
		if strings.Contains(msg, raw) {
			t.Fatalf("sync leaked raw git failure %q: %q", raw, msg)
		}
	}
	t.Fatalf("sync errored: %v", err)
}

// TestInternalModeSyncEndToEnd is the full internal-mode flow: agents copied into
// a NON-git dir, then `graft init` (which now seeds graft's own internal repo) and
// `graft sync agents` must succeed. Before the fix, Sync returned
// *UninitializedError because init never seeded the internal repo.
func TestInternalModeSyncEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	isolateXDG(t)
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir() // NO git in root
	seedAgentFile(t, root)

	g := openGate(t, root)

	res, err := g.Init()
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if res.GitMode != contract.GitInternal {
		t.Fatalf("Init GitMode = %q, want %q (graft seeds its own repo)", res.GitMode, contract.GitInternal)
	}
	if !res.Created {
		t.Fatalf("Init Created = false, want true on first init")
	}

	res2, err := g.Sync(contract.SyncOpts{Ingest: true})
	if err != nil {
		// CRITICAL: this used to be *UninitializedError, and deeper a raw
		// git rev-parse failure. Surface the actual message and flag any leak.
		assertNoRawGitLeak(t, err)
	}
	if res2.Status != contract.RunDone {
		t.Fatalf("Sync status = %q, want %q", res2.Status, contract.RunDone)
	}

	// The canonical agent must have been created.
	canPath := filepath.Join(root, ".graft", "agents", "code-reviewer", "agent.yaml")
	if _, err := os.Stat(canPath); err != nil {
		t.Fatalf("canonical agent.yaml missing at %s: %v", canPath, err)
	}

	// Providers were fanned out: the run reports the agent as changed.
	var found bool
	for _, c := range res2.Changed {
		if c == "code-reviewer" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Changed = %v, want it to contain %q", res2.Changed, "code-reviewer")
	}
}

// TestInternalModeSyncIdempotent: a second sync of an unchanged internal-mode
// workspace is a no-op (RunDone, empty Changed).
func TestInternalModeSyncIdempotent(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	isolateXDG(t)
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	seedAgentFile(t, root)

	g := openGate(t, root)
	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	res, err := g.Sync(contract.SyncOpts{Ingest: true})
	if err != nil {
		assertNoRawGitLeak(t, err)
	}
	if res.Status != contract.RunDone {
		t.Fatalf("first Sync status = %q, want %q", res.Status, contract.RunDone)
	}

	res3, err := g.Sync(contract.SyncOpts{Ingest: true})
	if err != nil {
		assertNoRawGitLeak(t, err)
	}
	if res3.Status != contract.RunDone {
		t.Fatalf("second Sync status = %q, want %q", res3.Status, contract.RunDone)
	}
	if len(res3.Changed) != 0 {
		t.Fatalf("second Sync Changed = %v, want empty (in-sync no-op)", res3.Changed)
	}
}

// TestSyncNoGitDirWithoutInitStillGuards: a truly-uninitialized no-git workspace
// (no `graft init`) must still surface the actionable "run 'graft init' first"
// message — the seed-on-init fix must NOT regress this guard.
func TestSyncNoGitDirWithoutInitStillGuards(t *testing.T) {
	isolateXDG(t)
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	seedAgentFile(t, root)

	g := openGate(t, root)
	_, err := g.Sync(contract.SyncOpts{Ingest: true})
	assertUninitialized(t, err)
}
