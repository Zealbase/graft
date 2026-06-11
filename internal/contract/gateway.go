package contract

// SyncOpts parameterizes a sync run.
type SyncOpts struct {
	Names    []string // empty = all changed
	Scope    string   // "agents" (default), later "skills"/"slash"
	DryRun   bool
	Continue bool // resume an open conflict run
}

// RunResult is the outcome of a sync.
type RunResult struct {
	RunID     string     `json:"run_id"`
	Status    RunStatus  `json:"status"`
	Changed   []string   `json:"changed,omitempty"`
	Conflicts []Conflict `json:"conflicts,omitempty"`
}

// AgentStatus is one agent's per-provider sync state.
type AgentStatus struct {
	Name      string          `json:"name"`
	Providers map[string]bool `json:"providers"` // provider -> inSync
	InSync    bool            `json:"in_sync"`
}

// StatusReport aggregates drift across agents and providers.
type StatusReport struct {
	Agents             []AgentStatus  `json:"agents"`
	OutOfSyncProviders map[string]int `json:"out_of_sync_providers"` // provider -> #agents drifted
}

// Finding is a single validation result.
type Finding struct {
	Agent    string `json:"agent"`
	Provider string `json:"provider,omitempty"`
	Path     string `json:"path,omitempty"`
	Message  string `json:"message"`
	Severity string `json:"severity"` // error | warning
}

// Validator runs schema + semantic checks. Owned by the `core`/`validate` work.
type Validator interface {
	Validate(scope string) ([]Finding, error)
}

// EntryGate is the single object the CLI talks to. It holds store + engine +
// locks. Owned by the `cli` agent (internal/gateway). The CLI must call only
// this interface — never store/core/gitx/transform/providers directly.
type EntryGate interface {
	List() ([]AgentStatus, error)
	Status(name *string) (StatusReport, error) // nil name = all agents
	Sync(opts SyncOpts) (RunResult, error)
	Validate(scope string) ([]Finding, error)
	Close() error
}
