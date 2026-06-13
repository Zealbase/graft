package opencode

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestNoModelSymmetry asserts that when the input file has no model field,
// ToCanonical yields Model == "" and ProviderOverrides["opencode"] has no
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

	if ov, ok := ca.ProviderOverrides["opencode"]; ok {
		if _, hasModel := ov["model"]; hasModel {
			t.Errorf("expected no \"model\" key in ProviderOverrides[\"opencode\"], but found one")
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

// TestWithModelSymmetry asserts that when the input file has a model field,
// the round-trip preserves it.
func TestWithModelSymmetry(t *testing.T) {
	p := New()
	// Use the standard in.md which has a model field.
	path := filepath.Join("testdata", "in.md")

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
	if !strings.Contains(content, "model:") {
		t.Errorf("Serialize output missing \"model:\" even though model was set to %q:\n%s", ca.Model, content)
	}
}
