package store

import (
	"path/filepath"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// openTemp opens a fresh store backed by a temp-dir sqlite file.
func openTemp(t *testing.T) contract.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "graft.db")
	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// concrete reaches the *sqlStore for white-box agent seeding (no contract
// method creates agent rows; tests seed canonical hashes directly).
func concrete(t *testing.T, st contract.Store) *sqlStore {
	t.Helper()
	s, ok := st.(*sqlStore)
	if !ok {
		t.Fatalf("store is not *sqlStore")
	}
	return s
}

func TestMigrationApplied(t *testing.T) {
	st := openTemp(t)
	s := concrete(t, st)
	// schema_migration table exists and has exactly one row.
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM schema_migration`).Scan(&n); err != nil {
		t.Fatalf("schema_migration query: %v", err)
	}
	if n != 1 {
		t.Fatalf("schema_migration rows = %d, want 1", n)
	}
	// All 7 base tables are present.
	for _, tbl := range []string{
		"workspaces", "sync_runs", "branches", "agents",
		"provider_links", "agent_states", "conflicts",
	} {
		var name string
		err := s.db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl,
		).Scan(&name)
		if err != nil {
			t.Fatalf("table %q missing: %v", tbl, err)
		}
	}
}

func TestWorkspaceUpsertIdentity(t *testing.T) {
	st := openTemp(t)
	// Create as internal: git_mode must round-trip, not be hardcoded.
	ws1, err := st.Workspace("/repo", "origin", "main", contract.GitInternal)
	if err != nil {
		t.Fatalf("Workspace: %v", err)
	}
	if ws1.ID == "" || ws1.GitMode != contract.GitInternal || ws1.CreatedAt == 0 {
		t.Fatalf("unexpected workspace: %+v", ws1)
	}
	// Same identity -> same row (id + created_at preserved), and mode updated to
	// tracked (internal->tracked migration per plan-02).
	ws2, err := st.Workspace("/repo", "origin", "main", contract.GitTracked)
	if err != nil {
		t.Fatalf("Workspace repeat: %v", err)
	}
	if ws2.ID != ws1.ID || ws2.CreatedAt != ws1.CreatedAt {
		t.Fatalf("identity not stable: %+v vs %+v", ws1, ws2)
	}
	if ws2.GitMode != contract.GitTracked {
		t.Fatalf("git_mode not updated to tracked: %+v", ws2)
	}
	// Re-read confirms the updated mode persisted.
	ws2b, err := st.Workspace("/repo", "origin", "main", contract.GitTracked)
	if err != nil {
		t.Fatalf("Workspace re-read: %v", err)
	}
	if ws2b.GitMode != contract.GitTracked {
		t.Fatalf("git_mode did not persist: %+v", ws2b)
	}
	// Different branch -> different row, with its own (internal) mode.
	ws3, err := st.Workspace("/repo", "origin", "dev", contract.GitInternal)
	if err != nil {
		t.Fatalf("Workspace dev: %v", err)
	}
	if ws3.ID == ws1.ID {
		t.Fatalf("expected distinct workspace for different branch")
	}
	if ws3.GitMode != contract.GitInternal {
		t.Fatalf("dev workspace git_mode wrong: %+v", ws3)
	}
}

func TestRunRoundTripAndUpdate(t *testing.T) {
	st := openTemp(t)
	ws, _ := st.Workspace("/repo", "origin", "main", contract.GitTracked)

	run, err := st.OpenRun(ws.ID, "main", "abc123")
	if err != nil {
		t.Fatalf("OpenRun: %v", err)
	}
	if run.Status != contract.RunRunning || run.StartedAt == 0 {
		t.Fatalf("unexpected run: %+v", run)
	}

	run.Status = contract.RunConflict
	run.Phase = "merge"
	run.BetaBranch = "graft/" + run.RunID + "/beta/1"
	if err := st.UpdateRun(run); err != nil {
		t.Fatalf("UpdateRun: %v", err)
	}

	got, err := st.OpenConflictRun(ws.ID)
	if err != nil {
		t.Fatalf("OpenConflictRun: %v", err)
	}
	if got == nil {
		t.Fatalf("expected a resumable run")
	}
	if got.RunID != run.RunID || got.Phase != "merge" || got.BetaBranch != run.BetaBranch {
		t.Fatalf("resume mismatch: %+v", got)
	}
}

func TestOpenConflictRunNone(t *testing.T) {
	st := openTemp(t)
	ws, _ := st.Workspace("/repo", "origin", "main", contract.GitTracked)
	if _, err := st.OpenRun(ws.ID, "main", "h"); err != nil {
		t.Fatalf("OpenRun: %v", err)
	}
	// Running (not conflict) -> nothing to resume.
	got, err := st.OpenConflictRun(ws.ID)
	if err != nil {
		t.Fatalf("OpenConflictRun: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestBranchesRoundTrip(t *testing.T) {
	st := openTemp(t)
	ws, _ := st.Workspace("/repo", "origin", "main", contract.GitTracked)
	run, _ := st.OpenRun(ws.ID, "main", "h")

	a := contract.Branch{RunID: run.RunID, Name: "graft/r/agent/foo", Kind: contract.BranchAgent, HeadHash: "h1", State: "merged"}
	b := contract.Branch{RunID: run.RunID, Name: "graft/r/beta/1", Kind: contract.BranchBeta, HeadHash: "h2", State: "active"}
	if err := st.SaveBranch(a); err != nil {
		t.Fatalf("SaveBranch a: %v", err)
	}
	if err := st.SaveBranch(b); err != nil {
		t.Fatalf("SaveBranch b: %v", err)
	}
	// Upsert by (run_id, name): update head hash, expect no new row.
	a.HeadHash = "h1b"
	if err := st.SaveBranch(a); err != nil {
		t.Fatalf("SaveBranch a update: %v", err)
	}

	got, err := st.Branches(run.RunID)
	if err != nil {
		t.Fatalf("Branches: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 branches, got %d", len(got))
	}
	var foundUpdated bool
	for _, br := range got {
		if br.Name == "graft/r/agent/foo" {
			if br.HeadHash != "h1b" || br.Kind != contract.BranchAgent {
				t.Fatalf("branch not updated: %+v", br)
			}
			foundUpdated = true
		}
	}
	if !foundUpdated {
		t.Fatalf("updated branch missing")
	}
}

func TestSaveConflict(t *testing.T) {
	st := openTemp(t)
	ws, _ := st.Workspace("/repo", "origin", "main", contract.GitTracked)
	run, _ := st.OpenRun(ws.ID, "main", "h")

	if err := st.SaveConflict(run.RunID, contract.Conflict{Path: "a.md", Agent: "foo"}); err != nil {
		t.Fatalf("SaveConflict: %v", err)
	}
	// Repeat same path is idempotent (no duplicate, no error).
	if err := st.SaveConflict(run.RunID, contract.Conflict{Path: "a.md", Agent: "foo2"}); err != nil {
		t.Fatalf("SaveConflict repeat: %v", err)
	}

	s := concrete(t, st)
	var count int
	var agent, status string
	if err := s.db.QueryRow(
		`SELECT COUNT(*), MAX(agent_name), MAX(status) FROM conflicts WHERE run_id=?`, run.RunID,
	).Scan(&count, &agent, &status); err != nil {
		t.Fatalf("conflict query: %v", err)
	}
	if count != 1 || agent != "foo2" || status != "open" {
		t.Fatalf("conflict state wrong: count=%d agent=%q status=%q", count, agent, status)
	}
}

func TestSaveAgentState(t *testing.T) {
	st := openTemp(t)
	// Production order: workspace -> agent -> run -> agent_state (FKs enforced).
	ws, _ := st.Workspace("/repo", "origin", "main", contract.GitTracked)
	agent, err := st.UpsertAgent(contract.Agent{WsID: ws.ID, Name: "agent-1", CanonicalHash: "h"})
	if err != nil {
		t.Fatalf("UpsertAgent: %v", err)
	}
	agentID := agent.ID
	run, _ := st.OpenRun(ws.ID, "main", "h")

	if err := st.SaveAgentState(contract.AgentState{
		RunID: run.RunID, AgentID: agentID, InSync: false, Reason: "drift",
	}); err != nil {
		t.Fatalf("SaveAgentState: %v", err)
	}
	// Upsert by (run_id, agent_id).
	if err := st.SaveAgentState(contract.AgentState{
		RunID: run.RunID, AgentID: agentID, InSync: true, Reason: "",
	}); err != nil {
		t.Fatalf("SaveAgentState update: %v", err)
	}

	s := concrete(t, st)
	var inSync int
	var count int
	if err := s.db.QueryRow(
		`SELECT COUNT(*), MAX(in_sync) FROM agent_states WHERE run_id=? AND agent_id=?`,
		run.RunID, agentID,
	).Scan(&count, &inSync); err != nil {
		t.Fatalf("agent_states query: %v", err)
	}
	if count != 1 || inSync != 1 {
		t.Fatalf("agent state wrong: count=%d in_sync=%d", count, inSync)
	}
}

func TestUpsertProviderLinkAndDrift(t *testing.T) {
	st := openTemp(t)
	ws, _ := st.Workspace("/repo", "origin", "main", contract.GitTracked)
	s := concrete(t, st)

	// Seed an agent row with a canonical hash (no contract method does this).
	agentID := "agent-x"
	if _, err := s.db.Exec(
		`INSERT INTO agents (id, ws_id, name, canonical_hash) VALUES (?, ?, 'foo', 'CANON')`,
		agentID, ws.ID,
	); err != nil {
		t.Fatalf("seed agent: %v", err)
	}

	// In-sync provider link.
	if err := st.UpsertProviderLink(contract.ProviderLink{
		AgentID: agentID, Provider: "claudecode", FilePath: "p", ContentHash: "CANON", CommitHash: "c1",
	}); err != nil {
		t.Fatalf("UpsertProviderLink: %v", err)
	}
	drifted, reason, err := st.Drift(ws.ID, "foo")
	if err != nil {
		t.Fatalf("Drift: %v", err)
	}
	if drifted {
		t.Fatalf("expected no drift, got reason %q", reason)
	}

	// Diverging provider -> drift.
	if err := st.UpsertProviderLink(contract.ProviderLink{
		AgentID: agentID, Provider: "codex", FilePath: "q", ContentHash: "OTHER", CommitHash: "c2",
	}); err != nil {
		t.Fatalf("UpsertProviderLink codex: %v", err)
	}
	drifted, reason, err = st.Drift(ws.ID, "foo")
	if err != nil {
		t.Fatalf("Drift: %v", err)
	}
	if !drifted || reason == "" {
		t.Fatalf("expected drift, got drifted=%v reason=%q", drifted, reason)
	}

	// Upsert by (agent_id, provider) updates rather than duplicates.
	if err := st.UpsertProviderLink(contract.ProviderLink{
		AgentID: agentID, Provider: "codex", FilePath: "q", ContentHash: "CANON", CommitHash: "c3",
	}); err != nil {
		t.Fatalf("UpsertProviderLink codex update: %v", err)
	}
	var links int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM provider_links WHERE agent_id=?`, agentID,
	).Scan(&links); err != nil {
		t.Fatalf("link count: %v", err)
	}
	if links != 2 {
		t.Fatalf("want 2 provider links, got %d", links)
	}
	drifted, _, err = st.Drift(ws.ID, "foo")
	if err != nil {
		t.Fatalf("Drift: %v", err)
	}
	if drifted {
		t.Fatalf("expected no drift after realign")
	}
}

func TestDriftUntracked(t *testing.T) {
	st := openTemp(t)
	ws, _ := st.Workspace("/repo", "origin", "main", contract.GitTracked)
	drifted, reason, err := st.Drift(ws.ID, "ghost")
	if err != nil {
		t.Fatalf("Drift: %v", err)
	}
	if drifted || reason == "" {
		t.Fatalf("untracked agent should not drift, reason=%q", reason)
	}
}

func TestUpsertAgentAndDriftReachable(t *testing.T) {
	// End-to-end proof that Drift is reachable via the public contract only:
	// UpsertAgent sets identity + canonical hash, UpsertProviderLink sets the
	// provider content hash, Drift compares them. No direct DB seeding.
	st := openTemp(t)
	ws, _ := st.Workspace("/repo", "origin", "main", contract.GitTracked)

	// Insert.
	a, err := st.UpsertAgent(contract.Agent{WsID: ws.ID, Name: "foo", CanonicalHash: "CANON"})
	if err != nil {
		t.Fatalf("UpsertAgent insert: %v", err)
	}
	if a.ID == "" || a.WsID != ws.ID || a.Name != "foo" || a.CanonicalHash != "CANON" {
		t.Fatalf("unexpected agent: %+v", a)
	}

	// Update by (ws_id, name): same id, new canonical hash, no duplicate row.
	a2, err := st.UpsertAgent(contract.Agent{WsID: ws.ID, Name: "foo", CanonicalHash: "CANON2"})
	if err != nil {
		t.Fatalf("UpsertAgent update: %v", err)
	}
	if a2.ID != a.ID {
		t.Fatalf("id not stable on update: %q vs %q", a.ID, a2.ID)
	}
	if a2.CanonicalHash != "CANON2" {
		t.Fatalf("canonical_hash not updated: %q", a2.CanonicalHash)
	}
	s := concrete(t, st)
	var rows int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM agents WHERE ws_id=? AND name='foo'`, ws.ID).Scan(&rows); err != nil {
		t.Fatalf("agent count: %v", err)
	}
	if rows != 1 {
		t.Fatalf("want 1 agent row, got %d", rows)
	}

	// In-sync provider -> Drift reachable, no drift.
	if err := st.UpsertProviderLink(contract.ProviderLink{
		AgentID: a2.ID, Provider: "claudecode", ContentHash: "CANON2",
	}); err != nil {
		t.Fatalf("UpsertProviderLink: %v", err)
	}
	drifted, reason, err := st.Drift(ws.ID, "foo")
	if err != nil {
		t.Fatalf("Drift: %v", err)
	}
	if drifted {
		t.Fatalf("expected no drift, got %q", reason)
	}

	// Diverging provider -> real drift verdict via public contract.
	if err := st.UpsertProviderLink(contract.ProviderLink{
		AgentID: a2.ID, Provider: "codex", ContentHash: "STALE",
	}); err != nil {
		t.Fatalf("UpsertProviderLink codex: %v", err)
	}
	drifted, reason, err = st.Drift(ws.ID, "foo")
	if err != nil {
		t.Fatalf("Drift: %v", err)
	}
	if !drifted || reason == "" {
		t.Fatalf("expected drift via public contract, got drifted=%v reason=%q", drifted, reason)
	}
}

func TestProviderLinkEnforcesAgentFK(t *testing.T) {
	// With foreign_keys ON, UpsertProviderLink for an unknown agent must be
	// rejected (the lazy ensure cannot fabricate a valid ws_id). Production
	// order is workspace -> agent -> provider_link, so the agent must exist.
	st := openTemp(t)
	if err := st.UpsertProviderLink(contract.ProviderLink{
		AgentID: "ghost", Provider: "claudecode", ContentHash: "h",
	}); err == nil {
		t.Fatalf("expected FK rejection for unknown agent, got nil")
	}

	// Proper order: create agent first, then the link succeeds.
	ws, _ := st.Workspace("/repo", "origin", "main", contract.GitTracked)
	agent, err := st.UpsertAgent(contract.Agent{WsID: ws.ID, Name: "foo", CanonicalHash: "h"})
	if err != nil {
		t.Fatalf("UpsertAgent: %v", err)
	}
	if err := st.UpsertProviderLink(contract.ProviderLink{
		AgentID: agent.ID, Provider: "claudecode", ContentHash: "h",
	}); err != nil {
		t.Fatalf("UpsertProviderLink after UpsertAgent: %v", err)
	}
	s := concrete(t, st)
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM provider_links WHERE agent_id=?`, agent.ID).Scan(&n); err != nil {
		t.Fatalf("link query: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 provider link, got %d", n)
	}
}
