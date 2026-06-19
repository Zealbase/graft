package e2e

import (
	"sort"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/canonical"
)

// v0.0.4 verify task 2: a freshly-scaffolded `graft agent init` agent has a
// canonical but no provider files and an empty .meta.json provider map. The
// FIRST `sync agents` must fan it out to EVERY enabled provider (it used to be
// skipped because Hash(canonical)==meta.CanonicalHash made canonChanged false
// and there were no provider sources). A SECOND sync must be a no-op (no churn).
//
// Init UX fix: a scaffolded agent now gets a non-empty default description
// ("<name> agent"), so it passes validation immediately. This test scaffolds
// with --no-sync (to assert the never-synced precondition) and then drives the
// fan-out explicitly to assert the first-sync/second-sync behavior.
func TestVerify_ScaffoldFansOutOnFirstSync(t *testing.T) {
	root := newGitWorkspace(t)
	// A base commit so the workspace has a resolvable HEAD for the sync run.
	writeFile(t, root, "README.md", "seed\n")
	gitCommitAll(t, root, "seed")
	mustGraft(t, root, "init")

	// Scaffold a brand-new canonical agent WITHOUT the auto-sync (no provider
	// files yet) so we can assert the never-synced precondition.
	mustGraft(t, root, "agent", "init", "dev", "You are the dev agent.", "--no-sync")

	// Sanity: canonical exists, but no provider files and an empty meta provider
	// map (the never-synced precondition the fix keys on).
	if !exists(root, ".graft/agents/dev/agent.yaml") {
		t.Fatal("agent init did not write canonical agent.yaml")
	}
	meta0, err := canonical.LoadMeta(canonical.AgentDir(root, "dev"))
	if err != nil {
		t.Fatalf("LoadMeta after init: %v", err)
	}
	if len(meta0.Providers) != 0 {
		t.Fatalf("freshly-scaffolded agent should have NO provider meta, got %v", meta0.Providers)
	}
	if exists(root, ".claude/agents/dev.md") {
		t.Fatal("agent init --no-sync should not have written any provider file yet")
	}

	// First sync MUST fan the scaffold out: status done + dev in changed.
	var res runResultJSON
	decodeJSON(t, mustGraft(t, root, "sync", "agents", "-o", "json"), &res)
	if res.Status != "done" {
		t.Fatalf("first sync status=%q, want done", res.Status)
	}
	if !containsStr(res.Changed, "dev") {
		t.Fatalf("first sync changed=%v, want it to include dev (scaffold must fan out)", res.Changed)
	}

	// db: every enabled provider got a provider_link (full fan-out).
	db := openDB(t, root)
	links := providerLinkHashes(t, db, "dev")
	got := make([]string, 0, len(links))
	for p := range links {
		got = append(got, p)
	}
	sort.Strings(got)
	if !equalStrings(got, allProviders) {
		t.Fatalf("after first sync provider_links=%v, want all %v", got, allProviders)
	}

	// file: at least the in-repo (ScopeProject) provider file now exists.
	if !exists(root, ".claude/agents/dev.md") {
		t.Fatal("first sync did not write the claude-code provider file for the scaffold")
	}

	// meta now records every enabled provider (so a re-sync is a no-op).
	meta1, err := canonical.LoadMeta(canonical.AgentDir(root, "dev"))
	if err != nil {
		t.Fatalf("LoadMeta after sync: %v", err)
	}
	if len(meta1.Providers) != len(allProviders) {
		t.Fatalf("after sync meta has %d providers, want %d", len(meta1.Providers), len(allProviders))
	}

	// Second sync MUST be a no-op: already in sync, nothing changed (no spurious
	// re-fan-out of an already-propagated agent).
	var res2 runResultJSON
	decodeJSON(t, mustGraft(t, root, "sync", "agents", "-o", "json"), &res2)
	if res2.Status != "done" {
		t.Fatalf("second sync status=%q, want done", res2.Status)
	}
	if len(res2.Changed) != 0 {
		t.Fatalf("second sync changed=%v, want empty (no churn for already-synced scaffold)", res2.Changed)
	}
}
