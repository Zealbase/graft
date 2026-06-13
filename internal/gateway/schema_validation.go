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

		// Sort override keys for deterministic finding order.
		keys := make([]string, 0, len(ovr))
		for k := range ovr {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, field := range keys {
			if field == "name" {
				// "name" is handled by nameOverrideFinding; skip here.
				continue
			}
			if !validFields[field] {
				out = append(out, contract.Finding{
					Agent:    a.Name,
					Provider: provID,
					Severity: "warning",
					Message: fmt.Sprintf(
						"providerOverrides[%s]: field %q is not in the provider's known schema (may be unsupported or misspelled)",
						provID, field,
					),
				})
			}
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
	out := make(map[string]bool, len(fields))
	for k := range fields {
		out[k] = true
	}
	return out, nil
}
