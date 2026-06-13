package gateway

// Host-isolated unit tests for the pure schema-field validation core
// (schemaFieldFindings + extractSchemaFields). These need NO catalog, registry,
// or filesystem — they exercise the exact false-positive bug fixed in
// v0.0.4 conformance r1 (HIGH 1) plus the null-value warning (LOW).

import (
	"strings"
	"testing"
)

// TestSchemaFieldFindings_NilValidFields_NoFalsePositive is the HIGH 1 regression:
// a provider whose schema lacks a frontmatter section yields a NIL validFields
// set. A valid override field must then produce NO unknown-field warning (the old
// code flagged EVERY field because nil-map lookups are always false).
func TestSchemaFieldFindings_NilValidFields_NoFalsePositive(t *testing.T) {
	ovr := map[string]any{
		"description": "a real description",
		"model":       "some-model",
		"temperature": 0.7,
	}
	// validFields == nil simulates a schema with no "frontmatter" section.
	findings := schemaFieldFindings("agent-x", "no-frontmatter-provider", ovr, nil)
	if len(findings) != 0 {
		t.Fatalf("nil validFields must skip unknown-field check (no false positives), got: %+v", findings)
	}
}

// TestExtractSchemaFields_NoFrontmatter_ReturnsNil verifies the parse layer
// returns a nil set (not an empty set) for a schema without a frontmatter
// section — the precondition the HIGH 1 caller-guard relies on.
func TestExtractSchemaFields_NoFrontmatter_ReturnsNil(t *testing.T) {
	schema := []byte(`{"someOtherSection": {"foo": "bar"}}`)
	fields, err := extractSchemaFields(schema)
	if err != nil {
		t.Fatalf("extractSchemaFields: %v", err)
	}
	if fields != nil {
		t.Fatalf("schema without frontmatter must return nil set, got: %+v", fields)
	}
}

// TestSchemaFieldFindings_NonNilSet_FlagsUnknown verifies the normal path still
// works: with an authoritative (non-nil) field set, an unknown field is flagged
// as a warning and a known field is not.
func TestSchemaFieldFindings_NonNilSet_FlagsUnknown(t *testing.T) {
	valid := map[string]bool{"description": true, "model": true}
	ovr := map[string]any{
		"model":           "x", // known -> no finding
		"bogus_field_zzz": "y", // unknown -> warning
	}
	findings := schemaFieldFindings("agent-x", "p", ovr, valid)
	if len(findings) != 1 {
		t.Fatalf("expected exactly 1 unknown-field warning, got %d: %+v", len(findings), findings)
	}
	f := findings[0]
	if f.Severity != "warning" {
		t.Errorf("severity=%q, want warning", f.Severity)
	}
	if !strings.Contains(f.Message, "bogus_field_zzz") {
		t.Errorf("message %q should mention the unknown field", f.Message)
	}
}

// TestSchemaFieldFindings_EmptyNonNilSet_FlagsAll verifies a non-nil but EMPTY
// set is the distinct "everything is unknown" case (a schema declaring an empty
// frontmatter), so a field IS flagged — proving the nil/empty distinction.
func TestSchemaFieldFindings_EmptyNonNilSet_FlagsAll(t *testing.T) {
	empty := map[string]bool{}
	ovr := map[string]any{"description": "x"}
	findings := schemaFieldFindings("agent-x", "p", ovr, empty)
	if len(findings) != 1 {
		t.Fatalf("non-nil empty set must flag the field as unknown, got: %+v", findings)
	}
}

// TestSchemaFieldFindings_NullValue_Warns is the LOW regression: an explicit
// null override value warns (it is silently ignored at serialize time), and the
// warning fires even when validFields is nil (the null check precedes the
// schema check).
func TestSchemaFieldFindings_NullValue_Warns(t *testing.T) {
	ovr := map[string]any{"description": nil}
	findings := schemaFieldFindings("agent-x", "p", ovr, nil)
	if len(findings) != 1 {
		t.Fatalf("null override value must produce exactly 1 warning, got: %+v", findings)
	}
	f := findings[0]
	if f.Severity != "warning" {
		t.Errorf("severity=%q, want warning", f.Severity)
	}
	if !strings.Contains(f.Message, "null") || !strings.Contains(f.Message, "description") {
		t.Errorf("message %q should explain the null override for 'description'", f.Message)
	}
}

// TestSchemaFieldFindings_NameSkipped verifies "name" is never schema-flagged
// here (it is handled by nameOverrideFindings).
func TestSchemaFieldFindings_NameSkipped(t *testing.T) {
	valid := map[string]bool{"description": true}
	ovr := map[string]any{"name": "x"}
	if f := schemaFieldFindings("agent-x", "p", ovr, valid); len(f) != 0 {
		t.Fatalf("'name' must not be schema-flagged, got: %+v", f)
	}
}
