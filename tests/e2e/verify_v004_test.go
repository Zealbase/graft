package e2e

import (
	"path/filepath"
	"sort"
	"testing"
)

// v0.0.4 verify task 3 (a): after an agent has been synced, deleting its
// canonical (.graft/agents/<name>) and re-syncing must propagate the DELETE —
// the agent's file is removed from EVERY provider and its db rows are gone. It
// must NOT be re-ingested from the provider dirs (no resurrection).
func TestVerify_SyncRespectsAgentDeletion(t *testing.T) {
	root := newGitWorkspace(t)
	provisionClaudeAgent(t, root, "code-reviewer")
	mustGraft(t, root, "init")
	mustGraft(t, root, "sync", "agents")

	// Precondition: synced — canonical present, provider file present, db rows.
	if !exists(root, ".graft/agents/code-reviewer/agent.yaml") {
		t.Fatal("setup: canonical not written by first sync")
	}
	if !exists(root, ".claude/agents/code-reviewer.md") {
		t.Fatal("setup: claude provider file not written by first sync")
	}
	db := openDB(t, root)
	if n := queryInt(t, db, "SELECT COUNT(*) FROM agents WHERE name=?", "code-reviewer"); n != 1 {
		t.Fatalf("setup: expected 1 agents row, got %d", n)
	}

	// Delete the canonical (the user removes the agent from graft).
	mustRemoveAll(t, filepath.Join(root, ".graft", "agents", "code-reviewer"))

	// Re-sync: must DELETE, not resurrect.
	mustGraft(t, root, "sync", "agents")

	// file: provider file removed from every detected provider. The claude file
	// (ScopeProject) is the one provisioned and re-fanned; assert it is gone.
	if exists(root, ".claude/agents/code-reviewer.md") {
		t.Fatal("deletion not respected: claude provider file resurrected/retained")
	}
	// All other in-repo provider files written by the first sync must be gone too.
	for _, rel := range []string{
		".opencode/agents/code-reviewer.md",
		".cursor/agents/code-reviewer.md",
	} {
		if exists(root, rel) {
			t.Fatalf("deletion not respected: provider file %s still present", rel)
		}
	}

	// canonical must NOT be re-created (no resurrection).
	if exists(root, ".graft/agents/code-reviewer/agent.yaml") {
		t.Fatal("deletion not respected: canonical resurrected on re-sync")
	}

	// db: agent rows gone (agents + provider_links).
	db2 := openDB(t, root)
	if n := queryInt(t, db2, "SELECT COUNT(*) FROM agents WHERE name=?", "code-reviewer"); n != 0 {
		t.Fatalf("deletion not respected: %d agents rows remain (want 0)", n)
	}
	if n := queryInt(t, db2,
		"SELECT COUNT(*) FROM provider_links pl JOIN agents a ON a.id=pl.agent_id WHERE a.name=?",
		"code-reviewer"); n != 0 {
		t.Fatalf("deletion not respected: %d provider_links remain (want 0)", n)
	}
}

// v0.0.4 verify task 3 (b): a fresh provider-only agent the db has NEVER seen is
// still INGESTED + fanned out — the deletion guard must not break ingestion of
// genuinely new, provider-authored agents.
func TestVerify_FreshProviderOnlyAgentStillIngested(t *testing.T) {
	root := newGitWorkspace(t)
	provisionClaudeAgent(t, root, "code-reviewer")
	mustGraft(t, root, "init")

	var res runResultJSON
	decodeJSON(t, mustGraft(t, root, "sync", "agents", "-o", "json"), &res)
	if res.Status != "done" {
		t.Fatalf("sync status=%q, want done", res.Status)
	}
	if !containsStr(res.Changed, "code-reviewer") {
		t.Fatalf("changed=%v, want code-reviewer ingested", res.Changed)
	}
	// canonical created from the provider file (ingestion happened).
	if !exists(root, ".graft/agents/code-reviewer/agent.yaml") {
		t.Fatal("fresh provider-only agent was not ingested (no canonical)")
	}
	// fanned out to all providers.
	db := openDB(t, root)
	links := providerLinkHashes(t, db, "code-reviewer")
	got := make([]string, 0, len(links))
	for p := range links {
		got = append(got, p)
	}
	sort.Strings(got)
	if !equalStrings(got, allProviders) {
		t.Fatalf("fresh ingest provider_links=%v, want all %v", got, allProviders)
	}
}

// v0.0.4 verify task 3 (c): deleting an agent's file from a SINGLE provider while
// the canonical STILL exists must NOT be treated as a full deletion — the
// no-resurrection guard fires ONLY when the canonical is absent (canonExists ==
// false). Here canonExists is true, so the guard is skipped: the canonical and
// the agent's db rows survive (the agent stays tracked), and no provider files
// are wiped.
//
// NOTE on scope: the engine does not currently *re-render* a single deleted
// provider file from the intact canonical on the next sync (the agent only
// becomes work when a provider file CHANGED, the canonical drifted, a provider
// is canon-stale, or it is a never-synced scaffold — none of which a pure file
// deletion trips). That re-render-on-missing-file behavior is a SEPARATE drift
// gap outside the v0.0.4 task-3 deletion semantics; this test asserts only that
// a single-provider delete is not mis-classified as an agent deletion.
func TestVerify_SingleProviderDeleteKeepsAgentTracked(t *testing.T) {
	root := newGitWorkspace(t)
	provisionClaudeAgent(t, root, "code-reviewer")
	mustGraft(t, root, "init")
	mustGraft(t, root, "sync", "agents")

	// Remove ONLY the claude provider file; canonical and the 9 other provider
	// files stay.
	if !exists(root, ".claude/agents/code-reviewer.md") {
		t.Fatal("setup: claude file missing")
	}
	mustRemoveAll(t, filepath.Join(root, ".claude", "agents", "code-reviewer.md"))
	if !exists(root, ".graft/agents/code-reviewer/agent.yaml") {
		t.Fatal("setup: canonical should still exist")
	}

	mustGraft(t, root, "sync", "agents")

	// canonical must survive — a single-provider delete is NOT an agent deletion.
	if !exists(root, ".graft/agents/code-reviewer/agent.yaml") {
		t.Fatal("single-provider delete wrongly removed the canonical (mis-classified as deletion)")
	}
	// Another provider's file (not deleted) must still be present (no wipe).
	if !exists(root, ".opencode/agents/code-reviewer.md") {
		t.Fatal("single-provider delete wrongly wiped other providers' files")
	}
	// db rows must survive — the agent stays tracked.
	db := openDB(t, root)
	if n := queryInt(t, db, "SELECT COUNT(*) FROM agents WHERE name=?", "code-reviewer"); n != 1 {
		t.Fatalf("single-provider delete should keep the agent tracked, got %d rows", n)
	}
}
