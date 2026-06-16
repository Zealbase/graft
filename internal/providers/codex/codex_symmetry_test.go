package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"gopkg.in/yaml.v3"
)

// TestNoModelSymmetry asserts that when the input file has no model field,
// ToCanonical yields Model == "" and ProviderOverrides["codex"] has no
// "model" key, and that Serialize writes no "model =" key in the output.
func TestNoModelSymmetry(t *testing.T) {
	p := New()
	path := filepath.Join("testdata", "nomodel.toml")

	pa, err := p.Parse(path)
	if err != nil {
		t.Fatalf("Parse(%q) error: %v", path, err)
	}

	ca, err := p.ToCanonical(pa)
	if err != nil {
		t.Fatalf("ToCanonical error: %v", err)
	}

	if ca.Model != "" {
		t.Errorf("expected Model == \"\", got %q", ca.Model)
	}

	if ov, ok := ca.ProviderOverrides["codex"]; ok {
		if _, hasModel := ov["model"]; hasModel {
			t.Errorf("expected no \"model\" key in ProviderOverrides[\"codex\"], but found one")
		}
	}

	// Serialize must not emit a model key.
	writes, err := p.Serialize(ca)
	if err != nil {
		t.Fatalf("Serialize error: %v", err)
	}
	if len(writes) == 0 {
		t.Fatal("Serialize returned no file writes")
	}
	content := string(writes[0].Data)
	// Check for the TOML key assignment specifically (not the word inside values).
	if strings.Contains(content, "model =") {
		t.Errorf("Serialize output contains \"model =\" key but none was set:\n%s", content)
	}
}

// TestSkillsConfigRoundTrip verifies the [[skills.config]] round-trip:
// enabled entries map to ca.Skills; disabled entries are stashed and restored.
func TestSkillsConfigRoundTrip(t *testing.T) {
	p := New()
	inPath := filepath.Join("testdata", "skills-in.toml")
	pa, err := p.Parse(inPath)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ca, err := p.ToCanonical(pa)
	if err != nil {
		t.Fatalf("ToCanonical: %v", err)
	}

	// Verify canonical YAML matches the want fixture.
	wantBytes, err := os.ReadFile(filepath.Join("testdata", "skills-want.canonical.yaml"))
	if err != nil {
		t.Fatalf("read want canonical: %v", err)
	}
	var want contract.CanonicalAgent
	if err := yaml.Unmarshal(wantBytes, &want); err != nil {
		t.Fatalf("unmarshal want: %v", err)
	}
	want.Body = ca.Body
	gotY, _ := yaml.Marshal(ca)
	wantY, _ := yaml.Marshal(want)
	if string(gotY) != string(wantY) {
		t.Errorf("canonical mismatch:\n--- got ---\n%s\n--- want ---\n%s", gotY, wantY)
	}

	// Verify enabled skills are in ca.Skills, not in override bucket.
	if len(ca.Skills) != 2 {
		t.Fatalf("expected 2 skills in ca.Skills, got %d: %v", len(ca.Skills), ca.Skills)
	}
	if ov, ok := ca.ProviderOverrides[name]; ok {
		if _, inOvr := ov["skills"]; inOvr {
			t.Error("raw 'skills' key must not appear in ProviderOverrides (should be parsed)")
		}
	}

	// Verify disabled entry is stashed.
	ov := ca.ProviderOverrides[name]
	if ov == nil {
		t.Fatal("expected providerOverrides[codex] to be set (disabled entry + sandbox_mode)")
	}
	if _, ok := ov[codexSkillsDisabledKey]; !ok {
		t.Errorf("expected %q key in providerOverrides[codex]", codexSkillsDisabledKey)
	}

	// Verify Serialize produces the expected TOML.
	writes, err := p.Serialize(ca)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	wantTOML, err := os.ReadFile(filepath.Join("testdata", "skills-want.toml"))
	if err != nil {
		t.Fatalf("read want toml: %v", err)
	}
	if string(writes[0].Data) != string(wantTOML) {
		t.Errorf("serialized TOML mismatch:\n--- got ---\n%s\n--- want ---\n%s", writes[0].Data, wantTOML)
	}

	// Verify full round-trip stability (parse→canonical→serialize→parse→canonical).
	dir := t.TempDir()
	tmp := filepath.Join(dir, "researcher.toml")
	if err := os.WriteFile(tmp, writes[0].Data, 0o644); err != nil {
		t.Fatal(err)
	}
	pa2, err := p.Parse(tmp)
	if err != nil {
		t.Fatalf("Parse round-trip: %v", err)
	}
	ca2, err := p.ToCanonical(pa2)
	if err != nil {
		t.Fatalf("ToCanonical round-trip: %v", err)
	}
	a, _ := yaml.Marshal(ca)
	b, _ := yaml.Marshal(ca2)
	if string(a) != string(b) {
		t.Errorf("round-trip canonical not stable:\n%s\nvs\n%s", a, b)
	}
}

// TestSkillsOverrideWins verifies that providerOverrides[codex].skills
// wins over canonical Skills on Serialize.
func TestSkillsOverrideWins(t *testing.T) {
	p := New()
	ca := contract.CanonicalAgent{
		Name:   "test-agent",
		Skills: []string{"canonical-skill"},
		ProviderOverrides: map[string]map[string]any{
			name: {"skills": []any{"override-skill-a", "override-skill-b"}},
		},
	}
	writes, err := p.Serialize(ca)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	content := string(writes[0].Data)
	if strings.Contains(content, "canonical-skill") {
		t.Errorf("canonical-skill must be suppressed by override:\n%s", content)
	}
	if !strings.Contains(content, "override-skill-a") || !strings.Contains(content, "override-skill-b") {
		t.Errorf("override skills must appear in output:\n%s", content)
	}
}

// TestWithModelSymmetry asserts that when the input file has a model field,
// the round-trip preserves it.
func TestWithModelSymmetry(t *testing.T) {
	p := New()
	path := inFile(t)

	pa, err := p.Parse(path)
	if err != nil {
		t.Fatalf("Parse(%q) error: %v", path, err)
	}

	ca, err := p.ToCanonical(pa)
	if err != nil {
		t.Fatalf("ToCanonical error: %v", err)
	}

	if ca.Model == "" {
		t.Skip("standard fixture has no model; skipping round-trip model check")
	}

	writes, err := p.Serialize(ca)
	if err != nil {
		t.Fatalf("Serialize error: %v", err)
	}
	if len(writes) == 0 {
		t.Fatal("Serialize returned no file writes")
	}
	content := string(writes[0].Data)
	if !strings.Contains(content, "model =") {
		t.Errorf("Serialize output missing \"model =\" even though model was set to %q:\n%s", ca.Model, content)
	}
}
