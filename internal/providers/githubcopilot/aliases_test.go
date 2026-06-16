package githubcopilot

import (
	"testing"
)

// TestAliasRoundTrip verifies that:
// 1. Secondary aliases parse to the correct canonical on input.
// 2. The PRIMARY alias is the only output on serialize (canonical→native).
func TestAliasRoundTrip(t *testing.T) {
	p := Provider{}

	// read group: primary=read, secondaries=Read,NotebookRead → canonical=read_file
	for _, secondary := range []string{"Read", "NotebookRead", "read"} {
		got, ok := p.CanonicalTool(secondary)
		if !ok || got != "read_file" {
			t.Errorf("CanonicalTool(%q) = (%q, %v); want (read_file, true)", secondary, got, ok)
		}
	}
	// Serialize: read_file → read (primary)
	if native, ok := p.NativeTool("read_file"); !ok || native != "read" {
		t.Errorf("NativeTool(read_file) = (%q, %v); want (read, true)", native, ok)
	}

	// edit group: primary=edit, secondaries=Edit,MultiEdit → canonical=file_edit
	for _, secondary := range []string{"Edit", "MultiEdit", "edit"} {
		got, ok := p.CanonicalTool(secondary)
		if !ok || got != "file_edit" {
			t.Errorf("CanonicalTool(%q) = (%q, %v); want (file_edit, true)", secondary, got, ok)
		}
	}
	// Serialize: file_edit → edit (primary)
	if native, ok := p.NativeTool("file_edit"); !ok || native != "edit" {
		t.Errorf("NativeTool(file_edit) = (%q, %v); want (edit, true)", native, ok)
	}

	// Write secondary maps to file_write
	for _, secondary := range []string{"Write", "write"} {
		got, ok := p.CanonicalTool(secondary)
		if !ok || got != "file_write" {
			t.Errorf("CanonicalTool(%q) = (%q, %v); want (file_write, true)", secondary, got, ok)
		}
	}
	// Serialize: file_write → write (primary)
	if native, ok := p.NativeTool("file_write"); !ok || native != "write" {
		t.Errorf("NativeTool(file_write) = (%q, %v); want (write, true)", native, ok)
	}

	// search group: primary=search, secondary=Grep → canonical=grep
	for _, secondary := range []string{"Grep", "search"} {
		got, ok := p.CanonicalTool(secondary)
		if !ok || got != "grep" {
			t.Errorf("CanonicalTool(%q) = (%q, %v); want (grep, true)", secondary, got, ok)
		}
	}
	// Serialize: grep → search (primary)
	if native, ok := p.NativeTool("grep"); !ok || native != "search" {
		t.Errorf("NativeTool(grep) = (%q, %v); want (search, true)", native, ok)
	}

	// execute group: primary=execute, secondaries=shell,Bash → canonical=bash
	for _, secondary := range []string{"shell", "Bash", "execute"} {
		got, ok := p.CanonicalTool(secondary)
		if !ok || got != "bash" {
			t.Errorf("CanonicalTool(%q) = (%q, %v); want (bash, true)", secondary, got, ok)
		}
	}
	// Serialize: bash → execute (primary)
	if native, ok := p.NativeTool("bash"); !ok || native != "execute" {
		t.Errorf("NativeTool(bash) = (%q, %v); want (execute, true)", native, ok)
	}

	// agent group: primary=agent, secondaries=Task,custom-agent → canonical=task
	for _, secondary := range []string{"Task", "custom-agent", "agent"} {
		got, ok := p.CanonicalTool(secondary)
		if !ok || got != "task" {
			t.Errorf("CanonicalTool(%q) = (%q, %v); want (task, true)", secondary, got, ok)
		}
	}
	// Serialize: task → agent (primary)
	if native, ok := p.NativeTool("task"); !ok || native != "agent" {
		t.Errorf("NativeTool(task) = (%q, %v); want (agent, true)", native, ok)
	}

	// todo group: primary=todo, secondary=TodoWrite → canonical=todo_write
	for _, secondary := range []string{"TodoWrite", "todo"} {
		got, ok := p.CanonicalTool(secondary)
		if !ok || got != "todo_write" {
			t.Errorf("CanonicalTool(%q) = (%q, %v); want (todo_write, true)", secondary, got, ok)
		}
	}
	// Serialize: todo_write → todo (primary)
	if native, ok := p.NativeTool("todo_write"); !ok || native != "todo" {
		t.Errorf("NativeTool(todo_write) = (%q, %v); want (todo, true)", native, ok)
	}

	// web group: primary=web, secondary=WebSearch → canonical=web_search
	for _, secondary := range []string{"WebSearch", "web"} {
		got, ok := p.CanonicalTool(secondary)
		if !ok || got != "web_search" {
			t.Errorf("CanonicalTool(%q) = (%q, %v); want (web_search, true)", secondary, got, ok)
		}
	}
	// Serialize: web_search → web (primary)
	if native, ok := p.NativeTool("web_search"); !ok || native != "web" {
		t.Errorf("NativeTool(web_search) = (%q, %v); want (web, true)", native, ok)
	}

	// web_fetch: primary=web_fetch, secondary=WebFetch
	got, ok := p.CanonicalTool("WebFetch")
	if !ok || got != "web_fetch" {
		t.Errorf("CanonicalTool(WebFetch) = (%q, %v); want (web_fetch, true)", got, ok)
	}
	if native, ok := p.NativeTool("web_fetch"); !ok || native != "web_fetch" {
		t.Errorf("NativeTool(web_fetch) = (%q, %v); want (web_fetch, true)", native, ok)
	}
}
