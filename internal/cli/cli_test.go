package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/cli"
	"github.com/Shaik-Sirajuddin/graft/internal/cli/config"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/gateway"
	"github.com/adrg/xdg"
)

const claudeAgent = `---
name: code-reviewer
description: Reviews code changes for correctness and style.
model: sonnet
tools: Read, Grep, Bash
---
You are a meticulous code reviewer. Inspect the diff and report bugs.
`

func newWorkspace(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	// Isolate XDG so the GLOBAL graft.db + locks (data) and global config are
	// per-test, never touching the real user dirs. Config isolation also makes
	// the first-run flow deterministic (a fresh config each test).
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	xdg.Reload()
	// Isolate HOME: the sync engine resolves ScopeHome providers (antigravity ->
	// ~/.gemini/antigravity-cli) against os.UserHomeDir. Without this a real sync
	// would read/pollute the host HOME. Point it at a temp dir.
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
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
	run("commit", "-q", "-m", "seed")
	return root
}

// execCLI runs the cobra tree with args against a gate opened at root, capturing
// stdout. resolver is the (test) config resolver; pass nil for default.
func execCLI(t *testing.T, root string, resolver config.Resolver, args ...string) (string, error) {
	t.Helper()
	g, err := gateway.Open(root)
	if err != nil {
		t.Fatalf("gateway.Open: %v", err)
	}
	defer g.Close()

	c := cli.EntrypointWithVersion(g, resolver, "test")
	var out, errBuf bytes.Buffer
	root2 := c.Root()
	root2.SetOut(&out)
	// stderr is kept separate: first-run branding / log lines go there, so the
	// captured stdout stays clean (results-only). Tests asserting on stderr can
	// read it via execCLIStreams.
	root2.SetErr(&errBuf)
	root2.SetArgs(args)
	err = root2.Execute()
	return out.String(), err
}

// execCLIStreams is execCLI but returns stdout and stderr separately (for tests
// asserting on first-run branding / prompts written to stderr).
func execCLIStreams(t *testing.T, root string, resolver config.Resolver, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	g, gerr := gateway.Open(root)
	if gerr != nil {
		t.Fatalf("gateway.Open: %v", gerr)
	}
	defer g.Close()

	c := cli.EntrypointWithVersion(g, resolver, "test")
	var out, errBuf bytes.Buffer
	r := c.Root()
	r.SetOut(&out)
	r.SetErr(&errBuf)
	r.SetArgs(args)
	err = r.Execute()
	return out.String(), errBuf.String(), err
}

// execNoGate runs commands that don't need the gateway (config), gate=nil.
func execNoGate(t *testing.T, resolver config.Resolver, args ...string) (string, error) {
	t.Helper()
	c := cli.EntrypointWithVersion(nil, resolver, "test")
	var out, errBuf bytes.Buffer
	r := c.Root()
	r.SetOut(&out)
	r.SetErr(&errBuf)
	r.SetArgs(args)
	err := r.Execute()
	return out.String(), err
}

func TestCLIInit(t *testing.T) {
	root := newWorkspace(t)
	out, err := execCLI(t, root, nil, "init", "-o", "json")
	if err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	var res contract.InitResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("parse init json: %v\n%s", err, out)
	}
	if !res.Created || res.GitMode != contract.GitTracked {
		t.Fatalf("unexpected init result: %+v", res)
	}
}

func TestCLIAgentListAndStatus(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := execCLI(t, root, nil, "sync", "agents"); err != nil {
		t.Fatalf("sync: %v", err)
	}

	out, err := execCLI(t, root, nil, "agent", "list", "-o", "json")
	if err != nil {
		t.Fatalf("agent list: %v\n%s", err, out)
	}
	var agents []contract.AgentStatus
	if err := json.Unmarshal([]byte(out), &agents); err != nil {
		t.Fatalf("parse agent list: %v\n%s", err, out)
	}
	if len(agents) != 1 || agents[0].Name != "code-reviewer" {
		t.Fatalf("agent list = %+v", agents)
	}

	// `agent <name> status` form (plan-03 surface).
	out, err = execCLI(t, root, nil, "agent", "code-reviewer", "status", "-o", "json")
	if err != nil {
		t.Fatalf("agent status: %v\n%s", err, out)
	}
	var rep contract.StatusReport
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("parse status: %v\n%s", err, out)
	}
	if len(rep.Agents) != 1 {
		t.Fatalf("status agents = %+v", rep.Agents)
	}

	// `agents status` aggregate.
	out, err = execCLI(t, root, nil, "agents", "status", "-o", "json")
	if err != nil {
		t.Fatalf("agents status: %v\n%s", err, out)
	}
}

func TestCLIValidateCleanExitZero(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := execCLI(t, root, nil, "sync", "agents"); err != nil {
		t.Fatalf("sync: %v", err)
	}
	out, err := execCLI(t, root, nil, "validate", "--all")
	if err != nil {
		t.Fatalf("validate clean should exit 0: %v\n%s", err, out)
	}
}

func TestCLIValidateFailureNonZero(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	// Seed an invalid canonical agent.
	bad := filepath.Join(root, ".graft", "agents", "broken")
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "agent.yaml"), []byte("model: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := execCLI(t, root, nil, "validate", "--all", "-o", "json")
	if err == nil {
		t.Fatalf("validate over invalid agent should exit non-zero\n%s", out)
	}
	if !strings.Contains(err.Error(), "validation failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCLIValidateProviderAllMutuallyExclusive(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	_, err := execCLI(t, root, nil, "validate", "--all", "--provider", "claude-code")
	if err == nil {
		t.Fatalf("expected mutually-exclusive error")
	}
}

func TestCLIConfigGetSet(t *testing.T) {
	dir := t.TempDir()
	resolver := &config.DefaultResolver{ConfigPath: filepath.Join(dir, "config.json")}

	// get defaults (yaml)
	out, err := execNoGate(t, resolver, "config", "get")
	if err != nil {
		t.Fatalf("config get: %v", err)
	}
	if !strings.Contains(out, "scope: agents") || !strings.Contains(out, "theme: dark") {
		t.Fatalf("default config get missing defaults: %q", out)
	}

	// set several keys (json output for parse)
	out, err = execNoGate(t, resolver, "config", "set",
		"--scope", "skills", "--sync.gitAuto", "true",
		"--theme", "light", "--providers.enabled", "claude-code,codex", "-o", "json")
	if err != nil {
		t.Fatalf("config set: %v\n%s", err, out)
	}
	var cfg config.Config
	if err := json.Unmarshal([]byte(out), &cfg); err != nil {
		t.Fatalf("parse config set: %v\n%s", err, out)
	}
	if cfg.Scope != "skills" || !cfg.Sync.GitAuto || cfg.Theme != "light" || len(cfg.Providers.Enabled) != 2 {
		t.Fatalf("config set not applied: %+v", cfg)
	}

	// invalid scope -> error
	if _, err := execNoGate(t, resolver, "config", "set", "--scope", "bogus"); err == nil {
		t.Fatalf("invalid scope should error")
	}
}

func TestCLIRawOutputNoANSI(t *testing.T) {
	// JSON output must never contain ANSI escapes.
	dir := t.TempDir()
	resolver := &config.DefaultResolver{ConfigPath: filepath.Join(dir, "config.json")}
	out, err := execNoGate(t, resolver, "config", "get", "-o", "json")
	if err != nil {
		t.Fatalf("config get json: %v", err)
	}
	if strings.Contains(out, "\033[") {
		t.Fatalf("json output contains ANSI escapes:\n%q", out)
	}
}
