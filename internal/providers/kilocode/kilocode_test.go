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
	// Body is not in the YAML fixture ("-" json tag); assert it against the
	// literal expected string from the fixture file.
	wantBody := "You are a planning expert. Break user requests into clear, actionable steps.\n"
	if ca.Body != wantBody {
		t.Errorf("canonical body mismatch:\n--- got ---\n%q\n--- want ---\n%q", ca.Body, wantBody)
	}
	// For the YAML comparison, set want.Body to match so the struct comparison
	// doesn't fail on the zero value from yaml.Unmarshal.
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
	// Parse with explicit slug suffix — mirrors what Detect emits.
	pa, err := p.Parse(inFile + "#reviewer")
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

// TestLegacyMultiModeSlugAware verifies that a .kilocodemodes file with two
// modes is parsed slug-by-slug, each returning its OWN name and body.
// Guards regression for bug: parseLegacy always returning CustomModes[0].
func TestLegacyMultiModeSlugAware(t *testing.T) {
	p := New()
	inFile := filepath.Join("testdata", "legacy", "in.kilocodemodes")

	cases := []struct {
		slug     string
		wantName string
		wantBody string
	}{
		{
			slug:     "reviewer",
			wantName: "reviewer",
			wantBody: "You are a thorough code reviewer.",
		},
		{
			slug:     "fixer",
			wantName: "fixer",
			wantBody: "You are an expert bug fixer.",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.slug, func(t *testing.T) {
			pa, err := p.Parse(inFile + "#" + tc.slug)
			if err != nil {
				t.Fatal(err)
			}
			if pa.Ref.Name != tc.wantName {
				t.Errorf("Ref.Name: got %q, want %q", pa.Ref.Name, tc.wantName)
			}
			if pa.Body != tc.wantBody {
				t.Errorf("Body: got %q, want %q", pa.Body, tc.wantBody)
			}
			ca, err := p.ToCanonical(pa)
			if err != nil {
				t.Fatal(err)
			}
			if ca.Name != tc.wantName {
				t.Errorf("ca.Name: got %q, want %q", ca.Name, tc.wantName)
			}
			if ca.Body != tc.wantBody {
				t.Errorf("ca.Body: got %q, want %q", ca.Body, tc.wantBody)
			}
		})
	}
}

// TestModernHashInDirRoundTrip guards the fix for ToCanonical incorrectly
// stripping the "#" when a modern agent's containing directory path includes a
// "#" character (e.g. /tmp/my#project/.kilo/agents/reviewer.md). Before the
// fix, LastIndex found the "#" in the dir name and truncated the path to
// "/tmp/my", causing the agent to be misparsed as LEGACY and losing tools.
func TestModernHashInDirRoundTrip(t *testing.T) {
	p := New()

	// Build a tempdir whose parent directory name contains "#".
	base := t.TempDir()
	projectDir := filepath.Join(base, "my#project")
	agentDir := filepath.Join(projectDir, ".kilo", "agents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write a modern .md agent file with a permission block.
	agentFile := filepath.Join(agentDir, "reviewer.md")
	content := `---
description: Thorough code reviewer
model: claude-opus-4-8
permission:
  allow:
    - read
    - edit
  deny: []
  ask: []
---
You are a thorough code reviewer.
`
	if err := os.WriteFile(agentFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// Parse must succeed and identify it as MODERN (Ref.Path ends in .md).
	pa, err := p.Parse(agentFile)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// ToCanonical must treat it as a modern agent — tools must be present.
	ca, err := p.ToCanonical(pa)
	if err != nil {
		t.Fatalf("ToCanonical: %v", err)
	}

	if ca.Name != "reviewer" {
		t.Errorf("Name: got %q, want %q", ca.Name, "reviewer")
	}
	if ca.Description != "Thorough code reviewer" {
		t.Errorf("Description: got %q, want %q", ca.Description, "Thorough code reviewer")
	}
	// Tools must be populated — if ToCanonical misparsed as legacy, tools would
	// be derived from the absent "groups" field and ca.Tools would be nil.
	if len(ca.Tools) == 0 {
		t.Fatal("Tools is empty — agent was likely misparsed as LEGACY (ToCanonical path-strip bug)")
	}
	// "read" and "edit" are native kilo tools that map to read_file and file_edit.
	wantTools := map[string]bool{"read_file": true, "file_edit": true}
	for _, tool := range ca.Tools {
		delete(wantTools, tool)
	}
	if len(wantTools) > 0 {
		t.Errorf("missing canonical tools after round-trip: %v (got %v)", wantTools, ca.Tools)
	}
}

// TestLegacyGroupsToCanonicalTools verifies that legacy groups are translated to
// canonical tools during ToCanonical, so Serialize emits a permission block.
// Guards regression for bug: toCanonicalLegacy ignoring groups → tools lost.
func TestLegacyGroupsToCanonicalTools(t *testing.T) {
	p := New()
	inFile := filepath.Join("testdata", "legacy", "in.kilocodemodes")

	// "fixer" has groups: [read, edit, command] → canonical [bash, file_edit, read_file]
	pa, err := p.Parse(inFile + "#fixer")
	if err != nil {
		t.Fatal(err)
	}
	ca, err := p.ToCanonical(pa)
	if err != nil {
		t.Fatal(err)
	}

	wantTools := []string{"bash", "file_edit", "read_file"}
	if len(ca.Tools) != len(wantTools) {
		t.Fatalf("tools length: got %d %v, want %d %v", len(ca.Tools), ca.Tools, len(wantTools), wantTools)
	}
	for i, w := range wantTools {
		if ca.Tools[i] != w {
			t.Errorf("tools[%d]: got %q, want %q", i, ca.Tools[i], w)
		}
	}

	// Serialize must produce a permission block derived from those tools.
	writes, err := p.Serialize(ca)
	if err != nil {
		t.Fatal(err)
	}
	if len(writes) != 1 {
		t.Fatalf("expected 1 file write, got %d", len(writes))
	}
	data := string(writes[0].Data)
	if !strings.Contains(data, "permission:") {
		t.Errorf("serialized output missing permission block:\n%s", data)
	}
	// Native tool names must appear: read→read, edit→edit, bash→bash
	for _, native := range []string{"read", "edit", "bash"} {
		if !strings.Contains(data, native) {
			t.Errorf("serialized permission missing native tool %q:\n%s", native, data)
		}
	}
}
