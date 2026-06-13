package gateway

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/Shaik-Sirajuddin/graft/internal/catalog"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// providerOverrideSchemaFindings validates each field in providerOverrides[p]
// against the catalog schema for provider p. It returns warning findings for:
//   - A field present in the override that is not in the provider's schema.
//   - A field whose value is nil (explicitly set to null — usually a mistake).
//
// Severity: always WARNING (catalog schemas are graft-derived and may be
// incomplete — we never hard-block on schema conformance). The existing
// providerOverrideKeyFindings produces error-severity for unknown provider ids
// and remains authoritative for that check.
//
// The "name" field is never flagged here: it is structurally protected at the
// Serialize layer (CanonicalAgent.FieldFor / RestoreOverrides), and users
// setting providerOverrides[p]["name"] will see a warning from nameOverrideFinding
// rather than a schema finding.
func (g *gate) providerOverrideSchemaFindings(a contract.CanonicalAgent) []contract.Finding {
	if len(a.ProviderOverrides) == 0 {
		return nil
	}
	cat, err := catalog.LoadOnce()
	if err != nil {
		// Catalog load failure: skip schema validation silently (offline-safe).
		return nil
	}

	var out []contract.Finding
	for provID, ovr := range a.ProviderOverrides {
		if len(ovr) == 0 {
			continue
		}
		schemaBytes, serr := cat.Schema(provID)
		if serr != nil {
			// Unknown provider at the schema level — the key-finding check
			// already emits an error for truly unknown providers; skip here.
			continue
		}

		// Parse the schema to extract the valid frontmatter field names.
		validFields, perr := extractSchemaFields(schemaBytes)
		if perr != nil {
			// Schema parse failure: skip this provider's schema check silently.
			continue
		}

		out = append(out, schemaFieldFindings(a.Name, provID, ovr, validFields)...)
	}
	return out
}

// schemaFieldFindings is the pure per-provider validation core (no catalog/IO):
// given an override bucket and the set of valid frontmatter fields parsed from a
// provider's schema, it returns warning findings. Split out so the validation
// logic is host-isolated testable.
//
// validFields semantics:
//   - nil  -> "no field list available" (schema lacks a frontmatter section);
//     the unknown-field check is SKIPPED so valid overrides do not produce
//     false positives (v0.0.4 conformance r1 HIGH 1).
//   - non-nil (possibly empty) -> an authoritative field allowlist; any field
//     not in the set is flagged.
func schemaFieldFindings(agent, provID string, ovr map[string]any, validFields map[string]bool) []contract.Finding {
	keys := make([]string, 0, len(ovr))
	for k := range ovr {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var out []contract.Finding
	for _, field := range keys {
		if field == "name" {
			// "name" is handled by nameOverrideFinding; skip here.
			continue
		}
		// LOW (v0.0.4 conformance r1): an explicit null value is almost always a
		// mistake (FieldFor treats a nil override as "unset" and silently falls
		// back to the canonical value). Warn regardless of whether the field is
		// schema-known.
		if ovr[field] == nil {
			out = append(out, contract.Finding{
				Agent:    agent,
				Provider: provID,
				Severity: "warning",
				Message: fmt.Sprintf(
					"providerOverrides[%s]: field %q is null; a null override is ignored and the canonical value is used",
					provID, field,
				),
			})
			continue
		}
		// HIGH (v0.0.4 conformance r1): a nil validFields set means the schema has
		// NO frontmatter section — treat every field as valid (skip the check)
		// rather than flagging all of them.
		if validFields == nil {
			continue
		}
		if !validFields[field] {
			out = append(out, contract.Finding{
				Agent:    agent,
				Provider: provID,
				Severity: "warning",
				Message: fmt.Sprintf(
					"providerOverrides[%s]: field %q is not in the provider's known schema (may be unsupported or misspelled)",
					provID, field,
				),
			})
		}
	}
	return out
}

// nameOverrideFindings emits a warning for each provider whose
// providerOverrides entry contains a "name" key. The "name" field is agent
// identity and is structurally ignored at serialize time; alerting the user
// avoids silent confusion.
func (g *gate) nameOverrideFindings(a contract.CanonicalAgent) []contract.Finding {
	if len(a.ProviderOverrides) == 0 {
		return nil
	}
	var out []contract.Finding
	// Sort providers for deterministic output.
	provs := make([]string, 0, len(a.ProviderOverrides))
	for p := range a.ProviderOverrides {
		provs = append(provs, p)
	}
	sort.Strings(provs)
	for _, provID := range provs {
		ovr := a.ProviderOverrides[provID]
		if _, nameSet := ovr["name"]; nameSet {
			out = append(out, contract.Finding{
				Agent:    a.Name,
				Provider: provID,
				Severity: "warning",
				Message: fmt.Sprintf(
					"providerOverrides[%s]: \"name\" is agent identity and is never overridable; this key is silently ignored during serialization",
					provID,
				),
			})
		}
	}
	return out
}

// extractSchemaFields parses a catalog schema JSON (graft-derived descriptive
// format, not a standard JSON Schema) and returns the set of valid frontmatter
// field names. The catalog schema format places field names as keys under the
// top-level "frontmatter" object.
func extractSchemaFields(schemaBytes []byte) (map[string]bool, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(schemaBytes, &doc); err != nil {
		return nil, fmt.Errorf("schema parse: %w", err)
	}
	fm, ok := doc["frontmatter"]
	if !ok {
		// Schema has no frontmatter section — treat all fields as valid.
		return nil, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(fm, &fields); err != nil {
		return nil, fmt.Errorf("schema frontmatter parse: %w", err)
	}
	if fields == nil {
		// "frontmatter": null unmarshals to a nil map — treat it identically to
		// "no frontmatter section" (return nil so the unknown-field check is
		// skipped) rather than an empty allowlist that flags every field.
		return nil, nil
	}
	out := make(map[string]bool, len(fields))
	for k := range fields {
		out[k] = true
	}
	return out, nil
}
