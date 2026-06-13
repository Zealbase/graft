package gateway

import (
	"fmt"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// levenshtein computes the Levenshtein edit distance between two strings.
// Used to produce "did you mean" suggestions for unknown provider keys.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	// dp row: dp[j] = edit distance between a[:i] and b[:j].
	dp := make([]int, lb+1)
	for j := range dp {
		dp[j] = j
	}
	for i := 1; i <= la; i++ {
		prev := dp[0]
		dp[0] = i
		for j := 1; j <= lb; j++ {
			tmp := dp[j]
			if ra[i-1] == rb[j-1] {
				dp[j] = prev
			} else {
				dp[j] = 1 + minInt3(prev, dp[j], dp[j-1])
			}
			prev = tmp
		}
	}
	return dp[lb]
}

func minInt3(a, b, c int) int {
	if a <= b && a <= c {
		return a
	}
	if b <= c {
		return b
	}
	return c
}

// nearestProvider returns the registered provider id closest (by Levenshtein
// edit distance) to key. If multiple are tied, the lexicographically first is
// returned. Returns "" only when the registry is empty.
func nearestProvider(key string, registered []string) string {
	best := ""
	bestDist := -1
	for _, p := range registered {
		d := levenshtein(key, p)
		if bestDist < 0 || d < bestDist || (d == bestDist && p < best) {
			best = p
			bestDist = d
		}
	}
	return best
}

// providerOverrideKeyFindings checks each key in agent.ProviderOverrides against
// the live transformer registry. Unknown keys produce an error-severity finding
// with a "did you mean" suggestion so the pre-sync gate blocks immediately.
func (g *gate) providerOverrideKeyFindings(a contract.CanonicalAgent) []contract.Finding {
	if len(a.ProviderOverrides) == 0 {
		return nil
	}
	registered := g.tr.Providers()
	knownSet := make(map[string]bool, len(registered))
	for _, p := range registered {
		knownSet[p] = true
	}
	var out []contract.Finding
	for key := range a.ProviderOverrides {
		if knownSet[key] {
			continue
		}
		suggestion := nearestProvider(key, registered)
		msg := fmt.Sprintf("providerOverrides: unknown provider %q (did you mean %q?)", key, suggestion)
		out = append(out, contract.Finding{
			Agent:    a.Name,
			Severity: "error",
			Message:  msg,
		})
	}
	return out
}

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
