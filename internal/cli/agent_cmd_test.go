package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// TestCLIAgentNoSubcommandShowsHelp: bare `graft agent` (and `graft agents`)
// should print the command help and exit 0, not an "[ERROR] usage:" error.
func TestCLIAgentNoSubcommandShowsHelp(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	for _, name := range []string{"agent", "agents"} {
		out, err := execCLI(t, root, nil, name)
		if err != nil {
			t.Fatalf("%s (no subcommand) should exit 0: %v\n%s", name, err, out)
		}
		if strings.Contains(out, "[ERROR]") {
			t.Fatalf("%s help must not be an error:\n%s", name, out)
		}
		if !strings.Contains(out, "Available Commands") {
			t.Fatalf("%s help missing available-commands listing:\n%s", name, out)
		}
	}
}

// TestCLIAgentInit scaffolds a new canonical agent, defaults its description, and
// auto-syncs it by default.
func TestCLIAgentInit(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := execCLI(t, root, nil, "agent", "init", "fixer", "Fix bugs carefully.")
	if err != nil {
		t.Fatalf("agent init: %v\n%s", err, out)
	}
	// Auto-sync ran by default: the message reflects the sync, NOT the manual hint.
	if !strings.Contains(out, "Syncing it to your providers") {
		t.Fatalf("expected auto-sync message:\n%s", out)
	}
	if strings.Contains(out, "Run `graft sync agent fixer` to fan it out") {
		t.Fatalf("manual next-step hint should not appear when auto-sync runs:\n%s", out)
	}
	// The canonical store now has the agent.
	if _, err := os.Stat(filepath.Join(root, ".graft", "agents", "fixer", "agent.yaml")); err != nil {
		t.Fatalf("agent.yaml not scaffolded: %v", err)
	}
	// The default description is non-empty ("<name> agent").
	data, err := os.ReadFile(filepath.Join(root, ".graft", "agents", "fixer", "agent.yaml"))
	if err != nil {
		t.Fatalf("read agent.yaml: %v", err)
	}
	if !strings.Contains(string(data), "fixer agent") {
		t.Fatalf("expected default description %q in agent.yaml:\n%s", "fixer agent", data)
	}
	// Re-init same name -> error.
	if _, err := execCLI(t, root, nil, "agent", "init", "fixer"); err == nil {
		t.Fatalf("duplicate agent init should error")
	}
}

// TestCLIAgentInitDefaultDescriptionUnblocksSync proves the previously-failing
// flow now works: init (no prompt) gives a non-empty default description so the
// agent passes the description-required validation gate and `graft sync` succeeds.
func TestCLIAgentInitDefaultDescriptionUnblocksSync(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	// init with --no-sync to isolate the description default from the auto-sync.
	if _, err := execCLI(t, root, nil, "agent", "init", "test", "--no-sync"); err != nil {
		t.Fatalf("agent init: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".graft", "agents", "test", "agent.yaml"))
	if err != nil {
		t.Fatalf("read agent.yaml: %v", err)
	}
	if !strings.Contains(string(data), "test agent") {
		t.Fatalf("expected default description \"test agent\":\n%s", data)
	}
	// The previously-failing flow: a manual sync must NOT be blocked by the
	// description-required validation.
	out, err := execCLI(t, root, nil, "sync", "agent", "test")
	if err != nil {
		t.Fatalf("sync after init should not be blocked by description validation: %v\n%s", err, out)
	}
}

// TestCLIAgentInitAutoSyncSideEffect: the default auto-sync fans the agent out to
// providers (a provider file appears), and --no-sync leaves none.
func TestCLIAgentInitAutoSyncSideEffect(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	// Default: auto-sync writes the claude-code provider file.
	if _, err := execCLI(t, root, nil, "agent", "init", "fixer"); err != nil {
		t.Fatalf("agent init: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude", "agents", "fixer.md")); err != nil {
		t.Fatalf("auto-sync should have written the claude-code provider file: %v", err)
	}
	// --no-sync: no provider file for a separate agent.
	if _, err := execCLI(t, root, nil, "agent", "init", "skipper", "--no-sync"); err != nil {
		t.Fatalf("agent init --no-sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude", "agents", "skipper.md")); err == nil {
		t.Fatalf("--no-sync should NOT have written a provider file")
	}
}

// TestCLIAgentInitOmniAutoSync: omni init also auto-syncs by default and keeps its
// recorded omni ref across the sync.
func TestCLIAgentInitOmniAutoSync(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, _, err := execCLIStreams(t, root, nil, "agent", "init", "fixer", "Body.", "--omni-agent")
	if err != nil {
		t.Fatalf("agent init --omni-agent: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Syncing it to your providers") {
		t.Fatalf("omni init should auto-sync:\n%s", out)
	}
	// Auto-sync wrote the provider file.
	if _, err := os.Stat(filepath.Join(root, ".claude", "agents", "fixer.md")); err != nil {
		t.Fatalf("omni init auto-sync should have written the provider file: %v", err)
	}
	// The omni ref survives the sync (sync must not clobber meta.Omni).
	meta := readMeta(t, root, "fixer")
	if meta.Omni == nil || meta.Omni.Ref != "fixer" {
		t.Fatalf("omni ref lost across auto-sync: %+v", meta.Omni)
	}
}

// TestCLIAgentInitNoSync: --no-sync skips the auto-sync and prints the manual hint.
func TestCLIAgentInitNoSync(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := execCLI(t, root, nil, "agent", "init", "fixer", "--no-sync")
	if err != nil {
		t.Fatalf("agent init --no-sync: %v\n%s", err, out)
	}
	// Manual hint printed, no auto-sync message.
	if !strings.Contains(out, "Run `graft sync agent fixer` to fan it out") {
		t.Fatalf("expected manual next-step hint with --no-sync:\n%s", out)
	}
	if strings.Contains(out, "Syncing it to your providers") {
		t.Fatalf("--no-sync must not auto-sync:\n%s", out)
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
	// --no-sync keeps the machine output a single CanonicalAgent document (the
	// auto-sync would otherwise append a second JSON doc for the sync result).
	out, err := execCLI(t, root, nil, "agent", "init", "fixer", "-o", "json", "--no-sync")
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
