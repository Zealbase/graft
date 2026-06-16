package githubcopilot

import (
	"testing"
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
