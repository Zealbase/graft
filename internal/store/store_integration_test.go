package store

// SQL-level integration tests for the store (build phase e, the d<->e gate).
//
// These go BEYOND the white-box unit tests in store_test.go: they drive the
// public contract.Store end to end and then re-verify the persisted state with
// RAW SQL through a SEPARATE read-only connection (database.OpenReadOnly), so a
// pass proves the bytes actually landed on disk in another connection's WAL
// snapshot — not merely that a method returned without error.
//
// Tests here use only exported symbols plus the package-private *sqlStore seam
// (already used by the unit tests) when a raw handle is needed.

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/store/database"
)

// dbPath opens a store at a fresh temp path and returns both the store and the
// on-disk path so a second (read-only) connection can be opened to the SAME db.
func dbPath(t *testing.T) (contract.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "graft.db")
	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st, path
}

// readonly opens a fresh read-only connection to path and registers cleanup.
// Every verification query runs through this independent connection so the test
// reads committed bytes, not the writer's in-memory state.
func readonly(t *testing.T, path string) *sql.DB {
	t.Helper()
	ro, err := database.OpenReadOnly(path)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	t.Cleanup(func() { ro.Close() })
	return ro
}

func scalarInt(t *testing.T, db *sql.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("query %q: %v", q, err)
	}
	return n
}

// -----------------------------------------------------------------------------
// 1. Fresh install: every table + the migration pointer table exist and the
//    migration pointer is stamped correctly, verified by querying sqlite_master
//    and schema_migration directly through a read-only connection.
// -----------------------------------------------------------------------------

func TestIntegration_FreshInstallSchema(t *testing.T) {
	_, path := dbPath(t)
	ro := readonly(t, path)

	wantTables := []string{
		"workspaces", "sync_runs", "branches", "agents",
		"provider_links", "agent_states", "conflicts",
	}
	for _, tbl := range wantTables {
		n := scalarInt(t, ro,
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, tbl)
		if n != 1 {
			t.Errorf("table %q: sqlite_master count=%d, want 1", tbl, n)
		}
	}

	// schema_migration exists with exactly one bookkeeping row.
	if n := scalarInt(t, ro,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_migration'`); n != 1 {
		t.Fatalf("schema_migration table missing (count=%d)", n)
	}
	if n := scalarInt(t, ro, `SELECT COUNT(*) FROM schema_migration`); n != 1 {
		t.Fatalf("schema_migration row count=%d, want exactly 1", n)
	}

	// Pointer must equal the highest migration index. There are currently NO
	// files under database/schema/migrations, so "max" is 0 and the runner
	// correctly leaves the freshly-applied base schema un-stamped (pointer 0).
	// We assert the pointer equals the discovered max so this test self-adjusts
	// the day a migration file is added (it would then need to be stamped-to-max
	// by the runner; if it is not, this catches the regression).
	var ptr, init int
	if err := ro.QueryRow(`SELECT pointer_value, is_initialized FROM schema_migration`).Scan(&ptr, &init); err != nil {
		t.Fatalf("read pointer: %v", err)
	}
	wantMax := maxMigrationIndex(t)
	if ptr != wantMax {
		t.Errorf("migration pointer=%d, want stamped-to-max=%d", ptr, wantMax)
	}
	// is_initialized is only set when there is at least one migration to stamp.
	if wantMax > 0 && init != 1 {
		t.Errorf("is_initialized=%d with %d migrations present, want 1", init, wantMax)
	}
	t.Logf("fresh install: 7 tables + schema_migration present; pointer=%d (max=%d) is_initialized=%d", ptr, wantMax, init)
}

// maxMigrationIndex reports the highest numeric migration filename under the
// embedded migrations dir, or 0 if none. Mirrors runMigrations' discovery so
// the schema test expects exactly what the runner stamps.
func maxMigrationIndex(t *testing.T) int {
	t.Helper()
	// The runner reads schema/migrations from the embedded FS; from the test we
	// read the same files off disk relative to the package.
	matches, _ := filepath.Glob(filepath.Join("database", "schema", "migrations", "*.sql"))
	max := 0
	for _, m := range matches {
		var n int
		base := filepath.Base(m)
		if _, err := fmt.Sscanf(base, "%d.sql", &n); err == nil && n > max {
			max = n
		}
	}
	return max
}

// -----------------------------------------------------------------------------
// 2. Full sync-run lifecycle through the public contract, re-verified with RAW
//    SQL on every step. Exercises resume (OpenConflictRun resumes the SAME run)
//    and final completion.
// -----------------------------------------------------------------------------

func TestIntegration_SyncRunLifecycle(t *testing.T) {
	st, path := dbPath(t)
	ro := readonly(t, path)

	// Workspace.
	ws, err := st.Workspace("/repo", "origin", "main", contract.GitTracked)
	if err != nil {
		t.Fatalf("Workspace: %v", err)
	}
	if got := scalarInt(t, ro, `SELECT COUNT(*) FROM workspaces WHERE id=?`, ws.ID); got != 1 {
		t.Fatalf("workspace not persisted (rows=%d)", got)
	}

	// UpsertAgent.
	agent, err := st.UpsertAgent(contract.Agent{WsID: ws.ID, Name: "reviewer", CanonicalHash: "CANON"})
	if err != nil {
		t.Fatalf("UpsertAgent: %v", err)
	}
	var aName, aHash string
	if err := ro.QueryRow(`SELECT name, canonical_hash FROM agents WHERE id=?`, agent.ID).
		Scan(&aName, &aHash); err != nil {
		t.Fatalf("agent readback: %v", err)
	}
	if aName != "reviewer" || aHash != "CANON" {
		t.Fatalf("agent row wrong: name=%q hash=%q", aName, aHash)
	}

	// OpenRun (running).
	run, err := st.OpenRun(ws.ID, "main", "START_HASH")
	if err != nil {
		t.Fatalf("OpenRun: %v", err)
	}
	var status, baseHash string
	if err := ro.QueryRow(`SELECT status, base_start_hash FROM sync_runs WHERE run_id=?`, run.RunID).
		Scan(&status, &baseHash); err != nil {
		t.Fatalf("run readback: %v", err)
	}
	if status != string(contract.RunRunning) || baseHash != "START_HASH" {
		t.Fatalf("run row wrong: status=%q baseHash=%q", status, baseHash)
	}

	// SaveBranch x2 (agent + beta kinds).
	if err := st.SaveBranch(contract.Branch{
		RunID: run.RunID, Name: "graft/" + run.RunID + "/agent/reviewer",
		Kind: contract.BranchAgent, HeadHash: "AH", State: "merged",
	}); err != nil {
		t.Fatalf("SaveBranch agent: %v", err)
	}
	if err := st.SaveBranch(contract.Branch{
		RunID: run.RunID, Name: "graft/" + run.RunID + "/beta/1",
		Kind: contract.BranchBeta, HeadHash: "BH", State: "active",
	}); err != nil {
		t.Fatalf("SaveBranch beta: %v", err)
	}
	if n := scalarInt(t, ro, `SELECT COUNT(*) FROM branches WHERE run_id=?`, run.RunID); n != 2 {
		t.Fatalf("branches persisted=%d, want 2", n)
	}
	if n := scalarInt(t, ro, `SELECT COUNT(*) FROM branches WHERE run_id=? AND kind='beta'`, run.RunID); n != 1 {
		t.Fatalf("beta branches=%d, want 1", n)
	}
	if n := scalarInt(t, ro, `SELECT COUNT(*) FROM branches WHERE run_id=? AND kind='agent'`, run.RunID); n != 1 {
		t.Fatalf("agent branches=%d, want 1", n)
	}

	// SaveConflict.
	if err := st.SaveConflict(run.RunID, contract.Conflict{Path: "reviewer.md", Agent: "reviewer"}); err != nil {
		t.Fatalf("SaveConflict: %v", err)
	}
	var cPath, cStatus, cAgent string
	if err := ro.QueryRow(`SELECT path, status, agent_name FROM conflicts WHERE run_id=?`, run.RunID).
		Scan(&cPath, &cStatus, &cAgent); err != nil {
		t.Fatalf("conflict readback: %v", err)
	}
	if cPath != "reviewer.md" || cStatus != "open" || cAgent != "reviewer" {
		t.Fatalf("conflict row wrong: path=%q status=%q agent=%q", cPath, cStatus, cAgent)
	}

	// UpdateRun -> conflict (halt, resumable).
	run.Status = contract.RunConflict
	run.Phase = "merge"
	run.BetaBranch = "graft/" + run.RunID + "/beta/1"
	if err := st.UpdateRun(run); err != nil {
		t.Fatalf("UpdateRun conflict: %v", err)
	}
	if got := scalarInt(t, ro, `SELECT COUNT(*) FROM sync_runs WHERE run_id=? AND status='conflict' AND phase='merge'`, run.RunID); got != 1 {
		t.Fatalf("run not in resumable conflict state")
	}

	// OpenConflictRun must resume the SAME run.
	resumed, err := st.OpenConflictRun(ws.ID)
	if err != nil {
		t.Fatalf("OpenConflictRun: %v", err)
	}
	if resumed == nil {
		t.Fatalf("expected a resumable run, got nil")
	}
	if resumed.RunID != run.RunID {
		t.Fatalf("resumed a DIFFERENT run: got %q want %q", resumed.RunID, run.RunID)
	}
	if resumed.Phase != "merge" || resumed.BetaBranch != run.BetaBranch || resumed.BaseStartHash != "START_HASH" {
		t.Fatalf("resume lost state: %+v", resumed)
	}

	// UpdateRun -> done.
	resumed.Status = contract.RunDone
	resumed.EndedAt = 999
	if err := st.UpdateRun(*resumed); err != nil {
		t.Fatalf("UpdateRun done: %v", err)
	}
	var finalStatus string
	var endedAt int64
	if err := ro.QueryRow(`SELECT status, ended_at FROM sync_runs WHERE run_id=?`, run.RunID).
		Scan(&finalStatus, &endedAt); err != nil {
		t.Fatalf("final readback: %v", err)
	}
	if finalStatus != string(contract.RunDone) || endedAt != 999 {
		t.Fatalf("run not finalized: status=%q ended_at=%d", finalStatus, endedAt)
	}
	// No conflict run should remain resumable now.
	if again, _ := st.OpenConflictRun(ws.ID); again != nil {
		t.Fatalf("done run still reported as resumable: %+v", again)
	}
	// Exactly one run row across the whole lifecycle (resume reused it).
	if n := scalarInt(t, ro, `SELECT COUNT(*) FROM sync_runs WHERE ws_id=?`, ws.ID); n != 1 {
		t.Fatalf("expected 1 run row (resume reuses), got %d", n)
	}
}

// -----------------------------------------------------------------------------
// 3. Concurrency: many goroutines through TWO independent store handles on the
//    SAME db file must serialize via WAL + busy_timeout — no "database is
//    locked" errors and no lost writes (final row count is exact).
// -----------------------------------------------------------------------------

func TestIntegration_ConcurrentWritersSerialize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "graft.db")
	stA, err := Open(path)
	if err != nil {
		t.Fatalf("Open A: %v", err)
	}
	defer stA.Close()
	stB, err := Open(path) // second independent handle to the same file
	if err != nil {
		t.Fatalf("Open B: %v", err)
	}
	defer stB.Close()

	ws, err := stA.Workspace("/repo", "origin", "main", contract.GitTracked)
	if err != nil {
		t.Fatalf("Workspace: %v", err)
	}
	run, err := stA.OpenRun(ws.ID, "main", "h")
	if err != nil {
		t.Fatalf("OpenRun: %v", err)
	}

	const perHandle = 40
	stores := []contract.Store{stA, stB}
	var wg sync.WaitGroup
	errs := make(chan error, len(stores)*perHandle)

	for hi, st := range stores {
		for i := 0; i < perHandle; i++ {
			wg.Add(1)
			go func(st contract.Store, hi, i int) {
				defer wg.Done()
				// Distinct branch name per (handle,i) so each is a fresh INSERT
				// (UNIQUE(run_id,name)); a lost write would drop the row count.
				name := fmt.Sprintf("graft/%s/agent/h%d-n%d", run.RunID, hi, i)
				if err := st.SaveBranch(contract.Branch{
					RunID: run.RunID, Name: name, Kind: contract.BranchAgent, HeadHash: "x",
				}); err != nil {
					errs <- fmt.Errorf("handle %d write %d: %w", hi, i, err)
				}
			}(st, hi, i)
		}
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		// A "database is locked" here means WAL+busy_timeout did not serialize.
		t.Errorf("concurrent write failed: %v", e)
	}

	ro := readonly(t, path)
	want := len(stores) * perHandle
	if got := scalarInt(t, ro, `SELECT COUNT(*) FROM branches WHERE run_id=?`, run.RunID); got != want {
		t.Fatalf("lost writes: branch rows=%d, want %d", got, want)
	}
}

// -----------------------------------------------------------------------------
// 4. Drift end-to-end across MULTIPLE provider_links for one agent: several
//    links in-sync, one divergent -> Drift reports drifted with a reason naming
//    the divergent provider. Driven via the public contract; verified by Drift
//    and by raw row inspection.
// -----------------------------------------------------------------------------

func TestIntegration_DriftMultiLink(t *testing.T) {
	st, path := dbPath(t)
	ro := readonly(t, path)

	ws, _ := st.Workspace("/repo", "origin", "main", contract.GitTracked)
	agent, err := st.UpsertAgent(contract.Agent{WsID: ws.ID, Name: "multi", CanonicalHash: "CANON"})
	if err != nil {
		t.Fatalf("UpsertAgent: %v", err)
	}

	// Three in-sync links + one divergent.
	links := []contract.ProviderLink{
		{AgentID: agent.ID, Provider: "claude-code", ContentHash: "CANON", FilePath: "a"},
		{AgentID: agent.ID, Provider: "codex", ContentHash: "CANON", FilePath: "b"},
		{AgentID: agent.ID, Provider: "cursor", ContentHash: "CANON", FilePath: "c"},
		{AgentID: agent.ID, Provider: "goose", ContentHash: "DIVERGENT", FilePath: "d"},
	}
	for _, l := range links {
		if err := st.UpsertProviderLink(l); err != nil {
			t.Fatalf("UpsertProviderLink %s: %v", l.Provider, err)
		}
	}
	if n := scalarInt(t, ro, `SELECT COUNT(*) FROM provider_links WHERE agent_id=?`, agent.ID); n != 4 {
		t.Fatalf("provider_links=%d, want 4", n)
	}
	// Raw confirmation: exactly one link diverges from the canonical hash.
	if n := scalarInt(t, ro,
		`SELECT COUNT(*) FROM provider_links pl JOIN agents a ON a.id=pl.agent_id
		 WHERE pl.agent_id=? AND pl.content_hash != a.canonical_hash`, agent.ID); n != 1 {
		t.Fatalf("diverging links by SQL=%d, want 1", n)
	}

	drifted, reason, err := st.Drift(ws.ID, "multi")
	if err != nil {
		t.Fatalf("Drift: %v", err)
	}
	if !drifted {
		t.Fatalf("expected drift across multi links, got in-sync (reason=%q)", reason)
	}
	if reason == "" {
		t.Fatalf("drift reason empty; should name the divergent provider")
	}

	// Realign the divergent link -> drift clears (all four now match canonical).
	if err := st.UpsertProviderLink(contract.ProviderLink{
		AgentID: agent.ID, Provider: "goose", ContentHash: "CANON", FilePath: "d",
	}); err != nil {
		t.Fatalf("UpsertProviderLink realign: %v", err)
	}
	// Still four links (upsert, not insert).
	if n := scalarInt(t, ro, `SELECT COUNT(*) FROM provider_links WHERE agent_id=?`, agent.ID); n != 4 {
		t.Fatalf("after realign provider_links=%d, want 4 (upsert must not duplicate)", n)
	}
	drifted, _, err = st.Drift(ws.ID, "multi")
	if err != nil {
		t.Fatalf("Drift after realign: %v", err)
	}
	if drifted {
		t.Fatalf("expected no drift after realigning all links")
	}
}

// -----------------------------------------------------------------------------
// 5. Identity / constraint integrity: the UNIQUE keys are enforced and
//    provider_link upsert is idempotent. Also probes FK behavior and the
//    foreign_keys pragma so the gate documents whether referential integrity is
//    actually enforced at runtime.
// -----------------------------------------------------------------------------

func TestIntegration_UniqueConstraints(t *testing.T) {
	st, path := dbPath(t)
	ro := readonly(t, path)
	s := concrete(t, st)

	ws, _ := st.Workspace("/repo", "origin", "main", contract.GitTracked)

	// UNIQUE(root, remote, branch): a raw duplicate INSERT must be rejected.
	_, err := s.db.Exec(
		`INSERT INTO workspaces (id, root, remote, branch, git_mode, created_at)
		 VALUES ('dup', '/repo', 'origin', 'main', 'tracked', 1)`)
	if err == nil {
		t.Errorf("UNIQUE(root,remote,branch) NOT enforced: duplicate workspace inserted")
	}
	if n := scalarInt(t, ro, `SELECT COUNT(*) FROM workspaces WHERE root='/repo' AND remote='origin' AND branch='main'`); n != 1 {
		t.Errorf("workspace identity rows=%d, want 1", n)
	}

	// UNIQUE(ws_id, name): two UpsertAgent calls with same (ws,name) collapse to
	// one row (the contract upsert path), and a raw duplicate INSERT is rejected.
	a1, _ := st.UpsertAgent(contract.Agent{WsID: ws.ID, Name: "dup-agent", CanonicalHash: "H1"})
	a2, _ := st.UpsertAgent(contract.Agent{WsID: ws.ID, Name: "dup-agent", CanonicalHash: "H2"})
	if a1.ID != a2.ID {
		t.Errorf("UpsertAgent created a second row for same (ws,name): %q vs %q", a1.ID, a2.ID)
	}
	if n := scalarInt(t, ro, `SELECT COUNT(*) FROM agents WHERE ws_id=? AND name='dup-agent'`, ws.ID); n != 1 {
		t.Errorf("agent (ws_id,name) rows=%d, want 1", n)
	}
	_, err = s.db.Exec(
		`INSERT INTO agents (id, ws_id, name, canonical_hash) VALUES ('dup2', ?, 'dup-agent', 'X')`, ws.ID)
	if err == nil {
		t.Errorf("UNIQUE(ws_id,name) NOT enforced: duplicate agent inserted")
	}

	// provider_link upsert is idempotent on (agent_id, provider).
	for i := 0; i < 3; i++ {
		if err := st.UpsertProviderLink(contract.ProviderLink{
			AgentID: a2.ID, Provider: "claude-code", ContentHash: fmt.Sprintf("h%d", i), FilePath: "p",
		}); err != nil {
			t.Fatalf("UpsertProviderLink iter %d: %v", i, err)
		}
	}
	var linkCount int
	var lastHash string
	if err := ro.QueryRow(
		`SELECT COUNT(*), MAX(content_hash) FROM provider_links WHERE agent_id=? AND provider='claude-code'`,
		a2.ID).Scan(&linkCount, &lastHash); err != nil {
		t.Fatalf("provider_link readback: %v", err)
	}
	if linkCount != 1 {
		t.Errorf("provider_link upsert NOT idempotent: rows=%d, want 1", linkCount)
	}
	if lastHash != "h2" {
		t.Errorf("provider_link upsert did not update content_hash: got %q want h2", lastHash)
	}
}

// TestIntegration_ForeignKeyEnforcement asserts referential integrity is now
// enforced at runtime. db enabled `_pragma=foreign_keys(on)` in the store DSN,
// so SQLite must (1) report PRAGMA foreign_keys = 1 and (2) REJECT any child row
// that references a non-existent parent.
func TestIntegration_ForeignKeyEnforcement(t *testing.T) {
	st, path := dbPath(t)
	s := concrete(t, st)
	ro := readonly(t, path)

	// (1) The pragma must be ON on the live write connection.
	var fk int
	if err := s.db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Fatalf("PRAGMA foreign_keys=%d, want 1 (FK enforcement not enabled in DSN)", fk)
	}

	// (2a) Orphan sync_run: ws_id references a workspace that does not exist.
	if _, err := s.db.Exec(
		`INSERT INTO sync_runs (run_id, ws_id) VALUES ('orphan-run', 'NO_SUCH_WS')`,
	); err == nil {
		t.Errorf("orphan sync_run insert was ACCEPTED; FK on sync_runs.ws_id not enforced")
	} else if !isFKConstraintErr(err) {
		t.Errorf("orphan sync_run rejected with non-FK error: %v", err)
	}
	if n := scalarInt(t, ro, `SELECT COUNT(*) FROM sync_runs WHERE run_id='orphan-run'`); n != 0 {
		t.Errorf("orphan sync_run row persisted (rows=%d), want 0", n)
	}

	// (2b) Orphan branch: run_id references a sync_run that does not exist.
	if _, err := s.db.Exec(
		`INSERT INTO branches (id, run_id, name, kind) VALUES ('orphan-br', 'NO_SUCH_RUN', 'x', 'agent')`,
	); err == nil {
		t.Errorf("orphan branch insert was ACCEPTED; FK on branches.run_id not enforced")
	} else if !isFKConstraintErr(err) {
		t.Errorf("orphan branch rejected with non-FK error: %v", err)
	}
	if n := scalarInt(t, ro, `SELECT COUNT(*) FROM branches WHERE id='orphan-br'`); n != 0 {
		t.Errorf("orphan branch row persisted (rows=%d), want 0", n)
	}

	// Sanity: with parents seeded in FK order (workspace -> run -> branch), the
	// same kind of children are accepted.
	ws, _ := st.Workspace("/repo", "origin", "main", contract.GitTracked)
	run, err := st.OpenRun(ws.ID, "main", "h")
	if err != nil {
		t.Fatalf("OpenRun with valid ws: %v", err)
	}
	if err := st.SaveBranch(contract.Branch{
		RunID: run.RunID, Name: "graft/ok/agent/foo", Kind: contract.BranchAgent,
	}); err != nil {
		t.Fatalf("SaveBranch with valid run: %v", err)
	}
}

// isFKConstraintErr reports whether err is a SQLite FOREIGN KEY constraint
// violation (modernc.org/sqlite surfaces it textually, code 787).
func isFKConstraintErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "foreign key constraint") ||
		strings.Contains(msg, "(787)")
}
