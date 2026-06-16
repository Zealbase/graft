package continueprov

import (
	"testing"
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
