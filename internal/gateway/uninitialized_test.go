package gateway_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/gateway"
)

// seedAgentFile writes a minimal Claude Code agent file under <root>/.claude/agents
// so the sync would have something to do once the workspace were initialized.
func seedAgentFile(t *testing.T, root string) {
	t.Helper()
	dir := filepath.Join(root, ".claude", "agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "code-reviewer.md"), []byte(claudeAgent), 0o644); err != nil {
		t.Fatal(err)
	}
}

// assertUninitialized asserts that err is the clear, actionable uninitialized
// error and that it does NOT leak the raw `git rev-parse` failure (v0.0.6 #3).
func assertUninitialized(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("Sync on uninitialized workspace = nil err, want UninitializedError")
	}
	if !gateway.IsUninitialized(err) {
		t.Fatalf("err is not *UninitializedError: %T %v", err, err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "run 'graft init' first") {
		t.Fatalf("message missing actionable hint: %q", msg)
	}
	// The raw git failure must never surface to the user.
	for _, raw := range []string{"rev-parse", "Needed a single revision", "exit status 128", "fatal:"} {
		if strings.Contains(msg, raw) {
			t.Fatalf("error leaked raw git text %q: %q", raw, msg)
		}
	}
}

// TestSyncNoGitDirIsActionable: a directory with NO git at all (agents just
// copied in) must fail `graft sync agents` with the clear actionable message,
// not the raw `git rev-parse ... Needed a single revision` crash.
func TestSyncNoGitDirIsActionable(t *testing.T) {
	isolateXDG(t)
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	seedAgentFile(t, root)

	g := openGate(t, root)
	_, err := g.Sync(contract.SyncOpts{Ingest: true})
	assertUninitialized(t, err)
}

// TestSyncGitNoCommitsIsActionable: a `git init`'d directory with NO commits yet
// must fail with the same clear actionable message (Resolve reports GitTracked
// but `rev-parse --verify <branch>` fails on an empty repo).
func TestSyncGitNoCommitsIsActionable(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	isolateXDG(t)
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	seedAgentFile(t, root)

	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")
	// Intentionally NO commit: HEAD is unresolvable.

	g := openGate(t, root)
	_, err := g.Sync(contract.SyncOpts{Ingest: true})
	assertUninitialized(t, err)
}

// TestSyncTrackedWithCommitProceeds: the guard must NOT trip for a normal
// initialized repo with a commit — sync proceeds to the engine and completes.
func TestSyncTrackedWithCommitProceeds(t *testing.T) {
	root := newGitWorkspace(t) // git repo with one committed agent
	g := openGate(t, root)
	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	res, err := g.Sync(contract.SyncOpts{Ingest: true})
	if err != nil {
		t.Fatalf("Sync on a committed repo errored: %v", err)
	}
	if res.Status != contract.RunDone {
		t.Fatalf("sync status=%q, want done", res.Status)
	}
}
