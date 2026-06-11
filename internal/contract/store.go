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
	Workspace(root, remote, branch string) (Workspace, error)
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
	SaveAgentState(s AgentState) error
	UpsertProviderLink(l ProviderLink) error
	Drift(wsID, name string) (drifted bool, reason string, err error)
	Close() error
}
