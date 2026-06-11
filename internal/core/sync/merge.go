package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Shaik-Sirajuddin/graft/internal/canonical"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// This file implements the provider-granular branch+merge algorithm (bug 3).
//
// The merge SURFACE is the canonical file (.graft/agents/<name>/agent.yaml +
// instructions.md). For every changed agent we:
//
//  1. Build a common-ancestor canonical (the agent's last-synced canonical, or an
//     empty skeleton on first sync) and commit it on a shared `canonbase` branch
//     cut from the workspace base branch.
//  2. For EACH changed provider file, cut its own branch from `canonbase`, fold
//     ONLY that provider's parsed canonical onto the ancestor, canonical.Save +
//     commit. Divergent providers that touch the SAME canonical field now change
//     the SAME line relative to the ancestor.
//  3. Merge those per-provider branches SEQUENTIALLY into the global beta branch
//     (also cut from `canonbase`). git performs a real three-way merge against
//     the common ancestor:
//       - non-overlapping edits auto-merge -> proceed (no user),
//       - same-line divergence -> standard git conflict markers land in beta's
//         worktree canonical file; we surface (agent,path), set status=conflict,
//         and STOP. The half-finished merge state is left on disk so --continue
//         can complete it after the user edits the markers out.
//
// Capability variance is never a conflict: a provider that cannot express a
// field simply does not set it in ToCanonical, so its per-provider canonical
// keeps the ancestor's value for that line (no divergence).

// providerSource is one changed provider file for an agent, already parsed.
type providerSource struct {
	provider string
	ref      contract.AgentRef
	parsed   contract.ProviderAgent
	srcHash  string // content hash of the on-disk provider file (Raw)
}

// agentWork is the per-agent unit of merge work: the agent name, its
// last-synced (ancestor) canonical, and the provider files that changed.
type agentWork struct {
	name     string
	ancestor contract.CanonicalAgent // common base canonical for the 3-way merge
	changed  []providerSource        // changed provider files (sorted by provider)
}

// canonBaseBranch is the shared common-ancestor branch for a run.
func canonBaseBranch(runID string) string {
	return fmt.Sprintf("graft/%s/canonbase", runID)
}

// providerBranch is the per-agent, per-provider branch ref.
func providerBranch(runID, agent, provider string) string {
	return fmt.Sprintf("graft/%s/agent/%s/%s", runID, agent, provider)
}

// hashBytes returns the content hash used for provider source-hash bookkeeping.
func hashBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// buildAgentWork turns the diffed agents into per-agent merge units, computing
// each agent's ancestor canonical (from the existing .graft on the base branch,
// if any) and the set of provider files whose content changed since last sync.
func (e *Engine) buildAgentWork(changed []changedAgent) ([]agentWork, error) {
	var works []agentWork
	for _, ca := range changed {
		ancestor, prevMeta := e.ancestorCanonical(ca.name)

		var srcs []providerSource
		for _, pa := range ca.sources {
			provName := pa.Provider
			srcHash := hashBytes(pa.Raw)
			// Changed if there is no recorded source hash for this provider, or it
			// differs from the recorded one.
			prev, ok := prevMeta.Providers[provName]
			if ok && prev.SourceHash == srcHash {
				continue // unchanged provider file
			}
			srcs = append(srcs, providerSource{
				provider: provName,
				ref:      pa.Ref,
				parsed:   pa,
				srcHash:  srcHash,
			})
		}
		if len(srcs) == 0 {
			continue // nothing actually changed for this agent
		}
		sort.Slice(srcs, func(i, j int) bool { return srcs[i].provider < srcs[j].provider })

		enriched, err := e.enrichAncestor(ancestor, srcs)
		if err != nil {
			return nil, err
		}
		works = append(works, agentWork{
			name:     ca.name,
			ancestor: enriched,
			changed:  srcs,
		})
	}
	return works, nil
}

// enrichAncestor builds the three-way merge common ancestor. Starting from the
// last-synced canonical, for each field it pre-seeds the value the changed
// providers AGREE on (or that only one provider expresses). This keeps the
// ancestor close to both sides so non-conflicting / capability-variance edits do
// not show up as spurious add/add insertions at the same anchor. Where the
// changed providers DISAGREE on a field, the ancestor keeps the prior value (or
// stays empty on first sync), so each per-provider branch changes that one line
// differently -> a genuine git conflict on exactly that field.
func (e *Engine) enrichAncestor(prior contract.CanonicalAgent, srcs []providerSource) (contract.CanonicalAgent, error) {
	folds := make([]contract.CanonicalAgent, len(srcs))
	for i, src := range srcs {
		f, err := e.foldProvider(prior, src)
		if err != nil {
			return contract.CanonicalAgent{}, err
		}
		folds[i] = f
	}

	anc := prior
	if anc.Name == "" && len(folds) > 0 {
		anc.Name = folds[0].Name
	}

	// description / model: agreed scalar -> seed; disagreement -> keep prior.
	anc.Description = agreedScalar(foldsField(folds, func(c contract.CanonicalAgent) string { return c.Description }), prior.Description)
	anc.Model = agreedScalar(foldsField(folds, func(c contract.CanonicalAgent) string { return c.Model }), prior.Model)
	anc.Body = agreedScalar(foldsField(folds, func(c contract.CanonicalAgent) string { return c.Body }), prior.Body)

	// tools / mcp: agreed slice (by joined form) -> seed; else keep prior.
	anc.Tools = agreedSlice(folds, func(c contract.CanonicalAgent) []string { return c.Tools }, prior.Tools)
	anc.MCP = agreedSlice(folds, func(c contract.CanonicalAgent) []string { return c.MCP }, prior.MCP)

	// providerOverrides: union the per-provider buckets (each provider owns its
	// own key, so there is never cross-provider disagreement on a bucket).
	merged := map[string]map[string]any{}
	for k, v := range prior.ProviderOverrides {
		merged[k] = v
	}
	for _, f := range folds {
		for k, v := range f.ProviderOverrides {
			merged[k] = v
		}
	}
	if len(merged) > 0 {
		anc.ProviderOverrides = merged
	}
	return anc, nil
}

// foldsField extracts one string field across all folds.
func foldsField(folds []contract.CanonicalAgent, get func(contract.CanonicalAgent) string) []string {
	out := make([]string, 0, len(folds))
	for _, f := range folds {
		out = append(out, get(f))
	}
	return out
}

// agreedScalar returns the common non-empty value if every fold that sets the
// field sets the SAME value; otherwise it returns the prior value (forcing a
// genuine conflict between the diverging per-provider branches).
func agreedScalar(vals []string, prior string) string {
	seen := ""
	for _, v := range vals {
		if v == "" {
			continue
		}
		if seen == "" {
			seen = v
		} else if seen != v {
			return prior // disagreement -> keep prior so the branches conflict
		}
	}
	if seen == "" {
		return prior
	}
	return seen
}

// agreedSlice is agreedScalar for string slices, comparing by their joined form.
func agreedSlice(folds []contract.CanonicalAgent, get func(contract.CanonicalAgent) []string, prior []string) []string {
	var seen []string
	have := false
	for _, f := range folds {
		v := get(f)
		if len(v) == 0 {
			continue
		}
		if !have {
			seen = v
			have = true
		} else if strings.Join(seen, "\x00") != strings.Join(v, "\x00") {
			return prior
		}
	}
	if !have {
		return prior
	}
	return seen
}

// ancestorCanonical loads the agent's last-synced canonical (the 3-way merge
// ancestor) and its recorded per-provider source hashes. On first sync there is
// no canonical on disk yet; we return an empty skeleton carrying just the name.
func (e *Engine) ancestorCanonical(name string) (contract.CanonicalAgent, canonical.Meta) {
	dir := canonical.AgentDir(e.root, name)
	can, err := canonical.Load(dir)
	if err != nil {
		return contract.CanonicalAgent{Name: name}, canonical.Meta{}
	}
	meta, _ := canonical.LoadMeta(dir)
	if can.Name == "" {
		can.Name = name
	}
	return can, meta
}

// foldProvider folds one provider's parsed canonical onto the ancestor: the
// provider's expressed fields override, and its ProviderOverrides are merged in.
// Fields the provider does not express keep the ancestor's value (so capability
// variance never shows up as a change).
func (e *Engine) foldProvider(ancestor contract.CanonicalAgent, src providerSource) (contract.CanonicalAgent, error) {
	pc, err := e.tr.ToCanonical(src.parsed)
	if err != nil {
		return contract.CanonicalAgent{}, fmt.Errorf("sync: tocanonical %s/%s: %w", src.ref.Name, src.provider, err)
	}
	out := ancestor
	out.Name = firstNonEmpty(ancestor.Name, pc.Name, src.ref.Name)
	if pc.Description != "" {
		out.Description = pc.Description
	}
	if pc.Model != "" {
		out.Model = pc.Model
	}
	if len(pc.Tools) > 0 {
		out.Tools = pc.Tools
	}
	if len(pc.MCP) > 0 {
		out.MCP = pc.MCP
	}
	if len(pc.Permissions) > 0 {
		out.Permissions = pc.Permissions
	}
	if pc.Body != "" {
		out.Body = pc.Body
	}
	// Merge this provider's overrides bucket.
	if len(pc.ProviderOverrides) > 0 {
		merged := map[string]map[string]any{}
		for k, v := range ancestor.ProviderOverrides {
			merged[k] = v
		}
		for k, v := range pc.ProviderOverrides {
			merged[k] = v
		}
		out.ProviderOverrides = merged
	}
	return out, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// prepareBranches builds the common-ancestor branch and the per-provider
// branches for every agent. It returns the canonbase branch name and, per agent,
// the ordered list of per-provider branch refs to merge.
func (e *Engine) prepareBranches(ws contract.Workspace, run contract.SyncRun, works []agentWork) (string, map[string][]string, error) {
	base := run.BaseBranch
	canonbase := canonBaseBranch(run.RunID)

	// 1. Common-ancestor branch with every agent's ancestor canonical committed.
	if err := e.git.Branch(canonbase, base); err != nil {
		return "", nil, fmt.Errorf("sync: canonbase branch: %w", err)
	}
	cbWT, err := e.git.Worktree(canonbase, canonbase)
	if err != nil {
		return "", nil, fmt.Errorf("sync: canonbase worktree: %w", err)
	}
	for _, w := range works {
		anc := w.ancestor
		if anc.Name == "" {
			anc.Name = w.name
		}
		if err := writeCanonical(cbWT, anc); err != nil {
			return "", nil, err
		}
	}
	if _, err := commitWorktree(cbWT, "graft: common ancestor canonical"); err != nil {
		return "", nil, fmt.Errorf("sync: commit canonbase: %w", err)
	}

	// 2. Per-provider branches folding one provider's changes onto the ancestor.
	order := map[string][]string{}
	for _, w := range works {
		for _, src := range w.changed {
			folded, err := e.foldProvider(w.ancestor, src)
			if err != nil {
				return "", nil, err
			}
			branch := providerBranch(run.RunID, w.name, src.provider)
			if err := e.git.Branch(branch, canonbase); err != nil {
				return "", nil, fmt.Errorf("sync: branch %s: %w", branch, err)
			}
			wt, err := e.git.Worktree(branch, branch)
			if err != nil {
				return "", nil, fmt.Errorf("sync: worktree %s: %w", branch, err)
			}
			if err := writeCanonical(wt, folded); err != nil {
				return "", nil, err
			}
			head, err := commitWorktree(wt, fmt.Sprintf("graft: %s via %s", w.name, src.provider))
			if err != nil {
				return "", nil, fmt.Errorf("sync: commit %s: %w", branch, err)
			}
			_ = e.store.SaveBranch(contract.Branch{
				RunID: run.RunID, Name: branch, Kind: contract.BranchAgent,
				HeadHash: head, State: branchPending,
			})
			order[w.name] = append(order[w.name], branch)
		}
		// Record the agent identity (canonical hash filled after merge resolves).
		if _, err := e.store.UpsertAgent(contract.Agent{
			WsID: ws.ID, Name: w.name, CanonicalHash: canonical.Hash(w.ancestor),
		}); err != nil {
			return "", nil, fmt.Errorf("sync: upsert agent %s: %w", w.name, err)
		}
	}
	return canonbase, order, nil
}

// branch state values recorded in the branches table (State column) so a
// --continue can resume the merge loop exactly where it stopped.
const (
	branchPending  = "pending"  // per-provider branch not yet merged into beta
	branchMerged   = "merged"   // already merged into beta
	branchConflict = "conflict" // the branch whose merge conflicted (in progress)
)

// mergeOrder returns the deterministic global order in which per-provider
// branches are merged into beta: agents sorted by name, providers sorted within.
func mergeOrder(order map[string][]string) []mergeStep {
	var agents []string
	for a := range order {
		agents = append(agents, a)
	}
	sort.Strings(agents)
	var steps []mergeStep
	for _, a := range agents {
		for _, b := range order[a] {
			steps = append(steps, mergeStep{agent: a, branch: b})
		}
	}
	return steps
}

type mergeStep struct {
	agent  string
	branch string
}

// mergeInto runs the sequential three-way merge of every per-provider branch in
// `steps` into the beta branch (via the GitX seam, so a test fake can intercept
// the merge OUTCOME). It begins after `startIdx` (so --continue can skip
// already-merged branches). On a conflict it records branch state and returns
// the conflict; the conflicted (marker-bearing) state is left in beta's worktree
// for the user to resolve. On success it returns the final step index.
func (e *Engine) mergeInto(run contract.SyncRun, betaName, betaWT string, steps []mergeStep, startIdx int) ([]contract.Conflict, int, error) {
	for i := startIdx; i < len(steps); i++ {
		step := steps[i]
		mr, err := e.git.Merge(betaName, step.branch)
		if err != nil {
			return nil, i, fmt.Errorf("sync: merge %s: %w", step.branch, err)
		}
		if mr.Clean {
			_ = e.store.SaveBranch(contract.Branch{
				RunID: run.RunID, Name: step.branch, Kind: contract.BranchAgent, State: branchMerged,
			})
			continue
		}

		// Real conflict. Prefer the actual unmerged paths in beta's worktree (they
		// carry the markers the user will edit); fall back to what the GitX impl
		// reported (e.g. a test fake that does not touch the worktree).
		_ = e.store.SaveBranch(contract.Branch{
			RunID: run.RunID, Name: step.branch, Kind: contract.BranchAgent, State: branchConflict,
		})
		var conflicts []contract.Conflict
		if paths, perr := conflictPathsIn(betaWT); perr == nil && len(paths) > 0 {
			for _, p := range paths {
				conflicts = append(conflicts, contract.Conflict{Path: p, Agent: step.agent})
			}
		} else {
			for _, c := range mr.Conflicts {
				c.Agent = step.agent
				conflicts = append(conflicts, c)
			}
		}
		return conflicts, i, nil
	}
	return nil, len(steps), nil
}

// conflictPathsIn lists unmerged (conflicted) paths in a worktree.
func conflictPathsIn(dir string) ([]string, error) {
	out, err := gitInDir(dir, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, p := range strings.Split(out, "\n") {
		if p = strings.TrimSpace(p); p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

// surfaceConflictToWorkspace copies the conflicted (marker-bearing) canonical
// files from the beta worktree into the user's working tree under root so the
// user edits the SAME .graft/agents/<name>/agent.yaml they see in `graft status`.
func (e *Engine) surfaceConflictToWorkspace(betaWT string, conflicts []contract.Conflict) error {
	for _, c := range conflicts {
		from := filepath.Join(betaWT, c.Path)
		to := filepath.Join(e.root, c.Path)
		data, err := os.ReadFile(from)
		if err != nil {
			return fmt.Errorf("sync: read conflict %s: %w", from, err)
		}
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(to, data, 0o644); err != nil {
			return fmt.Errorf("sync: write conflict %s: %w", to, err)
		}
	}
	return nil
}

// betaWorktree returns the path of the beta branch's linked worktree, creating
// it if needed.
func (e *Engine) betaWorktree(betaBranch string) (string, error) {
	return e.git.Worktree(betaBranch, betaBranch)
}

// worktreeHasStagedOrMerge reports whether the worktree has an in-progress merge
// (MERGE_HEAD) or any staged changes — i.e. whether `git commit` would produce a
// commit. Used to tell a real conflict resolution from a phantom (test-fake) one.
func (e *Engine) worktreeHasStagedOrMerge(dir string) bool {
	if gitDir, err := gitInDir(dir, "rev-parse", "--git-dir"); err == nil {
		gd := strings.TrimSpace(gitDir)
		if !filepath.IsAbs(gd) {
			gd = filepath.Join(dir, gd)
		}
		if _, err := os.Stat(filepath.Join(gd, "MERGE_HEAD")); err == nil {
			return true
		}
	}
	// Staged changes? `git diff --cached --quiet` exits non-zero when staged.
	if _, err := gitInDir(dir, "diff", "--cached", "--quiet"); err != nil {
		return true
	}
	return false
}
