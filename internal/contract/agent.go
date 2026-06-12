// Package contract is the frozen, head-owned shared contract for graft.
// It defines the cross-boundary domain types and interfaces that every other
// package codes against. It has no dependencies outside the standard library,
// so it can be frozen before any agent fans out.
package contract

// CanonicalAgent is the provider-neutral representation of one agent, stored
// under .graft/agents/<name>/. Its concrete shape is owned by the `canonical`
// agent (internal/canonical); the fields here are the frozen wire vocabulary
// that crosses package boundaries.
type CanonicalAgent struct {
	Name              string                            `json:"name"`
	Description       string                            `json:"description,omitempty"`
	Model             string                            `json:"model,omitempty"`
	Tools             []string                          `json:"tools,omitempty"`
	MCP               []string                          `json:"mcp,omitempty"`
	Permissions       map[string]string                 `json:"permissions,omitempty"`
	Body              string                            `json:"-"` // instructions.md content
	ProviderOverrides map[string]map[string]any         `json:"providerOverrides,omitempty"`
}

// ModelFor resolves the model to write for a given provider: the per-provider
// override (ProviderOverrides[provider]["model"]) when set, else the shared
// canonical default (Model). This makes per-provider models first-class while
// keeping a single canonical default.
func (a CanonicalAgent) ModelFor(provider string) string {
	if ov, ok := a.ProviderOverrides[provider]; ok {
		if m, ok := ov["model"].(string); ok && m != "" {
			return m
		}
	}
	return a.Model
}

// ProviderAgent is one agent as it exists in a specific provider's on-disk form.
type ProviderAgent struct {
	Provider string         `json:"provider"`
	Ref      AgentRef       `json:"ref"`
	Fields   map[string]any `json:"fields,omitempty"`
	Body     string         `json:"-"`
	Raw      []byte         `json:"-"`
}

// AgentRef locates an agent file for a provider within a workspace.
type AgentRef struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Path     string `json:"path"`
}

// FileWrite is a single file the engine should write when applying a canonical
// agent to a provider.
type FileWrite struct {
	Path string `json:"path"`
	Data []byte `json:"-"`
}
