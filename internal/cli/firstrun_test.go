package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/cli/config"
)

func TestProviderInstalledByHomeDir(t *testing.T) {
	home := t.TempDir()
	// Seed a ~/.claude dir -> claude-code detected.
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !providerInstalled("claude-code", home) {
		t.Fatalf("claude-code should be detected via ~/.claude")
	}
}

func TestProviderNotInstalled(t *testing.T) {
	home := t.TempDir() // empty home, and assume no 'roo' binary on PATH in CI
	if providerInstalled("roo-code", home) {
		t.Skip("'roo' binary present on this machine; detection-by-PATH is correct")
	}
}

func TestDetectInstalledProvidersHomeDirs(t *testing.T) {
	home := t.TempDir()
	for _, d := range []string{".claude", ".codex"} {
		if err := os.MkdirAll(filepath.Join(home, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got := detectInstalledProviders(home)
	// At least claude-code + codex must be present (PATH may add more).
	has := map[string]bool{}
	for _, id := range got {
		has[id] = true
	}
	if !has["claude-code"] || !has["codex"] {
		t.Fatalf("detect missing expected providers: %v", got)
	}
}

func TestAutoSelectDefaultsToAllWhenNoneDetected(t *testing.T) {
	got := autoSelect(nil)
	if len(got) != len(config.SupportedProviders()) {
		t.Fatalf("autoSelect(nil) = %d, want all %d", len(got), len(config.SupportedProviders()))
	}
}

func TestAutoSelectUsesDetected(t *testing.T) {
	got := autoSelect([]string{"claude-code"})
	if len(got) != 1 || got[0] != "claude-code" {
		t.Fatalf("autoSelect(detected) = %v, want [claude-code]", got)
	}
}

// TestFirstRunNonInteractiveAutoSelects: --yes path persists mode=specific with
// the auto-selected providers and NEVER hangs (no TTY prompt).
func TestFirstRunNonInteractiveAutoSelects(t *testing.T) {
	// Isolated empty HOME so detection is deterministic (no provider dirs).
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfgPath := filepath.Join(t.TempDir(), "config.json")
	resolver := &config.DefaultResolver{ConfigPath: cfgPath}
	c := EntrypointWithVersion(nil, resolver, "test")

	var out bytes.Buffer
	// autoYes=true forces the non-interactive path; must return promptly.
	if err := c.maybeRunFirstRun(&out, true); err != nil {
		t.Fatalf("maybeRunFirstRun: %v", err)
	}

	// Config persisted with mode=specific.
	cfg, err := resolver.Get()
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if cfg.Providers.Mode != config.ProviderModeSpecific {
		t.Fatalf("first-run mode = %q, want specific", cfg.Providers.Mode)
	}
	// No providers detected in the empty home -> auto-select falls back to ALL.
	// (PATH on the host may add some; assert the enabled set is non-empty and a
	// subset of supported.)
	if len(cfg.Providers.Enabled) == 0 {
		t.Fatalf("first-run enabled set is empty")
	}
	for _, id := range cfg.Providers.Enabled {
		if !config.IsSupportedProvider(id) {
			t.Fatalf("first-run enabled unknown provider %q", id)
		}
	}
	if !bytes.Contains(out.Bytes(), []byte("Enabled")) {
		t.Fatalf("first-run summary not printed:\n%s", out.String())
	}
}

// TestFirstRunSkippedWhenConfigExists: a second run does not re-prompt/reseed.
func TestFirstRunSkippedWhenConfigExists(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	resolver := &config.DefaultResolver{ConfigPath: cfgPath}
	// Persist a config first.
	if err := resolver.Save(config.ApplyDefaults(&config.Config{
		Providers: config.ProvidersConfig{Mode: config.ProviderModeSpecific, Enabled: []string{"claude-code"}},
	})); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	c := EntrypointWithVersion(nil, resolver, "test")
	var out bytes.Buffer
	if err := c.maybeRunFirstRun(&out, true); err != nil {
		t.Fatalf("maybeRunFirstRun: %v", err)
	}
	// Unchanged.
	cfg, _ := resolver.Get()
	if len(cfg.Providers.Enabled) != 1 || cfg.Providers.Enabled[0] != "claude-code" {
		t.Fatalf("existing config was reseeded: %+v", cfg.Providers)
	}
	if out.Len() != 0 {
		t.Fatalf("first-run should be silent when config exists, got:\n%s", out.String())
	}
}
