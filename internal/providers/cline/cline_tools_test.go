package clineprov

import (
	"testing"
)

// TestToolMapRoundTrip verifies that each native→canonical→native pair is consistent.
func TestToolMapRoundTrip(t *testing.T) {
	entries := []struct{ native, canonical string }{
		{"read_file", "read_file"},
		{"write_to_file", "file_write"},
		{"replace_in_file", "file_edit"},
		{"execute_command", "bash"},
		{"search_files", "grep"},
		{"list_files", "list_directory"},
		{"browser_action", "browser"},
		{"use_mcp_tool", "mcp"},
		{"web_fetch", "web_fetch"},
		{"web_search", "web_search"},
		{"new_task", "task"},
		{"apply_patch", "apply_patch"},
		{"use_skill", "skill"},
	}
	for _, e := range entries {
		c, ok := toolMap.CanonicalTool(e.native)
		if !ok || c != e.canonical {
			t.Errorf("native %q → canonical: got (%q, %v), want (%q, true)", e.native, c, ok, e.canonical)
		}
	}
}

// TestAccessMCPResourceFirstEntryWins verifies that access_mcp_resource maps to mcp
// and the canonical→native direction returns use_mcp_tool (first entry wins).
func TestAccessMCPResourceFirstEntryWins(t *testing.T) {
	c, ok := toolMap.CanonicalTool("access_mcp_resource")
	if !ok || c != "mcp" {
		t.Errorf("access_mcp_resource → canonical: got (%q, %v), want (\"mcp\", true)", c, ok)
	}
	// canonical→native: first entry (use_mcp_tool) wins
	n, ok := toolMap.NativeTool("mcp")
	if !ok || n != "use_mcp_tool" {
		t.Errorf("mcp → native: got (%q, %v), want (\"use_mcp_tool\", true)", n, ok)
	}
}

// TestUnmappedNativePassthrough verifies that unknown native tool names pass through unchanged.
func TestUnmappedNativePassthrough(t *testing.T) {
	unknowns := []string{"list_code_definition_names", "plan_mode_respond", "ask_followup_question"}
	for _, u := range unknowns {
		got := toolMap.MapToCanonical([]string{u})
		if len(got) != 1 || got[0] != u {
			t.Errorf("MapToCanonical(%q) = %v; want [%q] (passthrough)", u, got, u)
		}
	}
}

// TestSupportsTool verifies SupportsTool returns true for known native names and false for unknown.
func TestSupportsTool(t *testing.T) {
	p := Provider{}
	known := []string{"read_file", "write_to_file", "execute_command", "use_mcp_tool", "use_skill"}
	for _, k := range known {
		if !p.SupportsTool(k) {
			t.Errorf("SupportsTool(%q) = false, want true", k)
		}
	}
	unknown := []string{"Read", "bash", "file_edit", "does_not_exist"}
	for _, u := range unknown {
		if p.SupportsTool(u) {
			t.Errorf("SupportsTool(%q) = true, want false", u)
		}
	}
}

// TestMapToNative verifies canonical→native translation for serialization.
func TestMapToNative(t *testing.T) {
	cases := []struct{ canonical, wantNative string }{
		{"read_file", "read_file"},
		{"file_write", "write_to_file"},
		{"file_edit", "replace_in_file"},
		{"bash", "execute_command"},
		{"grep", "search_files"},
		{"list_directory", "list_files"},
		{"browser", "browser_action"},
		{"mcp", "use_mcp_tool"},
		{"web_fetch", "web_fetch"},
		{"web_search", "web_search"},
		{"task", "new_task"},
		{"apply_patch", "apply_patch"},
		{"skill", "use_skill"},
	}
	for _, tc := range cases {
		n, ok := toolMap.NativeTool(tc.canonical)
		if !ok || n != tc.wantNative {
			t.Errorf("NativeTool(%q) = (%q, %v); want (%q, true)", tc.canonical, n, ok, tc.wantNative)
		}
	}
}
