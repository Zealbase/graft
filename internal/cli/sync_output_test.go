package cli_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/cli/config"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// TestCLISyncOutputSummaryLine: the table-mode sync output renders the
// plan-revise task-2 line "{y} agents in sync with {x} providers".
func TestCLISyncOutputSummaryLine(t *testing.T) {
	root := newWorkspace(t)
	dir := t.TempDir()
	// Pin mode=specific + enabled so the effective set is deterministic (2).
	resolver := &config.DefaultResolver{ConfigPath: filepath.Join(dir, "config.json")}
	if _, err := execNoGate(t, resolver, "config", "set", "-g", "--providers.mode", "specific", "--providers.enabled", "claude-code,opencode"); err != nil {
		t.Fatalf("config set: %v", err)
	}

	// init with an explicit resolver that already has config -> no first-run.
	if _, err := execCLI(t, root, resolver, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := execCLI(t, root, resolver, "sync", "agents")
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	// One agent (code-reviewer) synced, two providers enabled.
	if !strings.Contains(out, "1 agent in sync with 2 providers") {
		t.Fatalf("sync summary line missing/incorrect:\n%s", out)
	}
}

// TestCLISyncOutputDefaultProviderCount: mode=all (no disabled) -> x is the full
// supported provider count (10).
func TestCLISyncOutputDefaultProviderCount(t *testing.T) {
	root := newWorkspace(t)
	dir := t.TempDir()
	resolver := &config.DefaultResolver{ConfigPath: filepath.Join(dir, "config.json")}
	// Explicit mode=all so the effective set is the full 10 (and config exists ->
	// no first-run reseeding the set from machine detection).
	if _, err := execNoGate(t, resolver, "config", "set", "-g", "--providers.mode", "all"); err != nil {
		t.Fatalf("config set: %v", err)
	}
	if _, err := execCLI(t, root, resolver, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := execCLI(t, root, resolver, "sync", "agents")
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if !strings.Contains(out, "in sync with 10 providers") {
		t.Fatalf("expected default 10-provider summary:\n%s", out)
	}
}

// TestCLISyncJSONStaysRawRunResult: json output is the raw RunResult (no
// presentation wrapper, no summary line, no ANSI).
func TestCLISyncJSONStaysRawRunResult(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := execCLI(t, root, nil, "sync", "agents", "-o", "json")
	if err != nil {
		t.Fatalf("sync json: %v\n%s", err, out)
	}
	if strings.Contains(out, "in sync with") {
		t.Fatalf("json output must not contain the human summary line:\n%s", out)
	}
	var res contract.RunResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("json is not a raw RunResult: %v\n%s", err, out)
	}
	if res.RunID == "" {
		t.Fatalf("RunResult missing run_id: %+v", res)
	}
}
