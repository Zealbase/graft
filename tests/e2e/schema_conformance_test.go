package e2e

// TestPostSyncProviderSchemaConformance is the v-conf gate:
//
// After a successful `graft sync agents`, re-validate ALL provider agent files
// against their per-provider catalog schema (the per-field jsonSchema entries).
// Any field value that violates its declared jsonSchema is a conformance bug.
//
// Skipped providers (not in the active sync set):
//   - gemini-cli: deprecated and dewired (2026-06-15)
//   - antigravity: planned but unregistered (pending research spike)
//
// Validation path:
//  1. Load the catalog schema.json for the provider.
//  2. Extract frontmatter[field].jsonSchema for each declared field.
//  3. Build a composed JSON Schema {"type":"object","properties":{...}} from
//     the per-field jsonSchema entries (additional properties allowed — the
//     provider file may carry override keys not declared in the catalog schema).
//  4. Locate the written provider file via prov.Detect(root) → Parse(path).
//  5. Validate the parsed Fields map (JSON-encoded) against the composed schema.
//  6. Assert zero violations.

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/catalog"
	"github.com/Shaik-Sirajuddin/graft/internal/transform"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// TestPostSyncProviderSchemaConformance provisions one canonical agent, syncs
// to all active providers, then for each provider parses the written file and
// validates every field value against the per-field jsonSchema from the catalog
// schema.
func TestPostSyncProviderSchemaConformance(t *testing.T) {
	root := newGitWorkspace(t)
	provisionClaudeAgent(t, root, "code-reviewer")
	mustGraft(t, root, "init")

	var res runResultJSON
	decodeJSON(t, mustGraft(t, root, "sync", "agents", "-o", "json"), &res)
	if res.Status != "done" {
		t.Fatalf("sync status=%q, want done", res.Status)
	}
	if !containsStr(res.Changed, "code-reviewer") {
		t.Fatalf("changed=%v, want code-reviewer", res.Changed)
	}

	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}

	tr := transform.Default()
	activeProviders := tr.Providers()

	// Track overall conformance result for the test summary.
	type providerResult struct {
		name   string
		errs   []string
	}

	var results []providerResult

	for _, provName := range activeProviders {
		provName := provName
		t.Run(provName, func(t *testing.T) {
			prov, ok := tr.Provider(provName)
			if !ok {
				t.Fatalf("provider %s not in registry", provName)
			}

			// Locate the written file.
			refs, err := prov.Detect(root)
			if err != nil {
				t.Fatalf("%s.Detect: %v", provName, err)
			}
			var path string
			for _, ref := range refs {
				if ref.Name == "code-reviewer" {
					path = ref.Path
					break
				}
			}
			if path == "" {
				t.Fatalf("%s: no file written for agent code-reviewer after sync", provName)
			}

			// Parse the file with the provider's own parser.
			pa, err := prov.Parse(path)
			if err != nil {
				t.Fatalf("%s.Parse(%s): %v", provName, path, err)
			}

			// Build the composed object schema from the catalog.
			schema, err := buildComposedSchema(cat, provName)
			if err != nil {
				t.Fatalf("%s: build schema: %v", provName, err)
			}

			// Validate the parsed Fields map.
			errs := validateFields(t, provName, pa.Fields, schema)

			pr := providerResult{name: provName, errs: errs}
			results = append(results, pr)

			if len(errs) > 0 {
				for _, e := range errs {
					t.Errorf("%s conformance error: %s", provName, e)
				}
				t.Fatalf("%s: %d conformance error(s) in serialized file %s", provName, len(errs), path)
			}
		})
	}

	// Summary (only printed when -v).
	t.Logf("schema conformance summary: %d active providers checked", len(activeProviders))
	for _, r := range results {
		if len(r.errs) == 0 {
			t.Logf("  %s: PASS", r.name)
		} else {
			t.Logf("  %s: FAIL (%d errors)", r.name, len(r.errs))
		}
	}
}

// buildComposedSchema constructs a JSON Schema object from the per-field
// jsonSchema entries in the catalog schema.json for the given provider. The
// composed schema is:
//
//	{
//	  "type": "object",
//	  "properties": { fieldName: <per-field jsonSchema>, ... },
//	  "additionalProperties": true
//	}
//
// additionalProperties is true because provider files may carry override keys
// not declared in the catalog schema (e.g. providerOverride keys, private
// underscore keys from the transform layer). We only validate DECLARED fields.
func buildComposedSchema(cat *catalog.Catalog, providerName string) (*jsonschema.Schema, error) {
	schemaBytes, err := cat.Schema(providerName)
	if err != nil {
		return nil, fmt.Errorf("catalog schema: %w", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(schemaBytes, &raw); err != nil {
		return nil, fmt.Errorf("parse catalog schema: %w", err)
	}

	// Extract frontmatter field definitions.
	fm, _ := raw["frontmatter"].(map[string]any)
	if len(fm) == 0 {
		return nil, fmt.Errorf("catalog schema for %s has no frontmatter section", providerName)
	}

	properties := map[string]any{}
	for fieldName, fieldDef := range fm {
		fd, ok := fieldDef.(map[string]any)
		if !ok {
			continue
		}
		js, ok := fd["jsonSchema"]
		if !ok {
			continue
		}
		properties[fieldName] = js
	}

	composed := map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": true, // allow override keys not in catalog
	}

	composedBytes, err := json.Marshal(composed)
	if err != nil {
		return nil, fmt.Errorf("marshal composed schema: %w", err)
	}

	doc, err := jsonschema.UnmarshalJSON(strings.NewReader(string(composedBytes)))
	if err != nil {
		return nil, fmt.Errorf("unmarshal composed schema: %w", err)
	}

	url := fmt.Sprintf("tfs://e2e/v-conf/%s/composed.json", providerName)
	c := jsonschema.NewCompiler()
	if err := c.AddResource(url, doc); err != nil {
		return nil, fmt.Errorf("add schema resource: %w", err)
	}
	sch, err := c.Compile(url)
	if err != nil {
		return nil, fmt.Errorf("compile composed schema: %w", err)
	}
	return sch, nil
}

// validateFields validates the given Fields map against the composed schema.
// Returns a list of error messages (empty means conformant).
func validateFields(t *testing.T, provName string, fields map[string]any, sch *jsonschema.Schema) []string {
	t.Helper()
	if sch == nil {
		return nil
	}

	// JSON-encode the fields map so we get a stable, typed representation.
	b, err := json.Marshal(fields)
	if err != nil {
		return []string{fmt.Sprintf("marshal fields: %v", err)}
	}

	var doc any
	if err := json.Unmarshal(b, &doc); err != nil {
		return []string{fmt.Sprintf("re-unmarshal fields: %v", err)}
	}

	if err := sch.Validate(doc); err != nil {
		if ve, ok := err.(*jsonschema.ValidationError); ok {
			return collectValidationErrors(ve)
		}
		return []string{err.Error()}
	}
	return nil
}

// collectValidationErrors recursively flattens a jsonschema.ValidationError
// tree into a list of human-readable messages.
func collectValidationErrors(ve *jsonschema.ValidationError) []string {
	if ve == nil {
		return nil
	}
	var msgs []string
	if len(ve.Causes) == 0 {
		// Leaf error — format location path.
		loc := strings.Join(ve.InstanceLocation, "/")
		if loc == "" {
			loc = "<root>"
		}
		msgs = append(msgs, fmt.Sprintf("at /%s: %v", loc, ve))
	}
	for _, cause := range ve.Causes {
		msgs = append(msgs, collectValidationErrors(cause)...)
	}
	return msgs
}
