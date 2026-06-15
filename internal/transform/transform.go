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
	"strings"

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
	// warnf is the sink for non-fatal transform warnings (e.g. a native tool
	// with no canonical mapping that is passed through rather than dropped). It
	// defaults to a no-op so the transform never depends on a logger; callers may
	// override it via SetWarnf to surface warnings (CLI, tests).
	warnf func(format string, args ...any)
}

// New returns an empty registry.
func New() *Registry {
	return &Registry{providers: map[string]contract.Provider{}, warnf: func(string, ...any) {}}
}

// SetWarnf installs a warning sink used for non-fatal transform diagnostics
// (unmapped tool pass-through). Passing nil restores the no-op default. Returns
// the registry for chaining.
func (r *Registry) SetWarnf(fn func(format string, args ...any)) *Registry {
	if fn == nil {
		fn = func(string, ...any) {}
	}
	r.warnf = fn
	return r
}

func (r *Registry) warn(format string, args ...any) {
	if r.warnf != nil {
		r.warnf(format, args...)
	}
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

// ToCanonical dispatches on the parsed agent's Provider id. After the provider
// canonicalizes its tools, any tool name that the provider's ToolMapper does not
// recognize is a pass-through (unmapped native tool). These are NEVER dropped —
// they remain in the canonical Tools verbatim — but a warning is emitted so the
// gap can be closed in the provider's tool map.
func (r *Registry) ToCanonical(p contract.ProviderAgent) (contract.CanonicalAgent, error) {
	prov, ok := r.providers[p.Provider]
	if !ok {
		return contract.CanonicalAgent{}, fmt.Errorf("transform: unknown provider %q", p.Provider)
	}
	ca, err := prov.ToCanonical(p)
	if err != nil {
		return ca, err
	}
	if tm, ok := prov.(contract.ToolMapper); ok {
		for _, tool := range ca.Tools {
			// A canonicalized tool that the mapper cannot translate BACK to a
			// native name (and is not a passthrough wildcard) had no mapping on the
			// way in: it was kept verbatim. Surface it, but keep it.
			if _, mapped := tm.NativeTool(tool); !mapped && !isPassthroughTool(tool) {
				r.warn("transform: %s tool %q has no canonical mapping; passing through unchanged", p.Provider, tool)
			}
		}
	}
	return ca, nil
}

// isPassthroughTool reports whether a tool name is a wildcard / MCP / spawn
// pattern that is intentionally provider-agnostic and never carries a canonical
// mapping (so it must not trigger an unmapped-tool warning).
func isPassthroughTool(tool string) bool {
	if tool == "*" {
		return true
	}
	return strings.HasPrefix(tool, "mcp_") ||
		strings.HasPrefix(tool, "mcp__") ||
		strings.HasPrefix(tool, "Agent(")
}

// FromCanonical dispatches on the named provider, applying optional-interface
// policies (ToolSupporter filtering) before delegating to Serialize.
func (r *Registry) FromCanonical(a contract.CanonicalAgent, provider string) ([]contract.FileWrite, error) {
	prov, ok := r.providers[provider]
	if !ok {
		return nil, fmt.Errorf("transform: unknown provider %q", provider)
	}

	// C-D3 per-provider tool override: providerOverrides[provider]["tools"] is a
	// CANONICAL-spelling tool list that REPLACES the canonical Tools set for this
	// provider only (other providers still receive the shared canonical set). It
	// is applied BEFORE MapToNative (which runs inside the provider's Serialize),
	// so the override is mapped to native exactly like the canonical set. The
	// resolution flows through CanonicalAgent.FieldFor (the same path as the model
	// override). The raw "tools" key is then removed from the overrides bucket so
	// the provider's RestoreOverrides does not re-write it verbatim over the
	// freshly mapped native tools.
	if ov, has := a.ProviderOverrides[provider]; has {
		if raw, exists := ov["tools"]; exists && raw != nil {
			if canonTools := toStringSlice(raw); canonTools != nil || isEmptySlice(raw) {
				a = withTools(a, provider, canonTools)
			}
		}
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

// withTools returns a copy of a whose Tools are replaced by the per-provider
// canonical override and whose ProviderOverrides[provider] no longer carries the
// raw "tools" key. The override map is deep-copied for the affected provider so
// the caller's canonical is never mutated.
func withTools(a contract.CanonicalAgent, provider string, tools []string) contract.CanonicalAgent {
	out := a
	out.Tools = tools
	if src, ok := a.ProviderOverrides[provider]; ok {
		newOuter := make(map[string]map[string]any, len(a.ProviderOverrides))
		for p, m := range a.ProviderOverrides {
			newOuter[p] = m
		}
		newInner := make(map[string]any, len(src))
		for k, v := range src {
			if k == "tools" {
				continue
			}
			newInner[k] = v
		}
		newOuter[provider] = newInner
		out.ProviderOverrides = newOuter
	}
	return out
}

// toStringSlice coerces a decoded override value into a []string. Accepts
// []string and []any (the post-YAML/JSON decode shape). Returns nil for any
// other type (including a non-list), which the caller treats as "not a tools
// override".
func toStringSlice(v any) []string {
	switch s := v.(type) {
	case []string:
		out := make([]string, 0, len(s))
		for _, e := range s {
			if e = strings.TrimSpace(e); e != "" {
				out = append(out, e)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(s))
		for _, e := range s {
			if str, ok := e.(string); ok {
				if str = strings.TrimSpace(str); str != "" {
					out = append(out, str)
				}
			}
		}
		return out
	}
	return nil
}

// isEmptySlice reports whether v is an empty list value ([]string{} or []any{}).
// An explicit empty tools override means "this provider gets NO tools" and must
// be honored (distinct from an absent override), so the caller applies it.
func isEmptySlice(v any) bool {
	switch s := v.(type) {
	case []string:
		return len(s) == 0
	case []any:
		return len(s) == 0
	}
	return false
}
