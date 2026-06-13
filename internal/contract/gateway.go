package contract

// SyncOpts parameterizes a sync run.
type SyncOpts struct {
	Names     []string // empty = all changed
	Scope     string   // "agents" (default), later "skills"/"slash"
	DryRun    bool
	Continue  bool     // resume an open conflict run
	Providers []string // enabled provider subset to sync to; empty = all supported
	Ingest    bool     // create canonical for provider-only agents (plan-sync task 5); default true at the CLI
}

// UpdateOpts parameterizes a self-update (plan-sync task 6).
type UpdateOpts struct {
	CheckOnly bool // report current vs latest without replacing the binary
}

// UpdateResult is the outcome of a self-update check/apply.
type UpdateResult struct {
	Current string `json:"current"`
	Latest  string `json:"latest"`
	Updated bool   `json:"updated"`
	Notes   string `json:"notes,omitempty"`
}

// DestroyOpts parameterizes teardown of a workspace's graft-managed state
// (v0.0.3 task 1). Provider agent files are NEVER deleted.
type DestroyOpts struct {
	KeepStore bool // retain .graft/agents canonical store; only drop config/db/lock
}

// DestroyResult reports what teardown removed.
type DestroyResult struct {
	RemovedDir  bool `json:"removed_dir"`  // .graft (or its non-store parts) removed
	RemovedRows int  `json:"removed_rows"` // workspace + cascade rows deleted from the global db
	RemovedLock bool `json:"removed_lock"` // per-workspace lock file removed
}

// RunResult is the outcome of a sync.
type RunResult struct {
	RunID   string    `json:"run_id"`
	Status  RunStatus `json:"status"`
	Changed []string  `json:"changed,omitempty"`
	// Deleted lists agents whose canonical was removed after a prior completed
	// sync and that this run propagated as a DELETE (provider files + db rows
	// removed). On a --dry-run these are the PENDING deletions (nothing was
	// mutated) so the caller can report what a real sync would delete (v0.0.4
	// verify r2 HIGH 1).
	Deleted   []string   `json:"deleted,omitempty"`
	Conflicts []Conflict `json:"conflicts,omitempty"`
	// SkillsLinked lists the "provider/skill" pairs whose canonical-skill symlink
	// this run newly created or repaired (was SkillMissing/SkillWrongLink, now
	// SkillLinked). Empty when skills are disabled or everything was already
	// linked. Used by the CLI to report "linked N skills" instead of claiming
	// "already in sync" when skill drift was actually healed (v0.0.4 verify).
	SkillsLinked []string `json:"skills_linked,omitempty"`
	// SkillsConflicted lists the "provider/skill" pairs that remain in
	// SkillConflict after the apply pass — a real (non-symlink) dir/file occupies
	// the link path and Apply cannot replace it without --override. These are
	// surfaced as a warning so the user is not told "in sync" while a skill is
	// actually unlinked (v0.0.4 verify).
	SkillsConflicted []string `json:"skills_conflicted,omitempty"`
	// SkillsPruned lists the "provider/skill" pairs whose DANGLING (dead) symlink
	// this run removed — a provider symlink pointing into .agents/skills whose
	// canonical target had been deleted, leaving a broken link that Apply/Status
	// (which iterate only canonical skills) never detected. Surfaced so the user
	// sees "pruned N dead skill links" instead of a silent cleanup. Non-fatal
	// (v0.0.4 verify).
	SkillsPruned []string `json:"skills_pruned,omitempty"`
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

// InitResult reports the outcome of initializing a workspace.
type InitResult struct {
	Root    string  `json:"root"`
	GitMode GitMode `json:"git_mode"`
	Created bool    `json:"created"` // false if the workspace already existed
}

// EntryGate is the single object the CLI talks to. It holds store + engine +
// locks. Owned by the `cli` agent (internal/gateway). The CLI must call only
// this interface — never store/core/gitx/transform/providers directly.
type EntryGate interface {
	// Init creates the .graft store + workspace row for the root; idempotent.
	Init() (InitResult, error)
	List() ([]AgentStatus, error)
	Status(name *string) (StatusReport, error) // nil name = all agents
	Sync(opts SyncOpts) (RunResult, error)
	Validate(scope string) ([]Finding, error)

	// CreateAgent scaffolds a default canonical agent in .graft/agents/<name>
	// (plan-sync task 2). prompt seeds instructions.md; empty meta makes the
	// next sync treat it as canonical-drifted and fan out to all providers.
	CreateAgent(name, prompt string) (CanonicalAgent, error)
	// SetAgentModel sets (or with model=="" clears) a per-provider model
	// override on an agent, returning any validation findings (v0.0.3 task 3).
	SetAgentModel(name, provider, model string) ([]Finding, error)
	// Update checks for / applies a newer graft binary (plan-sync task 6).
	Update(opts UpdateOpts) (UpdateResult, error)
	// Destroy removes this workspace's graft-managed state — .graft (per
	// opts), the global-db workspace rows, and the lock — leaving every
	// provider agent file in place (v0.0.3 task 1).
	Destroy(opts DestroyOpts) (DestroyResult, error)

	// --- skills (symlink-based; see plan-skills) ---
	SkillList() ([]Skill, error)
	SkillStatus(opts SkillOpts) ([]SkillStatus, error)
	SkillInstall(nameOrPath string, opts SkillOpts) ([]SkillStatus, error)
	SkillSync(opts SkillOpts) ([]SkillStatus, error)

	Close() error
}
