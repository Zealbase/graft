package gateway_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/gateway"
)

// claudeAgent is a minimal valid Claude Code agent file (YAML frontmatter + body).
const claudeAgent = `---
name: code-reviewer
description: Reviews code changes for correctness and style.
model: sonnet
tools: Read, Grep, Bash
---
You are a meticulous code reviewer. Inspect the diff and report bugs.
`

// newGitWorkspace creates a temp dir initialized as a git repo with one
// committed Claude Code agent file, returning the root.
func newGitWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	run("init", "-q")
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")
	run("config", "commit.gpgsign", "false")

	dir := filepath.Join(root, ".claude", "agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "code-reviewer.md"), []byte(claudeAgent), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-q", "-m", "seed agent")
	return root
}

func openGate(t *testing.T, root string) contract.EntryGate {
	t.Helper()
	g, err := gateway.Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { g.Close() })
	return g
}

func TestInitIdempotent(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)

	res1, err := g.Init()
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if !res1.Created {
		t.Fatalf("first Init Created=false, want true")
	}
	if res1.GitMode != contract.GitTracked {
		t.Fatalf("GitMode=%q, want tracked", res1.GitMode)
	}
	if _, err := os.Stat(filepath.Join(root, ".graft", "graft.db")); err != nil {
		t.Fatalf("graft.db not created: %v", err)
	}

	res2, err := g.Init()
	if err != nil {
		t.Fatalf("second Init: %v", err)
	}
	if res2.Created {
		t.Fatalf("second Init Created=true, want false (idempotent)")
	}
}

func TestSyncThenStatusInSync(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	res, err := g.Sync(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.Status != contract.RunDone {
		t.Fatalf("sync status=%q, want done (conflicts=%v)", res.Status, res.Conflicts)
	}

	// The canonical agent should now exist under .graft/agents.
	if _, err := os.Stat(filepath.Join(root, ".graft", "agents", "code-reviewer", "agent.yaml")); err != nil {
		t.Fatalf("canonical agent not written: %v", err)
	}

	agents, err := g.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(agents) != 1 || agents[0].Name != "code-reviewer" {
		t.Fatalf("List = %+v, want one code-reviewer", agents)
	}
	if !agents[0].InSync {
		t.Fatalf("agent not in sync after sync: %+v", agents[0])
	}

	name := "code-reviewer"
	rep, err := g.Status(&name)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(rep.Agents) != 1 || !rep.Agents[0].InSync {
		t.Fatalf("Status not in sync: %+v", rep)
	}
}

func TestValidateClean(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := g.Sync(contract.SyncOpts{}); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	findings, err := g.Validate("all")
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	for _, f := range findings {
		if f.Severity == "error" {
			t.Fatalf("unexpected error finding: %+v", f)
		}
	}
}

func TestValidateProviderScope(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := g.Sync(contract.SyncOpts{}); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// claude-code has the agent on disk -> validated.
	if _, err := g.Validate("claude-code"); err != nil {
		t.Fatalf("Validate(claude-code): %v", err)
	}
	// An unknown provider id is an error.
	if _, err := g.Validate("nope"); err == nil {
		t.Fatalf("Validate(nope) = nil err, want unknown-provider error")
	}
}

func TestSyncBlocksOnInvalidCanonical(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Seed an invalid canonical agent (missing required fields) directly so the
	// pre-sync validate gate trips.
	bad := filepath.Join(root, ".graft", "agents", "broken")
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	// agent.yaml with no name/description/body -> schema violation.
	if err := os.WriteFile(filepath.Join(bad, "agent.yaml"), []byte("model: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := g.Sync(contract.SyncOpts{Names: []string{"broken"}})
	if err == nil {
		t.Fatalf("Sync over invalid agent succeeded, want validation block")
	}
	if findings := gateway.FindingsOf(err); len(findings) == 0 {
		t.Fatalf("expected ValidationError findings, got: %v", err)
	}
}
