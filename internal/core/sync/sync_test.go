package sync

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/canonical"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/gitx"
	"github.com/Shaik-Sirajuddin/graft/internal/store"
	"github.com/Shaik-Sirajuddin/graft/internal/transform"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
}

// newWorkspace creates a temp git repo with a base commit and returns its dir.
func newWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustGit(t, dir, "init")
	mustGit(t, dir, "config", "user.email", "t@t")
	mustGit(t, dir, "config", "user.name", "t")
	// initial commit so HEAD exists
	writeFile(t, dir, "README.md", "repo\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "init")
	return dir
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
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

// writeClaudeAgent drops a Claude Code agent file under .claude/agents/<name>.md.
func writeClaudeAgent(t *testing.T, dir, name, desc, body string) {
	t.Helper()
	content := "---\nname: " + name + "\ndescription: " + desc + "\nmodel: sonnet\n---\n" + body + "\n"
	writeFile(t, dir, filepath.Join(".claude", "agents", name+".md"), content)
}

func newEngine(t *testing.T, dir string) (*Engine, contract.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(dir, "graft.db"))
	if err != nil {
		t.Fatal(err)
	}
	tr := transform.Default()
	g := gitx.New(dir)
	// Point ScopeHome providers (antigravity) at a hermetic temp HOME so tests
	// never read/write the real ~/.gemini/antigravity-cli.
	return New(st, tr, g, dir).SetHomeBase(t.TempDir()), st
}

func TestCleanSyncEndToEnd(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	writeClaudeAgent(t, dir, "reviewer", "reviews code", "You review code.")

	eng, st := newEngine(t, dir)
	defer st.Close()

	res, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res.Status != contract.RunDone {
		t.Fatalf("status = %s, want done (conflicts=%v)", res.Status, res.Conflicts)
	}
	if len(res.Changed) != 1 || res.Changed[0] != "reviewer" {
		t.Fatalf("changed = %v, want [reviewer]", res.Changed)
	}

	// Canonical form written.
	can, err := canonical.Load(canonical.AgentDir(dir, "reviewer"))
	if err != nil {
		t.Fatalf("load canonical: %v", err)
	}
	if can.Name != "reviewer" || can.Model != "sonnet" {
		t.Fatalf("canonical mismatch: %+v", can)
	}

	// Providers fanned out: e.g. codex/gemini files now exist for the agent.
	if _, err := os.Stat(filepath.Join(dir, ".graft", "agents", "reviewer", "agent.yaml")); err != nil {
		t.Fatalf("agent.yaml missing: %v", err)
	}

	// Temp graft branches pruned.
	out, _ := combinedGit(dir, "branch", "--list", "graft/*")
	if out != "" {
		t.Fatalf("temp branches survived: %q", out)
	}

	// Drift: agent should be in sync (claude link == canonical hash).
	drifted, _, err := st.Drift(workspaceID(t, st, dir), "reviewer")
	if err != nil {
		t.Fatalf("drift: %v", err)
	}
	if drifted {
		t.Fatalf("reviewer reported drifted right after sync")
	}
}

func TestNoChangeSync(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	eng, st := newEngine(t, dir)
	defer st.Close()

	res, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res.Status != contract.RunDone || len(res.Changed) != 0 {
		t.Fatalf("expected no-change done, got %+v", res)
	}
}

func TestDryRun(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	writeClaudeAgent(t, dir, "x", "d", "body")
	eng, st := newEngine(t, dir)
	defer st.Close()

	res, err := eng.Run(contract.SyncOpts{DryRun: true})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res.Status != contract.RunDone {
		t.Fatalf("dry-run status = %s", res.Status)
	}
	// No canonical writes on dry run.
	if _, err := os.Stat(canonical.AgentDir(dir, "x")); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote canonical: %v", err)
	}
}

func combinedGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}

func workspaceID(t *testing.T, st contract.Store, dir string) string {
	t.Helper()
	gctx := gitx.Resolve(dir)
	ws, err := st.Workspace(dir, gctx.Remote, gctx.Branch, gctx.Mode)
	if err != nil {
		t.Fatal(err)
	}
	return ws.ID
}
