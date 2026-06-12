package gateway

import (
	"fmt"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// EnabledProvidersConfigurable is the optional capability the CLI type-asserts
// to push the effective enabled-provider set into the gateway, restricting the
// real-time model validation to those providers. Implemented by *gate.
type EnabledProvidersConfigurable interface {
	SetEnabledProviders([]string)
}

// SetEnabledProviders stores the effective enabled-provider set on the gate.
// Empty/nil means "all providers the transformer knows".
func (g *gate) SetEnabledProviders(ids []string) { g.enabledProviders = ids }

// modelProviders returns the provider ids the model check runs against: the
// configured enabled set when present, else every provider the transformer
// knows. Unknown ids in the enabled set are skipped.
func (g *gate) modelProviders() []string {
	if len(g.enabledProviders) == 0 {
		return g.tr.Providers()
	}
	known := map[string]bool{}
	for _, p := range g.tr.Providers() {
		known[p] = true
	}
	var out []string
	for _, id := range g.enabledProviders {
		if known[id] {
			out = append(out, id)
		}
	}
	return out
}

// modelFindings checks the agent's resolved model for each enabled provider that
// implements contract.ModelLister. It returns WARNING findings for models the
// provider does not know. It never returns an error and never produces an
// error-severity finding: when a provider's model list is unavailable (offline /
// no cache) or the provider has no list, the check is silently skipped — so the
// pre-sync gate (which blocks only on errors) is never affected.
func (g *gate) modelFindings(a contract.CanonicalAgent) []contract.Finding {
	var out []contract.Finding
	for _, provName := range g.modelProviders() {
		prov, ok := g.tr.Provider(provName)
		if !ok {
			continue
		}
		lister, ok := prov.(contract.ModelLister)
		if !ok {
			continue // provider has no model list — skip
		}
		model := a.ModelFor(provName)
		if model == "" {
			continue // no model declared for this provider — nothing to check
		}
		known, err := lister.Models()
		if err != nil {
			// Unavailable (offline/no cache) or any other error: skip silently.
			continue
		}
		if !containsString(known, model) {
			out = append(out, contract.Finding{
				Agent:    a.Name,
				Provider: provName,
				Severity: "warning",
				Message:  fmt.Sprintf("model %q is not a known %s model", model, provName),
			})
		}
	}
	return out
}

func containsString(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
