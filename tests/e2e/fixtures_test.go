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
