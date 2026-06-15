package gateway

// Integration tests for providerOverrides schema validation (v0.0.4 conformance).
//
// Level: integration — needs the live transformer registry and catalog.
// Covers:
//   - Unknown field in providerOverrides[p] → warning (not error, never blocks).
//   - Known schema field → no finding.
//   - "name" in providerOverrides[p] → warning from nameOverrideFindings.
//   - name-override warning does NOT block the pre-sync gate (errorFindings).
//   - Table-driven over registered providers: schema check for each.
//   - Isolation: schema warning for one provider does not affect another.

import (
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/transform"
)

// TestProviderOverrideSchema_UnknownField_IsWarning verifies that a field
// not present in the provider's catalog schema produces a warning (not error).
// A warning never blocks the pre-sync gate.
func TestProviderOverrideSchema_UnknownField_IsWarning(t *testing.T) {
	g := &gate{tr: transform.Default()}
	a := contract.CanonicalAgent{
		Name:        "agent-x",
		Description: "desc",
		ProviderOverrides: map[string]map[string]any{
			"claude-code": {"totally_unknown_field_xyz": "value"},
		},
	}
	findings := g.providerOverrideSchemaFindings(a)
	if len(findings) == 0 {
		t.Fatal("expected a warning finding for unknown field, got none")
	}
	for _, f := range findings {
		if f.Severity != "warning" {
			t.Errorf("schema finding severity=%q, want warning", f.Severity)
		}
		if !strings.Contains(f.Message, "totally_unknown_field_xyz") {
			t.Errorf("finding message %q does not mention the unknown field", f.Message)
		}
		if !strings.Contains(f.Message, "claude-code") {
			t.Errorf("finding message %q does not mention the provider", f.Message)
		}
	}
	// Schema warnings must NOT block the gate.
	if blocking := errorFindings(findings); len(blocking) != 0 {
		t.Fatalf("schema warning must not block the pre-sync gate: %+v", blocking)
	}
}

// TestProviderOverrideSchema_KnownField_NoFinding verifies that a known schema
// field in providerOverrides[p] produces no schema finding.
// Uses well-known fields from each provider's catalog schema.
func TestProviderOverrideSchema_KnownField_NoFinding(t *testing.T) {
	g := &gate{tr: transform.Default()}

	cases := []struct {
		provID string
		field  string
		value  any
	}{
		{"claude-code", "model", "sonnet"},
		{"claude-code", "description", "my description"},
		{"claude-code", "tools", []string{"Read"}},
		{"cursor", "model", "gpt-4o"},
		{"cursor", "description", "desc"},
		// NOTE(2026-06-15): gemini-cli row removed — provider dewired (kept in code).
		{"opencode", "model", "anthropic/claude-sonnet-4"},
		{"opencode", "temperature", 0.5},
		{"codex", "model", "o4-mini"},
		{"codex", "description", "desc"},
		{"grok-cli", "model", "grok-3"},
		{"github-copilot", "model", "gpt-4.1"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run("provider="+tc.provID+"/field="+tc.field, func(t *testing.T) {
			a := contract.CanonicalAgent{
				Name: "agent-x",
				ProviderOverrides: map[string]map[string]any{
					tc.provID: {tc.field: tc.value},
				},
			}
			findings := g.providerOverrideSchemaFindings(a)
			for _, f := range findings {
				if strings.Contains(f.Message, tc.field) {
					t.Errorf("known field %q for provider %q produced unexpected schema finding: %+v",
						tc.field, tc.provID, f)
				}
			}
		})
	}
}

// TestProviderOverrideSchema_TableDriven_AllProviders verifies that each
// registered provider's catalog schema is loadable and that setting a clearly
// unknown field produces a warning. This is table-driven so new providers
// auto-extend coverage.
func TestProviderOverrideSchema_TableDriven_AllProviders(t *testing.T) {
	g := &gate{tr: transform.Default()}
	for _, provID := range g.tr.Providers() {
		provID := provID
		t.Run("provider="+provID, func(t *testing.T) {
			a := contract.CanonicalAgent{
				Name: "agent-x",
				ProviderOverrides: map[string]map[string]any{
					provID: {"__nonexistent_field_zzz__": "value"},
				},
			}
			findings := g.providerOverrideSchemaFindings(a)
			// Must produce at least one warning for the unknown field.
			foundWarning := false
			for _, f := range findings {
				if f.Severity == "warning" && strings.Contains(f.Message, "__nonexistent_field_zzz__") {
					foundWarning = true
				}
			}
			if !foundWarning {
				t.Errorf("provider %q: expected a warning for unknown field '__nonexistent_field_zzz__', got: %+v",
					provID, findings)
			}
			// Must never produce an error-severity finding from schema check.
			for _, f := range findings {
				if f.Severity == "error" {
					t.Errorf("schema check must never produce error findings, got: %+v", f)
				}
			}
		})
	}
}

// TestNameOverrideFindings_EmitsWarning verifies that nameOverrideFindings
// produces a warning when providerOverrides[p]["name"] is set.
func TestNameOverrideFindings_EmitsWarning(t *testing.T) {
	g := &gate{tr: transform.Default()}
	a := contract.CanonicalAgent{
		Name: "real-agent",
		ProviderOverrides: map[string]map[string]any{
			"claude-code": {"name": "hacker-name"},
			"codex":       {"name": "other-name"},
		},
	}
	findings := g.nameOverrideFindings(a)
	if len(findings) != 2 {
		t.Fatalf("expected 2 name-override warnings (one per provider), got %d: %+v", len(findings), findings)
	}
	for _, f := range findings {
		if f.Severity != "warning" {
			t.Errorf("name-override finding severity=%q, want warning", f.Severity)
		}
		if !strings.Contains(f.Message, "name") {
			t.Errorf("finding message %q should mention 'name'", f.Message)
		}
		if !strings.Contains(f.Message, "identity") {
			t.Errorf("finding message %q should explain identity protection", f.Message)
		}
	}
	// Must not block the gate.
	if blocking := errorFindings(findings); len(blocking) != 0 {
		t.Fatalf("name-override warning must not block the gate: %+v", blocking)
	}
}

// TestNameOverrideFindings_NoWarning_WhenNameAbsent verifies that no findings
// are emitted when providerOverrides does not contain "name" keys.
func TestNameOverrideFindings_NoWarning_WhenNameAbsent(t *testing.T) {
	g := &gate{tr: transform.Default()}
	a := contract.CanonicalAgent{
		Name: "real-agent",
		ProviderOverrides: map[string]map[string]any{
			"claude-code": {"model": "sonnet", "color": "blue"},
		},
	}
	findings := g.nameOverrideFindings(a)
	if len(findings) != 0 {
		t.Fatalf("expected no name-override findings when 'name' key absent, got: %+v", findings)
	}
}

// TestProviderOverrideSchema_NoFindings_EmptyOverrides verifies no schema
// findings when ProviderOverrides is nil or empty.
func TestProviderOverrideSchema_NoFindings_EmptyOverrides(t *testing.T) {
	g := &gate{tr: transform.Default()}
	a := contract.CanonicalAgent{Name: "x"}
	if f := g.providerOverrideSchemaFindings(a); len(f) != 0 {
		t.Fatalf("nil ProviderOverrides should produce no schema findings, got: %+v", f)
	}
	a.ProviderOverrides = map[string]map[string]any{"claude-code": {}}
	if f := g.providerOverrideSchemaFindings(a); len(f) != 0 {
		t.Fatalf("empty override map should produce no schema findings, got: %+v", f)
	}
}

// TestProviderOverrideSchema_Isolation verifies that an unknown-field warning
// for provider A does not leak into provider B's findings.
func TestProviderOverrideSchema_Isolation(t *testing.T) {
	g := &gate{tr: transform.Default()}
	a := contract.CanonicalAgent{
		Name: "agent-x",
		ProviderOverrides: map[string]map[string]any{
			"claude-code": {"__unknown_for_claude__": "val"},
			"codex":       {"description": "valid codex field"},
		},
	}
	findings := g.providerOverrideSchemaFindings(a)
	// Should have exactly 1 finding (for claude-code's unknown field).
	// codex's "description" is a known field so no finding for it.
	if len(findings) != 1 {
		t.Fatalf("expected 1 schema warning (for claude-code unknown field), got %d: %+v", len(findings), findings)
	}
	f := findings[0]
	if f.Provider != "claude-code" {
		t.Errorf("finding should be for claude-code, got provider=%q", f.Provider)
	}
	if !strings.Contains(f.Message, "__unknown_for_claude__") {
		t.Errorf("finding message should mention the unknown field, got: %q", f.Message)
	}
}

// TestValidateAgents_SchemaWarning_DoesNotBlock verifies the full
// validateAgents path: a schema warning for an unknown providerOverrides field
// flows through but does NOT block the pre-sync gate.
func TestValidateAgents_SchemaWarning_DoesNotBlock(t *testing.T) {
	root := t.TempDir()
	a := contract.CanonicalAgent{
		Name:        "agent-x",
		Description: "Reviews code changes for correctness.",
		Body:        "You are a reviewer.",
		ProviderOverrides: map[string]map[string]any{
			"claude-code": {"totally_unknown_field_xyz": "value"},
		},
	}
	writeCanonical(t, root, a)

	g := &gate{root: root, tr: transform.Default()}
	findings, err := g.validateAgents([]string{"agent-x"})
	if err != nil {
		t.Fatalf("validateAgents: %v", err)
	}

	// Must have at least one schema warning.
	hasSchemaWarning := false
	for _, f := range findings {
		if f.Severity == "warning" && strings.Contains(f.Message, "totally_unknown_field_xyz") {
			hasSchemaWarning = true
		}
	}
	if !hasSchemaWarning {
		t.Fatalf("expected a schema warning for unknown field, got findings: %+v", findings)
	}

	// Must NOT block the pre-sync gate.
	blocking := errorFindings(findings)
	for _, f := range blocking {
		if strings.Contains(f.Message, "totally_unknown_field_xyz") {
			t.Errorf("schema finding must be warning-only, not error: %+v", f)
		}
	}
}

// TestValidateAgents_NameOverride_DoesNotBlock verifies that a name-override
// warning is emitted but does not block the pre-sync gate.
func TestValidateAgents_NameOverride_DoesNotBlock(t *testing.T) {
	root := t.TempDir()
	a := contract.CanonicalAgent{
		Name:        "agent-x",
		Description: "Reviews code changes for correctness.",
		Body:        "You are a reviewer.",
		ProviderOverrides: map[string]map[string]any{
			"claude-code": {"name": "attempted-override"},
		},
	}
	writeCanonical(t, root, a)

	g := &gate{root: root, tr: transform.Default()}
	findings, err := g.validateAgents([]string{"agent-x"})
	if err != nil {
		t.Fatalf("validateAgents: %v", err)
	}

	// Must have at least one name-override warning.
	hasNameWarning := false
	for _, f := range findings {
		if f.Severity == "warning" && strings.Contains(f.Message, "name") && strings.Contains(f.Message, "identity") {
			hasNameWarning = true
		}
	}
	if !hasNameWarning {
		t.Fatalf("expected a name-override warning, got findings: %+v", findings)
	}

	// Must NOT block the pre-sync gate.
	blocking := errorFindings(findings)
	for _, f := range blocking {
		if strings.Contains(f.Message, "name") && strings.Contains(f.Message, "identity") {
			t.Errorf("name-override finding must be warning-only, not error: %+v", f)
		}
	}
}
