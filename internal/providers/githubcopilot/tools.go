package githubcopilot

import (
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolmapper"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolset"
)

// knownTools is the set of native tool names this provider understands on disk.
// Implements contract.ToolSupporter. Names follow the GitHub Copilot documented
// tool aliases (primary + compatible/secondary, case-insensitive).
// Source: internal/catalog/data/github-copilot/tools.json
var knownTools = toolset.New(
	// read group
	"read", "Read", "NotebookRead",
	// edit group
	"edit", "Edit", "MultiEdit",
	// write
	"write", "Write",
	// notebook
	"NotebookEdit",
	// search group
	"search", "Grep",
	// glob
	"glob", "Glob",
	// execute group
	"execute", "shell", "Bash",
	// powershell
	"powershell",
	// web group
	"web", "WebSearch",
	// web_fetch
	"web_fetch", "WebFetch",
	// agent group
	"agent", "Task", "custom-agent",
	// todo group
	"todo", "TodoWrite",
	// apply_patch
	"apply_patch",
)

// SupportsTool reports whether the provider understands the given native tool name.
// Implements contract.ToolSupporter.
func (Provider) SupportsTool(tool string) bool { return knownTools.Contains(tool) }

// toolMap is the bidirectional native↔canonical mapping for github-copilot.
//
// PRIMARY ALIAS SEMANTICS (GitHub Copilot docs):
// On Serialize (canonical→native), graft emits the PRIMARY copilot alias.
// On Parse (native→canonical), ALL secondary aliases are recognized.
// "First-entry-wins" in toolmapper.New gives the correct primary on reverse lookup.
// Native-side lookup is case-insensitive (strings.ToLower), so "Read" and "read"
// are the same key — only the first occurrence wins for the primary.
//
// Documented alias groups (primary → secondaries):
//
//	read    ← Read, NotebookRead        → canonical: read_file
//	edit    ← Edit, MultiEdit           → canonical: file_edit
//	write   ← Write                     → canonical: file_write  (own canonical)
//	NotebookEdit                        → canonical: notebook_edit
//	search  ← Grep                      → canonical: grep
//	glob    ← Glob                      → canonical: glob
//	execute ← shell, Bash               → canonical: bash
//	powershell                          → canonical: powershell
//	web     ← WebSearch                 → canonical: web_search
//	web_fetch ← WebFetch                → canonical: web_fetch
//	agent   ← Task, custom-agent        → canonical: task
//	todo    ← TodoWrite                 → canonical: todo_write
//
// Source: internal/catalog/data/github-copilot/tools.json
var toolMap = toolmapper.New([]toolmapper.Entry{
	// read group: "read" is primary → read_file
	{Native: "read",         Canonical: "read_file"},  // PRIMARY
	{Native: "Read",         Canonical: "read_file"},  // secondary alias (same key after ToLower)
	{Native: "NotebookRead", Canonical: "read_file"},  // secondary alias

	// edit group: "edit" is primary → file_edit
	{Native: "edit",      Canonical: "file_edit"},  // PRIMARY
	{Native: "Edit",      Canonical: "file_edit"},  // secondary alias
	{Native: "MultiEdit", Canonical: "file_edit"},  // secondary alias

	// write: "write" maps to file_write (own canonical)
	{Native: "write", Canonical: "file_write"},  // PRIMARY
	{Native: "Write", Canonical: "file_write"},  // secondary alias (same key after ToLower)

	// NotebookEdit: own canonical notebook_edit
	{Native: "NotebookEdit", Canonical: "notebook_edit"},  // PRIMARY

	// search group: "search" is primary → grep
	{Native: "search", Canonical: "grep"},  // PRIMARY
	{Native: "Grep",   Canonical: "grep"},  // secondary alias

	// glob: own canonical
	{Native: "glob", Canonical: "glob"},  // PRIMARY
	{Native: "Glob", Canonical: "glob"},  // secondary alias (same key after ToLower)

	// execute group: "execute" is primary → bash
	{Native: "execute", Canonical: "bash"},  // PRIMARY
	{Native: "shell",   Canonical: "bash"},  // secondary alias
	{Native: "Bash",    Canonical: "bash"},  // secondary alias

	// powershell: own canonical
	{Native: "powershell", Canonical: "powershell"},  // PRIMARY

	// web group: "web" is primary → web_search
	{Native: "web",       Canonical: "web_search"},  // PRIMARY
	{Native: "WebSearch", Canonical: "web_search"},  // secondary alias

	// web_fetch: own canonical
	{Native: "web_fetch", Canonical: "web_fetch"},  // PRIMARY
	{Native: "WebFetch",  Canonical: "web_fetch"},  // secondary alias

	// agent group: "agent" is primary → task
	{Native: "agent",        Canonical: "task"},  // PRIMARY
	{Native: "Task",         Canonical: "task"},  // secondary alias
	{Native: "custom-agent", Canonical: "task"},  // secondary alias

	// todo group: "todo" is primary → todo_write
	{Native: "todo",      Canonical: "todo_write"},  // PRIMARY
	{Native: "TodoWrite", Canonical: "todo_write"},  // secondary alias

	// apply_patch: own canonical
	{Native: "apply_patch", Canonical: "apply_patch"},  // PRIMARY
})

// CanonicalTool translates a native tool name to its canonical equivalent.
// Implements contract.ToolMapper. Lookup is case-insensitive.
func (Provider) CanonicalTool(native string) (string, bool) { return toolMap.CanonicalTool(native) }

// NativeTool translates a canonical tool name to this provider's native name.
// Implements contract.ToolMapper.
func (Provider) NativeTool(canonical string) (string, bool) { return toolMap.NativeTool(canonical) }

// Tools returns the sorted canonical names of all tools this provider supports.
// Implements contract.ToolMapper.
func (Provider) Tools() []string { return toolMap.Tools() }
