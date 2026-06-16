package gateway

import (
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// codexToolFindings emits a non-blocking warning when an agent has a non-empty
// canonical tools list AND codex is an enabled/target provider. codex's agent
// format has no per-agent tool-control field; tools are silently dropped during
// serialization. This warning makes the silence explicit.
func (g *gate) codexToolFindings(a contract.CanonicalAgent) []contract.Finding {
	if len(a.Tools) == 0 {
		return nil
	}
	if !g.isProviderEnabled("codex") {
		return nil
	}
	return []contract.Finding{{
		Agent:    a.Name,
		Provider: "codex",
		Severity: "warning",
		Message:  "codex: per-agent tool allowlists are not supported by the codex agent format; 'tools' is not written for codex (tool access derives from session/MCP/skills config)",
	}}
}

// isProviderEnabled returns true when providerID is active. When enabledProviders
// is empty (nil or zero-length), ALL transformer providers are considered enabled.
func (g *gate) isProviderEnabled(providerID string) bool {
	if len(g.enabledProviders) == 0 {
		// Check if providerID is in the transformer's registered set.
		for _, p := range g.tr.Providers() {
			if p == providerID {
				return true
			}
		}
		return false
	}
	for _, p := range g.enabledProviders {
		if p == providerID {
			return true
		}
	}
	return false
}
