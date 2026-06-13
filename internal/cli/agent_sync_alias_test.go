package cli_test

import (
	"path/filepath"
	"regexp"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/cli/config"
)

// stripRunID removes the per-run "run_id<tab>...<newline>" line so two
// otherwise-identical sync renderings (which differ only by run id) can be
// compared.
var runIDLine = regexp.MustCompile(`(?m)^run_id\s+\S+\n`)

func stripRunID(s string) string { return runIDLine.ReplaceAllString(s, "") }

// TestCLIAgentSyncAliasMatchesSyncAgents: `graft agent sync` shares the runSync
// path with `graft sync agents`, so on the same workspace both render the same
// human summary line (v0.0.4 verify). The two surfaces are kept side by side.
func TestCLIAgentSyncAliasMatchesSyncAgents(t *testing.T) {
	dir := t.TempDir()
	resolver := &config.DefaultResolver{ConfigPath: filepath.Join(dir, "config.json")}
	if _, err := execNoGate(t, resolver, "config", "set", "-g",
		"--providers.mode", "specific", "--providers.enabled", "claude-code,opencode"); err != nil {
		t.Fatalf("config set: %v", err)
	}

	// `graft agent sync` on a fresh workspace.
	rootA := newWorkspace(t)
	if _, err := execCLI(t, rootA, resolver, "init"); err != nil {
		t.Fatalf("init A: %v", err)
	}
	outAgentSync, err := execCLI(t, rootA, resolver, "agent", "sync")
	if err != nil {
		t.Fatalf("agent sync: %v\n%s", err, outAgentSync)
	}

	// `graft sync agents` on an identical fresh workspace.
	rootB := newWorkspace(t)
	if _, err := execCLI(t, rootB, resolver, "init"); err != nil {
		t.Fatalf("init B: %v", err)
	}
	outSyncAgents, err := execCLI(t, rootB, resolver, "sync", "agents")
	if err != nil {
		t.Fatalf("sync agents: %v\n%s", err, outSyncAgents)
	}

	if stripRunID(outAgentSync) != stripRunID(outSyncAgents) {
		t.Fatalf("`agent sync` and `sync agents` diverged:\nagent sync:\n%s\nsync agents:\n%s",
			outAgentSync, outSyncAgents)
	}
}

// TestCLIAgentSyncNameMatchesSyncAgentName: `graft agent sync <name>` shares the
// path with `graft sync agent <name>`.
func TestCLIAgentSyncNameMatchesSyncAgentName(t *testing.T) {
	dir := t.TempDir()
	resolver := &config.DefaultResolver{ConfigPath: filepath.Join(dir, "config.json")}
	if _, err := execNoGate(t, resolver, "config", "set", "-g",
		"--providers.mode", "specific", "--providers.enabled", "claude-code,opencode"); err != nil {
		t.Fatalf("config set: %v", err)
	}

	rootA := newWorkspace(t)
	if _, err := execCLI(t, rootA, resolver, "init"); err != nil {
		t.Fatalf("init A: %v", err)
	}
	outAgentSync, err := execCLI(t, rootA, resolver, "agent", "sync", "code-reviewer")
	if err != nil {
		t.Fatalf("agent sync <name>: %v\n%s", err, outAgentSync)
	}

	rootB := newWorkspace(t)
	if _, err := execCLI(t, rootB, resolver, "init"); err != nil {
		t.Fatalf("init B: %v", err)
	}
	outSyncAgent, err := execCLI(t, rootB, resolver, "sync", "agent", "code-reviewer")
	if err != nil {
		t.Fatalf("sync agent <name>: %v\n%s", err, outSyncAgent)
	}

	if stripRunID(outAgentSync) != stripRunID(outSyncAgent) {
		t.Fatalf("`agent sync <name>` and `sync agent <name>` diverged:\nagent sync:\n%s\nsync agent:\n%s",
			outAgentSync, outSyncAgent)
	}
}
