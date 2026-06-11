package e2e

import (
	"testing"
)

// Scenario 5 (merge conflict) — REACHABILITY NOTE.
//
// The plan asks for a predefined merge-conflict fixture where "two providers'
// versions of one agent diverge", driving `sync agents` to status=conflict with
// a conflict row + path surfaced, then resolve + `sync agents --continue` to
// done.
//
// With the current branch-per-agent sync engine, an *organic* merge conflict is
// structurally unreachable through the real binary:
//   - each changed agent gets its own deterministic branch graft/<run>/agent/<name>;
//   - that branch only writes .graft/agents/<name>/ (disjoint per agent);
//   - the beta branch is cut from the same base, and each agent branch is merged
//     into it sequentially. One branch merging into a beta cut from its own base
//     is always a clean (effectively fast-forward) merge — there is never a second
//     commit touching the same canonical file from a divergent common ancestor.
//   - the engine's own canonicalFor() collapses multiple provider sources of one
//     agent into a single canonical BEFORE branching, so "two providers diverge"
//     resolves in-memory (last-writer-wins on canonical fields) and never reaches
//     a git three-way merge.
//
// The only existing path to contract.RunConflict is the unit test injecting a
// fake GitX (internal/core/sync/conflict_test.go) that forces Merge to report a
// conflict. That seam is not exposed through the compiled binary.
//
// Consequently this e2e file exercises the reachable conflict PLUMBING and
// records the gap as a finding for head (owner: core). See the suite verdict.

// TestConflict_NoOrganicConflict documents that a divergent committed canonical
// plus a divergent provider edit still produces a clean sync (status=done),
// proving the conflict path cannot be reached organically.
func TestConflict_NoOrganicConflict(t *testing.T) {
	root := newGitWorkspace(t)
	provisionClaudeAgent(t, root, "code-reviewer")
	mustGraft(t, root, "init")

	// Commit a DIVERGENT canonical version on the base branch so the beta cut
	// from base already carries .graft/agents/code-reviewer/agent.yaml, while the
	// agent branch rewrites it from the provider source.
	writeFile(t, root, ".graft/agents/code-reviewer/agent.yaml",
		"name: code-reviewer\ndescription: COMMITTED DIVERGENT VERSION\nmodel: opus\n")
	writeFile(t, root, ".graft/agents/code-reviewer/instructions.md", "Committed body\n")
	gitCommitAll(t, root, "commit divergent canonical on base")

	// Also diverge the provider file so its canonical differs from the committed one.
	claude := readFile(t, root, ".claude/agents/code-reviewer.md")
	writeFile(t, root, ".claude/agents/code-reviewer.md", claude+"\nExtra provider-side line.\n")
	gitCommitAll(t, root, "diverge provider source")

	var res runResultJSON
	decodeJSON(t, mustGraft(t, root, "sync", "agents", "-o", "json"), &res)
	// Documented behaviour: still converges to done; no conflict is produced.
	if res.Status != "done" {
		t.Fatalf("expected done (organic conflict unreachable), got status=%q conflicts=%v",
			res.Status, res.Conflicts)
	}

	// db: conflicts table stays empty for an organic run.
	db := openDB(t, root)
	if n := queryInt(t, db, "SELECT COUNT(*) FROM conflicts"); n != 0 {
		t.Fatalf("conflicts rows=%d, want 0 for an organic run", n)
	}
}

// TestConflict_ContinueWithNoOpenRun is a safety check: `sync agents --continue`
// when there is no open conflict run is a benign no-op that converges to done.
func TestConflict_ContinueWithNoOpenRun(t *testing.T) {
	root := newGitWorkspace(t)
	provisionClaudeAgent(t, root, "code-reviewer")
	mustGraft(t, root, "init")
	mustGraft(t, root, "sync", "agents")

	var res runResultJSON
	decodeJSON(t, mustGraft(t, root, "sync", "agents", "--continue", "-o", "json"), &res)
	if res.Status != "done" {
		t.Fatalf("--continue with no open run status=%q, want done", res.Status)
	}
}
