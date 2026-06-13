package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// TestCLIAgentInit scaffolds a new canonical agent and prints a next-step hint.
func TestCLIAgentInit(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := execCLI(t, root, nil, "agent", "init", "fixer", "Fix bugs carefully.")
	if err != nil {
		t.Fatalf("agent init: %v\n%s", err, out)
	}
	if !strings.Contains(out, "graft sync agent fixer") {
		t.Fatalf("missing next-step hint:\n%s", out)
	}
	// The canonical store now has the agent.
	if _, err := os.Stat(filepath.Join(root, ".graft", "agents", "fixer", "agent.yaml")); err != nil {
		t.Fatalf("agent.yaml not scaffolded: %v", err)
	}
	// Re-init same name -> error.
	if _, err := execCLI(t, root, nil, "agent", "init", "fixer"); err == nil {
		t.Fatalf("duplicate agent init should error")
	}
}

// TestCLIAgentModelSetAndClear sets then clears a per-provider model override.
func TestCLIAgentModelSetAndClear(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := execCLI(t, root, nil, "agent", "init", "fixer"); err != nil {
		t.Fatalf("agent init: %v", err)
	}
	// Set a model override.
	if _, err := execCLI(t, root, nil, "agent", "model", "fixer",
		"--provider", "claude-code", "--model", "sonnet"); err != nil {
		t.Fatalf("agent model set: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".graft", "agents", "fixer", "agent.yaml"))
	if err != nil {
		t.Fatalf("read agent.yaml: %v", err)
	}
	if !strings.Contains(string(data), "sonnet") {
		t.Fatalf("model override not persisted:\n%s", data)
	}
	// Clear it.
	if _, err := execCLI(t, root, nil, "agent", "model", "fixer",
		"--provider", "claude-code", "--clear"); err != nil {
		t.Fatalf("agent model clear: %v", err)
	}
	data, _ = os.ReadFile(filepath.Join(root, ".graft", "agents", "fixer", "agent.yaml"))
	if strings.Contains(string(data), "sonnet") {
		t.Fatalf("model override survived --clear:\n%s", data)
	}
}

// TestCLIAgentModelValidationErrors covers the flag-combination guards.
func TestCLIAgentModelValidationErrors(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := execCLI(t, root, nil, "agent", "init", "fixer"); err != nil {
		t.Fatalf("agent init: %v", err)
	}
	// Missing --provider.
	if _, err := execCLI(t, root, nil, "agent", "model", "fixer", "--model", "x"); err == nil {
		t.Fatalf("missing --provider should error")
	}
	// Neither --model nor --clear.
	if _, err := execCLI(t, root, nil, "agent", "model", "fixer", "--provider", "claude-code"); err == nil {
		t.Fatalf("missing --model/--clear should error")
	}
	// Both --model and --clear.
	if _, err := execCLI(t, root, nil, "agent", "model", "fixer",
		"--provider", "claude-code", "--model", "x", "--clear"); err == nil {
		t.Fatalf("--model + --clear should error")
	}
	// Unknown provider.
	if _, err := execCLI(t, root, nil, "agent", "model", "fixer",
		"--provider", "bogus", "--model", "x"); err == nil {
		t.Fatalf("unknown provider should error")
	}
}

// TestCLIAgentInitJSON: machine output is the raw CanonicalAgent (no hint line).
func TestCLIAgentInitJSON(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := execCLI(t, root, nil, "agent", "init", "fixer", "-o", "json")
	if err != nil {
		t.Fatalf("agent init json: %v\n%s", err, out)
	}
	if strings.Contains(out, "graft sync agent") {
		t.Fatalf("json output must not contain the hint line:\n%s", out)
	}
	var a contract.CanonicalAgent
	if err := json.Unmarshal([]byte(out), &a); err != nil {
		t.Fatalf("not a raw CanonicalAgent: %v\n%s", err, out)
	}
	if a.Name != "fixer" {
		t.Fatalf("unexpected agent: %+v", a)
	}
}
