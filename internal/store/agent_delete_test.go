package store

import (
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// TestDeleteAgentCascade inserts a workspace, an agent, a provider_link, and an
// agent_state, then verifies that DeleteAgent removes the agent row and both
// dependent rows while leaving the workspace row intact.
func TestDeleteAgentCascade(t *testing.T) {
	st := openTemp(t)
	s := concrete(t, st)

	ws, err := st.Workspace("/repo-agent-del", "origin", "main", contract.GitTracked)
	if err != nil {
		t.Fatalf("Workspace: %v", err)
	}

	ag, err := st.UpsertAgent(contract.Agent{WsID: ws.ID, Name: "target-agent", CanonicalHash: "HASH1"})
	if err != nil {
		t.Fatalf("UpsertAgent: %v", err)
	}

	if err := st.UpsertProviderLink(contract.ProviderLink{
		AgentID: ag.ID, Provider: "claude-code", FilePath: "p1", ContentHash: "HASH1",
	}); err != nil {
		t.Fatalf("UpsertProviderLink: %v", err)
	}

	run, err := st.OpenRun(ws.ID, "main", "startHash")
	if err != nil {
		t.Fatalf("OpenRun: %v", err)
	}

	if err := st.SaveAgentState(contract.AgentState{
		RunID: run.RunID, AgentID: ag.ID, InSync: false, Reason: "drift",
	}); err != nil {
		t.Fatalf("SaveAgentState: %v", err)
	}

	// Confirm rows exist before deletion.
	assertCount := func(table, where string, args []any, want int) {
		t.Helper()
		var n int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE "+where, args...).Scan(&n); err != nil {
			t.Fatalf("count(%s): %v", table, err)
		}
		if n != want {
			t.Fatalf("%s (where %s): got %d rows, want %d", table, where, n, want)
		}
	}
	assertCount("agents", "id=?", []any{ag.ID}, 1)
	assertCount("provider_links", "agent_id=?", []any{ag.ID}, 1)
	assertCount("agent_states", "agent_id=?", []any{ag.ID}, 1)
	assertCount("workspaces", "id=?", []any{ws.ID}, 1)

	if err := st.DeleteAgent(ws.ID, "target-agent"); err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}

	// Agent and its children must be gone.
	assertCount("agents", "id=?", []any{ag.ID}, 0)
	assertCount("provider_links", "agent_id=?", []any{ag.ID}, 0)
	assertCount("agent_states", "agent_id=?", []any{ag.ID}, 0)

	// Workspace must still exist.
	assertCount("workspaces", "id=?", []any{ws.ID}, 1)

	// Sync run must still exist (run-scoped, not agent-scoped).
	assertCount("sync_runs", "run_id=?", []any{run.RunID}, 1)
}

// TestDeleteAgentUnknownNameIsNoOp verifies that DeleteAgent on a (wsID, name)
// pair that has no agents row returns nil without error.
func TestDeleteAgentUnknownNameIsNoOp(t *testing.T) {
	st := openTemp(t)

	ws, err := st.Workspace("/repo-agent-noop", "origin", "main", contract.GitTracked)
	if err != nil {
		t.Fatalf("Workspace: %v", err)
	}

	if err := st.DeleteAgent(ws.ID, "ghost-agent"); err != nil {
		t.Fatalf("DeleteAgent on unknown name: expected nil, got %v", err)
	}
}

// TestDeleteAgentOtherAgentUntouched seeds two agents under the same workspace,
// deletes one, and asserts the other and its children are completely intact.
func TestDeleteAgentOtherAgentUntouched(t *testing.T) {
	st := openTemp(t)
	s := concrete(t, st)

	ws, err := st.Workspace("/repo-agent-keep", "origin", "main", contract.GitTracked)
	if err != nil {
		t.Fatalf("Workspace: %v", err)
	}

	keep, err := st.UpsertAgent(contract.Agent{WsID: ws.ID, Name: "keeper", CanonicalHash: "KEEP"})
	if err != nil {
		t.Fatalf("UpsertAgent keep: %v", err)
	}
	del, err := st.UpsertAgent(contract.Agent{WsID: ws.ID, Name: "goner", CanonicalHash: "GONE"})
	if err != nil {
		t.Fatalf("UpsertAgent del: %v", err)
	}

	for _, ag := range []contract.Agent{keep, del} {
		if err := st.UpsertProviderLink(contract.ProviderLink{
			AgentID: ag.ID, Provider: "claude-code", FilePath: ag.Name + ".md", ContentHash: ag.CanonicalHash,
		}); err != nil {
			t.Fatalf("UpsertProviderLink %s: %v", ag.Name, err)
		}
	}

	run, err := st.OpenRun(ws.ID, "main", "h")
	if err != nil {
		t.Fatalf("OpenRun: %v", err)
	}

	for _, ag := range []contract.Agent{keep, del} {
		if err := st.SaveAgentState(contract.AgentState{
			RunID: run.RunID, AgentID: ag.ID, InSync: true,
		}); err != nil {
			t.Fatalf("SaveAgentState %s: %v", ag.Name, err)
		}
	}

	if err := st.DeleteAgent(ws.ID, "goner"); err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}

	// "goner" and its children gone.
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
	assertGone("agents", "id=?", del.ID)
	assertGone("provider_links", "agent_id=?", del.ID)
	assertGone("agent_states", "agent_id=?", del.ID)

	// "keeper" and its children untouched.
	assertPresent := func(table, where string, want int, args ...any) {
		t.Helper()
		var n int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE "+where, args...).Scan(&n); err != nil {
			t.Fatalf("assertPresent(%s): %v", table, err)
		}
		if n != want {
			t.Errorf("kept %s (where %s): got %d, want %d", table, where, n, want)
		}
	}
	assertPresent("agents", "id=?", 1, keep.ID)
	assertPresent("provider_links", "agent_id=?", 1, keep.ID)
	assertPresent("agent_states", "agent_id=?", 1, keep.ID)
	assertPresent("workspaces", "id=?", 1, ws.ID)
}

// TestDeleteAgentIdempotent verifies that calling DeleteAgent twice on the same
// (wsID, name) is safe: the second call must return nil.
func TestDeleteAgentIdempotent(t *testing.T) {
	st := openTemp(t)

	ws, err := st.Workspace("/repo-agent-idem", "origin", "main", contract.GitTracked)
	if err != nil {
		t.Fatalf("Workspace: %v", err)
	}
	if _, err := st.UpsertAgent(contract.Agent{WsID: ws.ID, Name: "idem-agent", CanonicalHash: "H"}); err != nil {
		t.Fatalf("UpsertAgent: %v", err)
	}

	if err := st.DeleteAgent(ws.ID, "idem-agent"); err != nil {
		t.Fatalf("DeleteAgent first: %v", err)
	}
	if err := st.DeleteAgent(ws.ID, "idem-agent"); err != nil {
		t.Fatalf("DeleteAgent second (idempotent): %v", err)
	}
}
