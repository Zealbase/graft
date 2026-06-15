package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestEffectiveProvidersModeAll(t *testing.T) {
	c := ApplyDefaults(&Config{})
	// default mode=all, no disabled -> every supported provider.
	got := c.EffectiveProviders()
	if !reflect.DeepEqual(got, SupportedProviders()) {
		t.Fatalf("mode=all default = %v, want all supported %v", got, SupportedProviders())
	}
}

func TestEffectiveProvidersModeAllWithDisabled(t *testing.T) {
	// Use two active providers as the disabled set (antigravity is not in
	// SupportedProviders — unregistered pending research spike).
	c := ApplyDefaults(&Config{Providers: ProvidersConfig{
		Mode:     ProviderModeAll,
		Disabled: []string{"grok-cli", "goose"},
	}})
	got := c.EffectiveProviders()
	for _, id := range got {
		if id == "grok-cli" || id == "goose" {
			t.Fatalf("disabled provider %q leaked into effective set: %v", id, got)
		}
	}
	if len(got) != len(SupportedProviders())-2 {
		t.Fatalf("mode=all disabled 2 -> %d, want %d", len(got), len(SupportedProviders())-2)
	}
}

func TestEffectiveProvidersModeSpecific(t *testing.T) {
	c := ApplyDefaults(&Config{Providers: ProvidersConfig{
		Mode:    ProviderModeSpecific,
		Enabled: []string{"opencode", "claude-code"},
	}})
	got := c.EffectiveProviders()
	want := []string{"claude-code", "opencode"} // sorted
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mode=specific = %v, want %v", got, want)
	}
}

func TestEffectiveProvidersSpecificEmpty(t *testing.T) {
	c := ApplyDefaults(&Config{Providers: ProvidersConfig{Mode: ProviderModeSpecific}})
	if got := c.EffectiveProviders(); len(got) != 0 {
		t.Fatalf("mode=specific empty enabled -> %v, want empty", got)
	}
}

func TestEffectiveProvidersIgnoresUnknownIDs(t *testing.T) {
	c := ApplyDefaults(&Config{Providers: ProvidersConfig{
		Mode:    ProviderModeSpecific,
		Enabled: []string{"claude-code", "not-a-real-provider"},
	}})
	got := c.EffectiveProviders()
	want := []string{"claude-code"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unknown id should be dropped: %v, want %v", got, want)
	}
}

func TestApplyDefaultsProviders(t *testing.T) {
	c := ApplyDefaults(&Config{})
	if c.Providers.Mode != ProviderModeAll {
		t.Fatalf("default mode = %q, want all", c.Providers.Mode)
	}
	if c.Providers.Enabled == nil || c.Providers.Disabled == nil {
		t.Fatalf("enabled/disabled should default to non-nil slices: %+v", c.Providers)
	}
}

func TestSupportedProvidersCount(t *testing.T) {
	// antigravity (agy) is unregistered pending research; gemini-cli is dewired
	// (kept in code, unregistered per user request 2026-06-15) — active count is 8.
	if n := len(SupportedProviders()); n != 8 {
		t.Fatalf("SupportedProviders() = %d, want 8", n)
	}
}

// TestWriteFileAtomicNoTempLeftover verifies the atomic-write helper (review r2):
// it creates the parent dir, writes the exact bytes, and leaves no .graft-cfg-*
// temp file behind after a successful rename.
func TestWriteFileAtomicNoTempLeftover(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "cfg")
	path := filepath.Join(dir, "config.json")
	want := []byte(`{"k":"v"}`)
	if err := writeFileAtomic(path, want); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("content = %q, want %q", got, want)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "config.json" {
			t.Fatalf("leftover temp/extra file in dir: %q", e.Name())
		}
	}
}

// TestSaveAtomicRename verifies the global resolver's Save round-trips and that
// an existing config is replaced (not corrupted) by a subsequent Save — the
// property the temp-write+rename guards.
func TestSaveAtomicRename(t *testing.T) {
	path := filepath.Join(t.TempDir(), "graft", "config.json")
	r := &DefaultResolver{ConfigPath: path}

	if err := r.Save(&Config{Scope: "skills"}); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	if err := r.Save(&Config{Scope: "slash", Theme: "light"}); err != nil {
		t.Fatalf("second Save: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("config not valid JSON after overwrite (corruption?): %v", err)
	}
	if cfg.Scope != "slash" || cfg.Theme != "light" {
		t.Fatalf("Save did not replace prior config: %+v", cfg)
	}
}
