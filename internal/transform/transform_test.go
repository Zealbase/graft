package transform

import (
	"sort"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// wantProviders is the active set of registered provider ids (11 after antigravity
// and gemini-cli deferred/dewired; cline, continue, kilo-code added 2026-06-16).
// NOTE(2026-06-13): antigravity (agy) unregistered pending research spike.
// NOTE(2026-06-15): gemini-cli dewired — kept in code but unregistered (user request).
var wantProviders = []string{
	"claude-code", "cline", "codex", "continue", "cursor",
	"github-copilot", "goose", "grok-cli", "kilo-code", "opencode", "roo-code",
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
// slice unchanged. All tool names in CanonicalAgent.Tools are canonical names
// (lowercase_snake_case); providers translate them to native via ToolMapper.
func TestToolSupporterFilteringInFromCanonical(t *testing.T) {
	r := Default()

	// Canonical tool names: read_file, bash, file_edit, web_search.
	// claude-code supports all four: Read, Bash, Edit, WebSearch respectively.
	ca := contract.CanonicalAgent{
		Name:  "tool-test",
		Tools: []string{"read_file", "bash", "file_edit", "web_search"},
	}

	writes, err := r.FromCanonical(ca, "claude-code")
	if err != nil {
		t.Fatal(err)
	}
	// All four tools are supported by claude-code (translated to native names).
	if !contains(writes[0].Data, "Read") {
		t.Error("claude-code file MUST contain 'Read' (canonical read_file → native Read)")
	}
	if !contains(writes[0].Data, "Bash") {
		t.Error("claude-code file MUST contain 'Bash' (canonical bash → native Bash)")
	}
	if !contains(writes[0].Data, "Edit") {
		t.Error("claude-code file MUST contain 'Edit' (canonical file_edit → native Edit)")
	}
	if !contains(writes[0].Data, "WebSearch") {
		t.Error("claude-code file MUST contain 'WebSearch' (canonical web_search → native WebSearch)")
	}

	// Canonical Tools slice must be unchanged (no mutation).
	if len(ca.Tools) != 4 {
		t.Errorf("canonical Tools mutated: len=%d, want 4: %v", len(ca.Tools), ca.Tools)
	}

	// NOTE(2026-06-15): the gemini-cli tool-filtering assertions were removed here
	// because gemini-cli is dewired (unregistered from the registry; kept in code).
}

// TestCrossProviderRename verifies that canonical tool names are serialized into
// each provider's native names correctly (read_file → Read for claude-code,
// read_file → view for github-copilot, read_file → read_file for gemini-cli).
func TestCrossProviderRename(t *testing.T) {
	r := Default()
	ca := contract.CanonicalAgent{
		Name:  "cross-provider",
		Tools: []string{"read_file", "bash"},
	}

	cases := []struct {
		provider string
		wantRead string
		wantBash string
	}{
		{"claude-code", "Read", "Bash"},
		{"github-copilot", "read", "execute"},
		// NOTE(2026-06-15): gemini-cli case removed — provider dewired (kept in code).
	}
	for _, tc := range cases {
		writes, err := r.FromCanonical(ca, tc.provider)
		if err != nil {
			t.Fatalf("%s: FromCanonical error: %v", tc.provider, err)
		}
		if !contains(writes[0].Data, tc.wantRead) {
			t.Errorf("%s: want %q in output; got:\n%s", tc.provider, tc.wantRead, writes[0].Data)
		}
		if !contains(writes[0].Data, tc.wantBash) {
			t.Errorf("%s: want %q in output; got:\n%s", tc.provider, tc.wantBash, writes[0].Data)
		}
	}
}

// TestCanonicalizeOnParse verifies that ToCanonical translates native tool names
// to canonical names (e.g. "Read" → "read_file" for claude-code).
func TestCanonicalizeOnParse(t *testing.T) {
	r := Default()
	pa := contract.ProviderAgent{
		Provider: "claude-code",
		Ref:      contract.AgentRef{Name: "tst", Provider: "claude-code"},
		Fields: map[string]any{
			"name":  "tst",
			"tools": "Read, Bash, Edit",
		},
	}
	ca, err := r.ToCanonical(pa)
	if err != nil {
		t.Fatal(err)
	}
	wantTools := []string{"read_file", "bash", "file_edit"}
	if len(ca.Tools) != len(wantTools) {
		t.Fatalf("got tools %v, want %v", ca.Tools, wantTools)
	}
	for i, w := range wantTools {
		if ca.Tools[i] != w {
			t.Errorf("Tools[%d] = %q, want %q", i, ca.Tools[i], w)
		}
	}
}

// TestUnmappedPassThrough verifies that a canonical tool name with no mapping
// in the provider's ToolMapper passes through verbatim in both directions.
func TestUnmappedPassThrough(t *testing.T) {
	r := Default()
	// "future_tool" has no mapping in any provider's ToolMapper.
	ca := contract.CanonicalAgent{
		Name:  "unmapped-test",
		Tools: []string{"read_file", "future_tool"},
	}

	// claude-code: "future_tool" is unmapped → always kept and serialized verbatim.
	writes, err := r.FromCanonical(ca, "claude-code")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(writes[0].Data, "future_tool") {
		t.Error("claude-code file MUST contain 'future_tool' (unmapped pass-through)")
	}
	if !contains(writes[0].Data, "Read") {
		t.Error("claude-code file MUST contain 'Read' (canonical read_file → Read)")
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
