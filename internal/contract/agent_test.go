package contract

import (
	"testing"
)

// TestFieldFor_ReturnsOverrideWhenSet verifies that FieldFor returns the
// provider-specific override value when one is set.
func TestFieldFor_ReturnsOverrideWhenSet(t *testing.T) {
	a := CanonicalAgent{
		Name:        "my-agent",
		Description: "canonical-desc",
		Model:       "canonical-model",
		Tools:       []string{"Read"},
		ProviderOverrides: map[string]map[string]any{
			"claude-code": {
				"description": "override-desc",
				"model":       "override-model",
				"temperature": 0.5,
			},
		},
	}

	cases := []struct {
		field   string
		wantVal any
	}{
		{"description", "override-desc"},
		{"model", "override-model"},
		{"temperature", 0.5},
	}
	for _, tc := range cases {
		v, ok := a.FieldFor("claude-code", tc.field)
		if !ok {
			t.Errorf("FieldFor(claude-code, %q): ok=false, want true", tc.field)
			continue
		}
		if v != tc.wantVal {
			t.Errorf("FieldFor(claude-code, %q) = %v, want %v", tc.field, v, tc.wantVal)
		}
	}
}

// TestFieldFor_CanonicalFallback verifies that FieldFor returns the canonical
// value when no override is set for the requested provider.
func TestFieldFor_CanonicalFallback(t *testing.T) {
	a := CanonicalAgent{
		Name:        "my-agent",
		Description: "canonical-desc",
		Model:       "canonical-model",
		Tools:       []string{"Read", "Write"},
		ProviderOverrides: map[string]map[string]any{
			"claude-code": {"model": "sonnet"},
		},
	}

	// For a provider with no overrides, all canonical fields fall back.
	cases := []struct {
		field   string
		wantVal any
		wantOK  bool
	}{
		{"name", "my-agent", true},
		{"description", "canonical-desc", true},
		{"model", "canonical-model", true},
	}
	for _, tc := range cases {
		v, ok := a.FieldFor("cursor", tc.field)
		if ok != tc.wantOK {
			t.Errorf("FieldFor(cursor, %q): ok=%v, want %v", tc.field, ok, tc.wantOK)
			continue
		}
		if ok && v != tc.wantVal {
			t.Errorf("FieldFor(cursor, %q) = %v, want %v", tc.field, v, tc.wantVal)
		}
	}
}

// TestFieldFor_NameNotOverridable verifies that "name" cannot be overridden by
// providerOverrides — FieldFor always returns the canonical Name for "name".
func TestFieldFor_NameNotOverridable(t *testing.T) {
	a := CanonicalAgent{
		Name: "real-name",
		ProviderOverrides: map[string]map[string]any{
			"claude-code": {"name": "attacker-name"},
			"codex":       {"name": "other-name"},
		},
	}

	for _, provider := range []string{"claude-code", "codex", "cursor"} {
		v, ok := a.FieldFor(provider, "name")
		if !ok {
			t.Errorf("FieldFor(%q, name): ok=false, want true", provider)
			continue
		}
		if v != "real-name" {
			t.Errorf("FieldFor(%q, name) = %v, want canonical 'real-name'", provider, v)
		}
	}
}

// TestFieldFor_UnknownField verifies that FieldFor returns false for fields
// that have no canonical home and no override.
func TestFieldFor_UnknownField(t *testing.T) {
	a := CanonicalAgent{Name: "x"}
	v, ok := a.FieldFor("claude-code", "nonexistent-field")
	if ok {
		t.Errorf("FieldFor for unknown field with no override should return ok=false, got v=%v", v)
	}
}

// TestFieldFor_UnknownFieldWithOverride verifies that FieldFor returns the
// override value for a provider-specific field that has no canonical home.
func TestFieldFor_UnknownFieldWithOverride(t *testing.T) {
	a := CanonicalAgent{
		Name: "x",
		ProviderOverrides: map[string]map[string]any{
			"claude-code": {"color": "purple"},
		},
	}
	v, ok := a.FieldFor("claude-code", "color")
	if !ok {
		t.Errorf("FieldFor for provider-specific override 'color': ok=false, want true")
	}
	if v != "purple" {
		t.Errorf("FieldFor(claude-code, color) = %v, want 'purple'", v)
	}
}

// TestFieldFor_Isolation verifies that an override for provider A does not
// appear when queried for provider B.
func TestFieldFor_Isolation(t *testing.T) {
	a := CanonicalAgent{
		Name:  "x",
		Model: "default-model",
		ProviderOverrides: map[string]map[string]any{
			"claude-code": {"model": "sonnet"},
		},
	}

	// claude-code gets the override
	v, _ := a.FieldFor("claude-code", "model")
	if v != "sonnet" {
		t.Errorf("FieldFor(claude-code, model) = %v, want sonnet", v)
	}

	// codex must get the canonical default
	v, _ = a.FieldFor("codex", "model")
	if v != "default-model" {
		t.Errorf("FieldFor(codex, model) = %v, want default-model (isolation violation)", v)
	}
}

// TestModelFor_DelegatesToFieldFor verifies that ModelFor delegates to FieldFor
// and thus respects the override-wins, name-protected semantics.
func TestModelFor_DelegatesToFieldFor(t *testing.T) {
	a := CanonicalAgent{
		Name:  "x",
		Model: "default",
		ProviderOverrides: map[string]map[string]any{
			"claude-code": {"model": "sonnet"},
		},
	}

	if got := a.ModelFor("claude-code"); got != "sonnet" {
		t.Errorf("ModelFor(claude-code) = %q, want sonnet", got)
	}
	if got := a.ModelFor("codex"); got != "default" {
		t.Errorf("ModelFor(codex) = %q, want default (fallback)", got)
	}
	if got := a.ModelFor("cursor"); got != "default" {
		t.Errorf("ModelFor(cursor) = %q, want default (no override)", got)
	}
}

// TestFieldFor_NilValueOverrideIgnored verifies that a nil override value
// is treated as absent (falls back to canonical).
func TestFieldFor_NilValueOverrideIgnored(t *testing.T) {
	a := CanonicalAgent{
		Name:  "x",
		Model: "canonical-model",
		ProviderOverrides: map[string]map[string]any{
			"claude-code": {"model": nil}, // explicitly nil
		},
	}
	v, ok := a.FieldFor("claude-code", "model")
	if !ok {
		t.Errorf("FieldFor with nil override for a canonical field: ok=false, want true (canonical fallback)")
	}
	if v != "canonical-model" {
		t.Errorf("FieldFor(claude-code, model) with nil override = %v, want canonical-model", v)
	}
}

// TestFieldFor_AllCanonicalFields ensures each canonical field name is
// recognized and returns the set value.
func TestFieldFor_AllCanonicalFields(t *testing.T) {
	a := CanonicalAgent{
		Name:        "agent-name",
		Description: "agent-desc",
		Model:       "agent-model",
		Tools:       []string{"Read"},
		MCP:         []string{"mcp-server"},
		Permissions: map[string]string{"tool": "allow"},
	}

	cases := []struct {
		field string
	}{
		{"name"},
		{"description"},
		{"model"},
		{"tools"},
		{"mcp"},
		{"permissions"},
	}
	for _, tc := range cases {
		_, ok := a.FieldFor("any-provider", tc.field)
		if !ok {
			t.Errorf("FieldFor(any-provider, %q): ok=false, want true for canonical field", tc.field)
		}
	}
}
