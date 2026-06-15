package e2e

// Concurrency e2e tests: two concurrent graft sync agents invocations on the
// SAME workspace root. The workspace lock (flock) serializes them — the second
// process blocks until the first completes. Both must exit 0, and the final
// DB state must be consistent.

import (
	"sync"
	"testing"
)

// TestConcurrentSync_SecondWaits launches two concurrent `graft sync agents`
// subprocesses on the same rootA (same XDG). Asserts:
//   - Both exit 0
//   - The flock serialized them (no "locked" error, no corruption)
//   - Final state is consistent: exactly the expected agents in sync, sync_run
//     rows present, no double-apply corruption.
func TestConcurrentSync_SecondWaits(t *testing.T) {
	root := newGitWorkspace(t)
	provisionClaudeAgent(t, root, "code-reviewer")
	mustGraft(t, root, "init")

	// Run two concurrent syncs on the same workspace.
	var (
		wg  sync.WaitGroup
		mu  sync.Mutex
		res [2]runResult
	)
	for i := 0; i < 2; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := graft(t, root, "sync", "agents", "-o", "json")
			mu.Lock()
			res[i] = r
			mu.Unlock()
		}()
	}
	wg.Wait()

	// CORRECT BEHAVIOR: both must exit 0. The flock ensures the second waits
	// for the first to complete, then runs (or finds no change and is a no-op).
	for i, r := range res {
		if r.exitCode != 0 {
			t.Errorf("concurrent sync[%d] exit=%d, want 0\nstdout: %s\nstderr: %s",
				i, r.exitCode, r.stdout, r.stderr)
		}
	}

	// Both must have produced valid JSON output.
	for i, r := range res {
		if r.exitCode == 0 {
			var jr runResultJSON
			decodeJSON(t, r, &jr)
			if jr.Status != "done" {
				t.Errorf("concurrent sync[%d] status=%q, want done", i, jr.Status)
			}
		}
	}

	// Final DB state: at least one sync_run row in status=done (the first sync
	// must have completed). Both syncs share the same XDG, so they share the
	// same global DB. The second sync (running after the first completed) may
	// be a no-op (no changed agents) — that is correct and expected.
	db := openDB(t, root)
	doneCount := queryInt(t, db, "SELECT COUNT(*) FROM sync_runs WHERE status='done'")
	if doneCount < 1 {
		t.Fatalf("expected at least 1 done sync_run after concurrent syncs, got %d", doneCount)
	}

	// The canonical agent must exist and be consistent.
	if !exists(root, ".graft/agents/code-reviewer/agent.yaml") {
		t.Fatal("canonical agent.yaml missing after concurrent syncs")
	}

	// No aborted runs (a race/corruption would manifest as an aborted run).
	abortedCount := queryInt(t, db, "SELECT COUNT(*) FROM sync_runs WHERE status='aborted'")
	if abortedCount > 0 {
		t.Fatalf("concurrent sync produced %d aborted sync_run(s) — possible corruption", abortedCount)
	}
}
