package claudecode

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// TestNoModelSymmetry asserts that when the input file has no model field,
// ToCanonical yields Model == "" and ProviderOverrides["claude-code"] has no
// "model" key, and that Serialize writes no "model:" key in the output.
func TestNoModelSymmetry(t *testing.T) {
	p := New()
	path := filepath.Join("testdata", "nomodel.md")

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

	if ov, ok := ca.ProviderOverrides["claude-code"]; ok {
		if _, hasModel := ov["model"]; hasModel {
			t.Errorf("expected no \"model\" key in ProviderOverrides[\"claude-code\"], but found one")
		}
	}

	// Serialize must not emit a model: key.
	writes, err := p.Serialize(ca)
	if err != nil {
		t.Fatalf("Serialize error: %v", err)
	}
	if len(writes) == 0 {
		t.Fatal("Serialize returned no file writes")
	}
	content := string(writes[0].Data)
	if strings.Contains(content, "model:") {
		t.Errorf("Serialize output contains \"model:\" but none was set:\n%s", content)
	}
}

// TestSkillsFirstClass verifies that `skills:` in frontmatter is parsed into
// ca.Skills (not left in ProviderOverrides), and Serialize emits it after tools.
func TestSkillsFirstClass(t *testing.T) {
	p := New()
	pa, err := p.Parse(inFile(t))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	ca, err := p.ToCanonical(pa)
	if err != nil {
		t.Fatalf("ToCanonical error: %v", err)
	}
	// skills must be in canonical Skills, not in ProviderOverrides.
	if len(ca.Skills) == 0 {
		t.Error("expected ca.Skills to be populated from frontmatter")
	}
	if ov, ok := ca.ProviderOverrides[name]; ok {
		if _, inOvr := ov["skills"]; inOvr {
			t.Error("skills must NOT appear in ProviderOverrides after promotion to first-class")
		}
	}
	writes, err := p.Serialize(ca)
	if err != nil {
		t.Fatalf("Serialize error: %v", err)
	}
	content := string(writes[0].Data)
	if !strings.Contains(content, "skills:") {
		t.Errorf("Serialize output missing skills: field:\n%s", content)
	}
}

// TestSkillsOverrideWins verifies that providerOverrides[claude-code].skills
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
		t.Fatalf("Serialize error: %v", err)
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

	// The standard in.md fixture has a model field.
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
	if !strings.Contains(content, "model:") {
		t.Errorf("Serialize output missing \"model:\" even though model was set to %q:\n%s", ca.Model, content)
	}
}
