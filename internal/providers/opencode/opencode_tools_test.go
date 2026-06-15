package opencode

// Track C — opencode `tools` field enters/leaves the canonical pipeline.
//
// Level: unit/provider — Parse/ToCanonical/Serialize, file IO via t.TempDir.
// Covers:
//   - enabled tools (true) -> canonical Tools (mapped native->canonical)
//   - disabled tools (false) preserved losslessly across a round-trip
//   - serialize maps canonical Tools back to the native object map (name: true)

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

const toolsFixture = `---
description: agent with tools
model: anthropic/claude-sonnet-4
tools:
  websearch: true
  read: true
  bash: false
---
body here
`

func parseFixture(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "tooled.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOpencodeTools_EnabledToCanonical(t *testing.T) {
	p := New()
	pa, err := p.Parse(parseFixture(t, toolsFixture))
	if err != nil {
		t.Fatal(err)
	}
	ca, err := p.ToCanonical(pa)
	if err != nil {
		t.Fatal(err)
	}
	// websearch->web_search, read->read_file are enabled; bash is disabled.
	got := map[string]bool{}
	for _, c := range ca.Tools {
		got[c] = true
	}
	if !got["web_search"] || !got["read_file"] {
		t.Errorf("enabled tools not canonicalized: %v", ca.Tools)
	}
	if got["bash"] {
		t.Errorf("disabled tool 'bash' wrongly appears in canonical Tools: %v", ca.Tools)
	}
	// The disabled native tool is preserved under the private overrides key.
	ov := ca.ProviderOverrides["opencode"]
	if ov == nil || ov[disabledToolsKey] == nil {
		t.Fatalf("disabled tools not preserved in overrides: %v", ov)
	}
}

func TestOpencodeTools_RoundTripLossless(t *testing.T) {
	p := New()
	path := parseFixture(t, toolsFixture)
	pa, err := p.Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := p.ToCanonical(pa)
	if err != nil {
		t.Fatal(err)
	}
	writes, err := p.Serialize(ca)
	if err != nil {
		t.Fatal(err)
	}
	// Re-parse the serialized output; the canonical form must be stable
	// (enabled tools recovered, disabled tool preserved).
	dir := t.TempDir()
	tmp := filepath.Join(dir, filepath.Base(writes[0].Path))
	if err := os.WriteFile(tmp, writes[0].Data, 0o644); err != nil {
		t.Fatal(err)
	}
	pa2, err := p.Parse(tmp)
	if err != nil {
		t.Fatal(err)
	}
	ca2, err := p.ToCanonical(pa2)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := yaml.Marshal(ca)
	b, _ := yaml.Marshal(ca2)
	if string(a) != string(b) {
		t.Errorf("opencode tools round-trip not stable:\n--- first ---\n%s\n--- second ---\n%s", a, b)
	}

	// The serialized tools object must carry the disabled entry (bash: false)
	// and the enabled native names.
	out := string(writes[0].Data)
	for _, want := range []string{"websearch", "read", "bash"} {
		if !contains(out, want) {
			t.Errorf("serialized output missing native tool %q:\n%s", want, out)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (indexOf(haystack, needle) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
