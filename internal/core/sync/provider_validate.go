package sync

// Runtime provider-schema validation gate (companion to validateCanonicalStore).
//
// WHY this gate exists: validateCanonicalStore re-validates the committed
// CANONICAL store after a sync, but the EMITTED provider files (the .claude /
// .codex / ... files applyProviders writes via FromCanonical) are only validated
// against each provider's own schema in the e2e suite — never during a real
// `graft sync`. A FromCanonical bug, a lossy override round-trip, or a missing
// required frontmatter field could therefore ship a provider file that violates
// the provider's own schema and go silently undetected. This gate closes that
// gap by re-parsing the just-written provider files and validating them against a
// schema COMPOSED from each provider's descriptive Schema() doc.
//
// Like the canonical gate, this is "report, don't undo": by the time it runs the
// merge is committed and the run is done. A violation surfaces as a
// *ProviderSchemaValidationError (reported loudly) but never rolls back.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Shaik-Sirajuddin/graft/internal/canonical"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/message"
)

// provMsgPrinter localizes jsonschema ErrorKind strings (LocalizedString panics
// on a nil printer), mirroring canonical/validate.go.
var provMsgPrinter = message.NewPrinter(message.MatchLanguage("en"))

// ProviderSchemaValidationError is returned by the sync engine when the runtime
// provider-schema gate finds error-severity findings in the just-emitted
// provider files. It mirrors PostSyncValidationError: it does NOT mean the sync
// was blocked or rolled back — the merge is committed and the run is done. It
// exists purely to report loudly that an emitted provider file violates that
// provider's own schema.
type ProviderSchemaValidationError struct {
	Findings []contract.Finding
}

// Error renders the carried findings as a single line, tagging each with the
// "<agent>/<provider>" location.
func (e *ProviderSchemaValidationError) Error() string {
	if len(e.Findings) == 0 {
		return "provider schema validation failed"
	}
	parts := make([]string, 0, len(e.Findings))
	for _, f := range e.Findings {
		loc := f.Agent
		if f.Provider != "" {
			loc += "/" + f.Provider
		}
		parts = append(parts, fmt.Sprintf("%s: %s", loc, f.Message))
	}
	out := "provider schema validation failed: "
	for i, p := range parts {
		if i > 0 {
			out += "; "
		}
		out += p
	}
	return out
}

// ProviderSchemaFindings returns the findings carried by err if it is a
// *ProviderSchemaValidationError, else nil.
func ProviderSchemaFindings(err error) []contract.Finding {
	var pse *ProviderSchemaValidationError
	if errors.As(err, &pse) {
		return pse.Findings
	}
	return nil
}

// cleanPrimitiveTypes is the set of JSON Schema primitive tokens we will enforce
// from a provider schema's prose `type` field — but ONLY when the trimmed,
// lowercased prose is EXACTLY one of these. Anything richer (e.g. "string
// (comma-separated) in files; string[] in CLI JSON / SDK") is NOT a clean token
// and yields an unconstrained property so prose types never cause false failures.
var cleanPrimitiveTypes = map[string]bool{
	"string":  true,
	"number":  true,
	"integer": true,
	"boolean": true,
	"object":  true,
	"array":   true,
}

// compileProviderSchema turns a provider's descriptive Schema() bytes into a
// *jsonschema.Schema usable to validate a parsed provider file's Fields map.
//
// The provider Schema() is a DESCRIPTIVE doc (not itself a JSON Schema with
// type:object/properties). We therefore COMPOSE an object schema from its
// top-level `frontmatter` map:
//
//	composed = {
//	  "$schema": "https://json-schema.org/draft/2020-12/schema",
//	  "type": "object",
//	  "properties": { <field>: <derived schema>, ... },
//	  "required":   [ <fields with required==true> ],
//	  "additionalProperties": true   // provider files carry override/private keys
//	}
//
// For each frontmatter field def:
//   - if it carries a clean `jsonSchema` object (future machine schema), use it
//     directly as the property schema;
//   - else, derive {"type": X} ONLY when the trimmed lowercased prose `type` is
//     EXACTLY one of the clean primitive tokens; otherwise emit {} (no
//     constraint) so prose types never cause false failures.
//
// Conservatism rationale: a clean sync MUST pass (prose `type` values are not
// valid JSON Schema and must not be enforced); the gate still catches missing
// required fields and (once schemas carry clean types / jsonSchema) wrong types.
//
// REQUIRED scoping (conservative — "a clean sync must pass; only enforce what
// graft actually emits"): a schema's `required` field NAMES are CONCEPTUAL —
// they need not be the literal flat frontmatter keys the provider's own Serialize
// emits and Parse reads back into pa.Fields. e.g. roo-code's schema requires
// [slug,name,roleDefinition,groups] but a clean emitted .roomodes parses to Fields
// with NO `name` / NO `groups` (Serialize never emits them); google-antigravity
// requires a NESTED dotted path that never appears as a flat key. Enforcing those
// produces FALSE FAILURES on clean syncs. So we INTERSECT the composed `required`
// list with `emittable` — the set of flat frontmatter keys a PRISTINE graft render
// of the SAME agent actually produces (computed by FromCanonical + the provider's
// own Parse of its primary write). Any required field NOT in `emittable` is
// DROPPED. A nil `emittable` drops ALL required fields — the LESS strict fallback
// so a clean sync never falsely fails. Note that `properties` are STILL composed
// for ALL frontmatter fields; only the `required` array is scoped. type /
// jsonSchema property constraints are UNCHANGED by this filter.
//
// Returns (nil, nil) — "nothing to validate, skip" — when there is no
// frontmatter section, the schema bytes are empty, or the composition yields
// zero properties AND zero required (after scoping). A non-nil error is a
// HARNESS failure (the composed schema would not compile / marshal).
func compileProviderSchema(provName string, schemaBytes []byte, emittable map[string]bool) (*jsonschema.Schema, error) {
	if len(schemaBytes) == 0 {
		return nil, nil
	}
	var doc map[string]any
	if err := json.Unmarshal(schemaBytes, &doc); err != nil {
		return nil, fmt.Errorf("provider schema %q: unmarshal: %w", provName, err)
	}
	fmAny, ok := doc["frontmatter"]
	if !ok {
		return nil, nil
	}
	fm, ok := fmAny.(map[string]any)
	if !ok {
		return nil, nil
	}

	properties := map[string]any{}
	var required []string
	for field, defAny := range fm {
		def, ok := defAny.(map[string]any)
		if !ok {
			continue
		}
		// Property schema: prefer a clean machine jsonSchema, else a conservative
		// type, else an empty (unconstrained) schema.
		if js, ok := def["jsonSchema"].(map[string]any); ok {
			properties[field] = js
		} else if t, ok := def["type"].(string); ok && cleanPrimitiveTypes[strings.ToLower(strings.TrimSpace(t))] {
			properties[field] = map[string]any{"type": strings.ToLower(strings.TrimSpace(t))}
		} else {
			properties[field] = map[string]any{}
		}
		if req, ok := def["required"].(bool); ok && req {
			// Conservative scoping: only enforce a required field a pristine graft
			// render of this same agent actually emits as a flat key. A nil
			// `emittable` drops ALL required (less-strict fallback).
			if emittable != nil && emittable[field] {
				required = append(required, field)
			}
		}
	}

	if len(properties) == 0 && len(required) == 0 {
		return nil, nil
	}

	composed := map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": true,
	}
	if len(required) > 0 {
		composed["required"] = required
	}

	composedBytes, err := json.Marshal(composed)
	if err != nil {
		return nil, fmt.Errorf("provider schema %q: marshal composed: %w", provName, err)
	}
	parsed, err := jsonschema.UnmarshalJSON(strings.NewReader(string(composedBytes)))
	if err != nil {
		return nil, fmt.Errorf("provider schema %q: unmarshal composed: %w", provName, err)
	}
	url := "graft://sync/provider-schema/" + provName
	c := jsonschema.NewCompiler()
	if err := c.AddResource(url, parsed); err != nil {
		return nil, fmt.Errorf("provider schema %q: add resource: %w", provName, err)
	}
	sch, err := c.Compile(url)
	if err != nil {
		return nil, fmt.Errorf("provider schema %q: compile: %w", provName, err)
	}
	return sch, nil
}

// validateProviderFields validates a parsed provider agent's Fields map against
// the compiled composed schema, returning error-severity findings (one per
// schema violation leaf), tagged with the agent + provider. A nil sch means
// "nothing to validate" and yields no findings.
func validateProviderFields(sch *jsonschema.Schema, agent, provName string, fields map[string]any) ([]contract.Finding, error) {
	if sch == nil {
		return nil, nil
	}
	// JSON round-trip the fields so the instance is plain JSON types (the
	// validator requires map[string]any / []any / float64, not arbitrary Go).
	raw, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("provider schema %q/%q: marshal fields: %w", agent, provName, err)
	}
	doc, err := jsonschema.UnmarshalJSON(strings.NewReader(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("provider schema %q/%q: unmarshal fields: %w", agent, provName, err)
	}
	verr := sch.Validate(doc)
	if verr == nil {
		return nil, nil
	}
	ve, ok := verr.(*jsonschema.ValidationError)
	if !ok {
		return []contract.Finding{{
			Severity: "error",
			Agent:    agent,
			Provider: provName,
			Message:  verr.Error(),
		}}, nil
	}
	var findings []contract.Finding
	collectProviderFindings(ve, agent, provName, &findings)
	if len(findings) == 0 {
		findings = append(findings, contract.Finding{
			Severity: "error",
			Agent:    agent,
			Provider: provName,
			Message:  ve.Error(),
		})
	}
	return findings, nil
}

// collectProviderFindings flattens the leaf causes of a jsonschema
// ValidationError into contract.Finding values, mirroring
// canonical.collectFindings but tagging each with the provider.
func collectProviderFindings(ve *jsonschema.ValidationError, agent, provName string, out *[]contract.Finding) {
	if len(ve.Causes) == 0 {
		loc := provInstanceLocation(ve)
		*out = append(*out, contract.Finding{
			Severity: "error",
			Agent:    agent,
			Provider: provName,
			Path:     loc,
			Message:  fmt.Sprintf("%s: %s", loc, ve.ErrorKind.LocalizedString(provMsgPrinter)),
		})
		return
	}
	for _, c := range ve.Causes {
		collectProviderFindings(c, agent, provName, out)
	}
}

func provInstanceLocation(ve *jsonschema.ValidationError) string {
	if len(ve.InstanceLocation) == 0 {
		return "(root)"
	}
	return "/" + strings.Join(ve.InstanceLocation, "/")
}

// validateEmittedProviders re-validates the just-emitted provider files for the
// named agents against each provider's composed schema. For each agent and each
// enabled provider it: compiles the provider's Schema() into a composed object
// schema (skipping providers with nothing to enforce), locates the emitted file
// via the provider's Detect under its scope base, parses it, and validates the
// parsed Fields.
//
// Missing provider files are NOT errors (skipped) — a provider that could not
// express this agent simply produced no file (matching applyProviders, which
// only links providers that wrote). A compile/parse/detect failure IS a harness
// error returned as a plain error (it marks the run aborted — a real failure).
// Content violations are collected and returned as a
// *ProviderSchemaValidationError (report, no rollback). Everything is terminal
// and bounded; there is no resume/re-sync loop.
//
// REQUIRED scoping: before compiling each (agent, provider) schema, we render a
// PRISTINE graft output for that same agent (pristineEmittableFields) and pass the
// resulting flat-key set to compileProviderSchema so the enforced `required` set is
// scoped to what graft actually emits — see compileProviderSchema for the why.

// pristineEmittableFields renders a clean graft output of the named agent for the
// named provider and returns (emittableKeys, providerCanExpress, error):
//   - It loads the canonical agent from the store and runs e.tr.FromCanonical (the
//     SAME path applyProviders uses) to get the provider FileWrites.
//   - If the provider produces ZERO writes it cannot express this agent: returns
//     (nil, false, nil) so the caller skips it (matching applyProviders, which
//     links nothing).
//   - Otherwise it writes the pristine writes to a temp dir and Parses the PRIMARY
//     (index 0) write back via the provider, collecting the keys of pa.Fields into
//     a set. Returns (set, true, nil).
//
// A load / render / write / parse failure is a HARNESS error (returned as the
// third value) — the caller surfaces it as a plain sync error, matching the rest
// of the gate's "harness failure aborts the run" contract.
func (e *Engine) pristineEmittableFields(name, provName string) (map[string]bool, bool, error) {
	prov, ok := e.tr.Provider(provName)
	if !ok {
		return nil, false, nil
	}
	can, err := canonical.Load(canonical.AgentDir(e.root, name))
	if err != nil {
		return nil, false, fmt.Errorf("sync: provider schema load canonical %s: %w", name, err)
	}
	writes, err := e.tr.FromCanonical(can, provName)
	if err != nil {
		return nil, false, fmt.Errorf("sync: provider schema fromcanonical %s/%s: %w", name, provName, err)
	}
	if len(writes) == 0 {
		return nil, false, nil // provider cannot express this agent
	}

	tmp, err := os.MkdirTemp("", "graft-provschema-")
	if err != nil {
		return nil, false, fmt.Errorf("sync: provider schema tempdir %s/%s: %w", name, provName, err)
	}
	defer os.RemoveAll(tmp)

	var primaryAbs string
	for i, w := range writes {
		abs := w.Path
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(tmp, w.Path)
		}
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return nil, false, fmt.Errorf("sync: provider schema mkdir %s/%s: %w", name, provName, err)
		}
		if err := os.WriteFile(abs, w.Data, 0o644); err != nil {
			return nil, false, fmt.Errorf("sync: provider schema write %s/%s: %w", name, provName, err)
		}
		if i == 0 {
			primaryAbs = abs
		}
	}

	pa, err := prov.Parse(primaryAbs)
	if err != nil {
		return nil, false, fmt.Errorf("sync: provider schema parse pristine %s/%s (%s): %w", name, provName, primaryAbs, err)
	}
	set := make(map[string]bool, len(pa.Fields))
	for k := range pa.Fields {
		set[k] = true
	}
	return set, true, nil
}

func (e *Engine) validateEmittedProviders(names []string) error {
	var errs []contract.Finding
	for _, name := range names {
		for _, provName := range e.tr.Providers() {
			if !e.providerEnabled(provName) {
				continue
			}
			prov, ok := e.tr.Provider(provName)
			if !ok {
				continue
			}
			schemaBytes := prov.Schema()
			if len(schemaBytes) == 0 {
				continue
			}
			// Scope the enforced `required` set to what a pristine graft render of
			// this agent actually emits. If the provider cannot express this agent
			// (zero pristine writes) skip it entirely — applyProviders linked
			// nothing, so there is no emitted file to validate.
			emittable, canExpress, err := e.pristineEmittableFields(name, provName)
			if err != nil {
				return err
			}
			if !canExpress {
				continue
			}
			sch, err := compileProviderSchema(provName, schemaBytes, emittable)
			if err != nil {
				return fmt.Errorf("sync: provider schema %s: %w", provName, err)
			}
			if sch == nil {
				continue // nothing enforceable for this provider
			}
			base, err := e.providerBase(provName)
			if err != nil {
				return err
			}
			refs, err := prov.Detect(base)
			if err != nil {
				return fmt.Errorf("sync: provider schema detect %s: %w", provName, err)
			}
			var ref *contract.AgentRef
			for i := range refs {
				if refs[i].Name == name {
					ref = &refs[i]
					break
				}
			}
			if ref == nil {
				continue // provider did not emit a file for this agent
			}
			pa, err := prov.Parse(ref.Path)
			if err != nil {
				return fmt.Errorf("sync: provider schema parse %s (%s): %w", provName, ref.Path, err)
			}
			fs, err := validateProviderFields(sch, name, provName, pa.Fields)
			if err != nil {
				return err
			}
			errs = append(errs, fs...)
		}
	}

	// Keep only error-severity findings; warnings never gate.
	var blocking []contract.Finding
	for _, f := range errs {
		if f.Severity == "error" {
			blocking = append(blocking, f)
		}
	}
	if len(blocking) > 0 {
		return &ProviderSchemaValidationError{Findings: blocking}
	}
	return nil
}
