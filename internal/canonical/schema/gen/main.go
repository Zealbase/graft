//go:build ignore

// Command gen composes internal/canonical/schema/common-agent-definition.schema.json
// from the base schema fragment and per-provider catalog data.
//
// Run via: go generate ./internal/canonical/...
// Or directly: go run ./internal/canonical/schema/gen/main.go
//
// What it does:
//  1. Reads the base schema (base-fragment.json next to this file, or inline).
//  2. For each of the 9 registered provider ids, reads catalog/data/<p>/schema.json
//     and catalog/data/<p>/tools.json, builds $defs/po-<p> (with `name` removed
//     and a machine-validatable `tools.items = anyOf[enum(native), pattern]`).
//  3. Adds a `providerOverrides` property: closed object
//     (additionalProperties:false) keyed by the 9 registered ids → $ref.
//  4. Updates the canonical `tools.items` to anyOf[enum(canonical), pattern].
//  5. Sets $id to the public raw GitHub URL (B-D2).
//  6. Writes the result to common-agent-definition.schema.json (the file next to
//     the gen/ directory).
//
// Decision notes (frozen per plan):
//   - B-D1: additionalProperties:false on providerOverrides → unknown key is a
//     schema error; per-field leniency follows the provider's own schema.
//   - B-D2: $id = https://raw.githubusercontent.com/Shaik-Sirajuddin/graft/main/
//     internal/canonical/schema/common-agent-definition.schema.json
//   - C-D4: tool enums are anyOf[{enum:[…]}, {pattern:"wildcard/MCP/Agent()"}]
//     — NEVER strict-enum-only (so * / mcp_* / Agent(…) remain valid).
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

// wildcardPattern matches the valid tool wildcard / MCP / Agent() syntax that
// must always pass validation regardless of enum membership.
// Patterns allowed:
//   - *                 (all tools)
//   - mcp_*             (all MCP tools)
//   - mcp__<srv>__<t>   (specific MCP tool)
//   - Agent(…)          (spawn-restriction syntax)
const wildcardPattern = `^(\*|mcp_.*|mcp__.+__.+|Agent\(.*\))$`

// providerIDs is the ordered canonical set of registered provider ids.
// This is the closed key-set for providerOverrides (additionalProperties:false).
var providerIDs = []string{
	"claude-code",
	"codex",
	"gemini-cli",
	"cursor",
	"github-copilot",
	"opencode",
	"roo-code",
	"goose",
	"grok-cli",
	"antigravity",
}

// toolEntry is the shape of one entry in catalog/data/<p>/tools.json.
type toolEntry struct {
	Native    string `json:"native"`
	Canonical string `json:"canonical"`
}

// toolsFile is the shape of catalog/data/<p>/tools.json.
type toolsFile struct {
	Provider string      `json:"provider"`
	Tools    []toolEntry `json:"tools"`
}

// reMarkdownTableRow matches a markdown table row that starts with a pipe.
// We extract only from the FIRST cell (canonical name column).
var reMarkdownTableRow = regexp.MustCompile(`^\|[[:space:]]*` + "`" + `([a-z][a-z0-9_]*)` + "`" + `[[:space:]]*\|`)

// reMarkdownToolName extracts backtick-quoted tool names from canonical-tools.md lines
// (kept for reference; actual parsing uses reMarkdownTableRow).
var reMarkdownToolName = regexp.MustCompile("`([a-z][a-z0-9_]*)`")

func main() {
	// Locate the repo root. We try several strategies in order:
	//  1. GRAFT_REPO_ROOT env var (explicit override, always wins).
	//  2. runtime.Caller — works when source is available (go generate / go run
	//     with source tree).
	//  3. Walk up from cwd looking for go.mod (fallback).
	repoRoot, err := findRepoRoot()
	if err != nil {
		log.Fatalf("gen: cannot locate repo root: %v", err)
	}

	catalogDir := filepath.Join(repoRoot, "internal", "catalog", "data")
	schemaDir := filepath.Join(repoRoot, "internal", "canonical", "schema")
	outFile := filepath.Join(schemaDir, "common-agent-definition.schema.json")

	// 1. Load canonical tool names from canonical-tools.md
	canonicalTools, err := loadCanonicalTools(filepath.Join(catalogDir, "canonical-tools.md"))
	if err != nil {
		log.Fatalf("gen: load canonical-tools.md: %v", err)
	}

	// 2. Load per-provider native tool names from tools.json.
	nativeTools := make(map[string][]string, len(providerIDs))
	for _, p := range providerIDs {
		toolsPath := filepath.Join(catalogDir, p, "tools.json")
		tools, err := loadNativeTools(toolsPath)
		if err != nil {
			// Some providers may not have a tools.json yet (antigravity). Log and skip.
			fmt.Fprintf(os.Stderr, "gen: warn: no tools.json for %s (%v); tool enum will be empty\n", p, err)
			tools = nil
		}
		nativeTools[p] = tools
	}

	// 3. Load per-provider catalog schema.json and strip `name` field.
	providerDefs := make(map[string]map[string]any, len(providerIDs))
	for _, p := range providerIDs {
		schemaPath := filepath.Join(catalogDir, p, "schema.json")
		def, err := loadProviderDef(schemaPath, p, nativeTools[p])
		if err != nil {
			log.Fatalf("gen: load schema for %s: %v", p, err)
		}
		providerDefs[p] = def
	}

	// 4. Load the base schema.
	baseSchema, err := loadBaseSchema(outFile)
	if err != nil {
		log.Fatalf("gen: load base schema: %v", err)
	}

	// 5. Compose the output schema.
	composed := composeSchema(baseSchema, providerDefs, canonicalTools)

	// 6. Write.
	out, err := json.MarshalIndent(composed, "", "  ")
	if err != nil {
		log.Fatalf("gen: marshal: %v", err)
	}
	out = append(out, '\n')
	if err := os.WriteFile(outFile, out, 0o644); err != nil {
		log.Fatalf("gen: write %s: %v", outFile, err)
	}
	fmt.Printf("gen: wrote %s (%d bytes)\n", outFile, len(out))
}

// descriptiveTerms is a set of backtick-quoted terms that appear in
// canonical-tools.md but are descriptive prose, not actual tool names.
var descriptiveTerms = map[string]bool{
	"lowercase_snake_case": true, // appears in the header "Canonical names are `lowercase_snake_case`"
}

// loadCanonicalTools parses canonical-tools.md and returns a sorted, deduplicated
// list of canonical tool names.
//
// Parsing strategy: canonical-tools.md uses a markdown table format where the
// FIRST column is the canonical name and subsequent columns are provider native
// names. We extract only from the first cell of each table row to avoid
// picking up native names (which are listed in the other columns).
//
// Table rows have the format:
//   | `canonical_name` | provider→`native` ... |
//
// We also skip the separator row (| --- | --- |) and header rows.
func loadCanonicalTools(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	for _, line := range strings.Split(string(b), "\n") {
		m := reMarkdownTableRow.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := m[1]
		if descriptiveTerms[name] {
			continue
		}
		seen[name] = true
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, nil
}

// loadNativeTools reads a provider's tools.json and returns the sorted list of
// native tool names.
func loadNativeTools(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var tf toolsFile
	if err := json.Unmarshal(b, &tf); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	names := make([]string, 0, len(tf.Tools))
	for _, t := range tf.Tools {
		names = append(names, t.Native)
	}
	sort.Strings(names)
	return names, nil
}

// loadProviderDef reads catalog/data/<p>/schema.json as a raw JSON object,
// removes the `name` field from the frontmatter section (name is not
// overridable), and injects a machine-validatable `tools` field with an
// anyOf[enum(native), pattern] constraint.
//
// The catalog schema format is a graft-derived descriptive format (not standard
// JSON Schema), so we treat it as an opaque object and only touch two things:
//  1. Remove `frontmatter.name` if present.
//  2. Inject/replace `frontmatter.tools` with the machine-validatable shape.
//
// The result is embedded as a $defs entry in the composed canonical schema.
// The shape is kept close to the catalog original (descriptive prose is fine
// in $defs; we only need it to be valid JSON, not full JSON Schema).
func loadProviderDef(path, providerID string, nativeToolNames []string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	// Remove top-level $schema and $id (will conflict with the composed schema).
	delete(doc, "$schema")
	delete(doc, "$id")

	// Remove `name` from the frontmatter section so it cannot be overridden.
	if fm, ok := doc["frontmatter"]; ok {
		if fmMap, ok := fm.(map[string]any); ok {
			delete(fmMap, "name")
			// Inject machine-validatable tools constraint into frontmatter.
			fmMap["tools"] = makeToolsSchema(nativeToolNames)
			doc["frontmatter"] = fmMap
		}
	}

	// Add a clear description of the machine-validatable tools constraint.
	// This is the $defs entry shape — it will be referenced by providerOverrides.
	// Wrap the entire catalog entry as a JSON Schema-compatible object:
	// we embed it as {"type":"object","description":"…","properties":…}
	// so validators see a real schema, not just opaque data.
	return buildProviderOverrideDef(doc, providerID, nativeToolNames), nil
}

// buildProviderOverrideDef converts a raw catalog schema doc into a JSON Schema
// $defs entry suitable for use in providerOverrides. The catalog format is
// descriptive prose, not standard JSON Schema, so we build a thin JSON Schema
// wrapper that:
//   - Declares `type: object`
//   - Lists the known frontmatter fields as `properties` with string type
//   - Forbids `name` in `properties` (not in the properties map)
//   - Adds the tools field with the machine-validatable anyOf shape
//   - Uses `additionalProperties: true` so unknown fields are warnings, not errors
//     (matching the "lenient fields" decision B-D1)
func buildProviderOverrideDef(catalogDoc map[string]any, providerID string, nativeToolNames []string) map[string]any {
	def := map[string]any{
		"type":                 "object",
		"additionalProperties": true, // lenient per B-D1: unknown field → warning only
		"description":          fmt.Sprintf("Per-provider overrides for %s. Fields correspond to native provider frontmatter (excluding `name` which is not overridable).", providerID),
	}

	// Extract frontmatter fields from catalog doc for `properties`.
	props := map[string]any{}
	if fm, ok := catalogDoc["frontmatter"]; ok {
		if fmMap, ok := fm.(map[string]any); ok {
			for k, v := range fmMap {
				if k == "name" {
					continue // explicitly forbidden
				}
				if k == "tools" {
					// Use the machine-validatable tools schema.
					props["tools"] = makeToolsSchema(nativeToolNames)
					continue
				}
				// For other fields, extract the type annotation if available.
				props[k] = fieldToSchema(v)
			}
		}
	}

	if len(props) > 0 {
		def["properties"] = props
	}

	return def
}

// makeToolsSchema returns the anyOf[enum(names), pattern(wildcards)] schema
// for a tools field. Accepts both array and object (bool-map) forms.
func makeToolsSchema(nativeToolNames []string) map[string]any {
	itemSchema := makeToolItemSchema(nativeToolNames)
	return map[string]any{
		"description": "Tool access control override. Use native provider tool names. Wildcards (*), MCP patterns (mcp_*), and Agent() spawn syntax are always accepted.",
		"oneOf": []any{
			map[string]any{
				"type":        "array",
				"description": "Allowlist of native tool names or wildcard patterns.",
				"items":       itemSchema,
			},
			map[string]any{
				"type":                 "object",
				"description":          "Map of tool-name → enabled (provider-specific bool-map form, e.g. opencode).",
				"additionalProperties": map[string]any{"type": "boolean"},
			},
		},
	}
}

// makeToolItemSchema builds the anyOf[{enum:[…]}, {pattern:…}] for array items.
func makeToolItemSchema(names []string) map[string]any {
	branches := []any{
		map[string]any{"pattern": wildcardPattern},
	}
	if len(names) > 0 {
		enumVals := make([]any, len(names))
		for i, n := range names {
			enumVals[i] = n
		}
		// Enum branch first (more specific), then wildcard pattern.
		branches = append([]any{map[string]any{"enum": enumVals}}, branches...)
	}
	return map[string]any{"anyOf": branches}
}

// fieldToSchema converts a catalog frontmatter field descriptor (which is a
// raw JSON value, typically an object with "type", "required", "description"
// sub-keys) into a minimal JSON Schema fragment.
//
// Priority order:
//  1. If the field carries a "jsonSchema" key (machine-readable, added as part
//     of D-final), use that directly — it is already valid JSON Schema.
//  2. Fall back to the prose "type" heuristic for fields that don't yet carry
//     a jsonSchema annotation (permissive {} when type is unrecognised prose).
func fieldToSchema(v any) map[string]any {
	m, ok := v.(map[string]any)
	if !ok {
		return map[string]any{} // permissive
	}

	// D-final path: prefer the explicit machine-readable jsonSchema sub-object.
	if js, ok := m["jsonSchema"].(map[string]any); ok && len(js) > 0 {
		// Optionally merge in the prose description for editor UX.
		result := deepCopyMap(js)
		if _, hasDesc := result["description"]; !hasDesc {
			if desc, ok := m["description"].(string); ok && desc != "" {
				result["description"] = desc
			}
		}
		return result
	}

	// Legacy/fallback path: parse prose "type" annotation.
	// Catalog format: {"type":"string","required":false,"description":"…",…}
	// We emit a simple JSON Schema: {"type":"string","description":"…"}
	result := map[string]any{}
	if desc, ok := m["description"].(string); ok && desc != "" {
		result["description"] = desc
	}
	if typeStr, ok := m["type"].(string); ok {
		// Catalog uses prose types like "YAML list (string[])", "object (map…)",
		// "boolean", "string", "number". Map known ones; others fall to permissive.
		switch {
		case typeStr == "string":
			result["type"] = "string"
		case typeStr == "boolean":
			result["type"] = "boolean"
		case typeStr == "number":
			result["type"] = "number"
		case strings.HasPrefix(typeStr, "string (number"):
			result["type"] = "string" // e.g. "string (number also accepted in SDK)"
		// For complex prose types, emit permissive so we don't false-positive.
		default:
			// leave result without "type" (permissive)
		}
	}
	// Always add allowed values if present.
	if allowed, ok := m["allowed"].(string); ok && allowed != "" {
		if _, hasDesc := result["description"]; !hasDesc {
			result["description"] = "Allowed: " + allowed
		}
	}
	return result
}

// loadBaseSchema reads the current common-agent-definition.schema.json as the
// base to modify. If the file doesn't exist, returns a minimal stub.
func loadBaseSchema(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{
				"$schema":              "https://json-schema.org/draft/2020-12/schema",
				"type":                 "object",
				"required":             []any{"name", "description", "systemPrompt"},
				"additionalProperties": false,
				"properties":           map[string]any{},
				"$defs":                map[string]any{},
			}, nil
		}
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("parse base schema: %w", err)
	}
	return doc, nil
}

// composeSchema takes the base schema and produces the composed output with:
//   - $id set to the public raw GitHub URL
//   - providerOverrides property added (closed set, additionalProperties:false)
//   - $defs populated with po-<id> entries for each provider
//   - canonical tools.items updated to anyOf[enum(canonical), pattern]
//   - x-provider property REMOVED (replaced by providerOverrides)
func composeSchema(base map[string]any, defs map[string]map[string]any, canonicalToolNames []string) map[string]any {
	// Work on a deep copy to avoid mutating the input.
	out := deepCopyMap(base)

	// B-D2: set the public $id.
	out["$id"] = "https://raw.githubusercontent.com/Shaik-Sirajuddin/graft/main/internal/canonical/schema/common-agent-definition.schema.json"

	// Build providerOverrides property.
	poProps := map[string]any{}
	for _, p := range providerIDs {
		poProps[p] = map[string]any{"$ref": "#/$defs/po-" + p}
	}
	providerOverrides := map[string]any{
		"type":                 "object",
		"description":          "Per-provider field overrides. Keys are the registered provider ids. Unknown provider ids are rejected (additionalProperties: false). The `name` field is never overridable — set it only at the top level.",
		"additionalProperties": false,
		"properties":           poProps,
	}

	// Add providerOverrides to properties; remove x-provider escape hatch.
	props, _ := out["properties"].(map[string]any)
	if props == nil {
		props = map[string]any{}
	}
	props["providerOverrides"] = providerOverrides
	delete(props, "x-provider") // replaced by the real typed property
	out["properties"] = props

	// Update additionalProperties at root: still false (we explicitly declare all).
	out["additionalProperties"] = false

	// Update canonical tools.items to anyOf[enum(canonical), pattern].
	if toolsProp, ok := props["tools"].(map[string]any); ok {
		updateCanonicalToolsItems(toolsProp, canonicalToolNames)
	}

	// Build $defs: existing defs (mcpServer etc.) + po-<id> entries.
	existingDefs, _ := out["$defs"].(map[string]any)
	if existingDefs == nil {
		existingDefs = map[string]any{}
	}
	for _, p := range providerIDs {
		existingDefs["po-"+p] = defs[p]
	}
	out["$defs"] = existingDefs

	return out
}

// updateCanonicalToolsItems modifies the tools property in-place to add the
// anyOf[enum(canonical), pattern] constraint on the array items branch.
func updateCanonicalToolsItems(toolsProp map[string]any, canonicalNames []string) {
	itemSchema := makeToolItemSchema(canonicalNames)

	// The tools property uses oneOf with two branches: array and object.
	// We update the array branch's items.
	if oneOf, ok := toolsProp["oneOf"].([]any); ok {
		for i, branch := range oneOf {
			bMap, ok := branch.(map[string]any)
			if !ok {
				continue
			}
			if bMap["type"] == "array" {
				bMap["items"] = itemSchema
				oneOf[i] = bMap
			}
		}
		toolsProp["oneOf"] = oneOf
	}
}

// deepCopyMap performs a deep copy of a map[string]any via JSON round-trip.
func deepCopyMap(m map[string]any) map[string]any {
	b, _ := json.Marshal(m)
	var out map[string]any
	json.Unmarshal(b, &out)
	return out
}

// findRepoRoot locates the repository root directory.
func findRepoRoot() (string, error) {
	// 1. Explicit env override.
	if r := os.Getenv("GRAFT_REPO_ROOT"); r != "" {
		abs, err := filepath.Abs(r)
		if err != nil {
			return "", fmt.Errorf("GRAFT_REPO_ROOT: %w", err)
		}
		return abs, nil
	}

	// 2. runtime.Caller — source-file path, works with go generate / go run.
	_, thisFile, _, ok := runtime.Caller(0)
	if ok && thisFile != "" && !strings.Contains(thisFile, "<") {
		// thisFile is .../internal/canonical/schema/gen/main.go
		// repoRoot is five directories up.
		candidate := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..", "..")
		candidate, err := filepath.Abs(candidate)
		if err == nil {
			if _, serr := os.Stat(filepath.Join(candidate, "go.mod")); serr == nil {
				return candidate, nil
			}
		}
	}

	// 3. Walk up from cwd.
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getwd: %w", err)
	}
	dir := cwd
	for {
		if _, serr := os.Stat(filepath.Join(dir, "go.mod")); serr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("go.mod not found walking up from %s", cwd)
}
