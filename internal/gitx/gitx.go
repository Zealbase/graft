// Package gitx abstracts the git topology operations the sync engine needs:
// init, head resolution, branch + worktree creation, diff against the working
// tree, sequential merge (clean + conflict), copy-a-tree-without-committing, and
// pruning of deterministic graft refs.
//
// It provides two implementations behind contract.GitX:
//
//   - shellGit  — drives the `git` CLI via os/exec. The reliable workhorse for
//     worktrees and three-way merges (including conflict surfacing).
//   - goGit     — uses github.com/go-git/go-git/v5 for the operations it does
//     cleanly (Init, HeadHash, Branch, Diff) and delegates the merge/worktree/
//     copy topology to an embedded shellGit, since go-git has no first-class
//     worktree-add or recursive-merge support.
//
// New picks goGit when a usable `git` binary is present (go-git still shells out
// for merge/worktree), otherwise falls back to a pure-CLI shellGit. Both satisfy
// contract.GitX.
//
// Deterministic refs (plan 00):
//
//	graft/<run_id>/agent/<name>
//	graft/<run_id>/beta/<n>
package gitx

import (
	"fmt"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// compile-time assertions
var (
	_ contract.GitX = (*shellGit)(nil)
	_ contract.GitX = (*goGit)(nil)
)

// AgentRef builds the deterministic agent branch ref for a run.
func AgentRef(runID, name string) string {
	return fmt.Sprintf("graft/%s/agent/%s", runID, name)
}

// BetaRef builds the deterministic beta branch ref for a run.
func BetaRef(runID string, n int) string {
	return fmt.Sprintf("graft/%s/beta/%d", runID, n)
}

// RunPrefix is the ref prefix that scopes every branch a run creates; passing it
// to Prune removes all of a run's temp branches.
func RunPrefix(runID string) string {
	return fmt.Sprintf("graft/%s/", runID)
}

// New returns a GitX rooted at dir. It prefers the go-git backed impl when a
// usable git binary is available (go-git delegates merge/worktree to the CLI),
// and otherwise returns the pure-CLI shell impl. dir is the repository root /
// working directory.
func New(dir string) contract.GitX {
	sh := newShellGit(dir)
	if hasGitBinary() {
		return &goGit{dir: dir, shell: sh}
	}
	return sh
}

// NewShell forces the pure-CLI implementation (test seam).
func NewShell(dir string) contract.GitX { return newShellGit(dir) }

// NewGoGit forces the go-git backed implementation (test seam).
func NewGoGit(dir string) contract.GitX {
	return &goGit{dir: dir, shell: newShellGit(dir)}
}
