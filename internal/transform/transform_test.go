package transform

import (
	"sort"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// wantProviders is the full set of registered provider ids.
var wantProviders = []string{
	"antigravity", "claude-code", "codex", "cursor", "gemini-cli",
	"github-copilot", "goose", "grok-cli", "opencode", "roo-code",
}

func TestDefaultRegistersAll(t *testing.T) {
	r := Default()
	got := r.Providers()
	want := append([]string(nil), wantProviders...)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("got %d providers %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("provider[%d]=%q want %q (full: %v)", i, got[i], want[i], got)
		}
	}
	for _, name := range want {
		p, ok := r.Provider(name)
		if !ok {
			t.Fatalf("Provider(%q) not found", name)
		}
		if p.Name() != name {
			t.Errorf("provider %q reports Name()=%q", name, p.Name())
		}
		if len(p.Schema()) == 0 {
			t.Errorf("provider %q has empty Schema()", name)
		}
	}
}

func TestDispatchRoundsThroughProvider(t *testing.T) {
	r := Default()
	// A claude-code provider agent should convert via the registry the same as
	// calling the provider directly.
	pa := contract.ProviderAgent{
		Provider: "claude-code",
		Ref:      contract.AgentRef{Name: "x", Provider: "claude-code"},
		Fields:   map[string]any{"name": "x", "description": "d", "model": "sonnet"},
		Body:     "hi",
	}
	ca, err := r.ToCanonical(pa)
	if err != nil {
		t.Fatal(err)
	}
	if ca.Name != "x" || ca.Description != "d" || ca.Model != "sonnet" {
		t.Fatalf("unexpected canonical: %+v", ca)
	}
	writes, err := r.FromCanonical(ca, "claude-code")
	if err != nil {
		t.Fatal(err)
	}
	if len(writes) != 1 {
		t.Fatalf("expected 1 write, got %d", len(writes))
	}
}

func TestUnknownProvider(t *testing.T) {
	r := Default()
	if _, err := r.ToCanonical(contract.ProviderAgent{Provider: "nope"}); err == nil {
		t.Error("expected error for unknown provider in ToCanonical")
	}
	if _, err := r.FromCanonical(contract.CanonicalAgent{}, "nope"); err == nil {
		t.Error("expected error for unknown provider in FromCanonical")
	}
}
