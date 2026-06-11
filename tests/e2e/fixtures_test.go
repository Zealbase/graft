package e2e

import (
	"os"
	"path/filepath"
	"testing"
)

// fixturesDir returns the absolute path to tests/fixtures (sibling of tests/e2e).
func fixturesDir() string {
	return filepath.Join(moduleRoot(), "tests", "fixtures")
}

// newGitWorkspace creates a temp dir, git-inits it, and returns the root.
func newGitWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	gitInit(t, root)
	return root
}

// provisionClaudeAgent copies a fixtures/agents/claude/<file> into the
// workspace's .claude/agents/ directory, then commits it. name is the agent id
// (the file is <name>.md).
func provisionClaudeAgent(t *testing.T, root, name string) {
	t.Helper()
	src := filepath.Join(fixturesDir(), "agents", "claude", name+".md")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read fixture %s: %v", src, err)
	}
	dst := filepath.Join(root, ".claude", "agents", name+".md")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommitAll(t, root, "provision agent "+name)
}

// provisionMergeCase copies a two-provider merge fixture
// (fixtures/merge/<caseName>/{claude,opencode}.md) into the workspace so the
// SAME agent ("dev") is defined by both providers, then commits. Each provider's
// file lands at its native path (.claude/agents/dev.md, .opencode/agents/dev.md)
// so both are detected as changed on the first sync.
func provisionMergeCase(t *testing.T, root, caseName string) {
	t.Helper()
	base := filepath.Join(fixturesDir(), "merge", caseName)
	for _, pf := range []struct{ file, dst string }{
		{"claude.md", ".claude/agents/dev.md"},
		{"opencode.md", ".opencode/agents/dev.md"},
	} {
		data, err := os.ReadFile(filepath.Join(base, pf.file))
		if err != nil {
			t.Fatalf("read merge fixture %s/%s: %v", caseName, pf.file, err)
		}
		abs := filepath.Join(root, pf.dst)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitCommitAll(t, root, "provision merge case "+caseName)
}

// provisionInvalidCanonical drops the invalid canonical agent fixture directly
// under .graft/agents/<name>/ so the validate gate trips. .graft is created by
// init first.
func provisionInvalidCanonical(t *testing.T, root, name string) {
	t.Helper()
	src := filepath.Join(fixturesDir(), "agents", "invalid", "agent.yaml")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read invalid fixture: %v", err)
	}
	dst := filepath.Join(root, ".graft", "agents", name, "agent.yaml")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
