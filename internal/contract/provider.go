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
	// ToCanonical maps a parsed provider agent into the neutral canonical form.
	// Fields with no canonical home MUST be preserved under
	// CanonicalAgent.ProviderOverrides[Name()] so sync stays lossless.
	ToCanonical(p ProviderAgent) (CanonicalAgent, error)
	// Serialize renders a canonical agent into this provider's file(s),
	// restoring any values stashed in ProviderOverrides[Name()].
	Serialize(a CanonicalAgent) ([]FileWrite, error)
	// Schema returns this provider's JSON Schema bytes for validation.
	Schema() []byte
}

// PathScope says where a provider's agent files live relative to a base dir.
type PathScope int

const (
	ScopeProject PathScope = iota // under the workspace root (default)
	ScopeHome                     // under $HOME (e.g. antigravity: ~/.gemini/antigravity-cli)
)

// ScopedProvider is an OPTIONAL capability: a provider implements it only when
// its files are NOT under the workspace root. The engine treats any provider
// that does not implement it as ScopeProject. (Fixes antigravity propagation.)
type ScopedProvider interface {
	PathScope() PathScope
}

// ModelLister is an OPTIONAL capability: a provider implements it when it can
// supply its set of known model ids (from a cached remote source). Used by
// `validate` to flag an unknown model — never hard-blocks sync when offline.
type ModelLister interface {
	Models() ([]string, error)
}

// ToolSupporter is an OPTIONAL capability: a provider implements it to declare
// which tool names it understands. The transformer propagates ONLY supported
// tools to that provider on Serialize; unsupported tools stay in canonical /
// ProviderOverrides (never dropped). A provider that does not implement it is
// treated as supporting every tool (current behavior).
type ToolSupporter interface {
	SupportsTool(tool string) bool
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
