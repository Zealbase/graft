package sync

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Shaik-Sirajuddin/graft/internal/canonical"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

func nowUnix() int64 { return time.Now().Unix() }

// writeCanonical materializes an agent's canonical .graft/agents/<name> files
// (agent.yaml, instructions.md, .meta.json) into the given working directory
// (a temp worktree). canonical.Save computes the paths and content; this owns
// the actual IO.
func writeCanonical(dir string, can contract.CanonicalAgent) error {
	writes, err := canonical.Save(dir, can)
	if err != nil {
		return fmt.Errorf("sync: canonical save %s: %w", can.Name, err)
	}
	for _, w := range writes {
		abs := w.Path
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(dir, w.Path)
		}
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(abs, w.Data, 0o644); err != nil {
			return fmt.Errorf("sync: write %s: %w", abs, err)
		}
	}
	return nil
}

// commitWorktree stages everything in dir and commits it, returning the new HEAD
// hash. Identity is forced so commits never fail on an unconfigured host.
func commitWorktree(dir, msg string) (string, error) {
	if _, err := gitInDir(dir, "add", "-A"); err != nil {
		return "", err
	}
	if _, err := gitInDir(dir, "commit", "--allow-empty", "-m", msg); err != nil {
		return "", err
	}
	out, err := gitInDir(dir, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// restoreBase guarantees the engine's main working tree is left checked out on
// the base branch. The merge loop runs in isolated worktrees, so the main tree
// should already be on base; this is a safety net that only acts when HEAD has
// drifted off base (e.g. a future/alternate git backend). It is NOT forced, so a
// user's uncommitted edits to tracked files are never discarded — only the
// branch checkout is corrected when needed.
func (e *Engine) restoreBase(baseBranch string) error {
	if baseBranch == "" {
		return nil
	}
	cur, err := gitInDir(e.root, "rev-parse", "--abbrev-ref", "HEAD")
	if err == nil && strings.TrimSpace(cur) == baseBranch {
		return nil // already on base; nothing to restore.
	}
	if _, err := gitInDir(e.root, "checkout", baseBranch); err != nil {
		return err
	}
	return nil
}

func gitInDir(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=graft", "GIT_AUTHOR_EMAIL=graft@local",
		"GIT_COMMITTER_NAME=graft", "GIT_COMMITTER_EMAIL=graft@local",
		"GIT_CONFIG_NOSYSTEM=1",
	)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}
