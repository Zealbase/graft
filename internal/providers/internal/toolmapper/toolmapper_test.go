package toolmapper_test

import (
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolmapper"
)

func TestNew_caseInsensitiveLookup(t *testing.T) {
	m := toolmapper.New([]toolmapper.Entry{
		{Native: "WebSearch", Canonical: "web_search"},
		{Native: "Read", Canonical: "read_file"},
	})
	for _, tc := range []struct{ input, want string }{
		{"WebSearch", "web_search"},
		{"websearch", "web_search"},
		{"WEBSEARCH", "web_search"},
		{"Read", "read_file"},
		{"read", "read_file"},
	} {
		got, ok := m.CanonicalTool(tc.input)
		if !ok || got != tc.want {
			t.Errorf("CanonicalTool(%q) = (%q, %v); want (%q, true)", tc.input, got, ok, tc.want)
		}
	}
}

func TestNew_unknownReturnsNotOK(t *testing.T) {
	m := toolmapper.New([]toolmapper.Entry{
		{Native: "Bash", Canonical: "bash"},
	})
	if _, ok := m.CanonicalTool("unknown_tool"); ok {
		t.Error("expected ok=false for unknown native tool")
	}
	if _, ok := m.NativeTool("unknown_canonical"); ok {
		t.Error("expected ok=false for unknown canonical tool")
	}
}

func TestNew_aliasFirstWins(t *testing.T) {
	// Two natives map to the same canonical; first one wins for reverse lookup.
	m := toolmapper.New([]toolmapper.Entry{
		{Native: "edit", Canonical: "file_edit"},
		{Native: "replace", Canonical: "file_edit"},
	})
	native, ok := m.NativeTool("file_edit")
	if !ok || native != "edit" {
		t.Errorf("NativeTool(file_edit) = (%q, %v); want (edit, true)", native, ok)
	}
	// Both aliases resolve forward
	for _, alias := range []string{"edit", "replace"} {
		c, ok := m.CanonicalTool(alias)
		if !ok || c != "file_edit" {
			t.Errorf("CanonicalTool(%q) = (%q, %v); want (file_edit, true)", alias, c, ok)
		}
	}
}

func TestNew_toolsAreSorted(t *testing.T) {
	m := toolmapper.New([]toolmapper.Entry{
		{Native: "z_tool", Canonical: "zzz"},
		{Native: "a_tool", Canonical: "aaa"},
		{Native: "m_tool", Canonical: "mmm"},
	})
	tools := m.Tools()
	for i := 1; i < len(tools); i++ {
		if tools[i] < tools[i-1] {
			t.Errorf("Tools() not sorted at index %d: %q < %q", i, tools[i], tools[i-1])
		}
	}
}

func TestNew_toolsReturnsCopy(t *testing.T) {
	m := toolmapper.New([]toolmapper.Entry{
		{Native: "Bash", Canonical: "bash"},
	})
	t1 := m.Tools()
	t1[0] = "mutated"
	t2 := m.Tools()
	if t2[0] == "mutated" {
		t.Error("Tools() returned a reference to internal state (not a copy)")
	}
}

func TestRoundTrip(t *testing.T) {
	entries := []toolmapper.Entry{
		{Native: "Read", Canonical: "read_file"},
		{Native: "Write", Canonical: "file_write"},
		{Native: "Edit", Canonical: "file_edit"},
		{Native: "Bash", Canonical: "bash"},
		{Native: "WebSearch", Canonical: "web_search"},
	}
	m := toolmapper.New(entries)
	for _, e := range entries {
		// native → canonical → native
		c, ok := m.CanonicalTool(e.Native)
		if !ok {
			t.Errorf("CanonicalTool(%q) not found", e.Native)
			continue
		}
		n, ok := m.NativeTool(c)
		if !ok {
			t.Errorf("NativeTool(%q) not found (from %q)", c, e.Native)
			continue
		}
		if n != e.Native {
			t.Errorf("round-trip %q → %q → %q; want %q", e.Native, c, n, e.Native)
		}
	}
}
