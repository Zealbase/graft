package config

import (
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
	c := ApplyDefaults(&Config{Providers: ProvidersConfig{
		Mode:     ProviderModeAll,
		Disabled: []string{"antigravity", "goose"},
	}})
	got := c.EffectiveProviders()
	for _, id := range got {
		if id == "antigravity" || id == "goose" {
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
	if n := len(SupportedProviders()); n != 10 {
		t.Fatalf("SupportedProviders() = %d, want 10", n)
	}
}
