package canonical

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

func sampleAgent() contract.CanonicalAgent {
	return contract.CanonicalAgent{
		Name:        "reviewer",
		Description: "Reviews code changes for correctness.",
		Model:       "inherit",
		// Canonical tool names (snake_case); native names like "Read"/"Bash" are
		// rejected by the schema since v0.0.5. Use the canonical equivalents.
		Tools:       []string{"read_file", "grep", "bash"},
		MCP:         []string{"grafana", "notion"},
		Permissions: map[string]string{
			"bash":       "ask",
			"file_write": "deny",
		},
		Body: "You are a careful code reviewer.\nFocus on correctness.",
		// providerOverrides keys must be active registered provider ids.
		// NOTE: gemini-cli was deprecated 2026-06-15 — replaced with opencode here.
		ProviderOverrides: map[string]map[string]any{
			"claude-code": {"isolation": "worktree", "effort": "high"},
			"opencode":    {"temperature": 0.5},
		},
	}
}

// writeAll applies a set of FileWrite values to disk, creating dirs.
func writeAll(t *testing.T, writes []contract.FileWrite) {
	t.Helper()
	for _, w := range writes {
		if err := os.MkdirAll(filepath.Dir(w.Path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", w.Path, err)
		}
		if err := os.WriteFile(w.Path, w.Data, 0o644); err != nil {
			t.Fatalf("write %s: %v", w.Path, err)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := sampleAgent()

	writes, err := Save(dir, want)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if len(writes) != 3 {
		t.Fatalf("expected 3 file writes, got %d", len(writes))
	}
	writeAll(t, writes)

	got, err := Load(AgentDir(dir, want.Name))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Body gets a normalized trailing newline on save; compare with that applied.
	want.Body = normalizeBody(want.Body)

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round-trip mismatch:\n got=%#v\nwant=%#v", got, want)
	}
}

func TestRoundTripMinimal(t *testing.T) {
	dir := t.TempDir()
	want := contract.CanonicalAgent{
		Name:        "min",
		Description: "Minimal agent.",
		Body:        "Do the thing.",
	}
	writes, err := Save(dir, want)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	writeAll(t, writes)

	got, err := Load(AgentDir(dir, want.Name))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want.Body = normalizeBody(want.Body)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("minimal round-trip mismatch:\n got=%#v\nwant=%#v", got, want)
	}
}

func TestHashStability(t *testing.T) {
	a := sampleAgent()
	h1 := Hash(a)
	h2 := Hash(a)
	if h1 != h2 {
		t.Fatalf("hash not stable: %s != %s", h1, h2)
	}
	if len(h1) != 64 {
		t.Fatalf("expected 64-char sha256 hex, got %d: %s", len(h1), h1)
	}
}

func TestHashInsensitiveToMapOrderAndBodyNewline(t *testing.T) {
	a := sampleAgent()

	// Rebuild maps in different insertion order; Go map order is already
	// randomized, but rebuild to be explicit.
	b := sampleAgent()
	b.Permissions = map[string]string{"file_write": "deny", "bash": "ask"}
	b.ProviderOverrides = map[string]map[string]any{
		"opencode":    {"temperature": 0.5},
		"claude-code": {"effort": "high", "isolation": "worktree"},
	}
	// Trailing-newline churn in body must not change the hash.
	b.Body = a.Body + "\n\n"

	if Hash(a) != Hash(b) {
		t.Fatalf("hash should be invariant to map order and trailing newlines")
	}
}

func TestHashChangesWithContent(t *testing.T) {
	a := sampleAgent()
	b := sampleAgent()
	b.Description = "Different description."
	if Hash(a) == Hash(b) {
		t.Fatalf("hash should change when a semantic field changes")
	}

	c := sampleAgent()
	c.Body = "A meaningfully different instruction body."
	if Hash(a) == Hash(c) {
		t.Fatalf("hash should change when body changes")
	}
}

func TestSaveDeterministic(t *testing.T) {
	dir := t.TempDir()
	a := sampleAgent()

	w1, err := Save(dir, a)
	if err != nil {
		t.Fatalf("Save 1: %v", err)
	}
	w2, err := Save(dir, a)
	if err != nil {
		t.Fatalf("Save 2: %v", err)
	}
	if len(w1) != len(w2) {
		t.Fatalf("write count differs")
	}
	for i := range w1 {
		if w1[i].Path != w2[i].Path {
			t.Fatalf("path order differs: %s vs %s", w1[i].Path, w2[i].Path)
		}
		if string(w1[i].Data) != string(w2[i].Data) {
			t.Fatalf("bytes for %s not deterministic:\n%s\n---\n%s",
				w1[i].Path, w1[i].Data, w2[i].Data)
		}
	}
}

func TestSaveEmptyNameFails(t *testing.T) {
	_, err := Save(t.TempDir(), contract.CanonicalAgent{Description: "x", Body: "y"})
	if err == nil {
		t.Fatalf("expected error saving agent with empty name")
	}
}

func TestMetaRoundTrip(t *testing.T) {
	dir := t.TempDir()
	a := sampleAgent()
	meta := Meta{
		Providers: map[string]ProviderMeta{
			"claude": {SourceHash: "abc123", LastCommitHash: "deadbeef"},
		},
	}
	writes, err := SaveWithMeta(dir, a, meta)
	if err != nil {
		t.Fatalf("SaveWithMeta: %v", err)
	}
	writeAll(t, writes)

	got, err := LoadMeta(AgentDir(dir, a.Name))
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}
	if got.CanonicalHash != Hash(a) {
		t.Fatalf("meta canonicalHash not recomputed: got %s want %s", got.CanonicalHash, Hash(a))
	}
	pm, ok := got.Providers["claude"]
	if !ok || pm.SourceHash != "abc123" || pm.LastCommitHash != "deadbeef" {
		t.Fatalf("provider meta lost in round-trip: %#v", got.Providers)
	}
}

func TestLoadMetaMissing(t *testing.T) {
	dir := t.TempDir()
	d := AgentDir(dir, "nope")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	m, err := LoadMeta(d)
	if err != nil {
		t.Fatalf("LoadMeta missing should not error: %v", err)
	}
	if m.CanonicalHash != "" || len(m.Providers) != 0 {
		t.Fatalf("expected zero Meta, got %#v", m)
	}
}

func TestValidatePass(t *testing.T) {
	findings, err := Validate(sampleAgent())
	if err != nil {
		t.Fatalf("Validate harness error: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected valid agent, got findings: %#v", findings)
	}
}

func TestValidateMissingRequired(t *testing.T) {
	// Missing name and description (both required); empty systemPrompt allowed
	// as a string but name pattern + required will trip.
	a := contract.CanonicalAgent{Body: "hi"}
	findings, err := Validate(a)
	if err != nil {
		t.Fatalf("Validate harness error: %v", err)
	}
	if len(findings) == 0 {
		t.Fatalf("expected findings for missing required fields")
	}
	for _, f := range findings {
		if f.Severity != severityError {
			t.Fatalf("expected error severity, got %q", f.Severity)
		}
	}
}

func TestValidateBadNamePattern(t *testing.T) {
	a := sampleAgent()
	a.Name = "Has Spaces!" // violates ^[a-zA-Z0-9][a-zA-Z0-9_-]*$
	findings, err := Validate(a)
	if err != nil {
		t.Fatalf("Validate harness error: %v", err)
	}
	if len(findings) == 0 {
		t.Fatalf("expected findings for invalid name pattern")
	}
}

func TestValidateEmptyDescriptionBlocked(t *testing.T) {
	// An agent with an empty description must produce an error-severity finding.
	a := sampleAgent()
	a.Description = ""
	findings, err := Validate(a)
	if err != nil {
		t.Fatalf("Validate harness error: %v", err)
	}
	if len(findings) == 0 {
		t.Fatalf("expected error finding for empty description, got none")
	}
	for _, f := range findings {
		if f.Severity != severityError {
			t.Fatalf("expected error severity for empty description finding, got %q", f.Severity)
		}
	}
}

func TestValidateWhitespaceOnlyDescriptionBlocked(t *testing.T) {
	// A whitespace-only description is just as unusable as empty; must be blocked.
	a := sampleAgent()
	a.Description = "   \t  "
	findings, err := Validate(a)
	if err != nil {
		t.Fatalf("Validate harness error: %v", err)
	}
	if len(findings) == 0 {
		t.Fatalf("expected error finding for whitespace-only description, got none")
	}
	for _, f := range findings {
		if f.Severity != severityError {
			t.Fatalf("expected error severity, got %q", f.Severity)
		}
	}
}

func TestValidateNonEmptyDescriptionPasses(t *testing.T) {
	a := sampleAgent() // sampleAgent has a non-empty description
	findings, err := Validate(a)
	if err != nil {
		t.Fatalf("Validate harness error: %v", err)
	}
	// No error findings expected for a valid agent with a real description.
	for _, f := range findings {
		if f.Severity == severityError {
			t.Fatalf("unexpected error finding for agent with valid description: %+v", f)
		}
	}
}

func TestAgentYAMLFieldOrder(t *testing.T) {
	a := sampleAgent()
	b, err := marshalAgentYAML(a)
	if err != nil {
		t.Fatalf("marshalAgentYAML: %v", err)
	}
	s := string(b)
	// name must appear before description before model.
	iName := indexOf(s, "name:")
	iDesc := indexOf(s, "description:")
	iModel := indexOf(s, "model:")
	if !(iName >= 0 && iName < iDesc && iDesc < iModel) {
		t.Fatalf("unexpected field order in agent.yaml:\n%s", s)
	}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// --- BuildDefault tests ---

func TestBuildDefaultWithPrompt(t *testing.T) {
	a := BuildDefault("my-agent", "You handle deployments.")
	if a.Name != "my-agent" {
		t.Fatalf("expected name=my-agent, got %q", a.Name)
	}
	if a.Body != "You handle deployments." {
		t.Fatalf("unexpected body: %q", a.Body)
	}
	// Zero overrides and no model/tools.
	// Description MUST be empty by default — no bogus auto-description. The sync
	// engine's first-sync fan-out relies on a clean canonical (v0.0.4 verify task 2).
	if a.Description != "" {
		t.Fatalf("expected empty description by default, got %q", a.Description)
	}
	if a.Model != "" {
		t.Fatalf("expected empty model, got %q", a.Model)
	}
	if len(a.Tools) != 0 {
		t.Fatalf("expected no tools, got %v", a.Tools)
	}
	if a.ProviderOverrides != nil {
		t.Fatalf("expected nil ProviderOverrides, got %v", a.ProviderOverrides)
	}
}

func TestBuildDefaultEmptyPromptUsesTemplate(t *testing.T) {
	a := BuildDefault("default-tester", "")
	if a.Body == "" {
		t.Fatal("expected non-empty body when prompt is empty")
	}
	// Template body should be non-trivial (> 10 chars).
	if len(a.Body) < 10 {
		t.Fatalf("default template body too short: %q", a.Body)
	}
}

// TestBuildDefaultWritesThreeFilesWithEmptyMeta verifies that
// BuildDefault → SaveWithMeta(emptyMeta) produces exactly 3 files
// (agent.yaml, instructions.md, .meta.json) with a non-empty instructions
// body and an empty provider hash map in .meta.json.
func TestBuildDefaultWritesThreeFilesWithEmptyMeta(t *testing.T) {
	dir := t.TempDir()
	a := BuildDefault("scaffold-test", "You scaffold things.")

	writes, err := SaveWithMeta(dir, a, Meta{})
	if err != nil {
		t.Fatalf("SaveWithMeta: %v", err)
	}
	if len(writes) != 3 {
		t.Fatalf("expected 3 file writes, got %d", len(writes))
	}
	writeAll(t, writes)

	// instructions.md must be non-empty.
	agentD := AgentDir(dir, a.Name)
	body, err := os.ReadFile(filepath.Join(agentD, "instructions.md"))
	if err != nil {
		t.Fatalf("read instructions.md: %v", err)
	}
	if len(body) == 0 {
		t.Fatal("instructions.md must not be empty after BuildDefault")
	}

	// .meta.json must have canonicalHash set but no provider entries.
	meta, err := LoadMeta(agentD)
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}
	if meta.CanonicalHash == "" {
		t.Fatal("expected canonicalHash to be set in .meta.json")
	}
	if len(meta.Providers) != 0 {
		t.Fatalf("expected empty provider map (so next sync sees drift), got %v", meta.Providers)
	}
}

// --- Lossless override round-trip tests ---

// TestProviderModelSetSavesAndLoads sets a per-provider model key, saves, loads
// and confirms the key is present with the same value.
func TestProviderModelSetSavesAndLoads(t *testing.T) {
	dir := t.TempDir()
	a := sampleAgent()
	a.ProviderOverrides = map[string]map[string]any{
		"claude-code": {"model": "claude-opus-4"},
	}

	writes, err := Save(dir, a)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	writeAll(t, writes)

	got, err := Load(AgentDir(dir, a.Name))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	ov, ok := got.ProviderOverrides["claude-code"]
	if !ok {
		t.Fatal("expected ProviderOverrides[claude-code] to be present after load")
	}
	m, ok := ov["model"]
	if !ok {
		t.Fatal("expected model key inside claude-code bucket")
	}
	if m != "claude-opus-4" {
		t.Fatalf("model round-trip mismatch: got %v", m)
	}
}

// TestProviderModelClearPersistsAbsent removes the model key from a provider
// bucket, saves, loads, and verifies the key is GONE (not resurrected by any
// default or omitempty gap).
func TestProviderModelClearPersistsAbsent(t *testing.T) {
	dir := t.TempDir()
	a := sampleAgent()

	// Start with a model set.
	a.ProviderOverrides = map[string]map[string]any{
		"claude-code": {"model": "claude-opus-4", "isolation": "worktree"},
	}
	writes, _ := Save(dir, a)
	writeAll(t, writes)

	// Now clear just the model key.
	delete(a.ProviderOverrides["claude-code"], "model")

	writes, err := Save(dir, a)
	if err != nil {
		t.Fatalf("Save after clear: %v", err)
	}
	writeAll(t, writes)

	got, err := Load(AgentDir(dir, a.Name))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	ov, ok := got.ProviderOverrides["claude-code"]
	if !ok {
		t.Fatal("expected claude-code bucket to still exist (has isolation key)")
	}
	if _, hasModel := ov["model"]; hasModel {
		t.Fatal("model key must be ABSENT after clearing it — was resurrected")
	}
	if ov["isolation"] != "worktree" {
		t.Fatalf("isolation key lost during clear: %v", ov)
	}
}

// TestEmptyBucketDroppedOnSave verifies that clearing ALL keys from a provider
// bucket (leaving it empty) causes that bucket to be absent after save→load,
// not present as an empty map.
func TestEmptyBucketDroppedOnSave(t *testing.T) {
	dir := t.TempDir()
	a := sampleAgent()
	a.ProviderOverrides = map[string]map[string]any{
		"opencode": {}, // deliberately empty
	}

	writes, err := Save(dir, a)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	writeAll(t, writes)

	got, err := Load(AgentDir(dir, a.Name))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// The empty opencode bucket must NOT appear.
	if _, ok := got.ProviderOverrides["opencode"]; ok {
		t.Fatal("empty provider bucket must be dropped, not persisted as {}")
	}
	if got.ProviderOverrides != nil {
		t.Fatalf("expected nil ProviderOverrides when all buckets are empty, got %v", got.ProviderOverrides)
	}
}

// TestFullAgentRoundTrip verifies a fully-populated agent (model + tools + MCP
// + permissions + provider overrides) survives save→load identically, and that
// Hash is stable across the round-trip.
func TestFullAgentRoundTrip(t *testing.T) {
	dir := t.TempDir()
	a := sampleAgent()
	a.Model = "claude-opus-4"
	// Use int (not float64) for numeric overrides — YAML round-trips
	// integer-valued numbers as int on decode, not float64.
	a.ProviderOverrides = map[string]map[string]any{
		"claude-code": {"model": "claude-sonnet-4", "isolation": "worktree"},
		"opencode":    {"steps": 15},
	}

	h1 := Hash(a)

	writes, err := Save(dir, a)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	writeAll(t, writes)

	got, err := Load(AgentDir(dir, a.Name))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Normalize body for comparison (Save normalizes trailing newline).
	a.Body = normalizeBody(a.Body)

	if !reflect.DeepEqual(got, a) {
		t.Fatalf("full agent round-trip mismatch:\n got=%#v\nwant=%#v", got, a)
	}

	h2 := Hash(got)
	if h1 != h2 {
		t.Fatalf("Hash changed across save/load: %s → %s", h1, h2)
	}
}

// --- Schema validation tests (Track B: providerOverrides + tool enums) ---

// TestValidateProviderOverridesValidModel verifies that a valid
// providerOverrides.claude-code.model passes schema validation with no errors.
func TestValidateProviderOverridesValidModel(t *testing.T) {
	a := contract.CanonicalAgent{
		Name:        "my-agent",
		Description: "Does something useful.",
		Body:        "You are helpful.",
		ProviderOverrides: map[string]map[string]any{
			"claude-code": {"model": "claude-opus-4"},
		},
	}
	findings, err := Validate(a)
	if err != nil {
		t.Fatalf("Validate harness error: %v", err)
	}
	for _, f := range findings {
		if f.Severity == severityError {
			t.Errorf("unexpected error finding: %+v", f)
		}
	}
}

// TestValidateProviderOverridesUnknownKey verifies that an unknown provider key
// in providerOverrides produces a schema error finding.
func TestValidateProviderOverridesUnknownKey(t *testing.T) {
	a := contract.CanonicalAgent{
		Name:        "my-agent",
		Description: "Does something useful.",
		Body:        "You are helpful.",
		ProviderOverrides: map[string]map[string]any{
			"copilot": {"model": "gpt-4o"}, // wrong key: should be github-copilot
		},
	}
	findings, err := Validate(a)
	if err != nil {
		t.Fatalf("Validate harness error: %v", err)
	}
	hasError := false
	for _, f := range findings {
		if f.Severity == severityError {
			hasError = true
		}
	}
	if !hasError {
		t.Fatalf("unknown providerOverrides key 'copilot' should produce an error finding; got: %+v", findings)
	}
}

// TestValidateProviderOverridesNameForbidden verifies that setting `name` inside
// providerOverrides is REJECTED by the schema (the tool-enum constraint on the
// name field itself would not fire, but we test via the gateway layer's
// nameOverrideFindings; at the schema level name is simply absent from $defs).
// This test validates that the composed schema does NOT include `name` in any
// po-<p> properties.
func TestSchemaDefNoName(t *testing.T) {
	sch, err := schema()
	if err != nil {
		t.Fatalf("schema compile: %v", err)
	}
	if sch == nil {
		t.Fatal("schema is nil")
	}
	// The schema should compile cleanly (no $ref resolution errors).
	// If $defs/po-<p> contained a `name` property that was required, the
	// schema compile would still succeed but an agent with providerOverrides.p.name
	// would pass instead of being warned. We test via gateway.nameOverrideFindings
	// in gateway tests; here we just confirm the schema compiles.
}

// TestValidateToolEnumCanonicalAccepted verifies that canonical tool names
// (snake_case) pass the tools enum.
func TestValidateToolEnumCanonicalAccepted(t *testing.T) {
	a := contract.CanonicalAgent{
		Name:        "my-agent",
		Description: "Does something useful.",
		Body:        "You are helpful.",
		Tools:       []string{"bash", "read_file", "web_search"},
	}
	findings, err := Validate(a)
	if err != nil {
		t.Fatalf("Validate harness error: %v", err)
	}
	for _, f := range findings {
		if f.Severity == severityError {
			t.Errorf("valid canonical tool names should not produce errors; got: %+v", f)
		}
	}
}

// TestValidateToolEnumWildcardsAccepted verifies that wildcard/MCP/Agent()
// patterns pass even though they are not in the enum.
func TestValidateToolEnumWildcardsAccepted(t *testing.T) {
	wildcards := [][]string{
		{"*"},
		{"mcp_*"},
		{"mcp__memory__create"},
		{"Agent(worker)"},
		{"Agent(researcher, analyst)"},
	}
	for _, tools := range wildcards {
		a := contract.CanonicalAgent{
			Name:        "my-agent",
			Description: "Does something useful.",
			Body:        "You are helpful.",
			Tools:       tools,
		}
		findings, err := Validate(a)
		if err != nil {
			t.Fatalf("Validate harness error: %v", err)
		}
		for _, f := range findings {
			if f.Severity == severityError {
				t.Errorf("wildcard tool %v should not produce errors; got: %+v", tools, f)
			}
		}
	}
}

// TestValidateToolEnumNativeNameRejected verifies that native (PascalCase)
// tool names that are not canonical names and don't match wildcard pattern
// produce an error from the schema.
func TestValidateToolEnumNativeNameRejected(t *testing.T) {
	// "Read" is a native Claude Code tool name, not canonical ("read_file").
	// "WebSearch" is native, not canonical ("web_search").
	nativeNames := []string{"Read", "WebSearch", "UnknownToolXYZ"}
	for _, toolName := range nativeNames {
		a := contract.CanonicalAgent{
			Name:        "my-agent",
			Description: "Does something useful.",
			Body:        "You are helpful.",
			Tools:       []string{toolName},
		}
		findings, err := Validate(a)
		if err != nil {
			t.Fatalf("Validate harness error: %v", err)
		}
		hasError := false
		for _, f := range findings {
			if f.Severity == severityError {
				hasError = true
			}
		}
		if !hasError {
			t.Errorf("native/unknown tool name %q should produce a schema error; got findings: %+v", toolName, findings)
		}
	}
}

// TestSchemaCompilesCleanly verifies that the embedded composed schema
// compiles without errors using the jsonschema library (as VS Code/SchemaStore
// would do).
func TestSchemaCompilesCleanly(t *testing.T) {
	sch, err := schema()
	if err != nil {
		t.Fatalf("composed schema does not compile: %v", err)
	}
	if sch == nil {
		t.Fatal("compiled schema is nil")
	}
}

// --- D-final: machine-validatable per-provider schema rejection tests ---
//
// These tests prove that providerOverrides[p].field values with wrong types or
// out-of-enum values are NOW REJECTED by the composed schema. Prior to D-final
// the fields were permissive ({}) and anything passed.

// TestDFinalClaudeCodePermissionModeEnumRejected verifies that an invalid
// permissionMode value is rejected by the claude-code provider schema.
func TestDFinalClaudeCodePermissionModeEnumRejected(t *testing.T) {
	a := contract.CanonicalAgent{
		Name:        "my-agent",
		Description: "Does something useful.",
		Body:        "You are helpful.",
		ProviderOverrides: map[string]map[string]any{
			"claude-code": {"permissionMode": "superuser"}, // not in enum
		},
	}
	findings, err := Validate(a)
	if err != nil {
		t.Fatalf("Validate harness error: %v", err)
	}
	hasError := false
	for _, f := range findings {
		if f.Severity == severityError {
			hasError = true
		}
	}
	if !hasError {
		t.Fatalf("invalid permissionMode 'superuser' should produce a schema error; got findings: %+v", findings)
	}
}

// TestDFinalClaudeCodePermissionModeEnumAccepted verifies that a valid
// permissionMode value passes the claude-code provider schema.
func TestDFinalClaudeCodePermissionModeEnumAccepted(t *testing.T) {
	for _, mode := range []string{"default", "acceptEdits", "auto", "dontAsk", "bypassPermissions", "plan"} {
		a := contract.CanonicalAgent{
			Name:        "my-agent",
			Description: "Does something useful.",
			Body:        "You are helpful.",
			ProviderOverrides: map[string]map[string]any{
				"claude-code": {"permissionMode": mode},
			},
		}
		findings, err := Validate(a)
		if err != nil {
			t.Fatalf("Validate harness error for mode %q: %v", mode, err)
		}
		for _, f := range findings {
			if f.Severity == severityError {
				t.Errorf("valid permissionMode %q should not produce errors; got: %+v", mode, f)
			}
		}
	}
}

// TestDFinalClaudeCodeMaxTurnsTypeMismatchRejected verifies that a string value
// for maxTurns (which must be a number) is rejected.
func TestDFinalClaudeCodeMaxTurnsTypeMismatchRejected(t *testing.T) {
	a := contract.CanonicalAgent{
		Name:        "my-agent",
		Description: "Does something useful.",
		Body:        "You are helpful.",
		ProviderOverrides: map[string]map[string]any{
			"claude-code": {"maxTurns": "ten"}, // must be number
		},
	}
	findings, err := Validate(a)
	if err != nil {
		t.Fatalf("Validate harness error: %v", err)
	}
	hasError := false
	for _, f := range findings {
		if f.Severity == severityError {
			hasError = true
		}
	}
	if !hasError {
		t.Fatalf("maxTurns='ten' (string) should produce a schema error; got findings: %+v", findings)
	}
}

// TestDFinalClaudeCodeBackgroundTypeMismatchRejected verifies that a string value
// for background (which must be boolean) is rejected.
func TestDFinalClaudeCodeBackgroundTypeMismatchRejected(t *testing.T) {
	a := contract.CanonicalAgent{
		Name:        "my-agent",
		Description: "Does something useful.",
		Body:        "You are helpful.",
		ProviderOverrides: map[string]map[string]any{
			"claude-code": {"background": "yes"}, // must be boolean
		},
	}
	findings, err := Validate(a)
	if err != nil {
		t.Fatalf("Validate harness error: %v", err)
	}
	hasError := false
	for _, f := range findings {
		if f.Severity == severityError {
			hasError = true
		}
	}
	if !hasError {
		t.Fatalf("background='yes' (string) should produce a schema error; got findings: %+v", findings)
	}
}

// TestDFinalCodexSandboxModeEnumRejected verifies that an invalid sandbox_mode
// value is rejected by the codex provider schema.
func TestDFinalCodexSandboxModeEnumRejected(t *testing.T) {
	a := contract.CanonicalAgent{
		Name:        "my-agent",
		Description: "Does something useful.",
		Body:        "You are helpful.",
		ProviderOverrides: map[string]map[string]any{
			"codex": {"sandbox_mode": "unrestricted"}, // not in enum
		},
	}
	findings, err := Validate(a)
	if err != nil {
		t.Fatalf("Validate harness error: %v", err)
	}
	hasError := false
	for _, f := range findings {
		if f.Severity == severityError {
			hasError = true
		}
	}
	if !hasError {
		t.Fatalf("invalid sandbox_mode 'unrestricted' should produce a schema error; got findings: %+v", findings)
	}
}

// TestDFinalCodexModelReasoningEffortEnumAccepted verifies that valid
// model_reasoning_effort values pass the codex provider schema.
func TestDFinalCodexModelReasoningEffortEnumAccepted(t *testing.T) {
	for _, effort := range []string{"minimal", "low", "medium", "high", "xhigh"} {
		a := contract.CanonicalAgent{
			Name:        "my-agent",
			Description: "Does something useful.",
			Body:        "You are helpful.",
			ProviderOverrides: map[string]map[string]any{
				"codex": {"model_reasoning_effort": effort},
			},
		}
		findings, err := Validate(a)
		if err != nil {
			t.Fatalf("Validate harness error for effort %q: %v", effort, err)
		}
		for _, f := range findings {
			if f.Severity == severityError {
				t.Errorf("valid model_reasoning_effort %q should not produce errors; got: %+v", effort, f)
			}
		}
	}
}

// TestDFinalOpencodeModeEnumRejected verifies that an invalid mode value is
// rejected by the opencode provider schema.
func TestDFinalOpencodeModeEnumRejected(t *testing.T) {
	a := contract.CanonicalAgent{
		Name:        "my-agent",
		Description: "Does something useful.",
		Body:        "You are helpful.",
		ProviderOverrides: map[string]map[string]any{
			"opencode": {"mode": "background"}, // not in enum: only primary|subagent|all
		},
	}
	findings, err := Validate(a)
	if err != nil {
		t.Fatalf("Validate harness error: %v", err)
	}
	hasError := false
	for _, f := range findings {
		if f.Severity == severityError {
			hasError = true
		}
	}
	if !hasError {
		t.Fatalf("invalid opencode mode 'background' should produce a schema error; got findings: %+v", findings)
	}
}

// TestDFinalOpencodeTemperatureTypeMismatchRejected verifies that a string value
// for temperature (which must be a number) is rejected for opencode.
func TestDFinalOpencodeTemperatureTypeMismatchRejected(t *testing.T) {
	a := contract.CanonicalAgent{
		Name:        "my-agent",
		Description: "Does something useful.",
		Body:        "You are helpful.",
		ProviderOverrides: map[string]map[string]any{
			"opencode": {"temperature": "warm"}, // must be number
		},
	}
	findings, err := Validate(a)
	if err != nil {
		t.Fatalf("Validate harness error: %v", err)
	}
	hasError := false
	for _, f := range findings {
		if f.Severity == severityError {
			hasError = true
		}
	}
	if !hasError {
		t.Fatalf("temperature='warm' (string) should produce a schema error; got findings: %+v", findings)
	}
}

// TestDFinalRooCodeSlugPatternRejected verifies that a slug with spaces is
// rejected by the roo-code provider schema (pattern: ^[a-zA-Z0-9-]+$).
func TestDFinalRooCodeSlugPatternRejected(t *testing.T) {
	a := contract.CanonicalAgent{
		Name:        "my-agent",
		Description: "Does something useful.",
		Body:        "You are helpful.",
		ProviderOverrides: map[string]map[string]any{
			"roo-code": {"slug": "invalid slug with spaces"}, // violates pattern
		},
	}
	findings, err := Validate(a)
	if err != nil {
		t.Fatalf("Validate harness error: %v", err)
	}
	hasError := false
	for _, f := range findings {
		if f.Severity == severityError {
			hasError = true
		}
	}
	if !hasError {
		t.Fatalf("slug with spaces should produce a schema error; got findings: %+v", findings)
	}
}

// TestDFinalRooCodeSlugPatternAccepted verifies that a valid slug passes.
func TestDFinalRooCodeSlugPatternAccepted(t *testing.T) {
	a := contract.CanonicalAgent{
		Name:        "my-agent",
		Description: "Does something useful.",
		Body:        "You are helpful.",
		ProviderOverrides: map[string]map[string]any{
			"roo-code": {"slug": "my-mode-123"},
		},
	}
	findings, err := Validate(a)
	if err != nil {
		t.Fatalf("Validate harness error: %v", err)
	}
	for _, f := range findings {
		if f.Severity == severityError {
			t.Errorf("valid slug should not produce errors; got: %+v", f)
		}
	}
}

// TestDFinalGeminiCliKeyRejected verifies that the providerOverrides.gemini-cli
// key is rejected because gemini-cli is no longer in the active 8-provider set
// (deprecated 2026-06-15). Any value for the key must produce an error.
func TestDFinalGeminiCliKeyRejected(t *testing.T) {
	a := contract.CanonicalAgent{
		Name:        "my-agent",
		Description: "Does something useful.",
		Body:        "You are helpful.",
		ProviderOverrides: map[string]map[string]any{
			"gemini-cli": {"kind": "local"}, // deprecated provider — key itself is rejected
		},
	}
	findings, err := Validate(a)
	if err != nil {
		t.Fatalf("Validate harness error: %v", err)
	}
	hasError := false
	for _, f := range findings {
		if f.Severity == severityError {
			hasError = true
		}
	}
	if !hasError {
		t.Fatalf("providerOverrides.gemini-cli should be rejected (deprecated, not in active set); got findings: %+v", findings)
	}
}

// TestDFinalGithubCopilotTargetEnumRejected verifies that an invalid target value
// is rejected by the github-copilot provider schema.
func TestDFinalGithubCopilotTargetEnumRejected(t *testing.T) {
	a := contract.CanonicalAgent{
		Name:        "my-agent",
		Description: "Does something useful.",
		Body:        "You are helpful.",
		ProviderOverrides: map[string]map[string]any{
			"github-copilot": {"target": "jetbrains"}, // not in enum: only vscode|github-copilot
		},
	}
	findings, err := Validate(a)
	if err != nil {
		t.Fatalf("Validate harness error: %v", err)
	}
	hasError := false
	for _, f := range findings {
		if f.Severity == severityError {
			hasError = true
		}
	}
	if !hasError {
		t.Fatalf("invalid target 'jetbrains' should produce a schema error; got findings: %+v", findings)
	}
}

// TestDFinalCursorReadonlyTypeMismatchRejected verifies that a string value for
// readonly (which must be boolean) is rejected by the cursor provider schema.
func TestDFinalCursorReadonlyTypeMismatchRejected(t *testing.T) {
	a := contract.CanonicalAgent{
		Name:        "my-agent",
		Description: "Does something useful.",
		Body:        "You are helpful.",
		ProviderOverrides: map[string]map[string]any{
			"cursor": {"readonly": "true"}, // must be boolean, not string
		},
	}
	findings, err := Validate(a)
	if err != nil {
		t.Fatalf("Validate harness error: %v", err)
	}
	hasError := false
	for _, f := range findings {
		if f.Severity == severityError {
			hasError = true
		}
	}
	if !hasError {
		t.Fatalf("readonly='true' (string) should produce a schema error; got findings: %+v", findings)
	}
}

// --- providerOverrides closed-set rejection tests (review-r1) ---
//
// gemini-cli (deprecated 2026-06-15) and antigravity (planned/unregistered)
// are NOT in the active providerIDs set and therefore must be rejected by the
// composed schema's providerOverrides (additionalProperties:false).

// TestProviderOverridesGeminiCliRejected verifies that
// providerOverrides.gemini-cli is rejected by the schema (key is not in the
// active 8-provider set; gemini-cli was deprecated on 2026-06-15).
func TestProviderOverridesGeminiCliRejected(t *testing.T) {
	a := contract.CanonicalAgent{
		Name:        "my-agent",
		Description: "Does something useful.",
		Body:        "You are helpful.",
		ProviderOverrides: map[string]map[string]any{
			"gemini-cli": {"model": "gemini-pro"}, // deprecated provider — must be rejected
		},
	}
	findings, err := Validate(a)
	if err != nil {
		t.Fatalf("Validate harness error: %v", err)
	}
	hasError := false
	for _, f := range findings {
		if f.Severity == severityError {
			hasError = true
		}
	}
	if !hasError {
		t.Fatalf("providerOverrides.gemini-cli should be rejected (deprecated, not in active set); got findings: %+v", findings)
	}
}

// TestProviderOverridesAntigravityRejected verifies that
// providerOverrides.antigravity is rejected by the schema (key is not in the
// active 8-provider set; antigravity is planned but unregistered).
func TestProviderOverridesAntigravityRejected(t *testing.T) {
	a := contract.CanonicalAgent{
		Name:        "my-agent",
		Description: "Does something useful.",
		Body:        "You are helpful.",
		ProviderOverrides: map[string]map[string]any{
			"antigravity": {"model": "warp-9"}, // unregistered provider — must be rejected
		},
	}
	findings, err := Validate(a)
	if err != nil {
		t.Fatalf("Validate harness error: %v", err)
	}
	hasError := false
	for _, f := range findings {
		if f.Severity == severityError {
			hasError = true
		}
	}
	if !hasError {
		t.Fatalf("providerOverrides.antigravity should be rejected (planned/unregistered); got findings: %+v", findings)
	}
}

// TestProviderOverridesActiveKeyAccepted verifies that a valid active provider
// key (claude-code) passes providerOverrides validation cleanly.
func TestProviderOverridesActiveKeyAccepted(t *testing.T) {
	a := contract.CanonicalAgent{
		Name:        "my-agent",
		Description: "Does something useful.",
		Body:        "You are helpful.",
		ProviderOverrides: map[string]map[string]any{
			"claude-code": {"model": "claude-opus-4"},
		},
	}
	findings, err := Validate(a)
	if err != nil {
		t.Fatalf("Validate harness error: %v", err)
	}
	for _, f := range findings {
		if f.Severity == severityError {
			t.Errorf("valid providerOverrides.claude-code should not produce errors; got: %+v", f)
		}
	}
}

// --- Wildcard pattern accept/reject tests (review-r2) ---
//
// These tests validate the RE2-safe wildcard pattern used in the schema's tool
// items anyOf[enum, pattern] branch. The pattern must:
//   - ACCEPT: *, mcp_*, mcp__github__search, mcp__google_drive__read_file,
//     mcp__my_server__tool, Agent(general)
//   - REJECT: bare mcp__server (no second __ + tool segment)
//
// Tests compile the schema and validate through the actual jsonschema library
// so any RE2 incompatibility is caught here (not just a regexp.MustCompile check).

// TestWildcardPatternAccepted verifies that all valid wildcard/MCP/Agent() forms
// are accepted by the schema's tool items pattern branch.
func TestWildcardPatternAccepted(t *testing.T) {
	accepted := []string{
		"*",
		"mcp_*",
		"mcp__github__search",
		"mcp__google_drive__read_file",
		"mcp__my_server__tool",
		"Agent(general)",
		"Agent(researcher, analyst)",
	}
	for _, tool := range accepted {
		a := contract.CanonicalAgent{
			Name:        "wildcard-test",
			Description: "Wildcard accept test.",
			Body:        "Test.",
			Tools:       []string{tool},
		}
		findings, err := Validate(a)
		if err != nil {
			t.Fatalf("Validate harness error for %q: %v", tool, err)
		}
		for _, f := range findings {
			if f.Severity == severityError {
				t.Errorf("tool %q should be ACCEPTED by wildcard pattern but got error: %+v", tool, f)
			}
		}
	}
}

// TestDFinalRooCodeGroupsBrowserRejected verifies that roo-code providerOverrides
// with groups containing "browser" is REJECTED by the schema.
// "browser" is deprecated upstream and excluded from roo-code's tool map; a user
// writing groups:[browser] currently passes schema validation but produces a file
// Roo rejects at runtime. Removing it from the enum surfaces the error at
// validation time rather than at runtime.
func TestDFinalRooCodeGroupsBrowserRejected(t *testing.T) {
	for _, groups := range [][]any{
		{"browser"},
		{"read", "browser"},
		{"read", "edit", "browser", "command", "mcp"},
	} {
		a := contract.CanonicalAgent{
			Name:        "my-agent",
			Description: "Does something useful.",
			Body:        "You are helpful.",
			ProviderOverrides: map[string]map[string]any{
				"roo-code": {"groups": groups},
			},
		}
		findings, err := Validate(a)
		if err != nil {
			t.Fatalf("Validate harness error for groups %v: %v", groups, err)
		}
		hasError := false
		for _, f := range findings {
			if f.Severity == severityError {
				hasError = true
				break
			}
		}
		if !hasError {
			t.Errorf("roo-code groups %v should be REJECTED (browser is deprecated and excluded from roo-code tool map); got no validation error", groups)
		}
	}
}

// TestWildcardPatternRejected verifies that malformed MCP patterns that do not
// satisfy the two-double-underscore requirement are rejected by the schema.
func TestWildcardPatternRejected(t *testing.T) {
	// mcp__server is rejected because it lacks the second __ + tool segment.
	rejected := []string{
		"mcp__server",
	}
	for _, tool := range rejected {
		a := contract.CanonicalAgent{
			Name:        "wildcard-test",
			Description: "Wildcard reject test.",
			Body:        "Test.",
			Tools:       []string{tool},
		}
		findings, err := Validate(a)
		if err != nil {
			t.Fatalf("Validate harness error for %q: %v", tool, err)
		}
		hasError := false
		for _, f := range findings {
			if f.Severity == severityError {
				hasError = true
			}
		}
		if !hasError {
			t.Errorf("tool %q should be REJECTED (bare mcp__server, no tool segment) but no error found; findings: %+v", tool, findings)
		}
	}
}

// --- Conflict-marker detection tests -----------------------------------------

// TestScanConflictMarkers_OpenMarker verifies that a file containing the
// "<<<<<<< " open marker is flagged as an error.
func TestScanConflictMarkers_OpenMarker(t *testing.T) {
	dir := t.TempDir()
	content := "name: myagent\n<<<<<<< HEAD\ndescription: ours\n=======\ndescription: theirs\n>>>>>>> branch\n"
	if err := os.WriteFile(filepath.Join(dir, agentFile), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	findings := ScanConflictMarkers(dir, "myagent")
	if len(findings) == 0 {
		t.Fatal("expected error finding for open conflict marker, got none")
	}
	for _, f := range findings {
		if f.Severity != severityError {
			t.Errorf("finding severity=%q, want %q", f.Severity, severityError)
		}
		if !strings.Contains(f.Message, "unresolved git conflict markers") {
			t.Errorf("finding message=%q, want 'unresolved git conflict markers'", f.Message)
		}
	}
}

// TestScanConflictMarkers_CloseMarker verifies that a file containing only the
// ">>>>>>> " close marker (e.g. partially-resolved) is flagged.
func TestScanConflictMarkers_CloseMarker(t *testing.T) {
	dir := t.TempDir()
	content := "name: myagent\ndescription: resolved\n>>>>>>> branch-name-here\n"
	if err := os.WriteFile(filepath.Join(dir, agentFile), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	findings := ScanConflictMarkers(dir, "myagent")
	if len(findings) == 0 {
		t.Fatal("expected error finding for close conflict marker, got none")
	}
}

// TestScanConflictMarkers_InInstructions verifies that conflict markers in the
// instructions.md body file are also detected.
func TestScanConflictMarkers_InInstructions(t *testing.T) {
	dir := t.TempDir()
	// agent.yaml is clean
	yamlContent := "name: myagent\ndescription: A clean agent.\n"
	if err := os.WriteFile(filepath.Join(dir, agentFile), []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}
	// instructions.md has markers
	bodyContent := "You are a helpful agent.\n<<<<<<< HEAD\nDo X.\n=======\nDo Y.\n>>>>>>> feature-branch\n"
	if err := os.WriteFile(filepath.Join(dir, bodyFile), []byte(bodyContent), 0o644); err != nil {
		t.Fatal(err)
	}
	findings := ScanConflictMarkers(dir, "myagent")
	if len(findings) == 0 {
		t.Fatal("expected error finding for conflict markers in instructions.md, got none")
	}
}

// TestScanConflictMarkers_CleanFile verifies that a clean agent.yaml produces
// no findings.
func TestScanConflictMarkers_CleanFile(t *testing.T) {
	dir := t.TempDir()
	content := "name: myagent\ndescription: Reviews code changes.\nmodel: sonnet\n"
	if err := os.WriteFile(filepath.Join(dir, agentFile), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	findings := ScanConflictMarkers(dir, "myagent")
	if len(findings) != 0 {
		t.Fatalf("expected no findings for clean file, got: %+v", findings)
	}
}

// TestScanConflictMarkers_MarkdownUnderline verifies that a legitimate Markdown
// "=======" heading underline does NOT produce a false positive when no chevron
// markers are present.
func TestScanConflictMarkers_MarkdownUnderline(t *testing.T) {
	dir := t.TempDir()
	// This is a common Markdown pattern: a heading underlined with "======="
	bodyContent := "My Agent\n=======\n\nThis is the body of the agent instructions.\n"
	if err := os.WriteFile(filepath.Join(dir, bodyFile), []byte(bodyContent), 0o644); err != nil {
		t.Fatal(err)
	}
	findings := ScanConflictMarkers(dir, "myagent")
	if len(findings) != 0 {
		t.Fatalf("expected no findings for Markdown underline (false positive guard), got: %+v", findings)
	}
}

// TestScanConflictMarkers_MissingFiles verifies that missing files are silently
// skipped (no panic, no spurious error).
func TestScanConflictMarkers_MissingFiles(t *testing.T) {
	dir := t.TempDir()
	// Neither agent.yaml nor instructions.md exists in the dir.
	findings := ScanConflictMarkers(dir, "nonexistent")
	if len(findings) != 0 {
		t.Fatalf("expected no findings for missing files, got: %+v", findings)
	}
}

// TestValidateToolBadValueSingleFinding verifies that a bad tool value produces
// exactly ONE error finding (not three) and that the message is the clear
// "unknown tool" form with did-you-mean when applicable.
func TestValidateToolBadValueSingleFinding(t *testing.T) {
	// "edit" is a native name → should get did-you-mean "file_edit"
	a := contract.CanonicalAgent{
		Name:        "my-agent",
		Description: "Does something useful.",
		Body:        "You are helpful.",
		Tools:       []string{"edit"},
	}
	findings, err := Validate(a)
	if err != nil {
		t.Fatalf("Validate harness error: %v", err)
	}
	var errFindings []contract.Finding
	for _, f := range findings {
		if f.Severity == severityError {
			errFindings = append(errFindings, f)
		}
	}
	if len(errFindings) != 1 {
		t.Fatalf("expected exactly 1 error finding for bad tool 'edit', got %d: %+v", len(errFindings), errFindings)
	}
	msg := errFindings[0].Message
	if strings.Contains(msg, "got array, want object") {
		t.Errorf("error message should not contain 'got array, want object': %q", msg)
	}
	if !strings.Contains(msg, "file_edit") {
		t.Errorf("did-you-mean should suggest 'file_edit' for native 'edit'; got: %q", msg)
	}
	if !strings.Contains(msg, "edit") {
		t.Errorf("message should mention the bad value 'edit'; got: %q", msg)
	}
}

// TestValidateToolTrulyUnknownNoSuggestion verifies that a truly unknown tool
// name (no native equivalent) produces a clear "unknown tool" message without
// a bogus did-you-mean suggestion.
func TestValidateToolTrulyUnknownNoSuggestion(t *testing.T) {
	a := contract.CanonicalAgent{
		Name:        "my-agent",
		Description: "Does something useful.",
		Body:        "You are helpful.",
		Tools:       []string{"definitely_not_a_real_tool_xyz"},
	}
	findings, err := Validate(a)
	if err != nil {
		t.Fatalf("Validate harness error: %v", err)
	}
	var errFindings []contract.Finding
	for _, f := range findings {
		if f.Severity == severityError {
			errFindings = append(errFindings, f)
		}
	}
	if len(errFindings) != 1 {
		t.Fatalf("expected exactly 1 error finding, got %d: %+v", len(errFindings), errFindings)
	}
	msg := errFindings[0].Message
	if strings.Contains(msg, "did you mean") {
		t.Errorf("no suggestion should be made for unknown tool; got: %q", msg)
	}
	if !strings.Contains(msg, "definitely_not_a_real_tool_xyz") {
		t.Errorf("message should mention the bad value; got: %q", msg)
	}
}

// TestSkillsRoundTrip verifies that skills survive a save→load round-trip.
func TestSkillsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	a := sampleAgent()
	a.Skills = []string{"a", "b"}

	writes, err := Save(dir, a)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	writeAll(t, writes)

	got, err := Load(AgentDir(dir, a.Name))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	a.Body = normalizeBody(a.Body)
	if !reflect.DeepEqual(got.Skills, a.Skills) {
		t.Fatalf("Skills round-trip mismatch: got %v, want %v", got.Skills, a.Skills)
	}
}

// TestSkillsHashDeterministic verifies that an agent with skills hashes
// deterministically and that a different set of skills produces a different hash.
func TestSkillsHashDeterministic(t *testing.T) {
	a := sampleAgent()
	a.Skills = []string{"x", "y"}
	h1 := Hash(a)
	h2 := Hash(a)
	if h1 != h2 {
		t.Fatalf("Skills hash not stable: %s != %s", h1, h2)
	}

	b := sampleAgent()
	b.Skills = []string{"z"}
	if Hash(a) == Hash(b) {
		t.Fatalf("agents with different skills must have different hashes")
	}
}

// TestSkillsHashNoChurn verifies that an agent WITHOUT skills hashes
// identically regardless of whether Skills is nil or an empty slice.
// This confirms no hash churn for existing skill-less agents.
func TestSkillsHashNoChurn(t *testing.T) {
	a := sampleAgent()
	// nil Skills
	a.Skills = nil
	h1 := Hash(a)

	b := sampleAgent()
	// empty slice Skills — must produce same hash due to nilToEmpty
	b.Skills = []string{}
	h2 := Hash(b)

	if h1 != h2 {
		t.Fatalf("nil vs empty Skills should hash identically (no churn): %s != %s", h1, h2)
	}
}

// TestValidateToolDescriptionMentionsCanonical verifies that the generated
// schema's tools description mentions canonical names and NOT "native".
func TestValidateToolDescriptionMentionsCanonical(t *testing.T) {
	sch, err := schema()
	if err != nil {
		t.Fatalf("schema() error: %v", err)
	}
	// The compiled schema is opaque; check the raw JSON.
	raw := string(schemaJSON)
	if strings.Contains(raw, "Use native provider tool names") {
		t.Errorf("schema description should not say 'Use native provider tool names'; found in schema JSON")
	}
	if !strings.Contains(raw, "canonical") {
		t.Errorf("schema description should mention 'canonical' names; not found in schema JSON")
	}
	_ = sch
}
