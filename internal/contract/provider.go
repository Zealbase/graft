package contract

// Provider is implemented once per target tool (claude-code, codex, gemini-cli,
// cursor, github-copilot, opencode, roo-code, goose, grok-cli, antigravity).
// Owned by the `provider` agent (internal/providers/<name>).
type Provider interface {
	// Name is the canonical provider id, e.g. "claude-code".
	Name() string
	// Detect returns the agent files this provider has under root.
	Detect(root string) ([]AgentRef, error)
	// Parse reads one provider agent file into its provider-shaped form.
	Parse(path string) (ProviderAgent, error)
	// Serialize renders a canonical agent into this provider's file(s).
	Serialize(a CanonicalAgent) ([]FileWrite, error)
	// Schema returns this provider's JSON Schema bytes for validation.
	Schema() []byte
}

// Transformer converts between canonical and provider forms and holds the
// provider registry. Owned by the `provider` agent (internal/transform).
type Transformer interface {
	ToCanonical(p ProviderAgent) (CanonicalAgent, error)
	FromCanonical(a CanonicalAgent, provider string) ([]FileWrite, error)
	Register(p Provider)
	Provider(name string) (Provider, bool)
	Providers() []string
}
