package gitx

import (
	"os/exec"
	"strings"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// Context describes the resolved git identity of a working directory: whether a
// real repo backs it (mode), its current branch, and its origin remote URL.
// The sync engine uses it to key the workspace row (root, remote, branch) and to
// pick the sync base branch.
type Context struct {
	Mode   contract.GitMode
	Branch string
	Remote string
}

// Resolve inspects dir and returns its git Context. A directory with no usable
// git repo resolves to GitInternal with a synthetic "main" branch so the engine
// can still operate against an internal repo.
func Resolve(dir string) Context {
	if !hasGitBinary() || !insideWorkTree(dir) {
		return Context{Mode: contract.GitInternal, Branch: "main"}
	}
	branch := gitOut(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if branch == "" || branch == "HEAD" {
		branch = "main"
	}
	remote := gitOut(dir, "config", "--get", "remote.origin.url")
	return Context{Mode: contract.GitTracked, Branch: branch, Remote: remote}
}

func insideWorkTree(dir string) bool {
	out := gitOut(dir, "rev-parse", "--is-inside-work-tree")
	return out == "true"
}

func gitOut(dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
