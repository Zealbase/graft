package sync

import (
	"path/filepath"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/gitx"
	"github.com/Shaik-Sirajuddin/graft/internal/store"
	"github.com/Shaik-Sirajuddin/graft/internal/transform"
)

// fakeGit wraps a real gitx.GitX but forces the first Merge call to conflict,
// then behaves normally on subsequent calls (so a --continue resume succeeds).
type fakeGit struct {
	inner        contract.GitX
	conflictOnce bool
	mergeCalls   int
}

func (f *fakeGit) Init() error                          { return f.inner.Init() }
func (f *fakeGit) HeadHash(ref string) (string, error)  { return f.inner.HeadHash(ref) }
func (f *fakeGit) Branch(name, from string) error       { return f.inner.Branch(name, from) }
func (f *fakeGit) Worktree(n, b string) (string, error) { return f.inner.Worktree(n, b) }
func (f *fakeGit) Diff(ref string) ([]contract.FileChange, error) {
	return f.inner.Diff(ref)
}
func (f *fakeGit) Copy(b string, p []string) error { return f.inner.Copy(b, p) }
func (f *fakeGit) Prune(prefix string) error       { return f.inner.Prune(prefix) }

func (f *fakeGit) Merge(into, from string) (contract.MergeResult, error) {
	f.mergeCalls++
	if f.conflictOnce {
		f.conflictOnce = false
		return contract.MergeResult{
			Clean:     false,
			Conflicts: []contract.Conflict{{Path: ".graft/agents/x/agent.yaml"}},
		}, nil
	}
	return f.inner.Merge(into, from)
}

func TestConflictThenResume(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	writeClaudeAgent(t, dir, "x", "desc", "body")

	st, err := store.Open(filepath.Join(dir, "graft.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	tr := transform.Default()
	real := gitx.New(dir)
	fg := &fakeGit{inner: real, conflictOnce: true}

	eng := New(st, tr, fg, dir)

	// First run hits the forced conflict.
	res, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if res.Status != contract.RunConflict {
		t.Fatalf("status = %s, want conflict", res.Status)
	}
	if len(res.Conflicts) == 0 {
		t.Fatal("expected conflicts surfaced")
	}
	if res.Conflicts[0].Agent != "x" {
		t.Fatalf("conflict agent = %q, want x", res.Conflicts[0].Agent)
	}

	// A non-continue run must refuse and point at --continue.
	if _, err := eng.Run(contract.SyncOpts{}); err == nil {
		t.Fatal("expected blocked error without --continue")
	}

	// Resume with --continue; merge now succeeds.
	res2, err := eng.Run(contract.SyncOpts{Continue: true})
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if res2.Status != contract.RunDone {
		t.Fatalf("resume status = %s, want done (conflicts=%v)", res2.Status, res2.Conflicts)
	}
	if res2.RunID != res.RunID {
		t.Fatalf("resume started a new run %s (orig %s)", res2.RunID, res.RunID)
	}
}
