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

// hookOpts derives SkillOpts for the implicit init/sync hook from the stored
// hook config: a single configured provider scopes the apply; AutoInstall maps
// to Yes (non-interactive install of referenced skills).
func (g *gate) hookOpts() contract.SkillOpts {
	opts := contract.SkillOpts{Yes: g.skillHook.AutoInstall}
	if len(g.skillHook.Providers) == 1 {
		opts.Provider = g.skillHook.Providers[0]
	}
	return opts
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
	states, err := g.skillManager().Apply(g.root, g.hookOpts())
	if err != nil {
		log.Printf("[WARN] skills apply hook: %v", err)
		return nil
	}
	// When the config restricts to multiple providers, filter the result set.
	return filterByProviders(states, g.skillHook.Providers)
}

// --- EntryGate skill methods ---------------------------------------------

// SkillList returns the canonical skills under .agent/skills.
func (g *gate) SkillList() ([]contract.Skill, error) {
	return g.skillManager().List()
}

// SkillStatus returns the live per-(provider,skill) link state.
func (g *gate) SkillStatus(opts contract.SkillOpts) ([]contract.SkillStatus, error) {
	return g.skillManager().Status(g.root, opts)
}

// SkillInstall copies a skill into .agent/skills (if absent) then symlinks it
// into the supporting providers, returning the resulting link states.
func (g *gate) SkillInstall(nameOrPath string, opts contract.SkillOpts) ([]contract.SkillStatus, error) {
	mgr := g.skillManager()
	if _, err := mgr.Install(nameOrPath, opts); err != nil {
		return nil, fmt.Errorf("gateway: skill install: %w", err)
	}
	// Report the resulting link states (Install runs Apply internally).
	return mgr.Status(g.root, opts)
}

// SkillSync re-applies: detect + symlink all canonical skills into all
// supporting providers, returning per-provider link states.
func (g *gate) SkillSync(opts contract.SkillOpts) ([]contract.SkillStatus, error) {
	return g.skillManager().Apply(g.root, opts)
}

// filterByProviders keeps only states whose provider is in allow (empty/one =
// no filtering, since a single provider is already scoped via SkillOpts).
func filterByProviders(in []contract.SkillStatus, allow []string) []contract.SkillStatus {
	if len(allow) <= 1 {
		return in
	}
	set := make(map[string]bool, len(allow))
	for _, p := range allow {
		set[p] = true
	}
	out := in[:0:0]
	for _, s := range in {
		if set[s.Provider] {
			out = append(out, s)
		}
	}
	return out
}
