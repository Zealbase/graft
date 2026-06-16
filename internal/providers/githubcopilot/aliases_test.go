package githubcopilot

import (
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// TestAliasRoundTrip verifies that primary aliases parse to the correct
// canonical on input, and that canonical→native emits the primary alias.
func TestAliasRoundTrip(t *testing.T) {
	p := Provider{}

	// read → read_file
	if got, ok := p.CanonicalTool("read"); !ok || got != "read_file" {
		t.Errorf("CanonicalTool(%q) = (%q, %v); want (read_file, true)", "read", got, ok)
	}
	if native, ok := p.NativeTool("read_file"); !ok || native != "read" {
		t.Errorf("NativeTool(read_file) = (%q, %v); want (read, true)", native, ok)
	}

	// edit → file_edit
	if got, ok := p.CanonicalTool("edit"); !ok || got != "file_edit" {
		t.Errorf("CanonicalTool(%q) = (%q, %v); want (file_edit, true)", "edit", got, ok)
	}
	if native, ok := p.NativeTool("file_edit"); !ok || native != "edit" {
		t.Errorf("NativeTool(file_edit) = (%q, %v); want (edit, true)", native, ok)
	}

	// search → grep
	if got, ok := p.CanonicalTool("search"); !ok || got != "grep" {
		t.Errorf("CanonicalTool(%q) = (%q, %v); want (grep, true)", "search", got, ok)
	}
	if native, ok := p.NativeTool("grep"); !ok || native != "search" {
		t.Errorf("NativeTool(grep) = (%q, %v); want (search, true)", native, ok)
	}

	// execute → bash
	if got, ok := p.CanonicalTool("execute"); !ok || got != "bash" {
		t.Errorf("CanonicalTool(%q) = (%q, %v); want (bash, true)", "execute", got, ok)
	}
	if native, ok := p.NativeTool("bash"); !ok || native != "execute" {
		t.Errorf("NativeTool(bash) = (%q, %v); want (execute, true)", native, ok)
	}

	// web → web_search
	if got, ok := p.CanonicalTool("web"); !ok || got != "web_search" {
		t.Errorf("CanonicalTool(%q) = (%q, %v); want (web_search, true)", "web", got, ok)
	}
	if native, ok := p.NativeTool("web_search"); !ok || native != "web" {
		t.Errorf("NativeTool(web_search) = (%q, %v); want (web, true)", native, ok)
	}

	// agent → task
	if got, ok := p.CanonicalTool("agent"); !ok || got != "task" {
		t.Errorf("CanonicalTool(%q) = (%q, %v); want (task, true)", "agent", got, ok)
	}
	if native, ok := p.NativeTool("task"); !ok || native != "agent" {
		t.Errorf("NativeTool(task) = (%q, %v); want (agent, true)", native, ok)
	}

	// todo → todo_write
	if got, ok := p.CanonicalTool("todo"); !ok || got != "todo_write" {
		t.Errorf("CanonicalTool(%q) = (%q, %v); want (todo_write, true)", "todo", got, ok)
	}
	if native, ok := p.NativeTool("todo_write"); !ok || native != "todo" {
		t.Errorf("NativeTool(todo_write) = (%q, %v); want (todo, true)", native, ok)
	}

	// apply_patch → apply_patch
	if got, ok := p.CanonicalTool("apply_patch"); !ok || got != "apply_patch" {
		t.Errorf("CanonicalTool(%q) = (%q, %v); want (apply_patch, true)", "apply_patch", got, ok)
	}
	if native, ok := p.NativeTool("apply_patch"); !ok || native != "apply_patch" {
		t.Errorf("NativeTool(apply_patch) = (%q, %v); want (apply_patch, true)", native, ok)
	}
}

// TestDroppedCanonicals verifies that the five canonical tools dropped in
// commit ee8d022 are present and map correctly in both directions.
func TestDroppedCanonicals(t *testing.T) {
	p := Provider{}

	cases := []struct {
		native    string
		canonical string
	}{
		{"write",        "file_write"},
		{"glob",         "glob"},
		{"web_fetch",    "web_fetch"},
		{"NotebookEdit", "notebook_edit"},
		{"powershell",   "powershell"},
	}

	for _, tc := range cases {
		// forward: native → canonical
		if got, ok := p.CanonicalTool(tc.native); !ok || got != tc.canonical {
			t.Errorf("CanonicalTool(%q) = (%q, %v); want (%q, true)", tc.native, got, ok, tc.canonical)
		}
		// reverse: canonical → native
		if got, ok := p.NativeTool(tc.canonical); !ok || got != tc.native {
			t.Errorf("NativeTool(%q) = (%q, %v); want (%q, true)", tc.canonical, got, ok, tc.native)
		}
	}
}

// TestSerialize_NeverEmitsSecondaryAlias verifies that Serialize maps canonical
// tool names to primary native aliases only, never to secondary aliases.
func TestSerialize_NeverEmitsSecondaryAlias(t *testing.T) {
	p := Provider{}

	a := contract.CanonicalAgent{
		Name:  "test-agent",
		Tools: []string{"file_write", "glob", "web_fetch", "notebook_edit", "powershell", "file_edit", "read_file", "bash", "web_search"},
	}

	files, err := p.Serialize(a)
	if err != nil {
		t.Fatalf("Serialize returned error: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("Serialize returned no files")
	}

	out := string(files[0].Data)

	// Secondary aliases that must NOT appear in the output as standalone tool entries.
	// We match "- <alias>" as a YAML list item to avoid false positives from
	// secondary names that are substrings of valid primary names
	// (e.g. "Edit" inside "NotebookEdit", "shell" inside "powershell").
	secondary := []string{"Read", "Write", "Edit", "MultiEdit", "Grep", "Glob", "WebFetch", "WebSearch", "Bash", "shell", "NotebookRead", "Task", "TodoWrite", "custom-agent"}
	for _, s := range secondary {
		needle := "- " + s
		if strings.Contains(out, needle) {
			t.Errorf("output contains secondary alias %q as a tool entry; want primary only\noutput:\n%s", s, out)
		}
	}

	// Primary native names that MUST appear
	primary := []string{"write", "glob", "web_fetch", "NotebookEdit", "edit", "read", "execute", "web"}
	for _, prim := range primary {
		if !strings.Contains(out, prim) {
			t.Errorf("output missing primary alias %q\noutput:\n%s", prim, out)
		}
	}
}
