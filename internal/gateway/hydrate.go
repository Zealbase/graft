package gateway

import (
	"fmt"

	"github.com/Shaik-Sirajuddin/graft/internal/canonical"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// HydrateCapable is the optional capability the CLI type-asserts to compute a
// machine-readable HydrateView for an agent — the consumer contract a host reads
// to spin a runner with the hydrated model/tools/sandbox. Kept off the frozen
// EntryGate contract (additive capability seam).
type HydrateCapable interface {
	Hydrate(name, provider string) (contract.HydrateView, error)
}

// Hydrate resolves one canonical agent into a HydrateView. Name/Tools/Skills/MCP
// come straight from the canonical form. Model and Sandbox are resolved PER the
// requested provider (O1 — sandbox stays provider-scoped, no canonical field):
//
//   - Model: can.ModelFor(provider) — the per-provider override, else canonical.
//   - Sandbox: provider-scoped knobs from providerOverrides[provider] (e.g. codex
//     "sandbox_mode"). Empty when no provider context or none are set.
//
// An empty provider yields an un-scoped view (canonical Model, no Sandbox).
func (g *gate) Hydrate(name, provider string) (contract.HydrateView, error) {
	can, err := canonical.Load(canonical.AgentDir(g.root, name))
	if err != nil {
		return contract.HydrateView{}, fmt.Errorf("gateway: hydrate load %q: %w", name, err)
	}

	model := can.Model
	if provider != "" {
		model = can.ModelFor(provider)
	}

	view := contract.HydrateView{
		Name:   can.Name,
		Model:  model,
		Tools:  can.Tools,
		Skills: can.Skills,
		MCP:    can.MCP,
	}
	if sb := sandboxFor(can, provider); len(sb) > 0 {
		view.Sandbox = sb
	}
	return view, nil
}

// sandboxKeys are the providerOverrides keys treated as sandbox configuration
// when hydrating per provider. Today only codex carries a sandbox knob
// (sandbox_mode); the list is the single place to extend as providers grow
// sandbox semantics, keeping sandbox provider-scoped (no canonical field).
var sandboxKeys = []string{"sandbox_mode"}

// sandboxFor extracts the provider-scoped sandbox knobs for provider from the
// agent's providerOverrides. Returns nil when there is no provider context or no
// sandbox keys are present.
func sandboxFor(a contract.CanonicalAgent, provider string) map[string]string {
	if provider == "" {
		return nil
	}
	bucket, ok := a.ProviderOverrides[provider]
	if !ok || len(bucket) == 0 {
		return nil
	}
	out := map[string]string{}
	for _, key := range sandboxKeys {
		if v, present := bucket[key]; present {
			if s, isStr := v.(string); isStr && s != "" {
				out[key] = s
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
