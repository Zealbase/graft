package sync

// v0.0.4 verify r2 — DESTRUCTIVE deletion-path fixes, driven through the REAL
// Engine over a real git workspace + real sqlite store (the same harness as
// apply_integration_test.go). Covers:
//
//	HIGH 1: `sync --dry-run` must mutate NOTHING — provider files AND db rows for
//	        a deleted-canonical agent survive a dry run, and the run reports the
//	        pending deletion in RunResult.Deleted.
//	HIGH 2: an agents row WITHOUT any provider_links (a prior aborted run that
//	        never reached applyProviders) must NOT be mis-read as "synced": a
//	        genuinely-new provider-authored agent in that state is INGESTED, not
//	        deleted.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// TestVerifyR2_DryRunDeletesNothing: after an agent has been fully synced,
// deleting its canonical and running `sync --dry-run` must NOT remove the
// provider file or the db rows; the would-be deletion is reported in
// RunResult.Deleted (HIGH 1).
func TestVerifyR2_DryRunDeletesNothing(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	writeClaudeAgent(t, dir, "reviewer", "reviews code", "You review code.")

	eng, st := newEngine(t, dir)
	defer st.Close()

	// First sync: completes -> canonical + provider files + provider_links exist.
	if _, err := eng.Run(contract.SyncOpts{}); err != nil {
		t.Fatalf("initial sync: %v", err)
	}
	claudeFile := filepath.Join(dir, ".claude", "agents", "reviewer.md")
	if _, err := os.Stat(claudeFile); err != nil {
		t.Fatalf("setup: claude provider file missing after first sync: %v", err)
	}
	wsID := workspaceID(t, st, dir)
	if synced, _ := st.AgentSynced(wsID, "reviewer"); !synced {
		t.Fatalf("setup: AgentSynced should be true after a completed sync")
	}

	// Delete the canonical (user removes the agent from graft).
	if err := os.RemoveAll(canonicalDir(dir, "reviewer")); err != nil {
		t.Fatalf("remove canonical: %v", err)
	}

	// Dry-run sync: must report the pending deletion but mutate NOTHING.
	res, err := eng.Run(contract.SyncOpts{DryRun: true})
	if err != nil {
		t.Fatalf("dry-run sync: %v", err)
	}
	if !hasName(res.Deleted, "reviewer") {
		t.Fatalf("[raw] dry-run Deleted=%v, want it to list pending deletion of reviewer", res.Deleted)
	}

	// file: provider file STILL present (dry-run removed nothing).
	if _, err := os.Stat(claudeFile); err != nil {
		t.Fatalf("[file] dry-run deleted the claude provider file (must not mutate): %v", err)
	}
	// db: agents row + provider_links STILL present.
	if synced, _ := st.AgentSynced(wsID, "reviewer"); !synced {
		t.Fatalf("[db] dry-run deleted the agent's db rows (AgentSynced=false); must not mutate")
	}
}

// TestVerifyR2_OrphanAgentRowIngestedNotDeleted: a provider-only agent whose db
// state is an agents row with ZERO provider_links (a prior ABORTED run that
// called UpsertAgent in prepareBranches but never reached applyProviders) must
// be INGESTED, not deleted — AgentSynced is false for it, so the deletion guard
// must not fire (HIGH 2).
func TestVerifyR2_OrphanAgentRowIngestedNotDeleted(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	writeClaudeAgent(t, dir, "newbie", "a new agent", "Fresh provider-authored body.")

	eng, st := newEngine(t, dir)
	defer st.Close()

	// Simulate the aborted-run state: an agents row exists for "newbie" with NO
	// provider_links (exactly what prepareBranches' UpsertAgent leaves on an abort
	// before applyProviders runs). NO canonical exists; the provider file does.
	wsID := workspaceID(t, st, dir)
	if _, err := st.UpsertAgent(contract.Agent{WsID: wsID, Name: "newbie", CanonicalHash: "ORPHAN"}); err != nil {
		t.Fatalf("seed orphan agents row: %v", err)
	}
	if synced, _ := st.AgentSynced(wsID, "newbie"); synced {
		t.Fatalf("setup invariant: orphan agents row (no links) must read AgentSynced=false")
	}

	// Sync: the provider-only agent must be INGESTED (canonical created + fanned
	// out), NOT deleted as a resurrection.
	res, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if !hasName(res.Changed, "newbie") {
		t.Fatalf("[raw] changed=%v deleted=%v, want newbie ingested (not deleted)", res.Changed, res.Deleted)
	}
	if hasName(res.Deleted, "newbie") {
		t.Fatalf("[raw] newbie was DELETED (false-positive); orphan agents row mis-read as synced")
	}
	// file: canonical created (ingestion happened) and provider file retained.
	if _, err := os.Stat(filepath.Join(canonicalDir(dir, "newbie"), "agent.yaml")); err != nil {
		t.Fatalf("[file] canonical not created — agent was not ingested: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "agents", "newbie.md")); err != nil {
		t.Fatalf("[file] claude provider file removed — agent was wrongly deleted: %v", err)
	}
	// db: now genuinely synced (provider_links written by applyProviders).
	if synced, _ := st.AgentSynced(wsID, "newbie"); !synced {
		t.Fatalf("[db] AgentSynced=false after ingest; provider_links should now exist")
	}
}

func canonicalDir(root, name string) string {
	return filepath.Join(root, ".graft", "agents", name)
}

func hasName(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
