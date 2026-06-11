package canonical

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

func sampleAgent() contract.CanonicalAgent {
	return contract.CanonicalAgent{
		Name:        "reviewer",
		Description: "Reviews code changes for correctness.",
		Model:       "inherit",
		Tools:       []string{"Read", "Grep", "Bash"},
		MCP:         []string{"grafana", "notion"},
		Permissions: map[string]string{
			"Bash":  "ask",
			"Write": "deny",
		},
		Body: "You are a careful code reviewer.\nFocus on correctness.",
		ProviderOverrides: map[string]map[string]any{
			"claude": {"isolation": "worktree", "effort": "high"},
			"gemini": {"timeout_mins": 10},
		},
	}
}

// writeAll applies a set of FileWrite values to disk, creating dirs.
func writeAll(t *testing.T, writes []contract.FileWrite) {
	t.Helper()
	for _, w := range writes {
		if err := os.MkdirAll(filepath.Dir(w.Path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", w.Path, err)
		}
		if err := os.WriteFile(w.Path, w.Data, 0o644); err != nil {
			t.Fatalf("write %s: %v", w.Path, err)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := sampleAgent()

	writes, err := Save(dir, want)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if len(writes) != 3 {
		t.Fatalf("expected 3 file writes, got %d", len(writes))
	}
	writeAll(t, writes)

	got, err := Load(AgentDir(dir, want.Name))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Body gets a normalized trailing newline on save; compare with that applied.
	want.Body = normalizeBody(want.Body)

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round-trip mismatch:\n got=%#v\nwant=%#v", got, want)
	}
}

func TestRoundTripMinimal(t *testing.T) {
	dir := t.TempDir()
	want := contract.CanonicalAgent{
		Name:        "min",
		Description: "Minimal agent.",
		Body:        "Do the thing.",
	}
	writes, err := Save(dir, want)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	writeAll(t, writes)

	got, err := Load(AgentDir(dir, want.Name))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want.Body = normalizeBody(want.Body)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("minimal round-trip mismatch:\n got=%#v\nwant=%#v", got, want)
	}
}

func TestHashStability(t *testing.T) {
	a := sampleAgent()
	h1 := Hash(a)
	h2 := Hash(a)
	if h1 != h2 {
		t.Fatalf("hash not stable: %s != %s", h1, h2)
	}
	if len(h1) != 64 {
		t.Fatalf("expected 64-char sha256 hex, got %d: %s", len(h1), h1)
	}
}

func TestHashInsensitiveToMapOrderAndBodyNewline(t *testing.T) {
	a := sampleAgent()

	// Rebuild maps in different insertion order; Go map order is already
	// randomized, but rebuild to be explicit.
	b := sampleAgent()
	b.Permissions = map[string]string{"Write": "deny", "Bash": "ask"}
	b.ProviderOverrides = map[string]map[string]any{
		"gemini": {"timeout_mins": 10},
		"claude": {"effort": "high", "isolation": "worktree"},
	}
	// Trailing-newline churn in body must not change the hash.
	b.Body = a.Body + "\n\n"

	if Hash(a) != Hash(b) {
		t.Fatalf("hash should be invariant to map order and trailing newlines")
	}
}

func TestHashChangesWithContent(t *testing.T) {
	a := sampleAgent()
	b := sampleAgent()
	b.Description = "Different description."
	if Hash(a) == Hash(b) {
		t.Fatalf("hash should change when a semantic field changes")
	}

	c := sampleAgent()
	c.Body = "A meaningfully different instruction body."
	if Hash(a) == Hash(c) {
		t.Fatalf("hash should change when body changes")
	}
}

func TestSaveDeterministic(t *testing.T) {
	dir := t.TempDir()
	a := sampleAgent()

	w1, err := Save(dir, a)
	if err != nil {
		t.Fatalf("Save 1: %v", err)
	}
	w2, err := Save(dir, a)
	if err != nil {
		t.Fatalf("Save 2: %v", err)
	}
	if len(w1) != len(w2) {
		t.Fatalf("write count differs")
	}
	for i := range w1 {
		if w1[i].Path != w2[i].Path {
			t.Fatalf("path order differs: %s vs %s", w1[i].Path, w2[i].Path)
		}
		if string(w1[i].Data) != string(w2[i].Data) {
			t.Fatalf("bytes for %s not deterministic:\n%s\n---\n%s",
				w1[i].Path, w1[i].Data, w2[i].Data)
		}
	}
}

func TestSaveEmptyNameFails(t *testing.T) {
	_, err := Save(t.TempDir(), contract.CanonicalAgent{Description: "x", Body: "y"})
	if err == nil {
		t.Fatalf("expected error saving agent with empty name")
	}
}

func TestMetaRoundTrip(t *testing.T) {
	dir := t.TempDir()
	a := sampleAgent()
	meta := Meta{
		Providers: map[string]ProviderMeta{
			"claude": {SourceHash: "abc123", LastCommitHash: "deadbeef"},
		},
	}
	writes, err := SaveWithMeta(dir, a, meta)
	if err != nil {
		t.Fatalf("SaveWithMeta: %v", err)
	}
	writeAll(t, writes)

	got, err := LoadMeta(AgentDir(dir, a.Name))
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}
	if got.CanonicalHash != Hash(a) {
		t.Fatalf("meta canonicalHash not recomputed: got %s want %s", got.CanonicalHash, Hash(a))
	}
	pm, ok := got.Providers["claude"]
	if !ok || pm.SourceHash != "abc123" || pm.LastCommitHash != "deadbeef" {
		t.Fatalf("provider meta lost in round-trip: %#v", got.Providers)
	}
}

func TestLoadMetaMissing(t *testing.T) {
	dir := t.TempDir()
	d := AgentDir(dir, "nope")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	m, err := LoadMeta(d)
	if err != nil {
		t.Fatalf("LoadMeta missing should not error: %v", err)
	}
	if m.CanonicalHash != "" || len(m.Providers) != 0 {
		t.Fatalf("expected zero Meta, got %#v", m)
	}
}

func TestValidatePass(t *testing.T) {
	findings, err := Validate(sampleAgent())
	if err != nil {
		t.Fatalf("Validate harness error: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected valid agent, got findings: %#v", findings)
	}
}

func TestValidateMissingRequired(t *testing.T) {
	// Missing name and description (both required); empty systemPrompt allowed
	// as a string but name pattern + required will trip.
	a := contract.CanonicalAgent{Body: "hi"}
	findings, err := Validate(a)
	if err != nil {
		t.Fatalf("Validate harness error: %v", err)
	}
	if len(findings) == 0 {
		t.Fatalf("expected findings for missing required fields")
	}
	for _, f := range findings {
		if f.Severity != severityError {
			t.Fatalf("expected error severity, got %q", f.Severity)
		}
	}
}

func TestValidateBadNamePattern(t *testing.T) {
	a := sampleAgent()
	a.Name = "Has Spaces!" // violates ^[a-zA-Z0-9][a-zA-Z0-9_-]*$
	findings, err := Validate(a)
	if err != nil {
		t.Fatalf("Validate harness error: %v", err)
	}
	if len(findings) == 0 {
		t.Fatalf("expected findings for invalid name pattern")
	}
}

func TestAgentYAMLFieldOrder(t *testing.T) {
	a := sampleAgent()
	b, err := marshalAgentYAML(a)
	if err != nil {
		t.Fatalf("marshalAgentYAML: %v", err)
	}
	s := string(b)
	// name must appear before description before model.
	iName := indexOf(s, "name:")
	iDesc := indexOf(s, "description:")
	iModel := indexOf(s, "model:")
	if !(iName >= 0 && iName < iDesc && iDesc < iModel) {
		t.Fatalf("unexpected field order in agent.yaml:\n%s", s)
	}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
