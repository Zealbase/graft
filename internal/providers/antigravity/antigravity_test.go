package antigravity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"gopkg.in/yaml.v3"
)

// TestScopeHome asserts that the antigravity provider declares ScopeHome so
// the engine writes to ~/.gemini/antigravity-cli/ instead of the workspace root.
func TestScopeHome(t *testing.T) {
	p := New()
	sp, ok := any(p).(contract.ScopedProvider)
	if !ok {
		t.Fatal("antigravity.Provider must implement contract.ScopedProvider")
	}
	if got := sp.PathScope(); got != contract.ScopeHome {
		t.Errorf("PathScope() = %v, want ScopeHome (%v)", got, contract.ScopeHome)
	}
}

// TestDetectUsesProvidedRoot asserts that Detect(root) scans
// <root>/.gemini/antigravity-cli/agents/, so when the engine passes $HOME as
// root the correct path is scanned.
func TestDetectUsesProvidedRoot(t *testing.T) {
	p := New()
	home := t.TempDir() // simulate $HOME

	// No agents yet — should return nil, nil.
	refs, err := p.Detect(home)
	if err != nil {
		t.Fatalf("Detect on empty home: %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("expected 0 refs, got %d", len(refs))
	}

	// Provision one agent dir with an agent.json.
	agentDir := filepath.Join(home, ".gemini", "antigravity-cli", "agents", "test-agent")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"name":"test-agent","description":"A test."}`)
	if err := os.WriteFile(filepath.Join(agentDir, "agent.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	refs, err = p.Detect(home)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d: %+v", len(refs), refs)
	}
	if refs[0].Name != "test-agent" {
		t.Errorf("ref.Name = %q, want test-agent", refs[0].Name)
	}
}

// TestSerializePathIsHomeRelative asserts that Serialize returns a path
// relative to $HOME (not to the workspace root), so the engine can write to
// the correct location when PathScope == ScopeHome.
func TestSerializePathIsHomeRelative(t *testing.T) {
	p := New()
	ca := contract.CanonicalAgent{Name: "myagent", Description: "Test agent."}
	writes, err := p.Serialize(ca)
	if err != nil {
		t.Fatal(err)
	}
	if len(writes) != 1 {
		t.Fatalf("expected 1 FileWrite, got %d", len(writes))
	}
	want := filepath.Join(".gemini", "antigravity-cli", "agents", "myagent", "agent.json")
	if writes[0].Path != want {
		t.Errorf("Serialize path = %q, want %q (HOME-relative)", writes[0].Path, want)
	}
}

// TestSerializeOverrideWins is the MED 3 regression: Serialize must use
// RestoreOverrides (override WINS), not the old stashed-extras Restore (which
// silently dropped an override whose key was already present). A
// providerOverrides["antigravity"] entry for a key already written by the
// canonical fields must take effect; "name" stays protected.
func TestSerializeOverrideWins(t *testing.T) {
	p := New()
	ca := contract.CanonicalAgent{
		Name:        "myagent",
		Description: "canonical description",
		ProviderOverrides: map[string]map[string]any{
			"antigravity": {
				"description": "OVERRIDDEN description",
				"name":        "attempted-name-override",
				"hidden":      true,
			},
		},
	}
	writes, err := p.Serialize(ca)
	if err != nil {
		t.Fatal(err)
	}
	got := string(writes[0].Data)
	if !strings.Contains(got, "OVERRIDDEN description") {
		t.Errorf("override did not win for description (RestoreOverrides not used):\n%s", got)
	}
	if strings.Contains(got, "canonical description") {
		t.Errorf("canonical description should have been overwritten by the override:\n%s", got)
	}
	if !strings.Contains(got, `"hidden": true`) {
		t.Errorf("override-only key 'hidden' missing:\n%s", got)
	}
	// "name" is protected identity: the override must NOT change it.
	if strings.Contains(got, "attempted-name-override") {
		t.Errorf("name override leaked (identity must be protected):\n%s", got)
	}
	if !strings.Contains(got, `"name": "myagent"`) {
		t.Errorf("canonical name not preserved:\n%s", got)
	}
}

const wantExt = ".json"

func inFile(t *testing.T) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join("testdata", "in.*"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("no input fixture: %v", err)
	}
	return matches[0]
}

func TestParseToCanonical(t *testing.T) {
	p := New()
	pa, err := p.Parse(inFile(t))
	if err != nil {
		t.Fatal(err)
	}
	ca, err := p.ToCanonical(pa)
	if err != nil {
		t.Fatal(err)
	}
	wantBytes, err := os.ReadFile(filepath.Join("testdata", "want.canonical.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var want contract.CanonicalAgent
	if err := yaml.Unmarshal(wantBytes, &want); err != nil {
		t.Fatal(err)
	}
	want.Body = ca.Body
	gotY, _ := yaml.Marshal(ca)
	wantY, _ := yaml.Marshal(want)
	if string(gotY) != string(wantY) {
		t.Errorf("canonical mismatch:\n--- got ---\n%s\n--- want ---\n%s", gotY, wantY)
	}
}

func TestRoundTripLossless(t *testing.T) {
	p := New()
	pa, err := p.Parse(inFile(t))
	if err != nil {
		t.Fatal(err)
	}
	ca, err := p.ToCanonical(pa)
	if err != nil {
		t.Fatal(err)
	}
	writes, err := p.Serialize(ca)
	if err != nil {
		t.Fatal(err)
	}
	if len(writes) != 1 {
		t.Fatalf("expected 1 file write, got %d", len(writes))
	}
	want, err := os.ReadFile(filepath.Join("testdata", "want"+wantExt))
	if err != nil {
		t.Fatal(err)
	}
	if string(writes[0].Data) != string(want) {
		t.Errorf("serialized mismatch:\n--- got ---\n%s\n--- want ---\n%s", writes[0].Data, want)
	}

	// Re-parse serialized output using the basename the provider chose, so
	// filename-derived identity stays stable, and assert canonical stability.
	dir := t.TempDir()
	tmp := filepath.Join(dir, filepath.Base(writes[0].Path))
	if err := os.WriteFile(tmp, writes[0].Data, 0o644); err != nil {
		t.Fatal(err)
	}
	pa2, err := p.Parse(tmp)
	if err != nil {
		t.Fatal(err)
	}
	ca2, err := p.ToCanonical(pa2)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := yaml.Marshal(ca)
	b, _ := yaml.Marshal(ca2)
	if string(a) != string(b) {
		t.Errorf("round-trip canonical not stable:\n%s\nvs\n%s", a, b)
	}
}
