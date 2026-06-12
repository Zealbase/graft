package sync

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/gitx"
	"github.com/Shaik-Sirajuddin/graft/internal/store"
	"github.com/Shaik-Sirajuddin/graft/internal/transform"
)

// movedBaseGit wraps a real GitX and intercepts HeadHash for a specific branch.
// headHashes is the ordered list of values to return for successive calls to
// HeadHash(targetBranch): index 0 = first call, index 1 = second call, etc.
// Once the list is exhausted every subsequent call returns the last entry.
// All other refs are passed through to the inner impl.
// mergeCount counts total Merge calls (used to verify the re-loop fired).
type movedBaseGit struct {
	inner        contract.GitX
	targetBranch string
	headHashes   []string // successive HeadHash return values for targetBranch
	callIdx      int
	mergeCount   int
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

func (m *movedBaseGit) HeadHash(ref string) (string, error) {
	if ref != m.targetBranch || len(m.headHashes) == 0 {
		return m.inner.HeadHash(ref)
	}
	idx := m.callIdx
	if idx >= len(m.headHashes) {
		idx = len(m.headHashes) - 1
	}
	m.callIdx++
	return m.headHashes[idx], nil
}

// realHash returns the actual HEAD hash for dir so tests can construct stable
// sequences that include the real hash where needed.
func realHash(t *testing.T, dir string) string {
	t.Helper()
	g := gitx.NewShell(dir)
	h, err := g.HeadHash("HEAD")
	if err != nil {
		t.Fatalf("realHash: %v", err)
	}
	return h
}

// newMovedBaseEngine wires a movedBaseGit into a fresh Engine over dir.
func newMovedBaseEngine(t *testing.T, dir string, mg *movedBaseGit) (*Engine, contract.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(dir, "graft.db"))
	if err != nil {
		t.Fatal(err)
	}
	return New(st, transform.Default(), mg, dir).SetHomeBase(t.TempDir()), st
}

// TestBaseMovedDuringFinalize_StabilizesOnFirstRetry verifies that when the
// base branch appears to advance exactly once during finalize (one spurious
// mismatch), the engine re-merges onto the new beta and returns RunDone.
func TestBaseMovedDuringFinalize_StabilizesOnFirstRetry(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	writeClaudeAgent(t, dir, "alpha", "test agent", "Does things.")

	gctx := gitx.Resolve(dir)
	real := gitx.New(dir)
	rh, err := real.HeadHash(gctx.Branch)
	if err != nil {
		t.Fatal(err)
	}

	// HeadHash sequence for targetBranch:
	//   call 1 (during Run startup)        -> real hash  (OpenRun records it as BaseStartHash)
	//   call 2 (finalize first check)      -> fake hash  (base appears to have moved)
	//   call 3 (finalize second check)     -> fake hash  (currentHash == run.BaseStartHash after update)
	//   call 4+ (post-loop stability check)-> fake hash  (stable: finalHash == run.BaseStartHash)
	fakeH := "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	mg := &movedBaseGit{
		inner:        real,
		targetBranch: gctx.Branch,
		headHashes:   []string{rh, fakeH, fakeH, fakeH, fakeH},
	}

	eng, st := newMovedBaseEngine(t, dir, mg)
	defer st.Close()

	res, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("sync with moved base (stabilizes): unexpected error: %v", err)
	}
	if res.Status != contract.RunDone {
		t.Fatalf("status = %s, want done (conflicts=%v)", res.Status, res.Conflicts)
	}
	// Initial merge + at least one re-apply merge.
	if mg.mergeCount < 2 {
		t.Fatalf("mergeCount = %d, want >= 2 (engine should have re-merged after base moved)", mg.mergeCount)
	}
}

// TestBaseMovedDuringFinalize_MoveTwiceThenStabilize verifies that the engine
// tolerates two consecutive base advances before the base settles, and still
// returns RunDone.
func TestBaseMovedDuringFinalize_MoveTwiceThenStabilize(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	writeClaudeAgent(t, dir, "beta", "another agent", "Does more things.")

	gctx := gitx.Resolve(dir)
	real := gitx.New(dir)
	rh, err := real.HeadHash(gctx.Branch)
	if err != nil {
		t.Fatal(err)
	}

	// Sequence: real, fake1 (move #1), fake2 (move #2), fake2 (stable), fake2...
	// After move #1 the engine records fake1 as BaseStartHash; after move #2 it
	// records fake2. On the 4th call fake2==BaseStartHash -> stable.
	fake1 := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	fake2 := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	mg := &movedBaseGit{
		inner:        real,
		targetBranch: gctx.Branch,
		headHashes:   []string{rh, fake1, fake2, fake2, fake2, fake2},
	}

	eng, st := newMovedBaseEngine(t, dir, mg)
	defer st.Close()

	res, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("sync with base moving twice: unexpected error: %v", err)
	}
	if res.Status != contract.RunDone {
		t.Fatalf("status = %s, want done", res.Status)
	}
	if mg.mergeCount < 3 {
		t.Fatalf("mergeCount = %d, want >= 3 (initial + 2 re-applies)", mg.mergeCount)
	}
}

// TestBaseMovedDuringFinalize_CapExceeded verifies that if the base branch
// keeps moving on every re-apply attempt (never stabilizes within maxReapply
// iterations), the engine returns an error rather than looping indefinitely.
// The cap is 3, so we supply 5 distinct fake hashes to guarantee instability
// throughout all retries AND the post-loop stability check.
func TestBaseMovedDuringFinalize_CapExceeded(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	writeClaudeAgent(t, dir, "gamma", "cap test agent", "Always moving.")

	gctx := gitx.Resolve(dir)
	real := gitx.New(dir)
	rh, err := real.HeadHash(gctx.Branch)
	if err != nil {
		t.Fatal(err)
	}

	// Every successive HeadHash call returns a new distinct hash so the base
	// never appears stable. After maxReapply (3) re-applies the engine must error.
	hashes := []string{
		rh,
		"1111111111111111111111111111111111111111", // finalize check 1 -> move
		"2222222222222222222222222222222222222222", // finalize check 2 -> move
		"3333333333333333333333333333333333333333", // finalize check 3 -> move
		"4444444444444444444444444444444444444444", // post-loop stability check -> still moved
	}
	mg := &movedBaseGit{
		inner:        real,
		targetBranch: gctx.Branch,
		headHashes:   hashes,
	}

	eng, st := newMovedBaseEngine(t, dir, mg)
	defer st.Close()

	_, err = eng.Run(contract.SyncOpts{})
	if err == nil {
		t.Fatal("expected error when base keeps moving past the cap, got nil")
	}
	if !strings.Contains(err.Error(), "re-applies") {
		t.Fatalf("error message does not mention re-applies: %q", err.Error())
	}
}
