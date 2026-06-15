package sync

// Phase-g integration tests for the sync engine apply path (the f<->g gate).
//
// These drive the REAL Engine over a real `git init` workspace, the real
// transform.Default() (8 active providers; antigravity + gemini-cli unregistered), and a real sqlite store.Open, then
// verify the outcome at the THREE levels mandated by plan-05:
//
//	file : provider bytes on disk + lossless round-trip (parse back -> re-render
//	       is a byte-for-byte fixed point for every provider).
//	db   : raw SQL through a SEPARATE read-only connection to graft.db
//	       (sync_runs / branches / provider_links / agent_states).
//	raw  : contract.RunResult fields returned by Engine.Run.
//
// Helpers (requireGit, newWorkspace, writeClaudeAgent, newEngine, combinedGit,
// workspaceID, mustGit, writeFile) and the fakeGit seam are defined in
// sync_test.go / conflict_test.go (same package) and reused here.
//
// KNOWN-DEFERRED features (per memory/agents/core/state/phase-f-sync-engine.md)
// are intentionally NOT asserted as failures: git_mode=internal repo path +
// migration, workspace lock/WAL concurrency, fine-grained mid-phase resume, and
// the store-hash diff short-circuit. The conflict/resume test follows the task
// note: --continue re-runs idempotently and must CONVERGE to done (not resume
// mid-phase).

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/canonical"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/core/status"
	"github.com/Shaik-Sirajuddin/graft/internal/gitx"
	"github.com/Shaik-Sirajuddin/graft/internal/store"
	"github.com/Shaik-Sirajuddin/graft/internal/store/database"
	"github.com/Shaik-Sirajuddin/graft/internal/transform"
)

// openRO opens a read-only connection to the workspace's graft.db for raw-SQL
// verification of persisted rows (db level).
func openRO(t *testing.T, dir string) *sql.DB {
	t.Helper()
	ro, err := database.OpenReadOnly(filepath.Join(dir, "graft.db"))
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	t.Cleanup(func() { ro.Close() })
	return ro
}

func intq(t *testing.T, db *sql.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("query %q: %v", q, err)
	}
	return n
}

func trimNL(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

// mustOut runs a git command and fails the test on error.
func mustOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := combinedGit(dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return out
}

// mustOutAllow runs a git command and returns "" on error (for queries that may
// legitimately fail, e.g. when HEAD is on an unborn branch).
func mustOutAllow(dir string, args ...string) string {
	out, _ := combinedGit(dir, args...)
	return out
}

// -----------------------------------------------------------------------------
// 1. Clean propagation: one seed provider -> canonical + all providers, lossless,
//    persisted, no base commit, temp refs pruned. Verified at file/db/raw levels.
// -----------------------------------------------------------------------------

func TestIntegration_CleanPropagation(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	writeClaudeAgent(t, dir, "reviewer", "reviews code changes", "You review code carefully.")

	// Record the BASE BRANCH and its ref BEFORE sync. The plan-02 invariant is
	// "no commit lands on base", which is a property of the base branch REF
	// (refs/heads/<branch>), not of HEAD (HEAD follows whatever branch the engine
	// leaves checked out). We capture both to assert the ref invariant and the
	// working-tree-restoration property independently.
	baseBranch := trimNL(mustOut(t, dir, "branch", "--show-current"))
	if baseBranch == "" {
		baseBranch = "main"
	}
	baseRefBefore := trimNL(mustOut(t, dir, "rev-parse", "refs/heads/"+baseBranch))

	// Construct the engine with an explicit temp HOME so antigravity (ScopeHome)
	// writes go to tmpHome, not the real ~/.gemini directory. We keep tmpHome
	// accessible so the per-provider Detect loop below can find antigravity output.
	tmpHome := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "graft.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	tr := transform.Default()
	eng := New(st, tr, gitx.New(dir), dir).SetHomeBase(tmpHome)
	defer st.Close()

	res, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}

	// --- raw level: RunResult fields ---
	if res.Status != contract.RunDone {
		t.Fatalf("[raw] status=%s, want done (conflicts=%v)", res.Status, res.Conflicts)
	}
	if res.RunID == "" {
		t.Fatalf("[raw] empty RunID")
	}
	if len(res.Changed) != 1 || res.Changed[0] != "reviewer" {
		t.Fatalf("[raw] changed=%v, want [reviewer]", res.Changed)
	}
	if len(res.Conflicts) != 0 {
		t.Fatalf("[raw] unexpected conflicts: %v", res.Conflicts)
	}

	// --- file level (a): canonical written ---
	can, err := canonical.Load(canonical.AgentDir(dir, "reviewer"))
	if err != nil {
		t.Fatalf("[file] load canonical: %v", err)
	}
	if can.Name != "reviewer" || can.Model != "sonnet" {
		t.Fatalf("[file] canonical mismatch: %+v", can)
	}

	// --- file level (b): every provider's file exists and is lossless ---
	providers := tr.Providers()
	if len(providers) < 8 {
		t.Fatalf("[file] expected 8 providers (antigravity + gemini-cli unregistered), got %d", len(providers))
	}
	for _, prov := range providers {
		p, ok := tr.Provider(prov)
		if !ok {
			t.Fatalf("[file] provider %q missing from registry", prov)
		}
		// ScopeHome providers (antigravity) write and detect under tmpHome, not dir.
		detectBase := dir
		if sp, ok2 := p.(contract.ScopedProvider); ok2 && sp.PathScope() == contract.ScopeHome {
			detectBase = tmpHome
		}
		refs, err := p.Detect(detectBase)
		if err != nil {
			t.Errorf("[file] %s Detect: %v", prov, err)
			continue
		}
		var ref *contract.AgentRef
		for i := range refs {
			if refs[i].Name == "reviewer" {
				ref = &refs[i]
				break
			}
		}
		if ref == nil {
			t.Errorf("[file] %s produced NO file for reviewer", prov)
			continue
		}
		if _, err := os.Stat(ref.Path); err != nil {
			t.Errorf("[file] %s file missing on disk: %v", prov, err)
			continue
		}

		// Lossless fixed point: re-render the canonical for this provider, parse
		// the on-disk file back to canonical, re-render again, and require the two
		// renders to be byte-identical. (Comparing renders rather than canonicals
		// avoids false negatives from fields a provider legitimately drops.)
		wantWrites, err := tr.FromCanonical(can, prov)
		if err != nil {
			t.Errorf("[file] %s FromCanonical: %v", prov, err)
			continue
		}
		pa, err := p.Parse(ref.Path)
		if err != nil {
			t.Errorf("[file] %s re-Parse: %v", prov, err)
			continue
		}
		rt, err := p.ToCanonical(pa)
		if err != nil {
			t.Errorf("[file] %s re-ToCanonical: %v", prov, err)
			continue
		}
		gotWrites, err := tr.FromCanonical(rt, prov)
		if err != nil {
			t.Errorf("[file] %s re-FromCanonical: %v", prov, err)
			continue
		}
		if len(wantWrites) != len(gotWrites) {
			t.Errorf("[file] %s round-trip write count %d != %d", prov, len(gotWrites), len(wantWrites))
			continue
		}
		for i := range wantWrites {
			if string(gotWrites[i].Data) != string(wantWrites[i].Data) {
				t.Errorf("[file] %s NOT lossless\n--- canonical render ---\n%s\n--- round-trip render ---\n%s",
					prov, wantWrites[i].Data, gotWrites[i].Data)
			}
		}
	}

	// --- db level: raw SQL via read-only connection ---
	ro := openRO(t, dir)
	wsID := workspaceID(t, st, dir)

	// sync_run: exactly one, status=done, for this workspace.
	if n := intq(t, ro, `SELECT COUNT(*) FROM sync_runs WHERE ws_id=? AND run_id=? AND status='done'`, wsID, res.RunID); n != 1 {
		t.Errorf("[db] sync_run done rows=%d, want 1", n)
	}
	// branches: at least one agent branch + one beta branch recorded.
	if n := intq(t, ro, `SELECT COUNT(*) FROM branches WHERE run_id=? AND kind='agent'`, res.RunID); n < 1 {
		t.Errorf("[db] agent branches=%d, want >=1", n)
	}
	if n := intq(t, ro, `SELECT COUNT(*) FROM branches WHERE run_id=? AND kind='beta'`, res.RunID); n < 1 {
		t.Errorf("[db] beta branches=%d, want >=1", n)
	}
	// agent row + canonical hash matches what is on disk.
	var agentID, canHashDB string
	if err := ro.QueryRow(
		`SELECT id, canonical_hash FROM agents WHERE ws_id=? AND name='reviewer'`, wsID,
	).Scan(&agentID, &canHashDB); err != nil {
		t.Fatalf("[db] agent row: %v", err)
	}
	if canHashDB != canonical.Hash(can) {
		t.Errorf("[db] stored canonical_hash=%q != on-disk hash %q", canHashDB, canonical.Hash(can))
	}
	// provider_links: one per provider, every content_hash == canonical hash.
	if n := intq(t, ro, `SELECT COUNT(*) FROM provider_links WHERE agent_id=?`, agentID); n != len(providers) {
		t.Errorf("[db] provider_links=%d, want %d (one per provider)", n, len(providers))
	}
	if n := intq(t, ro,
		`SELECT COUNT(*) FROM provider_links WHERE agent_id=? AND content_hash != ?`, agentID, canHashDB); n != 0 {
		t.Errorf("[db] %d provider_links diverge from canonical hash, want 0", n)
	}
	// agent_state: recorded in-sync for this run.
	var inSync int
	if err := ro.QueryRow(
		`SELECT in_sync FROM agent_states WHERE run_id=? AND agent_id=?`, res.RunID, agentID,
	).Scan(&inSync); err != nil {
		t.Fatalf("[db] agent_state row: %v", err)
	}
	if inSync != 1 {
		t.Errorf("[db] agent_state in_sync=%d, want 1", inSync)
	}

	// --- file level (d): no commit landed on base ref; temp refs pruned ---
	baseRefAfter := trimNL(mustOut(t, dir, "rev-parse", "refs/heads/"+baseBranch))
	if baseRefAfter != baseRefBefore {
		t.Errorf("[file] base branch ref %q moved: before=%q after=%q (sync must not commit to base)",
			baseBranch, baseRefBefore, baseRefAfter)
	}
	if out, _ := combinedGit(dir, "branch", "--list", "graft/*"); out != "" {
		t.Errorf("[file] temp graft branches not pruned: %q", out)
	}

	// Working-tree restoration: after a sync the working dir must be left on the
	// ORIGINAL base branch, not stranded on an internal temp branch. A user's
	// next `git`/`graft` op (and the next sync) depends on this. This is a
	// distinct invariant from the base-ref check above.
	cur := trimNL(mustOutAllow(dir, "branch", "--show-current"))
	if cur != baseBranch {
		t.Errorf("[file] working tree left on %q, want base branch %q "+
			"(engine does not restore the checkout after sync — see verdict)", cur, baseBranch)
	}

	// Drift via store: in-sync immediately after a clean sync.
	drifted, reason, err := st.Drift(wsID, "reviewer")
	if err != nil {
		t.Fatalf("[db] Drift: %v", err)
	}
	if drifted {
		t.Errorf("[db] reviewer drifted right after clean sync: %s", reason)
	}
}

// -----------------------------------------------------------------------------
// 2. Drift -> status: an out-of-band edit to one provider file makes status
//    report that provider drifted; a re-sync re-renders it and clears the drift.
// -----------------------------------------------------------------------------

func TestIntegration_DriftThenStatusThenResync(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	writeClaudeAgent(t, dir, "reviewer", "reviews code", "You review code.")

	eng, st := newEngine(t, dir)
	defer st.Close()

	if res, err := eng.Run(contract.SyncOpts{}); err != nil || res.Status != contract.RunDone {
		t.Fatalf("initial sync: res=%+v err=%v", res, err)
	}

	tr := transform.Default()
	rep := status.New(st, tr, dir)
	name := "reviewer"

	// Baseline: claude-code reports in-sync.
	rpt, err := rep.Status(&name)
	if err != nil {
		t.Fatalf("status baseline: %v", err)
	}
	if got, ok := agentProvider(rpt, "reviewer", "claude-code"); !ok || !got {
		t.Fatalf("[raw] baseline claude-code not in-sync (ok=%v inSync=%v)", ok, got)
	}
	if n := rpt.OutOfSyncProviders["claude-code"]; n != 0 {
		t.Fatalf("[raw] baseline OutOfSyncProviders[claude-code]=%d, want 0", n)
	}

	// Out-of-band hand edit of the claude-code provider file.
	claudePath := filepath.Join(dir, ".claude", "agents", "reviewer.md")
	orig, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("read claude file: %v", err)
	}
	if err := os.WriteFile(claudePath, append(orig, []byte("\nHAND-EDITED DRIFT LINE\n")...), 0o644); err != nil {
		t.Fatalf("hand edit: %v", err)
	}

	// status must now report claude-code drifted for reviewer.
	rpt2, err := rep.Status(&name)
	if err != nil {
		t.Fatalf("status after edit: %v", err)
	}
	if got, ok := agentProvider(rpt2, "reviewer", "claude-code"); !ok || got {
		t.Fatalf("[file/raw] expected claude-code OUT of sync after hand edit (ok=%v inSync=%v)", ok, got)
	}
	if n := rpt2.OutOfSyncProviders["claude-code"]; n != 1 {
		t.Errorf("[raw] OutOfSyncProviders[claude-code]=%d, want 1", n)
	}

	// Re-sync: the drifted provider file is re-canonicalized + re-rendered, so the
	// hand edit is reconciled into canonical and propagated -> status clears.
	// NOTE: this currently fails as a DOWNSTREAM SYMPTOM of the same core bug
	// surfaced by TestIntegration_CleanPropagation — the first sync left the
	// working tree stranded on a temp beta branch with pending local changes, so
	// the second sync's merge aborts with "local changes would be overwritten".
	if res, err := eng.Run(contract.SyncOpts{}); err != nil || res.Status != contract.RunDone {
		t.Fatalf("re-sync did not converge (root cause: working tree not restored to base after first sync): res=%+v err=%v", res, err)
	}
	rpt3, err := rep.Status(&name)
	if err != nil {
		t.Fatalf("status after re-sync: %v", err)
	}
	if got, ok := agentProvider(rpt3, "reviewer", "claude-code"); !ok || !got {
		t.Errorf("[file/raw] claude-code still drifted after re-sync (ok=%v inSync=%v)", ok, got)
	}
	if n := rpt3.OutOfSyncProviders["claude-code"]; n != 0 {
		t.Errorf("[raw] OutOfSyncProviders[claude-code]=%d after re-sync, want 0", n)
	}
}

// agentProvider returns the in-sync bool for one agent/provider pair from a
// StatusReport, and whether the pair was present.
func agentProvider(rpt contract.StatusReport, agent, provider string) (inSync bool, ok bool) {
	for _, a := range rpt.Agents {
		if a.Name != agent {
			continue
		}
		v, present := a.Providers[provider]
		return v, present
	}
	return false, false
}

// -----------------------------------------------------------------------------
// 3. Conflict + resume: a merge conflict halts the run (status=conflict, row
//    persisted), and a --continue re-run converges to done (idempotent re-run,
//    not mid-phase resume — per the task note and core's deferred-resume).
//    Real store + real transform + real git underneath; only the merge OUTCOME
//    is forced once via the GitX seam (fakeGit, defined in conflict_test.go).
// -----------------------------------------------------------------------------

func TestIntegration_ConflictThenResumeConverges(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	writeClaudeAgent(t, dir, "x", "desc", "body")

	st, err := store.Open(filepath.Join(dir, "graft.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer st.Close()
	tr := transform.Default()
	fg := &fakeGit{inner: gitx.New(dir), conflictOnce: true}
	eng := New(st, tr, fg, dir).SetHomeBase(t.TempDir())
	wsID := workspaceID(t, st, dir)

	// --- First run: forced conflict ---
	res, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	// raw level
	if res.Status != contract.RunConflict {
		t.Fatalf("[raw] status=%s, want conflict", res.Status)
	}
	if len(res.Conflicts) == 0 || res.Conflicts[0].Agent != "x" {
		t.Fatalf("[raw] conflicts=%v, want one for agent x", res.Conflicts)
	}

	// db level: run row resumable (status=conflict) + a conflict row persisted.
	ro := openRO(t, dir)
	if n := intq(t, ro, `SELECT COUNT(*) FROM sync_runs WHERE run_id=? AND status='conflict'`, res.RunID); n != 1 {
		t.Errorf("[db] conflict run row=%d, want 1 (resumable)", n)
	}
	if n := intq(t, ro, `SELECT COUNT(*) FROM conflicts WHERE run_id=?`, res.RunID); n < 1 {
		t.Errorf("[db] conflict rows=%d, want >=1", n)
	}
	var cAgent, cStatus string
	if err := ro.QueryRow(`SELECT agent_name, status FROM conflicts WHERE run_id=? LIMIT 1`, res.RunID).
		Scan(&cAgent, &cStatus); err != nil {
		t.Fatalf("[db] conflict row read: %v", err)
	}
	if cAgent != "x" || cStatus != "open" {
		t.Errorf("[db] conflict row agent=%q status=%q, want x/open", cAgent, cStatus)
	}

	// --- Bare re-run (no --continue) auto-continues the open conflict run. ---
	// `--continue` is now OPTIONAL: an open conflict run always takes precedence
	// and a bare `sync` resumes it (re-surfacing if the conflict persists, NOT a
	// blocked error). Here the fakeGit's forced conflict is already exhausted, so
	// this bare re-run converges to done.
	res2, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("[raw] bare auto-continue re-run errored: %v", err)
	}
	if res2.Status != contract.RunDone {
		t.Fatalf("[raw] bare auto-continue status=%s, want done (conflicts=%v)", res2.Status, res2.Conflicts)
	}
	if res2.RunID != res.RunID {
		t.Errorf("[raw] bare auto-continue started a NEW run %s (orig %s); the open conflict run must take precedence", res2.RunID, res.RunID)
	}

	// db level: same run is now done; no conflict run remains resumable.
	ro2 := openRO(t, dir)
	if n := intq(t, ro2, `SELECT COUNT(*) FROM sync_runs WHERE run_id=? AND status='done'`, res.RunID); n != 1 {
		t.Errorf("[db] run not finalized to done (rows=%d)", n)
	}
	if again, err := st.OpenConflictRun(wsID); err != nil {
		t.Fatalf("[db] OpenConflictRun: %v", err)
	} else if again != nil {
		t.Errorf("[db] a conflict run is still resumable after convergence: %+v", again)
	}

	// file level: agent x propagated to claude-code after convergence.
	if _, err := os.Stat(filepath.Join(dir, ".claude", "agents", "x.md")); err != nil {
		t.Errorf("[file] agent x not propagated after auto-continue: %v", err)
	}
}

// TestIntegration_ConflictResurfaceThenConverge exercises the marker-aware
// auto-continue contract: while the forced conflict persists, a BARE re-run
// (SyncOpts{}) RE-SURFACES the same conflict (status=conflict, same RunID, no
// error); once the conflict clears, a bare re-run CONVERGES to done. A trailing
// SyncOpts{Continue:true} run confirms --continue is a redundant alias (same
// outcome). Uses a local GitX seam that conflicts a configurable number of
// times so both the persist and clear paths are covered with real store +
// transform underneath.
func TestIntegration_ConflictResurfaceThenConverge(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	writeClaudeAgent(t, dir, "x", "desc", "body")

	st, err := store.Open(filepath.Join(dir, "graft.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer st.Close()
	tr := transform.Default()
	// Conflict on the first TWO merge attempts (initial run + first bare resume),
	// then merge cleanly so the second bare resume converges.
	cg := &countingConflictGit{inner: gitx.New(dir), conflicts: 2}
	eng := New(st, tr, cg, dir).SetHomeBase(t.TempDir())
	wsID := workspaceID(t, st, dir)

	// Initial run -> conflict.
	res, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if res.Status != contract.RunConflict {
		t.Fatalf("[raw] first status=%s, want conflict", res.Status)
	}

	// Bare re-run while the conflict PERSISTS -> re-surface, same run, no error.
	resurfaced, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("[raw] bare re-run (markers remain) errored: %v", err)
	}
	if resurfaced.Status != contract.RunConflict {
		t.Fatalf("[raw] re-surface status=%s, want conflict (markers still present)", resurfaced.Status)
	}
	if resurfaced.RunID != res.RunID {
		t.Errorf("[raw] re-surface started a new run %s (orig %s)", resurfaced.RunID, res.RunID)
	}
	if len(resurfaced.Conflicts) == 0 || resurfaced.Conflicts[0].Agent != "x" {
		t.Errorf("[raw] re-surfaced conflicts=%v, want one for agent x", resurfaced.Conflicts)
	}
	// db: still an open conflict run for this workspace.
	if again, _ := st.OpenConflictRun(wsID); again == nil || again.RunID != res.RunID {
		t.Errorf("[db] conflict run not still resumable after re-surface: %+v", again)
	}

	// Bare re-run after the conflict CLEARS -> converges to done, same run.
	done, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("[raw] bare re-run (markers cleared) errored: %v", err)
	}
	if done.Status != contract.RunDone {
		t.Fatalf("[raw] converge status=%s, want done (conflicts=%v)", done.Status, done.Conflicts)
	}
	if done.RunID != res.RunID {
		t.Errorf("[raw] converge started a new run %s (orig %s)", done.RunID, res.RunID)
	}
	if n := intq(t, openRO(t, dir), `SELECT COUNT(*) FROM sync_runs WHERE run_id=? AND status='done'`, res.RunID); n != 1 {
		t.Errorf("[db] run not finalized to done (rows=%d)", n)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "agents", "x.md")); err != nil {
		t.Errorf("[file] agent x not propagated after converge: %v", err)
	}

	// --continue is a redundant alias: with no open conflict run left, it simply
	// runs a fresh (no-change) sync and returns done without error.
	alias, err := eng.Run(contract.SyncOpts{Continue: true})
	if err != nil {
		t.Fatalf("[raw] --continue alias run errored: %v", err)
	}
	if alias.Status != contract.RunDone {
		t.Errorf("[raw] --continue alias status=%s, want done", alias.Status)
	}
}

// countingConflictGit wraps a real GitX and forces the first `conflicts` Merge
// calls to conflict, then delegates to the real merge. Models a conflict whose
// markers persist across N resume attempts before being resolved.
type countingConflictGit struct {
	inner     contract.GitX
	conflicts int
}

func (f *countingConflictGit) Init() error                          { return f.inner.Init() }
func (f *countingConflictGit) HeadHash(ref string) (string, error)  { return f.inner.HeadHash(ref) }
func (f *countingConflictGit) Branch(name, from string) error       { return f.inner.Branch(name, from) }
func (f *countingConflictGit) Worktree(n, b string) (string, error) { return f.inner.Worktree(n, b) }
func (f *countingConflictGit) Diff(ref string) ([]contract.FileChange, error) {
	return f.inner.Diff(ref)
}
func (f *countingConflictGit) Copy(b string, p []string) error { return f.inner.Copy(b, p) }
func (f *countingConflictGit) Prune(prefix string) error       { return f.inner.Prune(prefix) }

func (f *countingConflictGit) Merge(into, from string) (contract.MergeResult, error) {
	if f.conflicts > 0 {
		f.conflicts--
		return contract.MergeResult{
			Clean:     false,
			Conflicts: []contract.Conflict{{Path: ".graft/agents/x/agent.yaml"}},
		}, nil
	}
	return f.inner.Merge(into, from)
}
