package sync

import (
	"path/filepath"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/gitx"
	"github.com/Shaik-Sirajuddin/graft/internal/store"
	"github.com/Shaik-Sirajuddin/graft/internal/transform"
)

// movedBaseGit wraps a real GitX but intercepts HeadHash so that on the SECOND
// call for the base branch it returns a different (faked) hash, simulating a
// concurrent commit landing on the base during finalize(). After the second
// call it returns the real hash again, so the re-loop stabilises on iteration 1.
type movedBaseGit struct {
	inner         contract.GitX
	targetBranch  string // the branch whose HEAD we fake
	headHashCalls int
	fakeHash      string // returned on the 2nd call
	mergeCount    int    // how many times Merge was called in total
}

func (m *movedBaseGit) Init() error { return m.inner.Init() }
func (m *movedBaseGit) Branch(name, from string) error {
	return m.inner.Branch(name, from)
}
func (m *movedBaseGit) Worktree(n, b string) (string, error) {
	return m.inner.Worktree(n, b)
}
func (m *movedBaseGit) Diff(ref string) ([]contract.FileChange, error) {
	return m.inner.Diff(ref)
}
func (m *movedBaseGit) Copy(b string, p []string) error { return m.inner.Copy(b, p) }
func (m *movedBaseGit) Prune(prefix string) error       { return m.inner.Prune(prefix) }
func (m *movedBaseGit) Merge(into, from string) (contract.MergeResult, error) {
	m.mergeCount++
	return m.inner.Merge(into, from)
}

// HeadHash returns a fake hash on the second call for the target branch,
// simulating a commit that landed while the sync was running. Subsequent calls
// return the real value so the re-loop can stabilise.
func (m *movedBaseGit) HeadHash(ref string) (string, error) {
	if ref == m.targetBranch {
		m.headHashCalls++
		if m.headHashCalls == 2 {
			// Pretend the base moved: return a plausible fake hash.
			return m.fakeHash, nil
		}
		if m.headHashCalls > 2 {
			// Third+ call: the "new" base matches our recorded hash, so the
			// re-loop ends. Return the fake hash as if the base settled.
			return m.fakeHash, nil
		}
	}
	return m.inner.HeadHash(ref)
}

// TestBaseMovedDuringFinalize verifies that when the base branch advances
// between the end of the merge loop and the Copy step, the engine:
//  1. Detects the moved head (HeadHash differs from BaseStartHash).
//  2. Re-runs mergeInto at least once onto a new beta (mergeCount >= 2).
//  3. Still finishes with status RunDone (not an error).
func TestBaseMovedDuringFinalize(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	writeClaudeAgent(t, dir, "alpha", "test agent", "Does things.")

	st, err := store.Open(filepath.Join(dir, "graft.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	real := gitx.New(dir)
	gctx := gitx.Resolve(dir)

	mg := &movedBaseGit{
		inner:        real,
		targetBranch: gctx.Branch,
		// A fake SHA-1 that differs from any real hash in the repo.
		fakeHash: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
	}

	eng := New(st, transform.Default(), mg, dir)

	res, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("sync with moved base: unexpected error: %v", err)
	}
	if res.Status != contract.RunDone {
		t.Fatalf("status = %s, want done (conflicts=%v)", res.Status, res.Conflicts)
	}
	// The merge loop must have fired more than once: the initial run + at least
	// one re-apply for the moved base.
	if mg.mergeCount < 2 {
		t.Fatalf("mergeCount = %d, want >= 2 (engine should re-merge after base moved)", mg.mergeCount)
	}
}
