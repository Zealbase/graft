package transform

import (
	"sort"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// wantProviders is the active set of registered provider ids (9 after antigravity deferred).
// NOTE(2026-06-13): antigravity (agy) unregistered pending research spike.
var wantProviders = []string{
	"claude-code", "codex", "cursor", "gemini-cli",
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

// TestModelForPerProvider verifies that FromCanonical writes the per-provider
// model override when set, and falls back to the canonical default otherwise.
// Uses claude-code (which has a native "model" frontmatter field) and codex
// (TOML model field).
func TestModelForPerProvider(t *testing.T) {
	r := Default()

	// Canonical agent: default model = "default-model", but claude-code gets a
	// per-provider override "sonnet", and codex gets "gpt-5-codex".
	ca := contract.CanonicalAgent{
		Name:  "model-test",
		Model: "default-model",
		ProviderOverrides: map[string]map[string]any{
			"claude-code": {"model": "sonnet"},
			"codex":       {"model": "gpt-5-codex"},
		},
	}

	// claude-code should get "sonnet".
	writes, err := r.FromCanonical(ca, "claude-code")
	if err != nil {
		t.Fatal(err)
	}
	if len(writes) == 0 {
		t.Fatal("no writes from claude-code")
	}
	if !contains(writes[0].Data, "model: sonnet") {
		t.Errorf("claude-code serialized file should contain 'model: sonnet'; got:\n%s", writes[0].Data)
	}

	// cursor should get the default "default-model" (no override).
	writes, err = r.FromCanonical(ca, "cursor")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(writes[0].Data, "model: default-model") {
		t.Errorf("cursor serialized file should contain 'model: default-model'; got:\n%s", writes[0].Data)
	}

	// The original canonical must be unchanged.
	if ca.Model != "default-model" {
		t.Errorf("original ca.Model mutated to %q", ca.Model)
	}
}

// TestToolSupporterFilteringInFromCanonical verifies that FromCanonical only
// writes tools the target provider supports, while leaving the canonical Tools
// slice unchanged.
func TestToolSupporterFilteringInFromCanonical(t *testing.T) {
	r := Default()

	// "Read" is a claude-code tool; "bash" is supported by many providers;
	// "file_edit" is supported by codex and others but NOT by claude-code.
	ca := contract.CanonicalAgent{
		Name:  "tool-test",
		Tools: []string{"Read", "Bash", "file_edit", "web_search"},
	}

	// claude-code: supports Read, Bash, WebSearch (but "Bash" is wrong case —
	// use research names exactly). Actually claude-code tools: Read, Write, Edit,
	// Bash, Glob, Grep, WebFetch, WebSearch, Agent.
	// So of our list: Read=✓, Bash=✓, file_edit=✗, web_search=✗ (wrong case — "WebSearch").
	writes, err := r.FromCanonical(ca, "claude-code")
	if err != nil {
		t.Fatal(err)
	}
	if contains(writes[0].Data, "file_edit") {
		t.Error("claude-code file must NOT contain 'file_edit' (unsupported by this provider)")
	}
	if contains(writes[0].Data, "web_search") {
		t.Error("claude-code file must NOT contain 'web_search' (use WebSearch case; this lowercase form unsupported)")
	}
	if !contains(writes[0].Data, "Read") {
		t.Error("claude-code file MUST contain 'Read'")
	}
	if !contains(writes[0].Data, "Bash") {
		t.Error("claude-code file MUST contain 'Bash'")
	}

	// Canonical Tools slice must be unchanged (no mutation).
	if len(ca.Tools) != 4 {
		t.Errorf("canonical Tools mutated: len=%d, want 4: %v", len(ca.Tools), ca.Tools)
	}

	// gemini-cli: supports "bash" ✓, "web_search" ✓, "file_read" ✓, "file_write" ✓.
	// "Read" ✗, "Bash" ✗ (wrong case — gemini uses lowercase "bash"), "file_edit" ✗.
	caGemini := contract.CanonicalAgent{
		Name:  "tool-test-gemini",
		Tools: []string{"bash", "file_read", "Read", "file_edit"},
	}
	writes, err = r.FromCanonical(caGemini, "gemini-cli")
	if err != nil {
		t.Fatal(err)
	}
	if contains(writes[0].Data, "Read") {
		t.Error("gemini-cli file must NOT contain 'Read' (unsupported; gemini uses file_read)")
	}
	if contains(writes[0].Data, "file_edit") {
		t.Error("gemini-cli file must NOT contain 'file_edit' (unsupported by gemini-cli)")
	}
	if !contains(writes[0].Data, "bash") {
		t.Error("gemini-cli file MUST contain 'bash'")
	}
	if !contains(writes[0].Data, "file_read") {
		t.Error("gemini-cli file MUST contain 'file_read'")
	}

	// Canonical unchanged after both FromCanonical calls.
	if len(ca.Tools) != 4 {
		t.Errorf("canonical Tools mutated after gemini-cli call: len=%d, want 4: %v", len(ca.Tools), ca.Tools)
	}
}

func contains(data []byte, s string) bool {
	return len(data) > 0 && bytesContain(data, []byte(s))
}

func bytesContain(haystack, needle []byte) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i <= len(haystack)-len(needle); i++ {
		if string(haystack[i:i+len(needle)]) == string(needle) {
			return true
		}
	}
	return false
}
