package canonical

// Meta is the per-agent `.meta.json` sidecar stored alongside agent.yaml and
// instructions.md under .graft/agents/<name>/. It records, for each provider
// that this canonical agent was last synced from/to, the content hash of that
// provider's source file and the git commit hash at which it was recorded.
// This lets the sync engine detect provider-side drift without re-parsing.
type Meta struct {
	// CanonicalHash is the content hash of the canonical agent (see Hash).
	// It is recomputed on Save so the sidecar is self-describing.
	CanonicalHash string `json:"canonicalHash"`
	// Providers maps a provider id (e.g. "claude") to its recorded source state.
	Providers map[string]ProviderMeta `json:"providers,omitempty"`
}

// ProviderMeta is the recorded state of one provider's source file for an agent.
type ProviderMeta struct {
	// SourceHash is the content hash of the provider's on-disk file.
	SourceHash string `json:"sourceHash"`
	// LastCommitHash is the git commit hash at which SourceHash was observed.
	LastCommitHash string `json:"lastCommitHash"`
	// CanonicalHash is the content hash (see Hash) of the canonical agent this
	// provider's file was last (re)written FROM. A subset sync (--providers=P) can
	// advance the canonical without rewriting providers outside the subset; those
	// providers' SourceHash still equals their unchanged on-disk bytes, so a
	// SourceHash-only comparison would never flag them as stale. Recording the
	// canonical they were written from lets the next sync that includes them detect
	// CanonicalHash != Hash(currentCanonical) and force a rewrite, healing the stale
	// state instead of carrying it forever.
	CanonicalHash string `json:"canonicalHash,omitempty"`
}
