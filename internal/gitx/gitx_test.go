package gitx

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

func requireGit(t *testing.T) {
	t.Helper()
	if !hasGitBinary() {
		t.Skip("git binary not available")
	}
}

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gitCommit(t *testing.T, dir, msg string) string {
	t.Helper()
	if _, err := runGit(dir, "add", "-A"); err != nil {
		t.Fatal(err)
	}
	if _, err := runGit(dir, "commit", "--allow-empty", "-m", msg); err != nil {
		t.Fatal(err)
	}
	h, err := runGit(dir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// newTestRepos returns the two impls under test for the same temp dir helper.
func eachImpl(t *testing.T) []struct {
	name string
	make func(dir string) contract.GitX
} {
	requireGit(t)
	return []struct {
		name string
		make func(dir string) contract.GitX
	}{
		{"shell", func(dir string) contract.GitX { return NewShell(dir) }},
		{"gogit", func(dir string) contract.GitX { return NewGoGit(dir) }},
	}
}

func TestInitAndHead(t *testing.T) {
	for _, impl := range eachImpl(t) {
		t.Run(impl.name, func(t *testing.T) {
			dir := t.TempDir()
			g := impl.make(dir)
			if err := g.Init(); err != nil {
				t.Fatalf("init: %v", err)
			}
			h, err := g.HeadHash("HEAD")
			if err != nil || h == "" {
				t.Fatalf("head: %v %q", err, h)
			}
		})
	}
}

func TestBranchDiff(t *testing.T) {
	for _, impl := range eachImpl(t) {
		t.Run(impl.name, func(t *testing.T) {
			dir := t.TempDir()
			g := impl.make(dir)
			if err := g.Init(); err != nil {
				t.Fatal(err)
			}
			writeFile(t, dir, "a.txt", "one\n")
			base := gitCommit(t, dir, "base")

			if err := g.Branch("graft/r1/agent/x", base); err != nil {
				t.Fatalf("branch: %v", err)
			}
			if _, err := g.HeadHash("graft/r1/agent/x"); err != nil {
				t.Fatalf("branch head: %v", err)
			}

			// Modify working tree; Diff vs HEAD should report it.
			writeFile(t, dir, "a.txt", "two\n")
			writeFile(t, dir, "b.txt", "new\n")
			ch, err := g.Diff("HEAD")
			if err != nil {
				t.Fatalf("diff: %v", err)
			}
			got := map[string]string{}
			for _, c := range ch {
				got[c.Path] = c.Status
			}
			if got["a.txt"] != "modified" {
				t.Errorf("a.txt status = %q, want modified (%v)", got["a.txt"], got)
			}
			if got["b.txt"] != "added" {
				t.Errorf("b.txt status = %q, want added (%v)", got["b.txt"], got)
			}
		})
	}
}

func TestWorktree(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	g := NewShell(dir)
	if err := g.Init(); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "a.txt", "one\n")
	base := gitCommit(t, dir, "base")
	if err := g.Branch("graft/r1/agent/x", base); err != nil {
		t.Fatal(err)
	}
	wt, err := g.Worktree("graft/r1/agent/x", "graft/r1/agent/x")
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wt, "a.txt")); err != nil {
		t.Fatalf("worktree missing file: %v", err)
	}
}

func TestMergeClean(t *testing.T) {
	for _, impl := range eachImpl(t) {
		t.Run(impl.name, func(t *testing.T) {
			dir := t.TempDir()
			g := impl.make(dir)
			if err := g.Init(); err != nil {
				t.Fatal(err)
			}
			writeFile(t, dir, "a.txt", "one\n")
			base := gitCommit(t, dir, "base")

			// target branch is what we merge INTO (never the checked-out branch;
			// the engine always merges into a dedicated beta branch in a worktree).
			g.Branch("target", base)
			// feature branch adds a separate file → clean merge.
			g.Branch("feature", base)
			runGit(dir, "checkout", "feature")
			writeFile(t, dir, "b.txt", "feature\n")
			gitCommit(t, dir, "feat")
			runGit(dir, "checkout", base)

			res, err := g.Merge("target", "feature")
			if err != nil {
				t.Fatalf("merge: %v", err)
			}
			if !res.Clean {
				t.Fatalf("expected clean merge, got conflicts %v", res.Conflicts)
			}
		})
	}
}

func TestMergeConflict(t *testing.T) {
	for _, impl := range eachImpl(t) {
		t.Run(impl.name, func(t *testing.T) {
			dir := t.TempDir()
			g := impl.make(dir)
			if err := g.Init(); err != nil {
				t.Fatal(err)
			}
			writeFile(t, dir, "a.txt", "base\n")
			base := gitCommit(t, dir, "base")

			// target diverges from feature on the same line of a.txt → conflict.
			g.Branch("target", base)
			runGit(dir, "checkout", "target")
			writeFile(t, dir, "a.txt", "target change\n")
			gitCommit(t, dir, "target edit")

			g.Branch("feature", base)
			runGit(dir, "checkout", "feature")
			writeFile(t, dir, "a.txt", "feature change\n")
			gitCommit(t, dir, "feat edit")
			runGit(dir, "checkout", base)

			res, err := g.Merge("target", "feature")
			if err != nil {
				t.Fatalf("merge: %v", err)
			}
			if res.Clean {
				t.Fatal("expected conflict, got clean")
			}
			if len(res.Conflicts) == 0 || res.Conflicts[0].Path != "a.txt" {
				t.Fatalf("expected a.txt conflict, got %v", res.Conflicts)
			}
			// New contract: the conflicted merge is LEFT IN PLACE (markers present)
			// in target's linked worktree so the engine can surface + resolve it.
			// The MAIN working tree (dir) stays clean (conflict is in the worktree).
			if out, _ := runGit(dir, "ls-files", "-u"); out != "" {
				t.Fatalf("main worktree not clean after conflict: %q", out)
			}
		})
	}
}

func TestCopyNoCommit(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	g := NewShell(dir)
	if err := g.Init(); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "a.txt", "one\n")
	base := gitCommit(t, dir, "base")

	g.Branch("beta", base)
	runGit(dir, "checkout", "beta")
	writeFile(t, dir, "a.txt", "beta content\n")
	gitCommit(t, dir, "beta edit")
	runGit(dir, "checkout", "-")

	headBefore, _ := g.HeadHash("HEAD")
	if err := g.Copy("beta", []string{"a.txt"}); err != nil {
		t.Fatalf("copy: %v", err)
	}
	// Working tree updated...
	b, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(b) != "beta content\n" {
		t.Fatalf("copy did not update working tree: %q", b)
	}
	// ...but NO commit was made on the base.
	headAfter, _ := g.HeadHash("HEAD")
	if headBefore != headAfter {
		t.Fatalf("copy committed to base: %s -> %s", headBefore, headAfter)
	}
}

func TestPrune(t *testing.T) {
	for _, impl := range eachImpl(t) {
		t.Run(impl.name, func(t *testing.T) {
			dir := t.TempDir()
			g := impl.make(dir)
			if err := g.Init(); err != nil {
				t.Fatal(err)
			}
			writeFile(t, dir, "a.txt", "one\n")
			base := gitCommit(t, dir, "base")
			g.Branch("graft/r1/agent/x", base)
			g.Branch("graft/r1/beta/0", base)
			g.Branch("keep", base)

			if err := g.Prune("graft/r1/"); err != nil {
				t.Fatalf("prune: %v", err)
			}
			out, _ := runGit(dir, "branch", "--list", "graft/r1/*", "--format=%(refname:short)")
			if out != "" {
				t.Fatalf("graft branches survived prune: %q", out)
			}
			if _, err := g.HeadHash("keep"); err != nil {
				t.Fatalf("prune deleted unrelated branch: %v", err)
			}
		})
	}
}

func TestInitSeedsInternalBranch(t *testing.T) {
	for _, impl := range eachImpl(t) {
		t.Run(impl.name, func(t *testing.T) {
			dir := t.TempDir()
			g := impl.make(dir)
			if err := g.Init(); err != nil {
				t.Fatalf("init: %v", err)
			}
			h, err := g.HeadHash(InternalBranch)
			if err != nil {
				t.Fatalf("HeadHash(%q): %v", InternalBranch, err)
			}
			if h == "" {
				t.Fatalf("HeadHash(%q) returned empty hash", InternalBranch)
			}
			if got := currentBranch(t, dir); got != InternalBranch {
				t.Fatalf("current branch = %q, want %q", got, InternalBranch)
			}
		})
	}
}

func TestInitDoesNotRenameExistingBranch(t *testing.T) {
	requireGit(t)
	makers := []struct {
		name string
		make func(dir string) contract.GitX
	}{
		{"gogit", func(dir string) contract.GitX { return NewGoGit(dir) }},
		{"shell", func(dir string) contract.GitX { return NewShell(dir) }},
	}
	for _, m := range makers {
		t.Run(m.name, func(t *testing.T) {
			dir := t.TempDir()
			if _, err := runGit(dir, "init"); err != nil {
				t.Fatalf("init: %v", err)
			}
			if _, err := runGit(dir, "symbolic-ref", "HEAD", "refs/heads/release"); err != nil {
				t.Fatalf("symbolic-ref: %v", err)
			}
			writeFile(t, dir, "a.txt", "one\n")
			gitCommit(t, dir, "init")

			// Init must be a no-op on an existing repo: the branch stays "release".
			g := m.make(dir)
			if err := g.Init(); err != nil {
				t.Fatalf("init (no-op): %v", err)
			}
			if got := currentBranch(t, dir); got != "release" {
				t.Fatalf("current branch = %q, want %q (Init renamed an existing branch)", got, "release")
			}
		})
	}
}

func currentBranch(t *testing.T, dir string) string {
	t.Helper()
	out, err := runGit(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	return out
}
