package gateway

import (
	"fmt"
	"log"
	"sort"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/skills"
)

// skillSyncOutcome summarizes how the sync skill-apply hook affected skill link
// state, for folding into contract.RunResult. All fields are empty when skills
// are disabled or there are no canonical skills.
type skillSyncOutcome struct {
	// Linked lists "provider/skill" pairs newly created or repaired this run
	// (were missing/wrong-link before, linked after).
	Linked []string
	// Conflicted lists "provider/skill" pairs still in SkillConflict after apply
	// (a real dir/file occupies the link path; needs --override).
	Conflicted []string
	// Pruned lists "provider/skill" pairs whose dangling (dead) symlink this run
	// removed — a graft-managed link into .agents/skills whose target was deleted.
	Pruned []string
	// CanonicalSkills is the number of canonical skills under .agents/skills.
	// Used to claim "K skills" in the in-sync summary only when > 0.
	CanonicalSkills int
}

// SkillHookConfig carries the global skills config (XDG, owned by the CLI) that
// gates the implicit init/sync skill-apply hook. The gateway cannot import the
// CLI's config package (layering), so the CLI passes the resolved values via
// SetSkillHookConfig. The zero value disables the hook (Enabled=false); the CLI
// applies the documented default of Enabled=true.
type SkillHookConfig struct {
	Enabled     bool     // master switch for the init/sync hook (default true)
	AutoInstall bool     // install missing referenced skills without prompting
	Providers   []string // restrict which supporting providers get links (empty = all)
}

// SkillHookConfigurable is the optional capability the CLI type-asserts to push
// the skills hook config into the gateway before running hook-triggering
// commands (init/sync). Implemented by *gate.
type SkillHookConfigurable interface {
	SetSkillHookConfig(SkillHookConfig)
}

// SetSkillHookConfig stores the skills hook config on the gate.
func (g *gate) SetSkillHookConfig(c SkillHookConfig) { g.skillHook = c }

// skillManager lazily builds the skills.Manager rooted at the workspace.
func (g *gate) skillManager() *skills.Manager {
	if g.skills == nil {
		g.skills = skills.New(g.root)
	}
	return g.skills
}

// applySkillsHook runs the skill Apply pass after a successful agent init/sync.
// It is gated on skills.enabled and never blocks the agent operation: any error
// is logged (to stderr) and swallowed so a skill problem can't fail agent work.
// It returns the resulting per-(provider,skill) link states for inclusion in a
// summary (callers may ignore them).
func (g *gate) applySkillsHook() []contract.SkillStatus {
	if !g.skillHook.Enabled {
		return nil
	}
	// No provider restriction: apply across all supporting providers in one pass.
	if len(g.skillHook.Providers) == 0 {
		states, err := g.skillManager().Apply(g.root, contract.SkillOpts{Yes: g.skillHook.AutoInstall})
		if err != nil {
			log.Printf("[WARN] skills apply hook: %v", err)
		}
		return states
	}
	// Restricted: apply once PER configured provider (opts.Provider scopes Apply),
	// so symlinks are created only for the allow-listed providers — not created
	// everywhere and then merely filtered out of the returned status.
	var all []contract.SkillStatus
	for _, p := range g.skillHook.Providers {
		states, err := g.skillManager().Apply(g.root, contract.SkillOpts{Yes: g.skillHook.AutoInstall, Provider: p})
		if err != nil {
			log.Printf("[WARN] skills apply hook (%s): %v", p, err)
			continue
		}
		all = append(all, states...)
	}
	return all
}

// applySkillsHookOutcome runs the same Apply pass as applySkillsHook but also
// computes a skillSyncOutcome the caller (Sync) folds into the RunResult so skill
// link state becomes part of the agent-sync "in sync" determination and output.
//
// It is gated on skills.enabled (disabled => zero outcome, no skill claims) and
// is non-fatal: any error reading status or applying links is logged and
// swallowed so a skill problem can never fail the agent sync.
//
// Drift is measured by capturing the LIVE per-(provider,skill) state BEFORE the
// apply (via Status), running Apply to heal, then diffing:
//   - A pair that was SkillMissing/SkillWrongLink and is SkillLinked OR
//     SkillNativeLinked afterward counts as "newly linked" (reported once).
//   - A pair that was already SkillLinked or SkillNativeLinked is NOT counted
//     again on subsequent syncs (idempotent: codex native skills must not be
//     over-reported on every sync).
//   - A pair that is SkillConflict afterward (Apply cannot replace a real
//     dir/file without --override) counts as "conflicted".
func (g *gate) applySkillsHookOutcome() skillSyncOutcome {
	if !g.skillHook.Enabled {
		return skillSyncOutcome{}
	}

	mgr := g.skillManager()

	// Pre-check (v0.0.4 verify): scan every supporting provider's skills dir for
	// graft-managed DANGLING symlinks (target under .agents/skills, now missing)
	// and prune them. This runs UNCONDITIONALLY — NOT gated on canonical skills —
	// because the orphan case (canonical skill deleted, provider symlink left
	// dangling) is exactly when store.List() is empty for that skill, so an
	// Apply/Status pass that iterates only canonical skills would never see it.
	out := skillSyncOutcome{}
	pruned := g.pruneDeadSkillLinks()
	out.Pruned = pruned

	// Canonical skill count: only claim "K skills" in the summary when there are
	// canonical skills to reconcile.
	canon, lerr := mgr.List()
	if lerr != nil {
		log.Printf("[WARN] skills sync hook: list canonical: %v", lerr)
		return out
	}
	if len(canon) == 0 {
		// Still run the apply hook for parity (no-op), but no skill claims. Any
		// pruned dead links are still reported.
		g.applySkillsHook()
		return out
	}

	// Snapshot pre-apply state across the configured provider scope so we can tell
	// what THIS run healed versus what was already linked.
	before := g.skillStateMap()

	// Heal: create/repair the symlinks. Reuse the shared hook so provider scoping
	// and AutoInstall behave identically to init.
	after := g.applySkillsHook()

	out.CanonicalSkills = len(canon)
	seen := map[string]bool{}
	for _, s := range after {
		key := s.Provider + "/" + s.Skill
		seen[key] = true
		switch s.State {
		case contract.SkillLinked, contract.SkillNativeLinked:
			// Newly linked/repaired only if it was NOT already linked before.
			// SkillNativeLinked (codex native canonical discovery) is counted once
			// on first link, never over-reported on subsequent syncs.
			if prev, ok := before[key]; !ok || (prev != contract.SkillLinked && prev != contract.SkillNativeLinked) {
				out.Linked = append(out.Linked, key)
			}
		case contract.SkillConflict:
			out.Conflicted = append(out.Conflicted, key)
		}
	}
	// Apply may skip a provider/skill it failed to link (accumulated as an error
	// and dropped from the returned states). Fold any pre-apply conflict that the
	// apply did not resolve so a SkillConflict is never silently lost from the
	// summary.
	for key, st := range before {
		if seen[key] {
			continue
		}
		if st == contract.SkillConflict {
			out.Conflicted = append(out.Conflicted, key)
		}
	}
	sort.Strings(out.Linked)
	sort.Strings(out.Conflicted)
	return out
}

// pruneDeadSkillLinks runs the dangling-symlink pre-check across the configured
// provider scope and returns the pruned "provider/skill" pairs (sorted). It is
// non-fatal: any error is logged and the partial set of pruned links is still
// returned, so a prune failure never fails the agent sync.
func (g *gate) pruneDeadSkillLinks() []string {
	mgr := g.skillManager()
	var pruned []string
	prune := func(opts contract.SkillOpts) {
		states, err := mgr.PruneDeadLinks(g.root, opts)
		if err != nil {
			log.Printf("[WARN] skills sync hook: prune dead links: %v", err)
			// fall through: states holds the links pruned before the error
		}
		for _, s := range states {
			pruned = append(pruned, s.Provider+"/"+s.Skill)
		}
	}
	if len(g.skillHook.Providers) == 0 {
		prune(contract.SkillOpts{})
	} else {
		for _, p := range g.skillHook.Providers {
			prune(contract.SkillOpts{Provider: p})
		}
	}
	sort.Strings(pruned)
	return pruned
}

// skillStateMap returns the live per-(provider,skill) state keyed by
// "provider/skill" across the configured provider scope. Read-only; errors are
// logged and yield a partial/empty map (treated as "unknown prior state").
func (g *gate) skillStateMap() map[string]contract.SkillLinkState {
	m := map[string]contract.SkillLinkState{}
	mgr := g.skillManager()
	collect := func(opts contract.SkillOpts) {
		states, err := mgr.Status(g.root, opts)
		if err != nil {
			log.Printf("[WARN] skills sync hook: status probe: %v", err)
			return
		}
		for _, s := range states {
			m[s.Provider+"/"+s.Skill] = s.State
		}
	}
	if len(g.skillHook.Providers) == 0 {
		collect(contract.SkillOpts{})
		return m
	}
	for _, p := range g.skillHook.Providers {
		collect(contract.SkillOpts{Provider: p})
	}
	return m
}

// --- EntryGate skill methods ---------------------------------------------

// SkillList returns the canonical skills under .agents/skills.
func (g *gate) SkillList() ([]contract.Skill, error) {
	return g.skillManager().List()
}

// SkillStatus returns the live per-(provider,skill) link state.
func (g *gate) SkillStatus(opts contract.SkillOpts) ([]contract.SkillStatus, error) {
	return g.skillManager().Status(g.root, opts)
}

// SkillInstall copies a skill into .agents/skills (if absent) then symlinks it
// into the supporting providers, returning the resulting link states.
//
// opts.Provider scopes BOTH the install (Install's internal Apply only links the
// named provider) and the returned states symmetrically: when set, the link is
// created only at that provider and the returned states cover only that provider.
// This is intended — the returned states always describe exactly what was just
// linked, never a misleadingly partial view of a broader operation. When
// opts.Provider is empty, all supporting providers are linked and reported.
func (g *gate) SkillInstall(nameOrPath string, opts contract.SkillOpts) ([]contract.SkillStatus, error) {
	mgr := g.skillManager()
	if _, err := mgr.Install(nameOrPath, opts); err != nil {
		// Install runs Apply internally, which can partially succeed (some
		// providers linked) before returning an error. Surface that partial
		// state by re-reading live status with the SAME opts, so the CLI can show
		// which providers were linked before the failure — mirroring how SkillSync
		// returns partial states alongside its error. Status is read-only
		// (lstat/readlink); if it too fails we fall back to nil states.
		states, serr := mgr.Status(g.root, opts)
		if serr != nil {
			states = nil
		}
		return states, fmt.Errorf("gateway: skill install: %w", err)
	}
	// Report the resulting link states (Install runs Apply internally). Reuse the
	// same opts so the reported scope matches the scope that was just applied.
	return mgr.Status(g.root, opts)
}

// SkillSync re-applies: detect + symlink all canonical skills into all
// supporting providers, returning per-provider link states.
func (g *gate) SkillSync(opts contract.SkillOpts) ([]contract.SkillStatus, error) {
	return g.skillManager().Apply(g.root, opts)
}
