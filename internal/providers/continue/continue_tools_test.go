package continueprov

import (
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

func TestToolMappingsKnown(t *testing.T) {
	cases := []struct{ native, want string }{
		{"Read", "read_file"},
		{"Edit", "file_edit"},
		{"Bash", "bash"},
		{"Search", "grep"},
		{"List", "list_directory"},
		{"Fetch", "web_fetch"},
		{"AskQuestion", "ask_user_question"},
		{"Diff", "view_diff"},
		{"web_search", "web_search"},
		{"codebase_search", "codebase_search"},
	}
	for _, tc := range cases {
		got, ok := toolMap.CanonicalTool(tc.native)
		if !ok || got != tc.want {
			t.Errorf("CanonicalTool(%q) = %q,%v; want %q,true", tc.native, got, ok, tc.want)
		}
	}
}

func TestBashConstrainedPassthrough(t *testing.T) {
	tool := "Bash(git diff:*)"
	p := Provider{}
	if p.SupportsTool(tool) {
		t.Errorf("SupportsTool(%q) should be false (constrained form)", tool)
	}
	got := toolMap.MapToCanonical([]string{tool})
	if len(got) != 1 || got[0] != tool {
		t.Errorf("MapToCanonical(%q) = %v; want [%q]", tool, got, tool)
	}
	native := toolMap.MapToNative([]string{tool})
	if len(native) != 1 || native[0] != tool {
		t.Errorf("MapToNative(%q) = %v; want [%q]", tool, native, tool)
	}
}

func TestMCPSlugPassthrough(t *testing.T) {
	slug := "linear-mcp/tool-name"
	got := toolMap.MapToCanonical([]string{slug})
	if len(got) != 1 || got[0] != slug {
		t.Errorf("MapToCanonical(%q) = %v; want [%q]", slug, got, slug)
	}
}

// TestToCanonical_ConstrainedToolsRouting verifies the core bug fix: constrained
// Bash tokens like "Bash(git diff:*)" and MCP hub slugs like "org/pkg:tool" must
// NOT appear in canonical.Tools (the schema validator would reject them). They must
// survive in ProviderOverrides["continue"]["_passthrough_tools"] and round-trip losslessly.
func TestToCanonical_ConstrainedToolsRouting(t *testing.T) {
	p := Provider{}
	pa := contract.ProviderAgent{
		Provider: "continue",
		Ref:      contract.AgentRef{Name: "code-reviewer", Provider: "continue"},
		Fields: map[string]any{
			"name":        "code-reviewer",
			"description": "Reviews code.",
			"model":       "anthropic/claude-sonnet-4",
			"tools":       "Read, Edit, Bash, Bash(git diff:*), org/linear-mcp:create-issue",
		},
		Body: "You review code.\n",
	}

	ca, err := p.ToCanonical(pa)
	if err != nil {
		t.Fatal(err)
	}

	// Canonical Tools must contain only clean mapped names.
	wantCanonical := map[string]bool{"read_file": true, "file_edit": true, "bash": true}
	for _, tool := range ca.Tools {
		if !wantCanonical[tool] {
			t.Errorf("unexpected tool in canonical.Tools: %q (constrained/MCP tokens must go to overrides)", tool)
		}
	}
	for want := range wantCanonical {
		found := false
		for _, tool := range ca.Tools {
			if tool == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("canonical.Tools missing expected mapped tool %q", want)
		}
	}

	// Constrained/MCP tokens must live in ProviderOverrides["continue"]["_passthrough_tools"].
	// (Using "_passthrough_tools" rather than "tools" avoids the po-continue
	// schema validation on the "tools" property, which only accepts enum values.)
	ovTools, ok := ca.ProviderOverrides["continue"]["_passthrough_tools"]
	if !ok {
		t.Fatal("ProviderOverrides[continue][_passthrough_tools] not set; constrained tokens lost")
	}
	var passthroughSlice []string
	switch v := ovTools.(type) {
	case []string:
		passthroughSlice = v
	case []any:
		for _, e := range v {
			if s, ok := e.(string); ok {
				passthroughSlice = append(passthroughSlice, s)
			}
		}
	default:
		t.Fatalf("ProviderOverrides[continue][_passthrough_tools] has unexpected type %T", ovTools)
	}
	wantPassthrough := map[string]bool{
		"Bash(git diff:*)":            true,
		"org/linear-mcp:create-issue": true,
	}
	for _, tok := range passthroughSlice {
		if !wantPassthrough[tok] {
			t.Errorf("unexpected token in passthrough overrides: %q", tok)
		}
	}
	for want := range wantPassthrough {
		found := false
		for _, tok := range passthroughSlice {
			if tok == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("passthrough override missing expected token %q", want)
		}
	}
}

// TestSerialize_ConstrainedToolsRoundTrip verifies that after ToCanonical routes
// constrained tokens to overrides, Serialize reconstructs the original tools:
// frontmatter with both canonical-mapped natives and the constrained/MCP tokens.
func TestSerialize_ConstrainedToolsRoundTrip(t *testing.T) {
	p := Provider{}
	pa := contract.ProviderAgent{
		Provider: "continue",
		Ref:      contract.AgentRef{Name: "code-reviewer", Provider: "continue"},
		Fields: map[string]any{
			"name":        "code-reviewer",
			"description": "Reviews code.",
			"model":       "anthropic/claude-sonnet-4",
			"tools":       "Read, Edit, Bash, Bash(git diff:*), org/linear-mcp:create-issue",
		},
		Body: "You review code.\n",
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

	out := string(writes[0].Data)
	// All five original tokens must appear in the serialized output.
	for _, want := range []string{"Read", "Edit", "Bash", "Bash(git diff:*)", "org/linear-mcp:create-issue"} {
		if !contains(out, want) {
			t.Errorf("serialized output missing token %q:\n%s", want, out)
		}
	}
}

// TestSerialize_ToolOrderingIsKnownReordering documents the accepted behavior:
// ToCanonical routes constrained/MCP tokens to _passthrough_tools and Serialize
// appends them after the canonical-mapped natives. So an interleaved input like
// "Read, Bash(git diff:*), Edit" round-trips to "Read, Edit, Bash(git diff:*)".
// Continue's `tools:` field is a capability allowlist, not an ordered pipeline,
// so this re-ordering has no semantic effect for Continue agents.
func TestSerialize_ToolOrderingIsKnownReordering(t *testing.T) {
	p := Provider{}
	pa := contract.ProviderAgent{
		Provider: "continue",
		Ref:      contract.AgentRef{Name: "order-test", Provider: "continue"},
		Fields: map[string]any{
			"name":        "order-test",
			"description": "Tests tool ordering.",
			"model":       "anthropic/claude-sonnet-4",
			// Interleaved: canonical tool, passthrough token, canonical tool.
			"tools": "Read, Bash(git diff:*), Edit",
		},
		Body: "Test agent.\n",
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

	out := string(writes[0].Data)

	// Verify all three tokens are present (round-trip completeness).
	for _, want := range []string{"Read", "Edit", "Bash(git diff:*)"} {
		if !contains(out, want) {
			t.Errorf("serialized output missing token %q:\n%s", want, out)
		}
	}

	// Document the known re-ordering: canonical natives (Read, Edit) appear before
	// the passthrough token (Bash(git diff:*)). Continue treats tools as an
	// unordered allowlist so this is semantically correct.
	readIdx := indexIn(out, "Read")
	editIdx := indexIn(out, "Edit")
	passthroughIdx := indexIn(out, "Bash(git diff:*)")
	if readIdx < 0 || editIdx < 0 || passthroughIdx < 0 {
		t.Fatal("one or more tokens not found; already caught above")
	}
	if passthroughIdx < readIdx || passthroughIdx < editIdx {
		t.Errorf("expected passthrough token after canonical-mapped natives; "+
			"got Read@%d Edit@%d Bash(git diff:*)@%d — if Continue becomes order-sensitive, implement index-preservation in ToCanonical",
			readIdx, editIdx, passthroughIdx)
	}
}

func indexIn(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
