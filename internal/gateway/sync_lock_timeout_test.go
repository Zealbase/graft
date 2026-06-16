package gateway

// Bounded-lock-wait guard (build prompt suspect #3): a STALE / stuck workspace
// lock — e.g. a crashed graft process whose flock was never released — must NOT
// hang `graft sync` forever. Sync waits at most syncLockWait for the lock, then
// surfaces an actionable "workspace busy / stale lock" error.
//
// This test holds the workspace flock from a SEPARATE handle (modeling another
// process) and asserts Sync returns the bounded error well inside a hard test
// timeout, instead of blocking indefinitely.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/gitx"
	"github.com/Shaik-Sirajuddin/graft/internal/lock"
	"github.com/adrg/xdg"
)

func TestSyncBoundedLockWait(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	// Isolate XDG (global db + locks) and HOME (scope-home providers).
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	xdg.Reload()
	t.Setenv("HOME", t.TempDir())

	root := t.TempDir()
	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGit("init", "-q")
	runGit("config", "user.email", "t@t")
	runGit("config", "user.name", "t")
	runGit("config", "commit.gpgsign", "false")
	adir := filepath.Join(root, ".claude", "agents")
	if err := os.MkdirAll(adir, 0o755); err != nil {
		t.Fatal(err)
	}
	agent := "---\nname: reviewer\ndescription: reviews code\nmodel: sonnet\n---\nYou review code.\n"
	if err := os.WriteFile(filepath.Join(adir, "reviewer.md"), []byte(agent), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", ".")
	runGit("commit", "-q", "-m", "seed")

	g, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer g.Close()

	// Acquire the SAME workspace lock from a separate handle (models another live
	// graft process holding it) so Sync cannot get it.
	gctx := gitx.Resolve(root)
	lockPath, err := globalLockPath(root, gctx.Remote, gctx.Branch)
	if err != nil {
		t.Fatal(err)
	}
	held, err := lock.Lock(t.Context(), lockPath)
	if err != nil {
		t.Fatalf("pre-acquire lock: %v", err)
	}
	defer held.Unlock()

	// Shrink the wait so the busy path is fast and a regression (unbounded wait)
	// surfaces as the test -timeout firing rather than the assertion.
	orig := syncLockWait
	syncLockWait = 150 * time.Millisecond
	defer func() { syncLockWait = orig }()

	done := make(chan struct{})
	var serr error
	go func() {
		_, serr = g.Sync(contract.SyncOpts{Ingest: true})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Sync did not return while the workspace lock was held — unbounded wait (regression)")
	}

	if serr == nil {
		t.Fatal("Sync succeeded while the workspace lock was held; expected a busy error")
	}
	if !strings.Contains(serr.Error(), "workspace busy") {
		t.Fatalf("error = %q, want a bounded 'workspace busy' lock error", serr.Error())
	}
}
