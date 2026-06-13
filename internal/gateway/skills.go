package gateway

import (
	"fmt"
	"log"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/skills"
)

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
