// Package transform holds the provider registry and bridges the canonical and
// provider forms. It implements contract.Transformer: callers convert a parsed
// provider agent to canonical via ToCanonical (dispatching on the agent's
// Provider id) and render a canonical agent back to a provider's files via
// FromCanonical (dispatching on the named provider).
//
// The registry owns no format knowledge itself — every byte of provider syntax
// lives in the individual internal/providers/<name> packages. Default() wires
// up all ten providers.
package transform

import (
	"fmt"
	"sort"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/antigravity"
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

// Default returns a registry with all ten providers registered.
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
		antigravity.New(),
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

// FromCanonical dispatches on the named provider.
func (r *Registry) FromCanonical(a contract.CanonicalAgent, provider string) ([]contract.FileWrite, error) {
	prov, ok := r.providers[provider]
	if !ok {
		return nil, fmt.Errorf("transform: unknown provider %q", provider)
	}
	return prov.Serialize(a)
}
