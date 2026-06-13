package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/cli"
	"github.com/Shaik-Sirajuddin/graft/internal/gateway"
)

// TestCLIDestroyYesRemovesGraftKeepsProviders: --yes skips the prompt, removes
// .graft, and leaves provider files (.claude/agents/*.md) untouched.
func TestCLIDestroyYesRemovesGraftKeepsProviders(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".graft")); err != nil {
		t.Fatalf("expected .graft to exist after init: %v", err)
	}
	if _, err := execCLI(t, root, nil, "destroy", "--yes"); err != nil {
		t.Fatalf("destroy --yes: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".graft")); !os.IsNotExist(err) {
		t.Fatalf(".graft should be removed, got err=%v", err)
	}
	// Provider file must survive.
	if _, err := os.Stat(filepath.Join(root, ".claude", "agents", "code-reviewer.md")); err != nil {
		t.Fatalf("provider file must NOT be deleted by destroy: %v", err)
	}
}

// TestCLIDestroyCIIsYesAlias: --ci behaves like --yes (no prompt).
func TestCLIDestroyCIIsYesAlias(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := execCLI(t, root, nil, "destroy", "--ci"); err != nil {
		t.Fatalf("destroy --ci: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".graft")); !os.IsNotExist(err) {
		t.Fatalf(".graft should be removed by --ci")
	}
}

// TestCLIDestroyKeepStoreRetainsAgents: --keep-store keeps .graft/agents.
func TestCLIDestroyKeepStoreRetainsAgents(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	// Scaffold a canonical agent so the store has content to retain.
	if _, err := execCLI(t, root, nil, "agent", "init", "keeper"); err != nil {
		t.Fatalf("agent init: %v", err)
	}
	if _, err := execCLI(t, root, nil, "destroy", "--yes", "--keep-store"); err != nil {
		t.Fatalf("destroy --keep-store: %v", err)
	}
	// .graft/agents survives.
	if _, err := os.Stat(filepath.Join(root, ".graft", "agents", "keeper", "agent.yaml")); err != nil {
		t.Fatalf("--keep-store must retain .graft/agents: %v", err)
	}
}

// TestCLIDestroyPromptDeclined: an interactive "n" answer aborts without
// removing .graft.
func TestCLIDestroyPromptDeclined(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	out := destroyWithStdin(t, root, "n\n", "destroy")
	if !strings.Contains(out, "aborted") {
		t.Fatalf("declined destroy should print 'aborted':\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(root, ".graft")); err != nil {
		t.Fatalf(".graft must survive a declined destroy: %v", err)
	}
}

// TestCLIDestroyPromptAccepted: a "y" answer proceeds and removes .graft.
func TestCLIDestroyPromptAccepted(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	destroyWithStdin(t, root, "y\n", "destroy")
	if _, err := os.Stat(filepath.Join(root, ".graft")); !os.IsNotExist(err) {
		t.Fatalf(".graft should be removed after a 'y' answer")
	}
}

// destroyWithStdin runs `graft <args...>` with a fed stdin string and returns
// stdout. It wires the gate at root with an injected stdin reader.
func destroyWithStdin(t *testing.T, root, stdin string, args ...string) string {
	t.Helper()
	g, err := gateway.Open(root)
	if err != nil {
		t.Fatalf("gateway.Open: %v", err)
	}
	defer g.Close()
	c := cli.EntrypointWithVersion(g, nil, "test")
	var out, errBuf bytes.Buffer
	r := c.Root()
	r.SetOut(&out)
	r.SetErr(&errBuf)
	r.SetIn(strings.NewReader(stdin))
	r.SetArgs(args)
	if err := r.Execute(); err != nil {
		t.Fatalf("%v: %v\n%s", args, err, out.String())
	}
	return out.String()
}
