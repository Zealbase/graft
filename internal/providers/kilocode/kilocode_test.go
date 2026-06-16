package kilocode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"gopkg.in/yaml.v3"
)

// TestModernParseToCanonical parses a modern .kilo/agents/planner.md fixture,
// runs ToCanonical, and compares to want.canonical.yaml.
func TestModernParseToCanonical(t *testing.T) {
	p := New()
	inFile := filepath.Join("testdata", "modern", "planner.md")
	pa, err := p.Parse(inFile)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := p.ToCanonical(pa)
	if err != nil {
		t.Fatal(err)
	}
	wantBytes, err := os.ReadFile(filepath.Join("testdata", "modern", "want.canonical.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var want contract.CanonicalAgent
	if err := yaml.Unmarshal(wantBytes, &want); err != nil {
		t.Fatal(err)
	}
	// Body is not compared via YAML (it's a "-" field); compare separately.
	want.Body = ca.Body

	gotY, _ := yaml.Marshal(ca)
	wantY, _ := yaml.Marshal(want)
	if string(gotY) != string(wantY) {
		t.Errorf("canonical mismatch:\n--- got ---\n%s\n--- want ---\n%s", gotY, wantY)
	}
}

// TestModernRoundTripLossless exercises parse → ToCanonical → Serialize → re-parse → ToCanonical
// and asserts that canonical output is stable.
func TestModernRoundTripLossless(t *testing.T) {
	p := New()
	inFile := filepath.Join("testdata", "modern", "planner.md")
	pa, err := p.Parse(inFile)
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
	if len(writes) != 1 {
		t.Fatalf("expected 1 file write, got %d", len(writes))
	}

	// Compare serialized output against want.planner.md
	want, err := os.ReadFile(filepath.Join("testdata", "modern", "want.planner.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(writes[0].Data) != string(want) {
		t.Errorf("serialized mismatch:\n--- got ---\n%s\n--- want ---\n%s", writes[0].Data, want)
	}

	// Re-parse serialized output and assert canonical stability.
	dir := t.TempDir()
	tmp := filepath.Join(dir, filepath.Base(writes[0].Path))
	if err := os.MkdirAll(filepath.Dir(tmp), 0o755); err != nil {
		t.Fatal(err)
	}
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
		t.Errorf("round-trip canonical not stable:\n%s\nvs\n%s", a, b)
	}
}

// TestLegacyParseToModernSerialize parses the .kilocodemodes legacy fixture,
// runs ToCanonical, then Serialize — which must produce a modern .kilo/agents/<name>.md.
func TestLegacyParseToModernSerialize(t *testing.T) {
	p := New()
	inFile := filepath.Join("testdata", "legacy", "in.kilocodemodes")
	pa, err := p.Parse(inFile)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := p.ToCanonical(pa)
	if err != nil {
		t.Fatal(err)
	}
	if ca.Name != "reviewer" {
		t.Errorf("expected name=reviewer, got %q", ca.Name)
	}
	writes, err := p.Serialize(ca)
	if err != nil {
		t.Fatal(err)
	}
	if len(writes) != 1 {
		t.Fatalf("expected 1 file write, got %d", len(writes))
	}
	// Must serialize to modern .kilo/agents/<name>.md path
	if !strings.HasSuffix(writes[0].Path, filepath.Join(".kilo", "agents", "reviewer.md")) {
		t.Errorf("expected modern output path, got %q", writes[0].Path)
	}
	// The output must be a valid markdown-with-frontmatter file
	if !strings.HasPrefix(string(writes[0].Data), "---\n") {
		t.Errorf("expected YAML frontmatter, got:\n%s", writes[0].Data)
	}
}
