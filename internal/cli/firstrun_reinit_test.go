package cli

import (
	"bytes"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/cli/config"
)

// TestProjectSeedPrefillsFromExisting: when a project already has a providers
// override, the project checklist seed is PREFILLED from it (preserved=true) and
// the global set is ignored — the core of v0.0.6 issue #1.
func TestProjectSeedPrefillsFromExisting(t *testing.T) {
	existing := &config.ProjectConfig{
		Providers: &config.ProvidersConfig{
			Mode:    config.ProviderModeSpecific,
			Enabled: []string{"codex", "claude-code"},
		},
	}
	globalNow := []string{"claude-code", "cursor", "goose"}

	seed, preserved := projectSeed(existing, globalNow)
	if !preserved {
		t.Fatalf("preserved = false, want true (existing project override must be prefilled)")
	}
	want := []string{"claude-code", "codex"} // EffectiveProviders is sorted + filtered
	got := append([]string(nil), seed...)
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("seed = %v, want prefilled-from-project %v", got, want)
	}
}

// TestProjectSeedFallsBackToGlobal: a project with NO override (true first
// project init) seeds from the global effective set, preserved=false.
func TestProjectSeedFallsBackToGlobal(t *testing.T) {
	globalNow := []string{"claude-code", "cursor"}
	for _, existing := range []*config.ProjectConfig{nil, {}} {
		seed, preserved := projectSeed(existing, globalNow)
		if preserved {
			t.Fatalf("preserved = true for project with no override (%v)", existing)
		}
		if !reflect.DeepEqual(seed, globalNow) {
			t.Fatalf("seed = %v, want global %v", seed, globalNow)
		}
	}
}

// TestReinitPreservesExistingProjectAndGlobal: a non-interactive re-init of an
// ALREADY-initialized workspace must preserve the prior PROJECT provider
// selection (here: a deliberately narrowed/disabled set) and the custom GLOBAL
// settings (theme, scope) — it must NOT reset them to defaults. (issue #1)
func TestReinitPreservesExistingProjectAndGlobal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfgPath := filepath.Join(t.TempDir(), "config.json")
	resolver := &config.DefaultResolver{ConfigPath: cfgPath}

	// Pre-existing GLOBAL config: a specific (narrowed) provider set + custom
	// theme/scope settings the user deliberately chose.
	if err := resolver.Save(config.ApplyDefaults(&config.Config{
		Providers: config.ProvidersConfig{
			Mode:     config.ProviderModeAll,
			Disabled: []string{"goose", "cursor"},
		},
		Theme: "light",
		Scope: "skills",
	})); err != nil {
		t.Fatalf("seed global config: %v", err)
	}

	// Pre-existing PROJECT override: only claude-code enabled for this project.
	projRoot := t.TempDir()
	projResolver := &config.DefaultProjectResolver{WorkspaceRoot: projRoot}
	if err := projResolver.Save(&config.ProjectConfig{
		Providers: &config.ProvidersConfig{
			Mode:    config.ProviderModeSpecific,
			Enabled: []string{"claude-code"},
		},
	}); err != nil {
		t.Fatalf("seed project config: %v", err)
	}

	c := EntrypointWithVersion(nil, resolver, "test")
	c.SetProjectResolver(projResolver)

	// Re-init non-interactively (--yes path). It must be a no-op preserve: the
	// config already exists, so first-run seeding is skipped and nothing is reset.
	var out bytes.Buffer
	if err := c.maybeRunFirstRun(&out, true); err != nil {
		t.Fatalf("maybeRunFirstRun: %v", err)
	}

	// GLOBAL settings preserved (NOT reset to defaults).
	global, err := resolver.Get()
	if err != nil {
		t.Fatalf("read global: %v", err)
	}
	if global.Theme != "light" {
		t.Fatalf("theme reset on re-init: got %q, want light", global.Theme)
	}
	if global.Scope != "skills" {
		t.Fatalf("scope reset on re-init: got %q, want skills", global.Scope)
	}
	gotDisabled := append([]string(nil), global.Providers.Disabled...)
	sort.Strings(gotDisabled)
	if !reflect.DeepEqual(gotDisabled, []string{"cursor", "goose"}) {
		t.Fatalf("global disabled set reset on re-init: got %v", gotDisabled)
	}

	// PROJECT override preserved (still only claude-code).
	project, err := projResolver.Get()
	if err != nil {
		t.Fatalf("read project: %v", err)
	}
	if project.Providers == nil {
		t.Fatalf("project providers override cleared on re-init")
	}
	eff := project.Providers.EffectiveProviders()
	if !reflect.DeepEqual(eff, []string{"claude-code"}) {
		t.Fatalf("project providers reset on re-init: got %v, want [claude-code]", eff)
	}
}
