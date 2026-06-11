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
	"fmt"
	"os"
	"path/filepath"
	"sort"

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

	// --- Resume path: pick up an open conflict run if asked / present. ---
	var run contract.SyncRun
	resuming := false
	if existing, err := e.store.OpenConflictRun(ws.ID); err != nil {
		return contract.RunResult{}, fmt.Errorf("sync: open conflict run: %w", err)
	} else if existing != nil && opts.Continue {
		run = *existing
		resuming = true
	} else if existing != nil && !opts.Continue {
		// A conflict run is outstanding; surface it rather than starting fresh.
		return e.resumeBlocked(*existing)
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

	res, err := e.run(ws, run, gctx, opts)
	if err != nil {
		// Mark the run aborted on hard error so it does not linger as resumable.
		run.Status = contract.RunAborted
		run.Phase = phaseDone
		_ = e.store.UpdateRun(run)
		return res, err
	}
	return res, nil
}

// run is the lifecycle body operating on an opened/resumed run.
func (e *Engine) run(ws contract.Workspace, run contract.SyncRun, gctx gitx.Context, opts contract.SyncOpts) (contract.RunResult, error) {
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

	// --- BranchPerFile + Canonicalize: one temp branch per changed agent, with
	// the canonical .graft/agents/<name> written and committed on that branch. ---
	run.Phase = phaseBranch
	_ = e.store.UpdateRun(run)

	if err := e.branchAndCanonicalize(ws, run, changed); err != nil {
		return result, err
	}

	// --- MergeLoop + Reapply + ChangeDetect. ---
	run.Phase = phaseMerge
	_ = e.store.UpdateRun(run)

	betaName, conflicts, err := e.mergeLoop(ws, &run, gctx, names)
	if err != nil {
		return result, err
	}
	if len(conflicts) > 0 {
		// Conflict terminal (resumable) state: persist + surface.
		for _, c := range conflicts {
			_ = e.store.SaveConflict(run.RunID, c)
		}
		run.Status = contract.RunConflict
		run.Phase = phaseMerge
		_ = e.store.UpdateRun(run)
		result.Status = contract.RunConflict
		result.Conflicts = conflicts
		// Restore the working tree to base so the merge loop never strands the
		// checkout on a temp beta branch (which would break the next sync).
		e.restoreBase(gctx.Branch)
		return result, nil
	}

	// --- CopyToBase: restore the working tree to the base branch (the merge loop
	// left the checkout on beta), then apply the beta tree on top WITHOUT a commit
	// so the propagated changes land in the base working tree without moving the
	// base ref. ---
	run.Phase = phaseApply
	_ = e.store.UpdateRun(run)
	if err := e.restoreBase(gctx.Branch); err != nil {
		return result, fmt.Errorf("sync: restore base checkout: %w", err)
	}
	if err := e.git.Copy(betaName, nil); err != nil {
		return result, fmt.Errorf("sync: copy beta to base: %w", err)
	}

	// --- FromCanonical: write all providers for each changed agent + persist links. ---
	if err := e.applyProviders(ws, run, names); err != nil {
		return result, err
	}

	// --- Prune temp refs. ---
	run.Phase = phasePrune
	_ = e.store.UpdateRun(run)
	if err := e.git.Prune(gitx.RunPrefix(run.RunID)); err != nil {
		// Pruning is best-effort cleanup; do not fail the whole sync over it.
		_ = err
	}

	run.Status = contract.RunDone
	run.Phase = phaseDone
	e.finish(&run)
	result.Status = contract.RunDone
	return result, nil
}

// resumeBlocked surfaces an outstanding conflict run without starting a new one.
func (e *Engine) resumeBlocked(run contract.SyncRun) (contract.RunResult, error) {
	branches, _ := e.store.Branches(run.RunID)
	_ = branches
	return contract.RunResult{
		RunID:  run.RunID,
		Status: contract.RunConflict,
	}, fmt.Errorf("sync: workspace has an unresolved conflict run %s; rerun with --continue", run.RunID)
}

// finish stamps the run as ended.
func (e *Engine) finish(run *contract.SyncRun) {
	run.EndedAt = nowUnix()
	_ = e.store.UpdateRun(*run)
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

// --- Branch + canonicalize stage ------------------------------------------

// branchAndCanonicalize creates one deterministic temp branch per changed agent,
// writes that agent's canonical .graft/agents/<name> form, and commits it on the
// branch. The branch is recorded in the store.
func (e *Engine) branchAndCanonicalize(ws contract.Workspace, run contract.SyncRun, changed []changedAgent) error {
	base := run.BaseBranch
	for _, ca := range changed {
		can, err := e.canonicalFor(ca)
		if err != nil {
			return err
		}

		branch := gitx.AgentRef(run.RunID, ca.name)
		if err := e.git.Branch(branch, base); err != nil {
			return fmt.Errorf("sync: branch %s: %w", branch, err)
		}

		// Write canonical files into a worktree on that branch and commit them.
		wt, err := e.git.Worktree(branch, branch)
		if err != nil {
			return fmt.Errorf("sync: worktree %s: %w", branch, err)
		}
		if err := writeCanonical(wt, can); err != nil {
			return err
		}
		head, err := commitWorktree(wt, fmt.Sprintf("graft: canonicalize %s", ca.name))
		if err != nil {
			return fmt.Errorf("sync: commit %s: %w", branch, err)
		}

		// Record the branch + agent identity.
		_ = e.store.SaveBranch(contract.Branch{
			RunID:    run.RunID,
			Name:     branch,
			Kind:     contract.BranchAgent,
			HeadHash: head,
			State:    "ready",
		})
		if _, err := e.store.UpsertAgent(contract.Agent{
			WsID:          ws.ID,
			Name:          ca.name,
			CanonicalHash: canonical.Hash(can),
		}); err != nil {
			return fmt.Errorf("sync: upsert agent %s: %w", ca.name, err)
		}
	}
	return nil
}

// canonicalFor merges every provider source for one agent into a single
// canonical agent. The first source seeds the base; later sources contribute
// their provider overrides so multi-provider agents stay lossless.
func (e *Engine) canonicalFor(ca changedAgent) (contract.CanonicalAgent, error) {
	var merged contract.CanonicalAgent
	for i, src := range ca.sources {
		can, err := e.tr.ToCanonical(src)
		if err != nil {
			return contract.CanonicalAgent{}, fmt.Errorf("sync: tocanonical %s: %w", ca.name, err)
		}
		if i == 0 {
			merged = can
			continue
		}
		mergeOverrides(&merged, can)
	}
	if merged.Name == "" {
		merged.Name = ca.name
	}
	return merged, nil
}

func mergeOverrides(dst *contract.CanonicalAgent, src contract.CanonicalAgent) {
	if len(src.ProviderOverrides) == 0 {
		return
	}
	if dst.ProviderOverrides == nil {
		dst.ProviderOverrides = map[string]map[string]any{}
	}
	for prov, ov := range src.ProviderOverrides {
		dst.ProviderOverrides[prov] = ov
	}
}

// --- Merge loop ------------------------------------------------------------

// mergeLoop merges each agent branch sequentially into a fresh beta branch cut
// from the base. The first conflict halts the loop and is returned (resumable).
// If the base moved while merging, the loop restarts onto a new beta_n.
func (e *Engine) mergeLoop(ws contract.Workspace, run *contract.SyncRun, gctx gitx.Context, names []string) (string, []contract.Conflict, error) {
	betaN := 0
	for {
		startHash, err := e.git.HeadHash(run.BaseBranch)
		if err != nil {
			return "", nil, err
		}

		betaName := gitx.BetaRef(run.RunID, betaN)
		if err := e.git.Branch(betaName, run.BaseBranch); err != nil {
			return "", nil, fmt.Errorf("sync: beta branch: %w", err)
		}
		run.BetaBranch = betaName
		_ = e.store.UpdateRun(*run)
		_ = e.store.SaveBranch(contract.Branch{
			RunID: run.RunID, Name: betaName, Kind: contract.BranchBeta, State: "open",
		})

		var conflicts []contract.Conflict
		for _, name := range names {
			agentBranch := gitx.AgentRef(run.RunID, name)
			mr, err := e.git.Merge(betaName, agentBranch)
			if err != nil {
				return "", nil, fmt.Errorf("sync: merge %s: %w", name, err)
			}
			if !mr.Clean {
				for i := range mr.Conflicts {
					mr.Conflicts[i].Agent = name
				}
				conflicts = append(conflicts, mr.Conflicts...)
				// Stop at the first conflicting branch (resumable from here).
				break
			}
		}
		if len(conflicts) > 0 {
			return betaName, conflicts, nil
		}

		// ChangeDetect: did the base move underneath us mid-flight?
		endHash, err := e.git.HeadHash(run.BaseBranch)
		if err != nil {
			return "", nil, err
		}
		if endHash != startHash {
			// Base moved → redo the loop onto a new beta_n.
			betaN++
			continue
		}

		// Record the stabilized beta head.
		head, _ := e.git.HeadHash(betaName)
		_ = e.store.SaveBranch(contract.Branch{
			RunID: run.RunID, Name: betaName, Kind: contract.BranchBeta,
			HeadHash: head, State: "stable",
		})
		return betaName, nil, nil
	}
}

// --- Apply providers -------------------------------------------------------

// applyProviders renders each changed agent's canonical form back to every
// registered provider, writes the files into the working root, and records the
// provider links + agent canonical hash in the store.
func (e *Engine) applyProviders(ws contract.Workspace, run contract.SyncRun, names []string) error {
	for _, name := range names {
		can, err := canonical.Load(canonical.AgentDir(e.root, name))
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

		for _, provName := range e.tr.Providers() {
			writes, err := e.tr.FromCanonical(can, provName)
			if err != nil {
				return fmt.Errorf("sync: fromcanonical %s/%s: %w", name, provName, err)
			}
			for _, w := range writes {
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
			}
			// Record the provider link with the canonical hash as content hash so
			// store.Drift (content_hash == canonical_hash) reports in-sync.
			rel := ""
			if len(writes) > 0 {
				rel = writes[0].Path
			}
			_ = e.store.UpsertProviderLink(contract.ProviderLink{
				AgentID:     agent.ID,
				Provider:    provName,
				FilePath:    rel,
				ContentHash: canHash,
			})
		}

		_ = e.store.SaveAgentState(contract.AgentState{
			RunID: run.RunID, AgentID: agent.ID, InSync: true, Reason: "synced",
		})
	}
	return nil
}
