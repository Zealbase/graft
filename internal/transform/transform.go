// Package transform holds the provider registry and bridges the canonical and
// provider forms. It implements contract.Transformer: callers convert a parsed
// provider agent to canonical via ToCanonical (dispatching on the agent's
// Provider id) and render a canonical agent back to a provider's files via
// FromCanonical (dispatching on the named provider).
//
// The registry owns no format knowledge itself — every byte of provider syntax
// lives in the individual internal/providers/<name> packages. Default() wires
// up the nine active providers (antigravity is unregistered pending research).
//
// FromCanonical applies optional-interface policies before delegating to
// Serialize:
//
//   - contract.ToolSupporter + contract.ToolMapper: if a provider implements
//     SupportsTool, the CanonicalAgent.Tools slice passed to Serialize is
//     filtered to only the tools the provider supports. When the provider also
//     implements ToolMapper, mapped tools are checked by their native name;
//     unmapped (pass-through) tools are always kept.
//
// Model resolution is handled inside each provider's Serialize via
// contract.CanonicalAgent.ModelFor(name) — no extra logic needed here.
package transform

import (
	"fmt"
	"sort"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/claudecode"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/codex"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/cursor"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/geminicli"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/githubcopilot"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/goose"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/grokcli"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/opencode"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/roocode"
)

// Registry is the in-memory map of provider id -> Provider implementation.
// It satisfies contract.Transformer.
type Registry struct {
	providers map[string]contract.Provider
}

// New returns an empty registry.
func New() *Registry {
	return &Registry{providers: map[string]contract.Provider{}}
}

// Default returns a registry with the nine active providers registered.
func Default() *Registry {
	r := New()
	for _, p := range []contract.Provider{
		claudecode.New(),
		codex.New(),
		geminicli.New(),
		cursor.New(),
		githubcopilot.New(),
		opencode.New(),
		roocode.New(),
		goose.New(),
		grokcli.New(),
		// TODO(2026-06-13): antigravity (agy) unregistered — agent-def format/home-scope not yet clarified;
		// re-register after a research spike. See tasks/_draft/antigravity-deferred.yaml.
	} {
		r.Register(p)
	}
	return r
}

// Register adds (or replaces) a provider keyed by its Name().
func (r *Registry) Register(p contract.Provider) {
	if r.providers == nil {
		r.providers = map[string]contract.Provider{}
	}
	r.providers[p.Name()] = p
}

// Provider returns the registered provider for name.
func (r *Registry) Provider(name string) (contract.Provider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

// Providers returns the sorted list of registered provider ids.
func (r *Registry) Providers() []string {
	out := make([]string, 0, len(r.providers))
	for name := range r.providers {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// ToCanonical dispatches on the parsed agent's Provider id.
func (r *Registry) ToCanonical(p contract.ProviderAgent) (contract.CanonicalAgent, error) {
	prov, ok := r.providers[p.Provider]
	if !ok {
		return contract.CanonicalAgent{}, fmt.Errorf("transform: unknown provider %q", p.Provider)
	}
	return prov.ToCanonical(p)
}

// FromCanonical dispatches on the named provider, applying optional-interface
// policies (ToolSupporter filtering) before delegating to Serialize.
func (r *Registry) FromCanonical(a contract.CanonicalAgent, provider string) ([]contract.FileWrite, error) {
	prov, ok := r.providers[provider]
	if !ok {
		return nil, fmt.Errorf("transform: unknown provider %q", provider)
	}
	// Apply ToolSupporter filtering: remove tools the target provider does not
	// support. A shallow copy of a is made so the caller's canonical is unchanged.
	// When the provider also implements ToolMapper, mapped tools are checked by
	// their native name; unmapped (pass-through) tools are always kept.
	if ts, ok := prov.(contract.ToolSupporter); ok && len(a.Tools) > 0 {
		tm, hasMapper := prov.(contract.ToolMapper)
		filtered := a.Tools[:0:0]
		for _, canonical := range a.Tools {
			if hasMapper {
				native, ok := tm.NativeTool(canonical)
				if ok {
					// mapped: check support by native name
					if ts.SupportsTool(native) {
						filtered = append(filtered, canonical)
					}
					continue
				}
			}
			// unmapped (pass-through): always keep
			filtered = append(filtered, canonical)
		}
		if len(filtered) != len(a.Tools) {
			a = shallowCopy(a)
			a.Tools = filtered
		}
	}
	return prov.Serialize(a)
}

// shallowCopy returns a value copy of a (maps/slices are NOT deep-copied; only
// the top-level struct is copied, which is enough since we replace Tools).
func shallowCopy(a contract.CanonicalAgent) contract.CanonicalAgent { return a }
