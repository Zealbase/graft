package grokcli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"gopkg.in/yaml.v3"
)

const wantExt = ".json"

func inFile(t *testing.T) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join("testdata", "in.*"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("no input fixture: %v", err)
	}
	return matches[0]
}

func TestParseToCanonical(t *testing.T) {
	p := New()
	pa, err := p.Parse(inFile(t))
	if err != nil {
		t.Fatal(err)
	}
	ca, err := p.ToCanonical(pa)
	if err != nil {
		t.Fatal(err)
	}
	wantBytes, err := os.ReadFile(filepath.Join("testdata", "want.canonical.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var want contract.CanonicalAgent
	if err := yaml.Unmarshal(wantBytes, &want); err != nil {
		t.Fatal(err)
	}
	want.Body = ca.Body
	gotY, _ := yaml.Marshal(ca)
	wantY, _ := yaml.Marshal(want)
	if string(gotY) != string(wantY) {
		t.Errorf("canonical mismatch:\n--- got ---\n%s\n--- want ---\n%s", gotY, wantY)
	}
}

func TestRoundTripLossless(t *testing.T) {
	p := New()
	pa, err := p.Parse(inFile(t))
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
	want, err := os.ReadFile(filepath.Join("testdata", "want"+wantExt))
	if err != nil {
		t.Fatal(err)
	}
	if string(writes[0].Data) != string(want) {
		t.Errorf("serialized mismatch:\n--- got ---\n%s\n--- want ---\n%s", writes[0].Data, want)
	}

	// Re-parse serialized output using the basename the provider chose, so
	// filename-derived identity stays stable, and assert canonical stability.
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
		t.Errorf("round-trip canonical not stable:\n%s\nvs\n%s", a, b)
	}
}
