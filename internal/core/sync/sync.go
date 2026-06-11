// Package sync is the graft sync engine. It implements the plan-02 lifecycle:
// detect git-mode + base branch -> diff provider agent files vs base -> filter
// changed -> branch-per-changed-file -> parse + ToCanonical -> write
// .graft/agents/<name> -> sequential merge loop into a beta branch -> on conflict
// record + return status=conflict (resumable) -> change-detect (base moved ->
// new beta_n) -> copy the beta tree into the working base WITHOUT committing ->
// FromCanonical -> write all providers -> prune temp refs. State (run, branches,
// agents, provider links, conflicts) is persisted via contract.Store.
//
// The engine is dependency-injected (no global singletons): New wires a Store, a
// Transformer, a GitX, and the workspace root.
package sync

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Shaik-Sirajuddin/graft/internal/canonical"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/gitx"
)

// phase constants recorded on the run row to make it resumable.
const (
	phaseDiff    = "diff"
	phaseBranch  = "branch"
	phaseMerge   = "merge"
	phaseReapply = "reapply"
	phaseApply   = "apply"
	phasePrune   = "prune"
	phaseDone    = "done"
)

// Engine orchestrates a sync over a single workspace root.
type Engine struct {
	store contract.Store
	tr    contract.Transformer
	git   contract.GitX
	root  string
}

// New constructs an Engine. Dependencies are injected; the engine owns no global
// state. root is the workspace directory (the dir holding .graft/ and provider
// files).
func New(store contract.Store, tr contract.Transformer, git contract.GitX, root string) *Engine {
	return &Engine{store: store, tr: tr, git: git, root: root}
}

// Run executes (or resumes) a sync per the plan-02 state machine and returns the
// outcome. A clean run ends status=done; a merge conflict ends status=conflict
// with the run row left resumable and the conflicts surfaced in the result.
func (e *Engine) Run(opts contract.SyncOpts) (contract.RunResult, error) {
	// --- Detect: resolve git context + workspace identity. ---
	if err := e.git.Init(); err != nil {
		return contract.RunResult{}, fmt.Errorf("sync: git init: %w", err)
	}
	gctx := gitx.Resolve(e.root)
	ws, err := e.store.Workspace(e.root, gctx.Remote, gctx.Branch, gctx.Mode)
	if err != nil {
		return contract.RunResult{}, fmt.Errorf("sync: workspace: %w", err)
	}

	// --- Resume path: an OPEN conflict run always takes precedence. ---
	// A bare `graft sync` auto-continues a halted conflict run (--continue is an
	// accepted but now-redundant alias). We never silently start a fresh sync
	// while a conflict run is open: if the user has resolved the markers the
	// resume completes the merge; if markers remain, resume re-surfaces the same
	// conflict. opts.Continue no longer changes behavior here.
	var run contract.SyncRun
	resuming := false
	if existing, err := e.store.OpenConflictRun(ws.ID); err != nil {
		return contract.RunResult{}, fmt.Errorf("sync: open conflict run: %w", err)
	} else if existing != nil {
		run = *existing
		resuming = true
	}

	startHash, err := e.git.HeadHash(gctx.Branch)
	if err != nil {
		return contract.RunResult{}, fmt.Errorf("sync: head hash: %w", err)
	}

	if !resuming {
		run, err = e.store.OpenRun(ws.ID, gctx.Branch, startHash)
		if err != nil {
			return contract.RunResult{}, fmt.Errorf("sync: open run: %w", err)
		}
	}

	res, err := e.run(ws, run, gctx, opts, resuming)
	if err != nil {
		// Mark the run aborted on hard error so it does not linger as resumable.
		run.Status = contract.RunAborted
		run.Phase = phaseDone
		_ = e.store.UpdateRun(run)
		return res, err
	}
	return res, nil
}

// run is the lifecycle body operating on an opened/resumed run. When resuming a
// conflict run it skips diff/branch setup and re-enters the merge loop from the
// recorded position (fine-grained resume).
func (e *Engine) run(ws contract.Workspace, run contract.SyncRun, gctx gitx.Context, opts contract.SyncOpts, resuming bool) (contract.RunResult, error) {
	if resuming {
		return e.resume(ws, run, gctx)
	}

	result := contract.RunResult{RunID: run.RunID}

	// --- Diff: find changed provider agent files vs the base branch. ---
	run.Phase = phaseDiff
	_ = e.store.UpdateRun(run)

	changed, err := e.diffChangedAgents(opts)
	if err != nil {
		return result, err
	}
	if len(changed) == 0 {
		// NoChange terminal state.
		run.Status = contract.RunDone
		run.Phase = phaseDone
		e.finish(&run)
		result.Status = contract.RunDone
		return result, nil
	}

	names := agentNames(changed)
	result.Changed = names

	if opts.DryRun {
		run.Status = contract.RunDone
		run.Phase = phaseDone
		e.finish(&run)
		result.Status = contract.RunDone
		return result, nil
	}

	// --- BranchPerFile + Canonicalize: build the common-ancestor branch and one
	// branch PER CHANGED PROVIDER FILE, each folding only that provider's parsed
	// canonical onto the ancestor (so divergent providers conflict on the same
	// canonical line). ---
	run.Phase = phaseBranch
	_ = e.store.UpdateRun(run)

	works, err := e.buildAgentWork(changed)
	if err != nil {
		return result, err
	}
	if len(works) == 0 {
		// Detected agents but no provider FILE actually changed since last sync.
		run.Status = contract.RunDone
		run.Phase = phaseDone
		e.finish(&run)
		result.Status = contract.RunDone
		result.Changed = nil
		return result, nil
	}
	result.Changed = workNames(works)

	_, order, err := e.prepareBranches(ws, run, works)
	if err != nil {
		return result, err
	}
	steps := mergeOrder(order)

	// --- MergeLoop: cut beta from the common ancestor, merge per-provider
	// branches three-way into beta sequentially. ---
	run.Phase = phaseMerge
	betaName := gitx.BetaRef(run.RunID, 0)
	if err := e.git.Branch(betaName, canonBaseBranch(run.RunID)); err != nil {
		return result, fmt.Errorf("sync: beta branch: %w", err)
	}
	run.BetaBranch = betaName
	_ = e.store.SaveBranch(contract.Branch{
		RunID: run.RunID, Name: betaName, Kind: contract.BranchBeta, State: "open",
	})
	_ = e.store.UpdateRun(run)

	betaWT, err := e.betaWorktree(betaName)
	if err != nil {
		return result, fmt.Errorf("sync: beta worktree: %w", err)
	}
	if _, err := gitInDir(betaWT, "checkout", "-f", betaName); err != nil {
		return result, fmt.Errorf("sync: beta checkout: %w", err)
	}

	conflicts, _, err := e.mergeInto(run, betaName, betaWT, steps, 0)
	if err != nil {
		return result, err
	}
	if len(conflicts) > 0 {
		return e.haltOnConflict(run, gctx, betaWT, conflicts, &result)
	}

	return e.finalize(ws, run, gctx, betaName, betaWT, steps, &result)
}

// haltOnConflict persists the conflict, surfaces the marker-bearing canonical
// file into the user's working tree, sets status=conflict, and returns.
func (e *Engine) haltOnConflict(run contract.SyncRun, gctx gitx.Context, betaWT string, conflicts []contract.Conflict, result *contract.RunResult) (contract.RunResult, error) {
	if err := e.surfaceConflictToWorkspace(betaWT, conflicts); err != nil {
		return *result, err
	}
	for _, c := range conflicts {
		_ = e.store.SaveConflict(run.RunID, c)
	}
	run.Status = contract.RunConflict
	run.Phase = phaseMerge
	_ = e.store.UpdateRun(run)
	result.Status = contract.RunConflict
	result.Conflicts = conflicts
	// Keep the main working tree on base; the merge runs in an isolated worktree.
	_ = e.restoreBase(gctx.Branch)
	return *result, nil
}

// finalize copies the resolved beta canonical tree onto the base working dir
// WITHOUT committing the base, renders all providers, records links, and prunes.
// steps is the full ordered slice of per-provider merge steps (needed for the
// base-moved re-loop: if the base advanced during the sync we cut a new beta and
// re-run the merge).
func (e *Engine) finalize(ws contract.Workspace, run contract.SyncRun, gctx gitx.Context, betaName, betaWT string, steps []mergeStep, result *contract.RunResult) (contract.RunResult, error) {
	// --- plan-02 ChangeDetect / base-moved re-loop ---
	// If the base branch advanced while we were merging (e.g. concurrent push),
	// re-cut a new beta from the new HEAD and re-run the full merge. We allow up
	// to 3 re-tries; if still not stable after that, fail hard.
	const maxReapply = 3
	betaN := 0 // tracks which beta_N we are currently on (initial = 0)
	for i := 0; i < maxReapply; i++ {
		currentHash, err := e.git.HeadHash(gctx.Branch)
		if err != nil {
			return *result, fmt.Errorf("sync: head hash check: %w", err)
		}
		if currentHash == run.BaseStartHash {
			break // base has not moved; proceed to copy
		}
		// Base moved. Cut a new beta from the new HEAD.
		run.Phase = phaseReapply
		_ = e.store.UpdateRun(run)
		betaN++
		newBetaName := gitx.BetaRef(run.RunID, betaN)
		if err := e.git.Branch(newBetaName, gctx.Branch); err != nil {
			return *result, fmt.Errorf("sync: new beta branch (reapply %d): %w", i+1, err)
		}
		_ = e.store.SaveBranch(contract.Branch{
			RunID: run.RunID, Name: newBetaName, Kind: contract.BranchBeta, State: "open",
		})
		newBetaWT, err := e.betaWorktree(newBetaName)
		if err != nil {
			return *result, fmt.Errorf("sync: new beta worktree (reapply %d): %w", i+1, err)
		}
		if _, err := gitInDir(newBetaWT, "checkout", "-f", newBetaName); err != nil {
			return *result, fmt.Errorf("sync: new beta checkout (reapply %d): %w", i+1, err)
		}
		conflicts, _, err := e.mergeInto(run, newBetaName, newBetaWT, steps, 0)
		if err != nil {
			return *result, err
		}
		if len(conflicts) > 0 {
			return e.haltOnConflict(run, gctx, newBetaWT, conflicts, result)
		}
		// Update loop variables to the new beta.
		betaName = newBetaName
		betaWT = newBetaWT
		run.BetaBranch = newBetaName
		run.BaseStartHash = currentHash
		_ = e.store.UpdateRun(run)
		if i == maxReapply-1 {
			return *result, fmt.Errorf("sync: base branch moved %d times during sync; aborting to avoid loop", maxReapply)
		}
	}

	// Record stabilized beta head.
	if head, err := gitInDir(betaWT, "rev-parse", "HEAD"); err == nil {
		_ = e.store.SaveBranch(contract.Branch{
			RunID: run.RunID, Name: betaName, Kind: contract.BranchBeta,
			HeadHash: strings.TrimSpace(head), State: "stable",
		})
	}

	// --- CopyToBase: restore main tree to base, overlay beta's .graft tree. ---
	run.Phase = phaseApply
	_ = e.store.UpdateRun(run)
	if err := e.restoreBase(gctx.Branch); err != nil {
		return *result, fmt.Errorf("sync: restore base checkout: %w", err)
	}
	if err := e.git.Copy(betaName, nil); err != nil {
		return *result, fmt.Errorf("sync: copy beta to base: %w", err)
	}

	// --- FromCanonical: write all providers + persist links. ---
	if err := e.applyProviders(ws, run, result.Changed); err != nil {
		return *result, err
	}

	// --- Prune temp refs. ---
	run.Phase = phasePrune
	_ = e.store.UpdateRun(run)
	_ = e.git.Prune(gitx.RunPrefix(run.RunID))

	// A surfaced conflict that has now been resolved + finalized: close the rows.
	_ = e.store.ResolveConflicts(run.RunID)

	run.Status = contract.RunDone
	run.Phase = phaseDone
	e.finish(&run)
	result.Status = contract.RunDone
	return *result, nil
}

// outstandingConflicts returns the conflict set still pending in the beta
// worktree (the unmerged, marker-bearing canonical paths). Falls back to the
// conflicting step's agent when no unmerged paths are reported.
func (e *Engine) outstandingConflicts(betaWT string, steps []mergeStep, conflictIdx int) []contract.Conflict {
	paths, _ := conflictPathsIn(betaWT)
	agent := agentOfStep(steps, conflictIdx)
	var cs []contract.Conflict
	for _, p := range paths {
		cs = append(cs, contract.Conflict{Path: p, Agent: agent})
	}
	if len(cs) == 0 {
		cs = []contract.Conflict{{Path: ".graft", Agent: agent}}
	}
	return cs
}

// finish stamps the run as ended.
func (e *Engine) finish(run *contract.SyncRun) {
	run.EndedAt = nowUnix()
	_ = e.store.UpdateRun(*run)
}

// resume picks up a halted conflict run: the user has edited the conflicted
// canonical file(s) in the working tree to remove the git markers. It commits
// that resolution onto the in-progress beta merge, then continues merging the
// remaining per-provider branches from exactly where it stopped, and finalizes.
func (e *Engine) resume(ws contract.Workspace, run contract.SyncRun, gctx gitx.Context) (contract.RunResult, error) {
	result := contract.RunResult{RunID: run.RunID}

	betaName := run.BetaBranch
	if betaName == "" {
		return result, fmt.Errorf("sync: cannot resume run %s: no beta branch recorded", run.RunID)
	}
	betaWT, err := e.betaWorktree(betaName)
	if err != nil {
		return result, fmt.Errorf("sync: beta worktree: %w", err)
	}

	// Rebuild the deterministic merge order + per-branch state from the store.
	branches, err := e.store.Branches(run.RunID)
	if err != nil {
		return result, err
	}
	steps, conflictIdx, names := resumePlan(run.RunID, branches)
	if conflictIdx < 0 {
		return result, fmt.Errorf("sync: cannot resume run %s: no conflicting branch recorded", run.RunID)
	}
	result.Changed = names

	// Copy the user's resolved canonical file(s) from the working tree back into
	// the beta worktree, then COMPLETE the in-progress conflicted merge.
	if err := e.applyResolution(betaWT); err != nil {
		return result, err
	}
	// Capture marker-bearing paths BEFORE staging so --diff-filter=U is still
	// meaningful; after `git add -A` the index is updated and unmerged entries
	// may disappear, making the fallback path unreliable (fix #2).
	markerPaths, _ := markerFilesIn(betaWT)
	if _, err := gitInDir(betaWT, "add", "-A"); err != nil {
		return result, err
	}
	if err := assertNoMarkers(betaWT); err != nil {
		// User has NOT finished resolving (markers remain). Re-surface the SAME
		// conflict and keep the run resumable — this is not an error: a bare
		// `graft sync` re-run is expected to re-report the outstanding conflict.
		agent := agentOfStep(steps, conflictIdx)
		var cs []contract.Conflict
		for _, p := range markerPaths {
			cs = append(cs, contract.Conflict{Path: p, Agent: agent})
		}
		if len(cs) == 0 {
			cs = []contract.Conflict{{Path: ".graft", Agent: agent}}
		}
		run.Status = contract.RunConflict
		run.Phase = phaseMerge
		_ = e.store.UpdateRun(run)
		result.Status = contract.RunConflict
		result.Conflicts = cs
		_ = e.restoreBase(gctx.Branch)
		return result, nil
	}
	// Commit the resolution only when there is something to commit (a real
	// in-progress merge or staged edits). A phantom conflict (test fake that did
	// not touch the worktree) leaves nothing staged; in that case the conflicting
	// branch is simply (re-)merged below.
	if e.worktreeHasStagedOrMerge(betaWT) {
		if _, err := gitInDir(betaWT, "commit", "--no-edit",
			"-m", fmt.Sprintf("graft: resolve %s", steps[conflictIdx].branch)); err != nil {
			return result, fmt.Errorf("sync: commit resolution: %w", err)
		}
		_ = e.store.SaveBranch(contract.Branch{
			RunID: run.RunID, Name: steps[conflictIdx].branch, Kind: contract.BranchAgent, State: branchMerged,
		})
	} else {
		// Nothing staged: redo the conflicting merge itself in the continue loop.
		conflictIdx--
	}

	// Continue merging the remaining pending branches.
	conflicts, _, err := e.mergeInto(run, betaName, betaWT, steps, conflictIdx+1)
	if err != nil {
		return result, err
	}
	if len(conflicts) > 0 {
		return e.haltOnConflict(run, gctx, betaWT, conflicts, &result)
	}

	return e.finalize(ws, run, gctx, betaName, betaWT, steps, &result)
}

// applyResolution copies every .graft/agents/* canonical file the user may have
// edited from the working tree into the beta worktree, so the user's resolution
// becomes the merge result. Only files that exist in the beta worktree (i.e.
// part of this run) are copied back.
func (e *Engine) applyResolution(betaWT string) error {
	agentsDir := filepath.Join(betaWT, ".graft", "agents")
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		for _, f := range []string{"agent.yaml", "instructions.md", ".meta.json"} {
			rel := filepath.Join(".graft", "agents", ent.Name(), f)
			src := filepath.Join(e.root, rel)
			data, err := os.ReadFile(src)
			if err != nil {
				continue // user may not have all files; leave the worktree copy
			}
			if err := os.WriteFile(filepath.Join(betaWT, rel), data, 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

// markerFilesIn returns the list of tracked files in dir that still contain git
// conflict markers (by running `git grep -l`). It returns an empty slice (not
// an error) when no markers are found. This must be called BEFORE staging
// (`git add -A`) so the index still reflects the unmerged state.
func markerFilesIn(dir string) ([]string, error) {
	out, err := gitInDir(dir, "grep", "-l", "-e", "^<<<<<<< ")
	if err != nil {
		// git grep exits 1 for "no matches" — that is the clean case.
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 1 {
			return nil, nil
		}
		// exit ≥2 is a fatal git error; propagate.
		return nil, err
	}
	// exit 0 means matches were found.
	var paths []string
	for _, p := range strings.Split(out, "\n") {
		if p = strings.TrimSpace(p); p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

// assertNoMarkers fails if any tracked file in the worktree still contains git
// conflict markers (the user must remove them before resume can complete).
func assertNoMarkers(dir string) error {
	out, err := gitInDir(dir, "grep", "-l", "-e", "^<<<<<<< ", "-e", "^>>>>>>> ")
	if err != nil {
		// git grep exits 1 when no matches are found — that is the clean case.
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 1 {
			return nil
		}
		// exit ≥2 is a fatal git error; surface it.
		return fmt.Errorf("sync: assertNoMarkers git grep: %w", err)
	}
	// exit 0 means at least one match: markers remain.
	if strings.TrimSpace(out) != "" {
		return fmt.Errorf("sync: unresolved conflict markers remain in: %s", strings.TrimSpace(out))
	}
	return nil
}

// resumePlan reconstructs the deterministic merge order and the index of the
// branch that conflicted, from the persisted branch rows. names is the sorted
// set of agents involved.
func resumePlan(runID string, branches []contract.Branch) (steps []mergeStep, conflictIdx int, names []string) {
	type rec struct {
		agent, provider, branch, state string
	}
	prefix := gitx.RunPrefix(runID) + "agent/"
	var recs []rec
	nameSet := map[string]bool{}
	for _, b := range branches {
		if b.Kind != contract.BranchAgent {
			continue
		}
		if !strings.HasPrefix(b.Name, prefix) {
			continue
		}
		rest := strings.TrimPrefix(b.Name, prefix)
		slash := strings.LastIndex(rest, "/")
		if slash < 0 {
			continue
		}
		agent := rest[:slash]
		provider := rest[slash+1:]
		recs = append(recs, rec{agent: agent, provider: provider, branch: b.Name, state: b.State})
		nameSet[agent] = true
	}
	sort.Slice(recs, func(i, j int) bool {
		if recs[i].agent != recs[j].agent {
			return recs[i].agent < recs[j].agent
		}
		return recs[i].provider < recs[j].provider
	})
	conflictIdx = -1
	for i, r := range recs {
		steps = append(steps, mergeStep{agent: r.agent, branch: r.branch})
		if r.state == branchConflict {
			conflictIdx = i
		}
	}
	for n := range nameSet {
		names = append(names, n)
	}
	sort.Strings(names)
	return steps, conflictIdx, names
}

func agentOfStep(steps []mergeStep, idx int) string {
	if idx >= 0 && idx < len(steps) {
		return steps[idx].agent
	}
	return ""
}

// workNames returns the sorted agent names from a slice of agentWork.
func workNames(works []agentWork) []string {
	out := make([]string, 0, len(works))
	for _, w := range works {
		out = append(out, w.name)
	}
	sort.Strings(out)
	return out
}

// --- Diff stage ------------------------------------------------------------

// changedAgent groups a canonical agent name with the provider sources that
// changed for it.
type changedAgent struct {
	name    string
	sources []contract.ProviderAgent
}

// diffChangedAgents detects provider agent files, parses them, and keeps those
// whose canonical content differs from what the store recorded (drift) or that
// are not yet tracked. When opts.Names is set, only those agent names are kept.
func (e *Engine) diffChangedAgents(opts contract.SyncOpts) ([]changedAgent, error) {
	want := map[string]bool{}
	for _, n := range opts.Names {
		want[n] = true
	}

	byName := map[string][]contract.ProviderAgent{}
	for _, provName := range e.tr.Providers() {
		prov, ok := e.tr.Provider(provName)
		if !ok {
			continue
		}
		refs, err := prov.Detect(e.root)
		if err != nil {
			return nil, fmt.Errorf("sync: detect %s: %w", provName, err)
		}
		for _, ref := range refs {
			if len(want) > 0 && !want[ref.Name] {
				continue
			}
			pa, err := prov.Parse(ref.Path)
			if err != nil {
				return nil, fmt.Errorf("sync: parse %s: %w", ref.Path, err)
			}
			byName[ref.Name] = append(byName[ref.Name], pa)
		}
	}

	var out []changedAgent
	for name, sources := range byName {
		out = append(out, changedAgent{name: name, sources: sources})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out, nil
}

func agentNames(cs []changedAgent) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.name)
	}
	return out
}

// --- Apply providers -------------------------------------------------------

// applyProviders renders each changed agent's resolved canonical form back to
// every registered provider, writes the files into the working root, records the
// provider links + agent canonical hash in the store, and refreshes the
// .meta.json per-provider SourceHash so the NEXT sync's change detection knows
// the current on-disk provider bytes are already reconciled.
func (e *Engine) applyProviders(ws contract.Workspace, run contract.SyncRun, names []string) error {
	for _, name := range names {
		dir := canonical.AgentDir(e.root, name)
		can, err := canonical.Load(dir)
		if err != nil {
			return fmt.Errorf("sync: load canonical %s: %w", name, err)
		}
		canHash := canonical.Hash(can)
		agent, err := e.store.UpsertAgent(contract.Agent{
			WsID: ws.ID, Name: name, CanonicalHash: canHash,
		})
		if err != nil {
			return fmt.Errorf("sync: upsert agent %s: %w", name, err)
		}

		meta := canonical.Meta{Providers: map[string]canonical.ProviderMeta{}}
		for _, provName := range e.tr.Providers() {
			writes, err := e.tr.FromCanonical(can, provName)
			if err != nil {
				return fmt.Errorf("sync: fromcanonical %s/%s: %w", name, provName, err)
			}
			rel := ""
			var primaryBytes []byte
			for i, w := range writes {
				abs := w.Path
				if !filepath.IsAbs(abs) {
					abs = filepath.Join(e.root, w.Path)
				}
				if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(abs, w.Data, 0o644); err != nil {
					return fmt.Errorf("sync: write %s: %w", abs, err)
				}
				if i == 0 {
					rel = w.Path
					primaryBytes = w.Data
				}
			}
			// Record the provider link with the canonical hash as content hash so
			// store.Drift (content_hash == canonical_hash) reports in-sync.
			_ = e.store.UpsertProviderLink(contract.ProviderLink{
				AgentID:     agent.ID,
				Provider:    provName,
				FilePath:    rel,
				ContentHash: canHash,
			})
			// Source-hash bookkeeping: the bytes we just wrote are the reconciled
			// provider source. Only providers that actually produced a file (i.e.
			// can express this agent) get a recorded source hash.
			if len(writes) > 0 {
				meta.Providers[provName] = canonical.ProviderMeta{
					SourceHash: hashBytes(primaryBytes),
				}
			}
		}

		// Persist the refreshed .meta.json (recomputes CanonicalHash internally).
		metaWrites, err := canonical.SaveWithMeta(e.root, can, meta)
		if err != nil {
			return fmt.Errorf("sync: save meta %s: %w", name, err)
		}
		for _, w := range metaWrites {
			if filepath.Base(w.Path) != ".meta.json" {
				continue // agent.yaml/instructions.md already match the beta copy
			}
			abs := w.Path
			if !filepath.IsAbs(abs) {
				abs = filepath.Join(e.root, w.Path)
			}
			if err := os.WriteFile(abs, w.Data, 0o644); err != nil {
				return fmt.Errorf("sync: write meta %s: %w", abs, err)
			}
		}

		_ = e.store.SaveAgentState(contract.AgentState{
			RunID: run.RunID, AgentID: agent.ID, InSync: true, Reason: "synced",
		})
	}
	return nil
}
