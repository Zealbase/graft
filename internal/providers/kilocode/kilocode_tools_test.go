package kilocode

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/povr"
	"gopkg.in/yaml.v3"
)

// TestToolMapRoundTrip verifies that for each entry in toolMap,
// native→canonical→native and canonical→native→canonical are both identity.
func TestToolMapRoundTrip(t *testing.T) {
	entries := []struct{ native, canonical string }{
		{"read", "read_file"},
		{"edit", "file_edit"},
		{"bash", "bash"},
		{"glob", "glob"},
		{"grep", "grep"},
		{"task", "task_create"},
		{"webfetch", "web_fetch"},
		{"websearch", "web_search"},
		{"todowrite", "todo_write"},
		{"todoread", "todo_read"},
	}
	for _, e := range entries {
		c, ok := toolMap.CanonicalTool(e.native)
		if !ok || c != e.canonical {
			t.Errorf("native %q → canonical: got (%q, %v), want (%q, true)", e.native, c, ok, e.canonical)
		}
		n, ok := toolMap.NativeTool(e.canonical)
		if !ok || n != e.native {
			t.Errorf("canonical %q → native: got (%q, %v), want (%q, true)", e.canonical, n, ok, e.native)
		}
		// Round-trip canonical→native→canonical
		c2, _ := toolMap.CanonicalTool(n)
		if c2 != e.canonical {
			t.Errorf("canonical→native→canonical for %q: got %q, want %q", e.canonical, c2, e.canonical)
		}
	}
}

// TestPermissionToCanonicalTools verifies that permission.allow=["read","edit","bash","glob"]
// produces sorted canonical tools=["bash","file_edit","glob","read_file"].
func TestPermissionToCanonicalTools(t *testing.T) {
	src := "description: test\npermission:\n  allow:\n    - read\n    - edit\n    - bash\n    - glob\n  deny: []\n  ask: []\n"
	var fields map[string]any
	if err := yaml.Unmarshal([]byte(src), &fields); err != nil {
		t.Fatal(err)
	}
	pa := contract.ProviderAgent{
		Provider: name,
		Ref:      contract.AgentRef{Name: "test-agent", Provider: name, Path: "/ws/.kilo/agents/test-agent.md"},
		Fields:   fields,
		Body:     "",
	}
	p := New()
	ca, err := p.ToCanonical(pa)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"bash", "file_edit", "glob", "read_file"}
	got := make([]string, len(ca.Tools))
	copy(got, ca.Tools)
	sort.Strings(got)
	if len(got) != len(want) {
		t.Fatalf("tools length: got %d want %d: %v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("tools[%d]: got %q want %q", i, got[i], w)
		}
	}
}

// TestPermissionGlobPreservation verifies that a permission object with deny/ask
// arrays including glob patterns is preserved losslessly in overrides and
// restored on serialize.
func TestPermissionGlobPreservation(t *testing.T) {
	src := "description: glob-test\npermission:\n  allow:\n    - read\n  deny:\n    - \"edit:*.go\"\n  ask:\n    - bash\n"
	var fields map[string]any
	if err := yaml.Unmarshal([]byte(src), &fields); err != nil {
		t.Fatal(err)
	}
	pa := contract.ProviderAgent{
		Provider: name,
		Ref:      contract.AgentRef{Name: "glob-test", Provider: name, Path: "/ws/.kilo/agents/glob-test.md"},
		Fields:   fields,
		Body:     "body text",
	}
	p := New()
	ca, err := p.ToCanonical(pa)
	if err != nil {
		t.Fatal(err)
	}
	// permission must be in overrides
	ov, ok := ca.ProviderOverrides[name]
	if !ok {
		t.Fatal("expected provider overrides")
	}
	perm, ok := ov["permission"]
	if !ok {
		t.Fatal("permission not preserved in overrides")
	}
	permMap, ok := perm.(map[string]any)
	if !ok {
		t.Fatalf("permission type: %T", perm)
	}
	if permMap["deny"] == nil {
		t.Fatal("deny not preserved in overrides")
	}

	// Serialize and re-parse; permission must survive round-trip.
	writes, err := p.Serialize(ca)
	if err != nil {
		t.Fatal(err)
	}
	if len(writes) != 1 {
		t.Fatalf("expected 1 write, got %d", len(writes))
	}

	// Write to tempdir and re-parse.
	dir := t.TempDir()
	tmp := dir + "/glob-test.md"
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
	ov2, ok := ca2.ProviderOverrides[name]
	if !ok {
		t.Fatal("permission not preserved after round-trip")
	}
	perm2, ok := ov2["permission"]
	if !ok {
		t.Fatal("permission key not found after round-trip")
	}
	permMap2, _ := perm2.(map[string]any)
	if permMap2["deny"] == nil {
		t.Fatal("deny not preserved after round-trip")
	}
}

// TestSerializeToolsFromCanonical verifies that a canonical agent with Tools but
// NO ProviderOverrides["kilo-code"]["permission"] serializes to a .kilo/agents/<name>.md
// whose frontmatter contains a permission block derived from the canonical tools.
// Specifically: canonical tools [read_file, grep, bash] → permission.allow [bash, grep, read].
func TestSerializeToolsFromCanonical(t *testing.T) {
	ca := contract.CanonicalAgent{
		Name:        "tooled-agent",
		Description: "An agent with canonical tools but no kilo override",
		Tools:       []string{"read_file", "grep", "bash"},
		// No ProviderOverrides at all — simulates propagation from another provider.
	}

	p := New()
	writes, err := p.Serialize(ca)
	if err != nil {
		t.Fatal(err)
	}
	if len(writes) != 1 {
		t.Fatalf("expected 1 file write, got %d", len(writes))
	}

	// Serialized output must have YAML frontmatter.
	data := string(writes[0].Data)
	if !strings.HasPrefix(data, "---\n") {
		t.Fatalf("expected YAML frontmatter, got:\n%s", data)
	}

	// Parse output and extract permission block.
	dir := t.TempDir()
	tmp := filepath.Join(dir, "tooled-agent.md")
	if err := os.WriteFile(tmp, writes[0].Data, 0o644); err != nil {
		t.Fatal(err)
	}
	pa, err := p.Parse(tmp)
	if err != nil {
		t.Fatal(err)
	}

	perm, ok := pa.Fields["permission"]
	if !ok || perm == nil {
		t.Fatal("expected permission block in serialized output, got none")
	}
	permMap, ok := perm.(map[string]any)
	if !ok {
		t.Fatalf("permission type: %T", perm)
	}

	allow := povr.StringSlice(permMap["allow"])
	sort.Strings(allow)

	// canonical read_file→read, grep→grep, bash→bash
	wantAllow := []string{"bash", "grep", "read"}
	if len(allow) != len(wantAllow) {
		t.Fatalf("permission.allow length: got %d %v, want %d %v", len(allow), allow, len(wantAllow), wantAllow)
	}
	for i, w := range wantAllow {
		if allow[i] != w {
			t.Errorf("permission.allow[%d]: got %q, want %q", i, allow[i], w)
		}
	}

	// Round-trip: ToCanonical must recover the original tools.
	ca2, err := p.ToCanonical(pa)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(ca2.Tools))
	copy(got, ca2.Tools)
	sort.Strings(got)
	wantTools := []string{"bash", "grep", "read_file"}
	if len(got) != len(wantTools) {
		t.Fatalf("recovered tools length: got %d %v, want %d %v", len(got), got, len(wantTools), wantTools)
	}
	for i, w := range wantTools {
		if got[i] != w {
			t.Errorf("tools[%d]: got %q, want %q", i, got[i], w)
		}
	}
}

// TestSerializeNoToolsNoPermission verifies that a canonical agent with no Tools
// and no ProviderOverrides["kilo-code"]["permission"] does NOT emit a permission
// block (no spurious empty permission).
func TestSerializeNoToolsNoPermission(t *testing.T) {
	ca := contract.CanonicalAgent{
		Name:        "bare-agent",
		Description: "No tools, no permission override",
	}

	p := New()
	writes, err := p.Serialize(ca)
	if err != nil {
		t.Fatal(err)
	}
	if len(writes) != 1 {
		t.Fatalf("expected 1 file write, got %d", len(writes))
	}

	dir := t.TempDir()
	tmp := filepath.Join(dir, "bare-agent.md")
	if err := os.WriteFile(tmp, writes[0].Data, 0o644); err != nil {
		t.Fatal(err)
	}
	pa, err := p.Parse(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := pa.Fields["permission"]; ok {
		t.Error("expected no permission block for agent with no tools, but got one")
	}
}
