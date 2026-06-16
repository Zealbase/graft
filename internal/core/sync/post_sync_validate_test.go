package sync

// Post-sync canonical-store validation gate tests (v0.0.6 "missing validations
// after sync for .graft/").
//
// These prove the gate is REAL — it runs inside the actual Engine.Run code path
// (at the tail of finalize, after the merge is committed and the run is marked
// done), not just in a standalone unit test:
//
//	(a) a clean sync passes the post-sync validation (no PostSyncValidationError);
//	(b) a canonical agent.yaml that is corrupted DURING the run (after the beta
//	    tree is copied onto base, before the post-sync gate runs) is CAUGHT by the
//	    post-sync validation and surfaced as a *PostSyncValidationError — while the
//	    committed run is still left done (no rollback).
//
// Helpers (requireGit, newWorkspace, writeClaudeAgent, writeFile, mustGit) and
// the contract.GitX seam are defined in sync_test.go / conflict_test.go (same
// package) and reused here.

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/canonical"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/gitx"
	"github.com/Shaik-Sirajuddin/graft/internal/store"
	"github.com/Shaik-Sirajuddin/graft/internal/transform"
)

// corruptingGit wraps a real gitx.GitX and, after the real Copy of the resolved
// beta tree onto the working base, overwrites the named agent's canonical
// agent.yaml with a deliberately invalid document (empty description — a known
// semantic violation in canonical.Validate). This models merge-introduced
// corruption that lands in the committed canonical store, exercising the
// post-sync gate end-to-end through Engine.Run. The corruption is applied once.
type corruptingGit struct {
	inner contract.GitX
	root  string
	agent string
	yaml  string // replacement agent.yaml bytes
	done  bool
}

func (c *corruptingGit) Init() error                          { return c.inner.Init() }
func (c *corruptingGit) HeadHash(ref string) (string, error)  { return c.inner.HeadHash(ref) }
func (c *corruptingGit) Branch(name, from string) error       { return c.inner.Branch(name, from) }
func (c *corruptingGit) Worktree(n, b string) (string, error) { return c.inner.Worktree(n, b) }
func (c *corruptingGit) Diff(ref string) ([]contract.FileChange, error) {
	return c.inner.Diff(ref)
}
func (c *corruptingGit) Prune(prefix string) error { return c.inner.Prune(prefix) }
func (c *corruptingGit) Merge(into, from string) (contract.MergeResult, error) {
	return c.inner.Merge(into, from)
}

// Copy performs the real copy, then (once) corrupts the canonical agent.yaml so
// the bytes that applyProviders loads + round-trips carry the violation into the
// committed store, where the post-sync gate must catch it.
func (c *corruptingGit) Copy(b string, p []string) error {
	if err := c.inner.Copy(b, p); err != nil {
		return err
	}
	if !c.done {
		c.done = true
		path := filepath.Join(canonical.AgentDir(c.root, c.agent), "agent.yaml")
		if err := os.WriteFile(path, []byte(c.yaml), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// TestPostSyncValidationCleanPasses proves a clean sync passes the post-sync
// canonical validation gate: Engine.Run returns no error and no
// PostSyncValidationError, and the run is done.
func TestPostSyncValidationCleanPasses(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	writeClaudeAgent(t, dir, "reviewer", "reviews code", "You review code.")

	eng, st := newEngine(t, dir)
	defer st.Close()

	res, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("clean sync returned error: %v", err)
	}
	if fs := PostSyncFindings(err); len(fs) > 0 {
		t.Fatalf("clean sync produced post-sync findings: %+v", fs)
	}
	if res.Status != contract.RunDone {
		t.Fatalf("status = %s, want done", res.Status)
	}

	// Sanity: the committed canonical actually validates clean too.
	can, lerr := canonical.Load(canonical.AgentDir(dir, "reviewer"))
	if lerr != nil {
		t.Fatalf("load canonical: %v", lerr)
	}
	if vfs, verr := canonical.Validate(can); verr != nil || len(vfs) > 0 {
		t.Fatalf("clean canonical did not validate: findings=%+v err=%v", vfs, verr)
	}
}

// TestPostSyncValidationCatchesCorruption proves the gate is real: a canonical
// agent.yaml corrupted mid-run (after the beta tree is copied onto base) is
// CAUGHT by the post-sync validation. Engine.Run surfaces a
// *PostSyncValidationError carrying an error-severity finding, and the committed
// run is left done (state is reported, not rolled back).
func TestPostSyncValidationCatchesCorruption(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	writeClaudeAgent(t, dir, "reviewer", "reviews code", "You review code.")

	st, err := store.Open(filepath.Join(dir, "graft.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// Invalid canonical: empty description is a known semantic violation that
	// canonical.Validate flags as an error-severity finding.
	badYAML := "name: reviewer\ndescription: \"\"\nmodel: sonnet\n"
	cg := &corruptingGit{inner: gitx.New(dir), root: dir, agent: "reviewer", yaml: badYAML}

	eng := New(st, transform.Default(), cg, dir).SetHomeBase(t.TempDir())

	res, err := eng.Run(contract.SyncOpts{})
	if err == nil {
		t.Fatalf("expected post-sync validation error, got nil (status=%s)", res.Status)
	}

	var pe *PostSyncValidationError
	if !errors.As(err, &pe) {
		t.Fatalf("error is not *PostSyncValidationError: %T: %v", err, err)
	}
	fs := PostSyncFindings(err)
	if len(fs) == 0 {
		t.Fatalf("PostSyncValidationError carried no findings")
	}
	foundReviewer := false
	for _, f := range fs {
		if f.Severity != "error" {
			t.Fatalf("non-error finding leaked into gate: %+v", f)
		}
		if f.Agent == "reviewer" {
			foundReviewer = true
		}
	}
	if !foundReviewer {
		t.Fatalf("findings did not flag agent reviewer: %+v", fs)
	}

	// "Report, don't undo": the committed merge is NOT rolled back. The run is
	// still reported done and the (corrupt) canonical is on disk.
	if res.Status != contract.RunDone {
		t.Fatalf("status = %s, want done (gate reports, never rolls back)", res.Status)
	}
	if _, serr := os.Stat(filepath.Join(canonical.AgentDir(dir, "reviewer"), "agent.yaml")); serr != nil {
		t.Fatalf("canonical agent.yaml should remain on disk (no rollback): %v", serr)
	}
}
