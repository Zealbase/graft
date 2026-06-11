package contract

// FileChange is one path changed between a ref and the working tree.
type FileChange struct {
	Path   string `json:"path"`
	Status string `json:"status"` // added | modified | deleted
}

// Conflict is an unresolved merge at a path, surfaced for manual resolution.
type Conflict struct {
	Path  string `json:"path"`
	Agent string `json:"agent"`
}

// MergeResult reports the outcome of merging one branch into another.
type MergeResult struct {
	Clean     bool       `json:"clean"`
	Head      string     `json:"head"`
	Conflicts []Conflict `json:"conflicts,omitempty"`
}

// GitX abstracts git topology operations. Two impls: go-git (default) and a
// shell fallback. Owned by the `core` agent (internal/gitx).
type GitX interface {
	Init() error
	HeadHash(ref string) (string, error)
	Branch(name, from string) error
	Worktree(name, branch string) (path string, err error)
	Diff(ref string) ([]FileChange, error)
	Merge(into, from string) (MergeResult, error)
	// Copy applies paths from a branch into the working tree without committing.
	Copy(fromBranch string, paths []string) error
	Prune(prefix string) error
}
