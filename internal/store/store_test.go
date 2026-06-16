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
	// With zero migration files, a fresh install must still be marked initialized
	// at pointer 0, so a later first migration is not skipped as "fresh".
	var pointer, inited int
	if err := s.db.QueryRow(
		`SELECT pointer_value, is_initialized FROM schema_migration`,
	).Scan(&pointer, &inited); err != nil {
		t.Fatalf("schema_migration state: %v", err)
	}
	if pointer != 0 || inited != 1 {
		t.Fatalf("fresh install state = (pointer=%d, initialized=%d), want (0, 1)", pointer, inited)
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
	// Create as tracked: git_mode must round-trip, not be hardcoded.
	ws1, err := st.Workspace("/repo", "origin", "main", contract.GitTracked)
	if err != nil {
		t.Fatalf("Workspace: %v", err)
	}
	if ws1.ID == "" || ws1.GitMode != contract.GitTracked || ws1.CreatedAt == 0 {
		t.Fatalf("unexpected workspace: %+v", ws1)
	}
	// Same identity -> same row (id + created_at preserved), mode stays tracked.
	ws2, err := st.Workspace("/repo", "origin", "main", contract.GitTracked)
	if err != nil {
		t.Fatalf("Workspace repeat: %v", err)
	}
	if ws2.ID != ws1.ID || ws2.CreatedAt != ws1.CreatedAt {
		t.Fatalf("identity not stable: %+v vs %+v", ws1, ws2)
	}
	if ws2.GitMode != contract.GitTracked {
		t.Fatalf("git_mode wrong on repeat: %+v", ws2)
	}
	// Re-read confirms mode persisted.
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

// TestWorkspaceInternalModeSticky verifies that once a workspace is stored with
// git_mode=internal, a subsequent re-upsert with git_mode=tracked does NOT
// downgrade the stored mode. The invariant: internal is sticky once set.
// Control cases confirm that tracked->tracked and tracked->internal both work normally.
func TestWorkspaceInternalModeSticky(t *testing.T) {
	st := openTemp(t)

	// --- Primary case: internal must not be downgraded to tracked on re-init ---
	ws1, err := st.Workspace("/internal-repo", "origin", "main", contract.GitInternal)
	if err != nil {
		t.Fatalf("Workspace internal insert: %v", err)
	}
	if ws1.GitMode != contract.GitInternal {
		t.Fatalf("initial insert: want GitInternal, got %v", ws1.GitMode)
	}

	// Re-upsert same identity with tracked — the returned row must still be internal.
	ws2, err := st.Workspace("/internal-repo", "origin", "main", contract.GitTracked)
	if err != nil {
		t.Fatalf("Workspace re-upsert with tracked: %v", err)
	}
	if ws2.GitMode != contract.GitInternal {
		t.Fatalf("re-upsert must not downgrade internal: got %v, want %v", ws2.GitMode, contract.GitInternal)
	}
	// Identity (id + created_at) must be stable.
	if ws2.ID != ws1.ID || ws2.CreatedAt != ws1.CreatedAt {
		t.Fatalf("identity changed on re-upsert: %+v vs %+v", ws1, ws2)
	}

	// Fresh read via FindWorkspace must also show internal.
	found, err := st.FindWorkspace("/internal-repo", "origin", "main")
	if err != nil {
		t.Fatalf("FindWorkspace: %v", err)
	}
	if found == nil {
		t.Fatal("FindWorkspace returned nil, expected row")
	}
	if found.GitMode != contract.GitInternal {
		t.Fatalf("FindWorkspace: want GitInternal, got %v", found.GitMode)
	}

	// --- Control 1: tracked -> tracked (normal update, should stay tracked) ---
	wt1, err := st.Workspace("/tracked-repo", "origin", "main", contract.GitTracked)
	if err != nil {
		t.Fatalf("Workspace tracked insert: %v", err)
	}
	wt2, err := st.Workspace("/tracked-repo", "origin", "main", contract.GitTracked)
	if err != nil {
		t.Fatalf("Workspace tracked re-upsert: %v", err)
	}
	if wt2.GitMode != contract.GitTracked {
		t.Fatalf("tracked->tracked: want GitTracked, got %v", wt2.GitMode)
	}
	if wt2.ID != wt1.ID {
		t.Fatalf("tracked identity not stable")
	}

	// --- Control 2: tracked -> internal (allowed; non-internal can be upgraded) ---
	wi1, err := st.Workspace("/upgrade-repo", "origin", "main", contract.GitTracked)
	if err != nil {
		t.Fatalf("Workspace tracked insert for upgrade: %v", err)
	}
	wi2, err := st.Workspace("/upgrade-repo", "origin", "main", contract.GitInternal)
	if err != nil {
		t.Fatalf("Workspace upgrade to internal: %v", err)
	}
	if wi2.GitMode != contract.GitInternal {
		t.Fatalf("tracked->internal: want GitInternal, got %v", wi2.GitMode)
	}
	if wi2.ID != wi1.ID {
		t.Fatalf("upgraded identity not stable")
	}
}

func TestFindWorkspace(t *testing.T) {
	st := openTemp(t)

	// Absent -> (nil, nil), and no side-effect insert.
	got, err := st.FindWorkspace("/repo", "origin", "main")
	if err != nil {
		t.Fatalf("FindWorkspace absent: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for absent workspace, got %+v", got)
	}
	s := concrete(t, st)
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM workspaces`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("FindWorkspace must not insert; workspaces=%d", n)
	}

	// Present -> returns the exact row created by Workspace.
	want, err := st.Workspace("/repo", "origin", "main", contract.GitInternal)
	if err != nil {
		t.Fatalf("Workspace: %v", err)
	}
	got, err = st.FindWorkspace("/repo", "origin", "main")
	if err != nil {
		t.Fatalf("FindWorkspace present: %v", err)
	}
	if got == nil {
		t.Fatalf("expected workspace, got nil")
	}
	if *got != want {
		t.Fatalf("FindWorkspace mismatch: got %+v want %+v", *got, want)
	}

	// A different identity is still absent (no accidental match).
	other, err := st.FindWorkspace("/repo", "origin", "dev")
	if err != nil {
		t.Fatalf("FindWorkspace other: %v", err)
	}
	if other != nil {
		t.Fatalf("expected nil for different branch, got %+v", other)
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

// TestAgentSynced proves AgentSynced is true ONLY when an agents row AND ≥1
// provider_links row exist — the robust "a prior sync COMPLETED" discriminator
// the deletion path relies on (v0.0.4 verify r2 HIGH 2). The middle case — an
// agents row with ZERO provider_links (e.g. a prior ABORTED run that called
// UpsertAgent in prepareBranches but never reached applyProviders) — MUST read
// as NOT synced; the old Drift-reason probe mis-read it as "known" and would
// delete a genuinely-new provider-authored agent.
func TestAgentSynced(t *testing.T) {
	st := openTemp(t)
	ws, _ := st.Workspace("/repo", "origin", "main", contract.GitTracked)

	// 1. No agents row at all -> not synced.
	if synced, err := st.AgentSynced(ws.ID, "ghost"); err != nil || synced {
		t.Fatalf("no agents row: AgentSynced=(%v,%v), want (false,nil)", synced, err)
	}

	// 2. Agents row but ZERO provider_links (orphan / aborted-run state) -> NOT
	//    synced. This is the HIGH 2 case.
	a, err := st.UpsertAgent(contract.Agent{WsID: ws.ID, Name: "foo", CanonicalHash: "CANON"})
	if err != nil {
		t.Fatalf("UpsertAgent: %v", err)
	}
	if synced, err := st.AgentSynced(ws.ID, "foo"); err != nil || synced {
		t.Fatalf("orphan agents row (no links): AgentSynced=(%v,%v), want (false,nil)", synced, err)
	}

	// 3. Agents row + ≥1 provider_links row -> synced.
	if err := st.UpsertProviderLink(contract.ProviderLink{
		AgentID: a.ID, Provider: "claudecode", ContentHash: "CANON",
	}); err != nil {
		t.Fatalf("UpsertProviderLink: %v", err)
	}
	if synced, err := st.AgentSynced(ws.ID, "foo"); err != nil || !synced {
		t.Fatalf("agents row + link: AgentSynced=(%v,%v), want (true,nil)", synced, err)
	}
}

func TestProviderLinkEnforcesAgentFK(t *testing.T) {
	// With foreign_keys ON and no lazy placeholder, UpsertProviderLink for an
	// unknown agent must be rejected by the agents FK. Production order is
	// workspace -> agent -> provider_link, so the agent must exist.
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

func TestTwoDistinctUnknownAgentsRejected(t *testing.T) {
	// Regression lock for the placeholder-collision bug: two DISTINCT unknown
	// agents must each be rejected by the FK (no silent suppression), and once
	// each is created via UpsertAgent both child writes must succeed
	// independently — proving there is no shared placeholder identity.
	st := openTemp(t)
	ws, _ := st.Workspace("/repo", "origin", "main", contract.GitTracked)

	// Both unknown -> both rejected (no UNIQUE collision masking the failure).
	if err := st.UpsertProviderLink(contract.ProviderLink{
		AgentID: "ghost-1", Provider: "claudecode", ContentHash: "h",
	}); err == nil {
		t.Fatalf("expected FK rejection for ghost-1, got nil")
	}
	if err := st.SaveAgentState(contract.AgentState{
		RunID: "no-run", AgentID: "ghost-2", InSync: true,
	}); err == nil {
		t.Fatalf("expected FK rejection for ghost-2, got nil")
	}

	// Create two distinct real agents, then a run; child writes for BOTH succeed.
	a1, err := st.UpsertAgent(contract.Agent{WsID: ws.ID, Name: "alpha", CanonicalHash: "h1"})
	if err != nil {
		t.Fatalf("UpsertAgent alpha: %v", err)
	}
	a2, err := st.UpsertAgent(contract.Agent{WsID: ws.ID, Name: "beta", CanonicalHash: "h2"})
	if err != nil {
		t.Fatalf("UpsertAgent beta: %v", err)
	}
	if a1.ID == a2.ID {
		t.Fatalf("distinct agents share an id: %q", a1.ID)
	}
	run, _ := st.OpenRun(ws.ID, "main", "h")

	for _, a := range []contract.Agent{a1, a2} {
		if err := st.UpsertProviderLink(contract.ProviderLink{
			AgentID: a.ID, Provider: "claudecode", ContentHash: "h",
		}); err != nil {
			t.Fatalf("UpsertProviderLink %s: %v", a.Name, err)
		}
		if err := st.SaveAgentState(contract.AgentState{
			RunID: run.RunID, AgentID: a.ID, InSync: true,
		}); err != nil {
			t.Fatalf("SaveAgentState %s: %v", a.Name, err)
		}
	}

	s := concrete(t, st)
	var links, states int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM provider_links`).Scan(&links); err != nil {
		t.Fatalf("link count: %v", err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM agent_states`).Scan(&states); err != nil {
		t.Fatalf("state count: %v", err)
	}
	if links != 2 || states != 2 {
		t.Fatalf("want 2 links + 2 states, got links=%d states=%d", links, states)
	}
}
