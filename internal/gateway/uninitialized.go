package gateway

import "github.com/Shaik-Sirajuddin/graft/internal/gitx"

// UninitializedError is returned by Sync when the workspace has no resolvable
// git HEAD: either the directory is not a git repository at all, or it is a git
// repository with no commits yet. Without a HEAD the sync engine cannot resolve
// a base revision to diff/merge against, so it would otherwise fail deep inside
// gitx with a raw `git rev-parse --verify <branch>: ... Needed a single
// revision` error. We detect the condition up front and surface a clear,
// actionable message instead. The remedy is the same in both cases: run
// `graft init`, which initializes git (git_mode internal/tracked) and makes the
// seed commit the engine needs.
type UninitializedError struct {
	// Root is the workspace directory that lacks a resolvable HEAD.
	Root string
}

// Error returns a clear, actionable message that never leaks the raw git error.
func (e *UninitializedError) Error() string {
	return "not a git repository (or no commits yet) — run 'graft init' first"
}

// IsUninitialized reports whether err is (or wraps) an *UninitializedError.
func IsUninitialized(err error) bool {
	for err != nil {
		if _, ok := err.(*UninitializedError); ok {
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

// headResolvable reports whether the workspace has a resolvable git HEAD for the
// given resolved git context. It is the single detection point for the
// uninitialized/no-commits state: a no-git directory resolves to GitInternal but
// still has no real HEAD, and a git-init'd directory with no commits resolves to
// GitTracked but `rev-parse --verify <branch>` fails. In BOTH cases HeadHash
// returns an error, which we translate into a clean UninitializedError at the
// Sync entry rather than letting the engine surface the raw git failure.
func (g *gate) headResolvable(gctx gitx.Context) bool {
	if _, err := g.git.HeadHash(gctx.Branch); err != nil {
		return false
	}
	return true
}
