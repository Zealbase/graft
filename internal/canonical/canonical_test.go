package canonical

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

func sampleAgent() contract.CanonicalAgent {
	return contract.CanonicalAgent{
		Name:        "reviewer",
		Description: "Reviews code changes for correctness.",
		Model:       "inherit",
		Tools:       []string{"Read", "Grep", "Bash"},
		MCP:         []string{"grafana", "notion"},
		Permissions: map[string]string{
			"Bash":  "ask",
			"Write": "deny",
		},
		Body: "You are a careful code reviewer.\nFocus on correctness.",
		ProviderOverrides: map[string]map[string]any{
			"claude": {"isolation": "worktree", "effort": "high"},
			"gemini": {"timeout_mins": 10},
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
	b.Permissions = map[string]string{"Write": "deny", "Bash": "ask"}
	b.ProviderOverrides = map[string]map[string]any{
		"gemini": {"timeout_mins": 10},
		"claude": {"effort": "high", "isolation": "worktree"},
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
		"gemini": {}, // deliberately empty
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

	// The empty gemini bucket must NOT appear.
	if _, ok := got.ProviderOverrides["gemini"]; ok {
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
		"gemini":      {"timeout_mins": 15},
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
