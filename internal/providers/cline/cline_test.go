package clineprov

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"gopkg.in/yaml.v3"
)

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
	// Assert the body matches the literal content from the fixture BEFORE the
	// copy below makes the comparison vacuous.
	const wantBody = "\nYou are a security-focused code auditor.\n"
	if ca.Body != wantBody {
		t.Errorf("body mismatch:\n--- got ---\n%q\n--- want ---\n%q", ca.Body, wantBody)
	}
	// Body has no YAML tag; copy into want so the struct comparison below is
	// not blocked by a zero-value mismatch on the body field.
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
	want, err := os.ReadFile(filepath.Join("testdata", "want.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(writes[0].Data) != string(want) {
		t.Errorf("serialized mismatch:\n--- got ---\n%s\n--- want ---\n%s", writes[0].Data, want)
	}

	// Re-parse serialized output and assert canonical stability.
	dir := t.TempDir()
	tmp := filepath.Join(dir, filepath.Base(writes[0].Path))
	if err := os.MkdirAll(filepath.Dir(tmp), 0o755); err != nil {
		t.Fatal(err)
	}
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

func TestDetect(t *testing.T) {
	root := t.TempDir()
	agentsDir := filepath.Join(root, ".cline", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, fn := range []string{"alpha.yaml", "beta.yml", "not-an-agent.txt"} {
		if err := os.WriteFile(filepath.Join(agentsDir, fn), []byte("---\nname: "+fn+"\n---\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p := New()
	refs, err := p.Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d: %v", len(refs), refs)
	}
	names := make(map[string]bool)
	for _, r := range refs {
		names[r.Name] = true
		if r.Provider != name {
			t.Errorf("ref.Provider=%q want %q", r.Provider, name)
		}
	}
	if !names["alpha"] || !names["beta"] {
		t.Errorf("expected alpha and beta, got: %v", refs)
	}
}

func TestDetectHomeScope(t *testing.T) {
	// Simulate $HOME as a tempdir so os.UserHomeDir picks it up.
	home := t.TempDir()
	t.Setenv("HOME", home)

	agentsDir := filepath.Join(home, ".cline", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, fn := range []string{"home-agent.yaml", "another.yml", "skip.txt"} {
		if err := os.WriteFile(filepath.Join(agentsDir, fn), []byte("---\nname: "+fn+"\n---\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Pass an empty project root so only home-scope refs are returned.
	root := t.TempDir()
	p := New()
	refs, err := p.Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs from home scope, got %d: %v", len(refs), refs)
	}
	names := make(map[string]bool)
	for _, r := range refs {
		names[r.Name] = true
		if r.Provider != name {
			t.Errorf("ref.Provider=%q want %q", r.Provider, name)
		}
	}
	if !names["home-agent"] || !names["another"] {
		t.Errorf("expected home-agent and another, got: %v", refs)
	}
}

func TestCommaStringTools(t *testing.T) {
	// Cline tools field may be a comma-separated string; verify both forms.
	commaContent := []byte("---\nname: comma-agent\ntools: read_file, execute_command\n---\nbody\n")
	dir := t.TempDir()
	f := filepath.Join(dir, "comma-agent.yaml")
	if err := os.WriteFile(f, commaContent, 0o644); err != nil {
		t.Fatal(err)
	}
	p := New()
	pa, err := p.Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := p.ToCanonical(pa)
	if err != nil {
		t.Fatal(err)
	}
	if len(ca.Tools) != 2 {
		t.Fatalf("expected 2 tools from comma string, got %d: %v", len(ca.Tools), ca.Tools)
	}
	// read_file → read_file, execute_command → bash
	want := map[string]bool{"read_file": true, "bash": true}
	for _, tool := range ca.Tools {
		if !want[tool] {
			t.Errorf("unexpected canonical tool %q", tool)
		}
	}
}

func TestOverridePreservation(t *testing.T) {
	// Frontmatter key not in knownKeys must travel through ProviderOverrides.
	content := []byte("---\nname: override-test\ncustom_key: hello\nanother: 42\n---\nbody\n")
	dir := t.TempDir()
	f := filepath.Join(dir, "override-test.yaml")
	if err := os.WriteFile(f, content, 0o644); err != nil {
		t.Fatal(err)
	}
	p := New()
	pa, err := p.Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := p.ToCanonical(pa)
	if err != nil {
		t.Fatal(err)
	}
	ov, ok := ca.ProviderOverrides[name]
	if !ok {
		t.Fatal("expected ProviderOverrides[cline] to be set")
	}
	if ov["custom_key"] != "hello" {
		t.Errorf("custom_key override: got %v, want %q", ov["custom_key"], "hello")
	}

	// Serialize and re-parse — overrides must survive.
	writes, err := p.Serialize(ca)
	if err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(dir, "out.yaml")
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
	ov2 := ca2.ProviderOverrides[name]
	if ov2["custom_key"] != "hello" {
		t.Errorf("custom_key after round-trip: got %v, want %q", ov2["custom_key"], "hello")
	}
}
