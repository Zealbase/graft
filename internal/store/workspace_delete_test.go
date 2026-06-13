package store

import (
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// seedFullWorkspace seeds a workspace with agents, agent_states, provider_links,
// sync_runs, branches, and conflicts, then returns the workspace and all row IDs
// for post-delete assertion.
type seedIDs struct {
	wsID       string
	agentID    string
	agentID2   string
	runID      string
	branchID   string
	conflictID string
	stateID    string
	linkID     string
}

func seedFullWorkspace(t *testing.T, st contract.Store, root string) seedIDs {
	t.Helper()
	s := concrete(t, st)

	ws, err := st.Workspace(root, "origin", "main", contract.GitTracked)
	if err != nil {
		t.Fatalf("Workspace: %v", err)
	}

	// Two agents under this workspace.
	ag1, err := st.UpsertAgent(contract.Agent{WsID: ws.ID, Name: "agent-one", CanonicalHash: "HASH1"})
	if err != nil {
		t.Fatalf("UpsertAgent 1: %v", err)
	}
	ag2, err := st.UpsertAgent(contract.Agent{WsID: ws.ID, Name: "agent-two", CanonicalHash: "HASH2"})
	if err != nil {
		t.Fatalf("UpsertAgent 2: %v", err)
	}

	// Provider links for agent-one.
	if err := st.UpsertProviderLink(contract.ProviderLink{
		AgentID: ag1.ID, Provider: "claude-code", FilePath: "p1", ContentHash: "HASH1",
	}); err != nil {
		t.Fatalf("UpsertProviderLink: %v", err)
	}

	// A sync run.
	run, err := st.OpenRun(ws.ID, "main", "startHash")
	if err != nil {
		t.Fatalf("OpenRun: %v", err)
	}

	// Branches.
	if err := st.SaveBranch(contract.Branch{
		RunID: run.RunID, Name: "graft/r/agent/foo", Kind: contract.BranchAgent, HeadHash: "hh",
	}); err != nil {
		t.Fatalf("SaveBranch: %v", err)
	}
	if err := st.SaveBranch(contract.Branch{
		RunID: run.RunID, Name: "graft/r/beta/1", Kind: contract.BranchBeta, HeadHash: "bh",
	}); err != nil {
		t.Fatalf("SaveBranch beta: %v", err)
	}

	// Conflicts.
	if err := st.SaveConflict(run.RunID, contract.Conflict{Path: "a.md", Agent: "agent-one"}); err != nil {
		t.Fatalf("SaveConflict: %v", err)
	}

	// Agent states.
	if err := st.SaveAgentState(contract.AgentState{
		RunID: run.RunID, AgentID: ag1.ID, InSync: false, Reason: "drift",
	}); err != nil {
		t.Fatalf("SaveAgentState: %v", err)
	}
	if err := st.SaveAgentState(contract.AgentState{
		RunID: run.RunID, AgentID: ag2.ID, InSync: true,
	}); err != nil {
		t.Fatalf("SaveAgentState 2: %v", err)
	}

	// Verify seeding is complete before returning.
	assertCount := func(table, where string, args []any, want int) {
		t.Helper()
		var n int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE "+where, args...).Scan(&n); err != nil {
			t.Fatalf("seed count(%s): %v", table, err)
		}
		if n != want {
			t.Fatalf("seed(%s): got %d rows, want %d", table, n, want)
		}
	}
	assertCount("workspaces", "id=?", []any{ws.ID}, 1)
	assertCount("agents", "ws_id=?", []any{ws.ID}, 2)
	assertCount("sync_runs", "ws_id=?", []any{ws.ID}, 1)
	assertCount("branches", "run_id=?", []any{run.RunID}, 2)
	assertCount("conflicts", "run_id=?", []any{run.RunID}, 1)
	assertCount("agent_states", "agent_id=?", []any{ag1.ID}, 1)
	assertCount("provider_links", "agent_id=?", []any{ag1.ID}, 1)

	return seedIDs{
		wsID:     ws.ID,
		agentID:  ag1.ID,
		agentID2: ag2.ID,
		runID:    run.RunID,
	}
}

// TestDeleteWorkspaceCascade seeds a full workspace, deletes it, and asserts
// ALL dependent rows are gone with no orphans left in any table.
func TestDeleteWorkspaceCascade(t *testing.T) {
	st := openTemp(t)
	s := concrete(t, st)
	ids := seedFullWorkspace(t, st, "/repo-del")

	if err := st.DeleteWorkspace(ids.wsID); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}

	// Helper: assert a table has 0 rows for the given condition.
	assertGone := func(table, where string, args ...any) {
		t.Helper()
		var n int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE "+where, args...).Scan(&n); err != nil {
			t.Fatalf("assertGone(%s): %v", table, err)
		}
		if n != 0 {
			t.Errorf("orphan rows in %s (where %s): got %d, want 0", table, where, n)
		}
	}

	// Root row gone.
	assertGone("workspaces", "id=?", ids.wsID)
	// All agents for this workspace gone.
	assertGone("agents", "ws_id=?", ids.wsID)
	// Sync runs gone.
	assertGone("sync_runs", "ws_id=?", ids.wsID)
	// Branches gone (via run_id).
	assertGone("branches", "run_id=?", ids.runID)
	// Conflicts gone (via run_id).
	assertGone("conflicts", "run_id=?", ids.runID)
	// Agent states gone (via agent_id).
	assertGone("agent_states", "agent_id=?", ids.agentID)
	assertGone("agent_states", "agent_id=?", ids.agentID2)
	// Provider links gone (via agent_id).
	assertGone("provider_links", "agent_id=?", ids.agentID)
}

// TestDeleteWorkspaceOtherUntouched seeds TWO workspaces, deletes one, and
// asserts the other and all its children remain completely intact.
func TestDeleteWorkspaceOtherUntouched(t *testing.T) {
	st := openTemp(t)
	s := concrete(t, st)

	keep := seedFullWorkspace(t, st, "/repo-keep")
	del := seedFullWorkspace(t, st, "/repo-del")

	if err := st.DeleteWorkspace(del.wsID); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}

	// Deleted workspace rows are gone.
	var n int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM workspaces WHERE id=?", del.wsID).Scan(&n); err != nil {
		t.Fatalf("del workspace check: %v", err)
	}
	if n != 0 {
		t.Errorf("deleted workspace still present: %d rows", n)
	}

	// Kept workspace and its children are untouched.
	check := func(table, where string, want int, args ...any) {
		t.Helper()
		var got int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE "+where, args...).Scan(&got); err != nil {
			t.Fatalf("check(%s): %v", table, err)
		}
		if got != want {
			t.Errorf("kept %s (where %s): got %d, want %d", table, where, got, want)
		}
	}
	check("workspaces", "id=?", 1, keep.wsID)
	check("agents", "ws_id=?", 2, keep.wsID)
	check("sync_runs", "ws_id=?", 1, keep.wsID)
	check("branches", "run_id=?", 2, keep.runID)
	check("conflicts", "run_id=?", 1, keep.runID)
	check("agent_states", "agent_id=?", 1, keep.agentID)
	check("provider_links", "agent_id=?", 1, keep.agentID)
}

// TestDeleteWorkspaceNonExistent deletes a workspace ID that was never
// inserted. This should succeed without error (DELETE of 0 rows is not an error).
func TestDeleteWorkspaceNonExistent(t *testing.T) {
	st := openTemp(t)
	if err := st.DeleteWorkspace("no-such-id"); err != nil {
		t.Fatalf("DeleteWorkspace non-existent: %v", err)
	}
}

// TestDeleteWorkspaceIdempotent calls DeleteWorkspace twice on the same ID.
// The second call must succeed (no rows to delete; FK is satisfied vacuously).
func TestDeleteWorkspaceIdempotent(t *testing.T) {
	st := openTemp(t)
	ids := seedFullWorkspace(t, st, "/repo-idem")

	if err := st.DeleteWorkspace(ids.wsID); err != nil {
		t.Fatalf("DeleteWorkspace first: %v", err)
	}
	if err := st.DeleteWorkspace(ids.wsID); err != nil {
		t.Fatalf("DeleteWorkspace second (idempotent): %v", err)
	}
}

// TestDeleteWorkspaceNoOrphansAnywhere seeds one workspace, deletes it, and
// checks every table is entirely empty — no orphan rows remain anywhere in the DB.
func TestDeleteWorkspaceNoOrphansAnywhere(t *testing.T) {
	st := openTemp(t)
	s := concrete(t, st)

	ids := seedFullWorkspace(t, st, "/repo-orphan")

	if err := st.DeleteWorkspace(ids.wsID); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}

	for _, tbl := range []string{
		"workspaces", "agents", "sync_runs",
		"branches", "conflicts", "agent_states", "provider_links",
	} {
		var n int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM " + tbl).Scan(&n); err != nil {
			t.Fatalf("total count(%s): %v", tbl, err)
		}
		if n != 0 {
			t.Errorf("orphan rows in %s: got %d, want 0", tbl, n)
		}
	}
}

// TestUpsertAgentCanonicalHashRoundTrip verifies that UpsertAgent persists and
// returns canonical_hash correctly on both insert and update. This is the
// store-side confirmation that plan-sync §meta (CanonicalHash) round-trips
// through the store layer unchanged.
func TestUpsertAgentCanonicalHashRoundTrip(t *testing.T) {
	st := openTemp(t)
	ws, err := st.Workspace("/repo", "origin", "main", contract.GitTracked)
	if err != nil {
		t.Fatalf("Workspace: %v", err)
	}

	const hash1 = "sha256:aabbccddeeff001122334455"
	const hash2 = "sha256:ffeeddc cbbaa998877665544"

	// Insert: canonical_hash must be returned verbatim.
	a, err := st.UpsertAgent(contract.Agent{WsID: ws.ID, Name: "canon-test", CanonicalHash: hash1})
	if err != nil {
		t.Fatalf("UpsertAgent insert: %v", err)
	}
	if a.CanonicalHash != hash1 {
		t.Fatalf("insert: canonical_hash=%q, want %q", a.CanonicalHash, hash1)
	}

	// Update: new hash must replace the old one; the agent ID is stable.
	a2, err := st.UpsertAgent(contract.Agent{WsID: ws.ID, Name: "canon-test", CanonicalHash: hash2})
	if err != nil {
		t.Fatalf("UpsertAgent update: %v", err)
	}
	if a2.ID != a.ID {
		t.Fatalf("agent ID changed on update: %q → %q", a.ID, a2.ID)
	}
	if a2.CanonicalHash != hash2 {
		t.Fatalf("update: canonical_hash=%q, want %q", a2.CanonicalHash, hash2)
	}

	// Re-read via a second UpsertAgent call (idempotent with same hash): still
	// returns the exact hash — no silent truncation or mangling.
	a3, err := st.UpsertAgent(contract.Agent{WsID: ws.ID, Name: "canon-test", CanonicalHash: hash2})
	if err != nil {
		t.Fatalf("UpsertAgent re-read: %v", err)
	}
	if a3.CanonicalHash != hash2 {
		t.Fatalf("re-read: canonical_hash=%q, want %q", a3.CanonicalHash, hash2)
	}
	if a3.ID != a.ID {
		t.Fatalf("agent ID not stable on re-read: %q vs %q", a3.ID, a.ID)
	}

	// Verify via raw SQL that the DB row holds the final hash.
	s := concrete(t, st)
	var dbHash string
	if err := s.db.QueryRow(
		`SELECT canonical_hash FROM agents WHERE id=?`, a.ID,
	).Scan(&dbHash); err != nil {
		t.Fatalf("raw SQL read: %v", err)
	}
	if dbHash != hash2 {
		t.Fatalf("DB row: canonical_hash=%q, want %q", dbHash, hash2)
	}
}
