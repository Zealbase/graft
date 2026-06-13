package e2e

import (
	"testing"
)

// Scenario 7: config get/set roundtrip (scope, sync.gitAuto, providers.enabled);
// invalid scope non-zero. The harness isolates XDG_CONFIG_HOME per workspace so
// these never touch the real user config.
func TestConfig_GetDefaults(t *testing.T) {
	root := newGitWorkspace(t)
	var cfg configJSON
	decodeJSON(t, mustGraft(t, root, "config", "get", "-o", "json"), &cfg)
	if cfg.Scope != "agents" {
		t.Fatalf("default scope=%q, want agents", cfg.Scope)
	}
	if cfg.Theme != "dark" {
		t.Fatalf("default theme=%q, want dark", cfg.Theme)
	}
	if cfg.Sync.GitAuto {
		t.Fatalf("default sync.gitAuto=true, want false")
	}
}

func TestConfig_SetGetRoundtrip(t *testing.T) {
	root := newGitWorkspace(t)
	// Project-overridable keys (scope/providers) require an initialized
	// workspace (.graft/) so `config set` never creates .graft/ outside one.
	mustGraft(t, root, "init")

	var set configJSON
	decodeJSON(t, mustGraft(t, root,
		"config", "set",
		"--sync.gitAuto", "true",
		"--scope", "skills",
		"--providers.enabled", "claude-code,codex",
		"-o", "json",
	), &set)
	if !set.Sync.GitAuto {
		t.Fatalf("set sync.gitAuto=false, want true")
	}
	if set.Scope != "skills" {
		t.Fatalf("set scope=%q, want skills", set.Scope)
	}
	if !equalStrings(set.Providers.Enabled, []string{"claude-code", "codex"}) {
		t.Fatalf("set providers.enabled=%v, want [claude-code codex]", set.Providers.Enabled)
	}

	// get must reflect the persisted values.
	var got configJSON
	decodeJSON(t, mustGraft(t, root, "config", "get", "-o", "json"), &got)
	if !got.Sync.GitAuto || got.Scope != "skills" ||
		!equalStrings(got.Providers.Enabled, []string{"claude-code", "codex"}) {
		t.Fatalf("config get after set mismatch: %+v", got)
	}
}

func TestConfig_InvalidScope_NonZero(t *testing.T) {
	root := newGitWorkspace(t)
	mustGraft(t, root, "init")
	r := graft(t, root, "config", "set", "--scope", "nonsense", "-o", "json")
	if r.exitCode == 0 {
		t.Fatalf("config set --scope nonsense exit=0, want non-zero")
	}
	if !contains(r.stderr, "scope") {
		t.Fatalf("expected scope error on stderr, got: %s", r.stderr)
	}
}

func TestConfig_EmptyLeavesUnchanged(t *testing.T) {
	root := newGitWorkspace(t)
	mustGraft(t, root, "init")
	// Set scope first.
	mustGraft(t, root, "config", "set", "--scope", "skills")
	// A set with no flags must not reset scope back to default.
	mustGraft(t, root, "config", "set")
	var got configJSON
	decodeJSON(t, mustGraft(t, root, "config", "get", "-o", "json"), &got)
	if got.Scope != "skills" {
		t.Fatalf("scope=%q after empty set, want skills (unchanged)", got.Scope)
	}
}
