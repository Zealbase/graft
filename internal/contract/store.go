package contract

// GitMode is how a workspace's git is backed.
type GitMode string

const (
	GitTracked  GitMode = "tracked"  // workspace uses its own real git repo/branch
	GitInternal GitMode = "internal" // no real git; graft keeps an internal repo
)

// RunStatus is the lifecycle state of a sync run.
type RunStatus string

const (
	RunRunning  RunStatus = "running"
	RunConflict RunStatus = "conflict" // halted awaiting manual resolution; resumable
	RunDone     RunStatus = "done"
	RunAborted  RunStatus = "aborted"
)

// BranchKind classifies a deterministic graft-created ref.
type BranchKind string

const (
	BranchAgent BranchKind = "agent" // graft/<run_id>/agent/<name>
	BranchBeta  BranchKind = "beta"  // graft/<run_id>/beta/<n>
)

// Workspace identity = (Root, Remote, Branch). One .graft/ tree per row.
type Workspace struct {
	ID        string
	Root      string
	Remote    string
	Branch    string
	GitMode   GitMode
	CreatedAt int64
}

type SyncRun struct {
	RunID         string
	WsID          string
	BaseBranch    string
	BaseStartHash string
	BetaBranch    string
	Phase         string
	Status        RunStatus
	StartedAt     int64
	EndedAt       int64
}

type Branch struct {
	ID       string
	RunID    string
	Name     string
	Kind     BranchKind
	HeadHash string
	State    string
}

type Agent struct {
	ID            string
	WsID          string
	Name          string
	CanonicalHash string
}

type ProviderLink struct {
	ID          string
	AgentID     string
	Provider    string
	FilePath    string
	ContentHash string
	CommitHash  string
}

type AgentState struct {
	ID      string
	RunID   string
	AgentID string
	InSync  bool
	Reason  string
}

// Store is the sqlite-backed persistence layer. Owned by the `db` agent
// (internal/store). Cross-table writes thread a transaction internally.
type Store interface {
	// Workspace gets or creates the workspace row for identity (root,remote,branch),
	// persisting/updating its git_mode (supports internal->tracked migration).
	Workspace(root, remote, branch string, mode GitMode) (Workspace, error)
	// FindWorkspace is a read-only probe: it returns the existing workspace row
	// for (root,remote,branch) or nil if none exists, WITHOUT creating one.
	// Used to derive "initialized?" (replacing the .initialized sentinel) and to
	// gate conflict-run checks without side effects.
	FindWorkspace(root, remote, branch string) (*Workspace, error)
	// UpsertAgent creates or updates an agent row (by ws_id+name), setting
	// name/canonical_hash, and returns it with ID populated. The sync engine
	// calls this before UpsertProviderLink so Drift has identity to compare.
	UpsertAgent(a Agent) (Agent, error)
	OpenRun(wsID, baseBranch, startHash string) (SyncRun, error)
	UpdateRun(run SyncRun) error
	OpenConflictRun(wsID string) (*SyncRun, error) // nil if none to resume
	SaveBranch(b Branch) error
	Branches(runID string) ([]Branch, error)
	SaveConflict(runID string, c Conflict) error
	// ResolveConflicts marks all open conflicts for a run as resolved, called by
	// the engine when a run finalizes successfully after a conflict was surfaced.
	ResolveConflicts(runID string) error
	SaveAgentState(s AgentState) error
	UpsertProviderLink(l ProviderLink) error
	Drift(wsID, name string) (drifted bool, reason string, err error)
	// AgentSynced reports whether a PRIOR sync COMPLETED for (wsID, name): an
	// agents row exists AND has ≥1 provider_links row. A provider link is only
	// recorded by the engine AFTER the resolved canonical lands (applyProviders),
	// so true here means at least one sync ran to completion for this agent — the
	// robust signal the deletion path uses to tell a deleted-after-sync agent from
	// a genuinely-new provider-authored one (an orphan agents row with no links is
	// NOT synced). Returns false (no error) when no agents row exists.
	AgentSynced(wsID, name string) (synced bool, err error)
	// DeleteWorkspace removes a workspace row and all rows that cascade from it
	// (agents, agent_states, provider_links, sync_runs, branches, conflicts),
	// in FK-safe order within a transaction (v0.0.3 task 1 / destroy).
	DeleteWorkspace(wsID string) error
	// DeleteAgent removes one agent's rows (agent_states, provider_links, agents)
	// for (wsID, name), FK-safe leaf-to-root, in a single transaction. A name
	// with no agents row is a no-op (no error). Used by the sync engine to
	// propagate a canonical deletion (v0.0.4 verify task 3 / no-resurrection).
	// Note: sync_runs, branches, and conflicts are run-scoped and are NOT removed.
	DeleteAgent(wsID, name string) error
	Close() error
}
