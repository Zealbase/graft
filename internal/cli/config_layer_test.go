package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/cli/config"
)

// TestEffectiveProvidersProjectOverGlobal: a project provider selection wins
// over global; with no project override the global effective set applies.
func TestEffectiveProvidersProjectOverGlobal(t *testing.T) {
	global := config.ApplyDefaults(&config.Config{
		Providers: config.ProvidersConfig{Mode: "specific", Enabled: []string{"claude-code", "codex"}},
	})
	// No project override -> inherit global (2 providers).
	if got := config.EffectiveProviders(global, &config.ProjectConfig{}); len(got) != 2 {
		t.Fatalf("inherit-global = %v, want 2", got)
	}
	// Project override -> project wins (1 provider).
	project := &config.ProjectConfig{Providers: &config.ProvidersConfig{Mode: "specific", Enabled: []string{"opencode"}}}
	got := config.EffectiveProviders(global, project)
	if len(got) != 1 || got[0] != "opencode" {
		t.Fatalf("project-override = %v, want [opencode]", got)
	}
}

// TestCLIConfigSetProjectDefault: `config set` (no -g) writes
// .graft/config.json and is reflected in `config get` (resolved) while the
// global config is untouched.
func TestCLIConfigSetProjectDefault(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	// Project-scoped provider selection.
	if _, err := execCLI(t, root, nil, "config", "set",
		"--providers.mode", "specific", "--providers.enabled", "opencode"); err != nil {
		t.Fatalf("config set (project): %v", err)
	}
	// The project config file exists with the override.
	data, err := os.ReadFile(filepath.Join(root, ".graft", "config.json"))
	if err != nil {
		t.Fatalf("project config not written: %v", err)
	}
	if !strings.Contains(string(data), "opencode") {
		t.Fatalf("project config missing override:\n%s", data)
	}
	// Resolved `config get` reflects the project override.
	out, err := execCLI(t, root, nil, "config", "get", "-o", "json")
	if err != nil {
		t.Fatalf("config get: %v", err)
	}
	var cfg config.Config
	if err := json.Unmarshal([]byte(out), &cfg); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	eff := cfg.EffectiveProviders()
	if len(eff) != 1 || eff[0] != "opencode" {
		t.Fatalf("resolved effective = %v, want [opencode]", eff)
	}
}

// TestCLIConfigSetGlobalKeyRoutesToGlobal: a global-only key passed without -g
// is transparently routed to the global config (no project meaning) and does NOT
// create a project override for it.
func TestCLIConfigSetGlobalKeyRoutesToGlobal(t *testing.T) {
	dir := t.TempDir()
	resolver := &config.DefaultResolver{ConfigPath: filepath.Join(dir, "config.json")}
	root := newWorkspace(t)
	if _, err := execCLI(t, root, resolver, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	// --theme without -g: must succeed and land in the global config.
	if _, err := execCLI(t, root, resolver, "config", "set", "--theme", "light"); err != nil {
		t.Fatalf("--theme at project scope should route to global, got: %v", err)
	}
	g, _ := resolver.Get()
	if g.Theme != "light" {
		t.Fatalf("theme not written to global: %q", g.Theme)
	}
}

// TestCLIConfigGetGlobalFlag: `config get -g` shows the global config, ignoring
// any project override.
func TestCLIConfigGetGlobalFlag(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	// Project narrows to one provider.
	if _, err := execCLI(t, root, nil, "config", "set",
		"--providers.mode", "specific", "--providers.enabled", "opencode"); err != nil {
		t.Fatalf("config set: %v", err)
	}
	// Global get ignores the project override (default global = mode all = 10).
	out, err := execCLI(t, root, nil, "config", "get", "-g", "-o", "json")
	if err != nil {
		t.Fatalf("config get -g: %v", err)
	}
	var cfg config.Config
	if err := json.Unmarshal([]byte(out), &cfg); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if len(cfg.EffectiveProviders()) != len(config.SupportedProviders()) {
		t.Fatalf("global get should not see project override: %v", cfg.EffectiveProviders())
	}
}

// TestCLISyncUsesProjectProviders: a project provider override is the set the
// sync reports against.
func TestCLISyncUsesProjectProviders(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := execCLI(t, root, nil, "config", "set",
		"--providers.mode", "specific", "--providers.enabled", "claude-code,opencode"); err != nil {
		t.Fatalf("config set: %v", err)
	}
	out, err := execCLI(t, root, nil, "sync", "agents")
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if !strings.Contains(out, "with 2 providers") {
		t.Fatalf("sync should use the project's 2-provider set:\n%s", out)
	}
}
