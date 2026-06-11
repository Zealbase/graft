package gitx

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// shellGit implements contract.GitX by shelling out to the `git` binary. It is
// the reliable backend for worktrees and three-way merges.
type shellGit struct {
	dir string // repository working directory
}

func newShellGit(dir string) *shellGit { return &shellGit{dir: dir} }

// hasGitBinary reports whether a `git` executable is on PATH.
func hasGitBinary() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// run executes git with args in the repo dir and returns trimmed stdout.
func (g *shellGit) run(args ...string) (string, error) {
	return runGit(g.dir, args...)
}

// runIn executes git in an explicit working directory (e.g. a worktree path).
func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	// Deterministic identity so commits in temp branches never fail on a host
	// without a configured user.
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=graft", "GIT_AUTHOR_EMAIL=graft@local",
		"GIT_COMMITTER_NAME=graft", "GIT_COMMITTER_EMAIL=graft@local",
		"GIT_CONFIG_NOSYSTEM=1",
	)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return strings.TrimRight(out.String(), "\n"),
			fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return strings.TrimRight(out.String(), "\n"), nil
}

// Init initialises a git repo at the working directory if one is not present.
func (g *shellGit) Init() error {
	if g.isRepo() {
		return nil
	}
	if err := os.MkdirAll(g.dir, 0o755); err != nil {
		return err
	}
	if _, err := g.run("init"); err != nil {
		return err
	}
	// Establish an initial commit so HEAD and branch ops are well-defined.
	return g.ensureInitialCommit()
}

func (g *shellGit) isRepo() bool {
	out, err := g.run("rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

// ensureInitialCommit creates a root commit if the repo has no HEAD yet.
func (g *shellGit) ensureInitialCommit() error {
	if _, err := g.run("rev-parse", "--verify", "HEAD"); err == nil {
		return nil
	}
	if _, err := g.run("commit", "--allow-empty", "-m", "graft: init"); err != nil {
		return err
	}
	return nil
}

// HeadHash resolves a ref (branch, tag, or "HEAD") to its commit hash.
func (g *shellGit) HeadHash(ref string) (string, error) {
	if ref == "" {
		ref = "HEAD"
	}
	out, err := g.run("rev-parse", "--verify", ref)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// Branch creates branch name pointing at from (a ref/commit). If it already
// exists it is force-reset to from.
func (g *shellGit) Branch(name, from string) error {
	if from == "" {
		from = "HEAD"
	}
	_, err := g.run("branch", "-f", name, from)
	return err
}

// Worktree adds a linked worktree checked out at branch and returns its path.
// The worktree dir lives under <repo>/.git/graft-worktrees/<sanitized-name> by
// default; an existing worktree at that path is reused.
func (g *shellGit) Worktree(name, branch string) (string, error) {
	wtPath := filepath.Join(g.dir, ".git", "graft-worktrees", sanitizeRef(name))
	// If already registered, return it.
	if list, err := g.run("worktree", "list", "--porcelain"); err == nil {
		if strings.Contains(list, wtPath) {
			return wtPath, nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(wtPath), 0o755); err != nil {
		return "", err
	}
	// Branch must already exist; check it out into the new worktree.
	if _, err := g.run("worktree", "add", "--force", wtPath, branch); err != nil {
		return "", err
	}
	return wtPath, nil
}

// Diff returns the paths that differ between ref and the current working tree
// (including untracked files), with an added|modified|deleted status.
func (g *shellGit) Diff(ref string) ([]contract.FileChange, error) {
	if ref == "" {
		ref = "HEAD"
	}
	// Tracked changes vs the ref.
	out, err := g.run("diff", "--name-status", ref)
	if err != nil {
		return nil, err
	}
	changes := parseNameStatus(out)
	seen := map[string]bool{}
	for _, c := range changes {
		seen[c.Path] = true
	}
	// Untracked files show as added.
	un, err := g.run("ls-files", "--others", "--exclude-standard")
	if err == nil {
		for _, p := range strings.Split(un, "\n") {
			p = strings.TrimSpace(p)
			if p == "" || seen[p] {
				continue
			}
			changes = append(changes, contract.FileChange{Path: p, Status: "added"})
		}
	}
	return changes, nil
}

func parseNameStatus(out string) []contract.FileChange {
	var changes []contract.FileChange
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		status := mapGitStatus(fields[0])
		path := fields[len(fields)-1]
		changes = append(changes, contract.FileChange{Path: path, Status: status})
	}
	return changes
}

func mapGitStatus(code string) string {
	switch code[0] {
	case 'A':
		return "added"
	case 'D':
		return "deleted"
	default: // M, R, C, T, ...
		return "modified"
	}
}

// Merge merges from into into. The result reports clean/conflicted, the new head
// of into on success, and the conflicting paths otherwise.
//
// The merge runs inside a dedicated linked worktree checked out on `into` rather
// than in the main working tree. This isolation is essential: the main working
// tree typically carries the previous sync's UNCOMMITTED propagated files (the
// "no commit on base" invariant), which would otherwise make `git checkout into`
// fail with "local changes would be overwritten". The worktree shares the repo's
// object DB, so the resulting merge commit advances the `into` branch ref for
// everyone.
//
// On CONFLICT the half-finished merge is deliberately LEFT IN PLACE in the
// worktree (standard git conflict markers present, MERGE_HEAD set). The sync
// engine surfaces those markers to the user and, on --continue, completes the
// merge after the user resolves them. (Callers that want a clean tree on
// conflict can run a fresh merge later; the worktree is run-scoped and pruned.)
func (g *shellGit) Merge(into, from string) (contract.MergeResult, error) {
	wt, err := g.Worktree(into, into)
	if err != nil {
		return contract.MergeResult{}, err
	}
	// Make sure the worktree is exactly at `into`, discarding any leftover state
	// from a previous (aborted) attempt so the merge starts clean.
	if _, err := runGit(wt, "merge", "--abort"); err == nil {
		// there was an in-progress merge; it is now cleared
	}
	if _, err := runGit(wt, "checkout", "-f", into); err != nil {
		return contract.MergeResult{}, err
	}

	// Attempt a real merge that always creates a commit on success.
	_, mergeErr := runGit(wt, "merge", "--no-edit", "--no-ff",
		"-m", fmt.Sprintf("graft: merge %s into %s", from, into), from)
	if mergeErr == nil {
		head, err := runGit(wt, "rev-parse", "HEAD")
		if err != nil {
			return contract.MergeResult{}, err
		}
		return contract.MergeResult{Clean: true, Head: strings.TrimSpace(head)}, nil
	}

	// Determine whether the failure is a genuine content conflict.
	conflictPaths, cerr := conflictedPathsIn(wt)
	if cerr != nil || len(conflictPaths) == 0 {
		// Not a content conflict (operational error): clean up and surface it.
		_, _ = runGit(wt, "merge", "--abort")
		if cerr != nil {
			return contract.MergeResult{}, cerr
		}
		return contract.MergeResult{}, mergeErr
	}
	var conflicts []contract.Conflict
	for _, p := range conflictPaths {
		conflicts = append(conflicts, contract.Conflict{Path: p})
	}
	// Leave the conflicted worktree IN PLACE (markers + MERGE_HEAD) so the engine
	// can surface and later complete the resolution.
	return contract.MergeResult{Clean: false, Conflicts: conflicts}, nil
}

func (g *shellGit) conflictedPaths() ([]string, error) {
	return conflictedPathsIn(g.dir)
}

func conflictedPathsIn(dir string) ([]string, error) {
	out, err := runGit(dir, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, p := range strings.Split(out, "\n") {
		if p = strings.TrimSpace(p); p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

// Copy applies the given paths from fromBranch into the working tree WITHOUT
// creating a commit on the current branch. With an empty paths slice it applies
// the entire tree of fromBranch. The working tree is updated and staged; the
// caller is responsible for any later commit (the sync engine deliberately does
// not commit the base).
func (g *shellGit) Copy(fromBranch string, paths []string) error {
	args := []string{"checkout", fromBranch, "--"}
	if len(paths) == 0 {
		args = append(args, ".")
	} else {
		args = append(args, paths...)
	}
	_, err := g.run(args...)
	return err
}

// Prune deletes every local branch whose name starts with prefix and removes any
// graft worktrees, leaving the base branch and its history intact.
func (g *shellGit) Prune(prefix string) error {
	out, err := g.run("branch", "--list", prefix+"*", "--format=%(refname:short)")
	if err != nil {
		return err
	}
	// Detach any worktrees that hold these branches first, then delete.
	_, _ = g.run("worktree", "prune")
	for _, b := range strings.Split(out, "\n") {
		b = strings.TrimSpace(b)
		if b == "" {
			continue
		}
		if _, err := g.run("branch", "-D", b); err != nil {
			// Branch may be checked out in a worktree; remove the worktree then retry.
			g.removeWorktreeFor(b)
			if _, err2 := g.run("branch", "-D", b); err2 != nil {
				return err2
			}
		}
	}
	// Clean up the worktree staging directory if empty.
	_ = os.RemoveAll(filepath.Join(g.dir, ".git", "graft-worktrees"))
	return nil
}

func (g *shellGit) removeWorktreeFor(branch string) {
	wtPath := filepath.Join(g.dir, ".git", "graft-worktrees", sanitizeRef(branch))
	_, _ = g.run("worktree", "remove", "--force", wtPath)
}

// sanitizeRef turns a ref name into a single filesystem-safe path segment.
func sanitizeRef(ref string) string {
	r := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_")
	return r.Replace(ref)
}
