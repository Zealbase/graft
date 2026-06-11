package gitx

import (
	"errors"
	"os"
	"path/filepath"
	"sort"

	"time"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// goGit implements contract.GitX using go-git for the operations it handles
// cleanly (Init, HeadHash, Branch, Diff) and delegates the topology-heavy
// operations (Worktree, Merge, Copy, Prune) to an embedded shell impl, since
// go-git lacks first-class linked-worktree and recursive-merge support.
type goGit struct {
	dir   string
	shell *shellGit
}

func (g *goGit) open() (*gogit.Repository, error) {
	return gogit.PlainOpen(g.dir)
}

// Init initialises a repo via go-git, creating an initial commit so HEAD is
// defined. If a repo already exists this is a no-op.
func (g *goGit) Init() error {
	if _, err := gogit.PlainOpen(g.dir); err == nil {
		return nil
	}
	if err := os.MkdirAll(g.dir, 0o755); err != nil {
		return err
	}
	repo, err := gogit.PlainInit(g.dir, false)
	if err != nil {
		if errors.Is(err, gogit.ErrRepositoryAlreadyExists) {
			return nil
		}
		return err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}
	// Empty initial commit so branch/merge topology is well-defined.
	_, err = wt.Commit("graft: init", &gogit.CommitOptions{
		AllowEmptyCommits: true,
		Author:            graftSignature(),
	})
	return err
}

// graftSignature is the deterministic author/committer for graft-created commits.
func graftSignature() *object.Signature {
	return &object.Signature{Name: "graft", Email: "graft@local", When: time.Unix(0, 0)}
}

// HeadHash resolves a ref to its commit hash. Empty/"HEAD" resolves HEAD.
func (g *goGit) HeadHash(ref string) (string, error) {
	repo, err := g.open()
	if err != nil {
		return "", err
	}
	if ref == "" || ref == "HEAD" {
		h, err := repo.Head()
		if err != nil {
			return "", err
		}
		return h.Hash().String(), nil
	}
	h, err := repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		// Fall back to the CLI resolver for revisions go-git cannot parse.
		return g.shell.HeadHash(ref)
	}
	return h.String(), nil
}

// Branch creates (or force-updates) a branch ref pointing at from.
func (g *goGit) Branch(name, from string) error {
	repo, err := g.open()
	if err != nil {
		return err
	}
	var target plumbing.Hash
	if from == "" || from == "HEAD" {
		h, err := repo.Head()
		if err != nil {
			return err
		}
		target = h.Hash()
	} else {
		h, err := repo.ResolveRevision(plumbing.Revision(from))
		if err != nil {
			return g.shell.Branch(name, from)
		}
		target = *h
	}
	ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName(name), target)
	return repo.Storer.SetReference(ref)
}

// Diff returns paths differing between ref and the working tree, mapping go-git
// status codes onto added|modified|deleted. Untracked files count as added.
func (g *goGit) Diff(ref string) ([]contract.FileChange, error) {
	repo, err := g.open()
	if err != nil {
		return nil, err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return nil, err
	}
	st, err := wt.Status()
	if err != nil {
		// go-git status can be brittle on some repos; fall back to CLI.
		return g.shell.Diff(ref)
	}
	var out []contract.FileChange
	for path, s := range st {
		code := s.Worktree
		if code == gogit.Unmodified {
			code = s.Staging
		}
		if code == gogit.Unmodified {
			continue
		}
		out = append(out, contract.FileChange{Path: path, Status: mapGoGitStatus(code)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func mapGoGitStatus(c gogit.StatusCode) string {
	switch c {
	case gogit.Added, gogit.Untracked:
		return "added"
	case gogit.Deleted:
		return "deleted"
	default:
		return "modified"
	}
}

// Worktree delegates to the shell impl (go-git has no linked-worktree add).
func (g *goGit) Worktree(name, branch string) (string, error) {
	return g.shell.Worktree(name, branch)
}

// Merge delegates to the shell impl (go-git has no recursive three-way merge).
func (g *goGit) Merge(into, from string) (contract.MergeResult, error) {
	return g.shell.Merge(into, from)
}

// Copy delegates to the shell impl (checkout-into-worktree, no commit).
func (g *goGit) Copy(fromBranch string, paths []string) error {
	return g.shell.Copy(fromBranch, paths)
}

// Prune removes branches under prefix. Implemented directly via go-git refs for
// the branch deletes, with shell worktree cleanup.
func (g *goGit) Prune(prefix string) error {
	repo, err := g.open()
	if err != nil {
		return err
	}
	refs, err := repo.Branches()
	if err != nil {
		return err
	}
	var toDelete []plumbing.ReferenceName
	_ = refs.ForEach(func(r *plumbing.Reference) error {
		short := r.Name().Short()
		if len(short) >= len(prefix) && short[:len(prefix)] == prefix {
			toDelete = append(toDelete, r.Name())
		}
		return nil
	})
	// Detach worktrees first so checked-out branches can be removed.
	_, _ = g.shell.run("worktree", "prune")
	for _, name := range toDelete {
		if err := repo.Storer.RemoveReference(name); err != nil {
			// Likely checked out in a worktree; clean it then retry via CLI.
			g.shell.removeWorktreeFor(name.Short())
			_, _ = g.shell.run("branch", "-D", name.Short())
		}
	}
	_ = os.RemoveAll(filepath.Join(g.dir, ".git", "graft-worktrees"))
	return nil
}
