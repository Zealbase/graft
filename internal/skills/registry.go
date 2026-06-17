// Package skills is the symlink-based skills module. Unlike agents (which are
// transformed/merged and tracked in sqlite), skills are reconciled purely by the
// FILESYSTEM: one canonical copy under <root>/.agents/skills/<name>/ is symlinked
// into each supporting provider's skills dir. There is NO database and no git
// involvement — link state is always computed live (lstat/readlink).
//
// The module is additive: it imports only the frozen contract and the provider
// packages' SkillProvider() plugins. The agent sync engine is untouched.
package skills

import (
	"sort"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/antigravity"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/claudecode"
	clineprov "github.com/Shaik-Sirajuddin/graft/internal/providers/cline"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/codex"
	continueprov "github.com/Shaik-Sirajuddin/graft/internal/providers/continue"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/cursor"
	// deprecated 2026-06-15: gemini-cli removed from the active set (kept in code,
	// unregistered). Import intentionally dropped here so its SkillProvider() is
	// not registered; re-add with geminicli.SkillProvider() below to restore.
	// Package remains in internal/providers/geminicli.
	"github.com/Shaik-Sirajuddin/graft/internal/providers/githubcopilot"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/goose"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/grokcli"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/kilocode"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/opencode"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/roocode"
)

// Registry holds skill-provider plugins. Register accepts every provider, but
// Supporting() (and therefore all symlink/detect actions) returns ONLY those
// whose SkillsSupported() == true; non-supporters are silently skipped.
type Registry struct {
	providers []contract.SkillProvider
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{} }

// Default returns a registry with the active providers' SkillProvider() registered.
// Only the supporting ones (claude-code, codex, grok-cli, opencode) are acted upon.
// NOTE: gemini-cli is DEPRECATED as of 2026-06-15 — it previously was a
// skills-supporting provider but has been removed from the active set (kept in
// code, unregistered). This is distinct from antigravity, which is registered
// here because it is a PLANNED provider still being wired into skills.
func Default() *Registry {
	r := NewRegistry()
	for _, p := range []contract.SkillProvider{
		claudecode.SkillProvider(),
		clineprov.SkillProvider(),
		codex.SkillProvider(),
		continueprov.SkillProvider(),
		// deprecated 2026-06-15: gemini-cli removed from the active set (kept in
		// code, unregistered). Re-add geminicli.SkillProvider() (and its import)
		// to restore it as a skills-supporting provider.
		// geminicli.SkillProvider(),
		cursor.SkillProvider(),
		githubcopilot.SkillProvider(),
		kilocode.SkillProvider(),
		opencode.SkillProvider(),
		roocode.SkillProvider(),
		goose.SkillProvider(),
		grokcli.SkillProvider(),
		antigravity.SkillProvider(),
	} {
		r.Register(p)
	}
	return r
}

// Register adds a skill provider to the registry (supporting or not).
func (r *Registry) Register(p contract.SkillProvider) {
	r.providers = append(r.providers, p)
}

// Supporting returns only the registered providers that support skills, sorted
// by provider name for deterministic fan-out.
func (r *Registry) Supporting() []contract.SkillProvider {
	var out []contract.SkillProvider
	for _, p := range r.providers {
		if p.SkillsSupported() {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// All returns every registered provider (supporting or not), sorted by name.
func (r *Registry) All() []contract.SkillProvider {
	out := make([]contract.SkillProvider, len(r.providers))
	copy(out, r.providers)
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}
