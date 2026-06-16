package githubcopilot

import (
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolmapper"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolset"
)

// knownTools is the set of native tool names this provider understands on disk.
// Implements contract.ToolSupporter. Names follow the GitHub Copilot documented
// primary tool aliases only.
// Source: internal/catalog/data/github-copilot/tools.json
var knownTools = toolset.New(
	// read group
	"read",
	// edit group
	"edit",
	// write group
	"write",
	// notebook group
	"NotebookEdit",
	// search group
	"search",
	// glob group
	"glob",
	// execute group
	"execute",
	// powershell group
	"powershell",
	// web group
	"web",
	// web_fetch group
	"web_fetch",
	// agent group
	"agent",
	// todo group
	"todo",
	// apply_patch
	"apply_patch",
)

// SupportsTool reports whether the provider understands the given native tool name.
// Implements contract.ToolSupporter.
func (Provider) SupportsTool(tool string) bool { return knownTools.Contains(tool) }

// toolMap is the bidirectional native↔canonical mapping for github-copilot.
//
// PRIMARY ALIAS ONLY (GitHub Copilot docs):
//
//	read        → canonical: read_file
//	edit        → canonical: file_edit
//	write       → canonical: file_write
//	NotebookEdit → canonical: notebook_edit
//	search      → canonical: grep
//	glob        → canonical: glob
//	execute     → canonical: bash
//	powershell  → canonical: powershell
//	web         → canonical: web_search
//	web_fetch   → canonical: web_fetch
//	agent       → canonical: task
//	todo        → canonical: todo_write
//	apply_patch → canonical: apply_patch
//
// Source: internal/catalog/data/github-copilot/tools.json
var toolMap = toolmapper.New([]toolmapper.Entry{
	{Native: "read",        Canonical: "read_file"},
	{Native: "edit",        Canonical: "file_edit"},
	{Native: "write",       Canonical: "file_write"},
	{Native: "NotebookEdit", Canonical: "notebook_edit"},
	{Native: "search",      Canonical: "grep"},
	{Native: "glob",        Canonical: "glob"},
	{Native: "execute",     Canonical: "bash"},
	{Native: "powershell",  Canonical: "powershell"},
	{Native: "web",         Canonical: "web_search"},
	{Native: "web_fetch",   Canonical: "web_fetch"},
	{Native: "agent",       Canonical: "task"},
	{Native: "todo",        Canonical: "todo_write"},
	{Native: "apply_patch", Canonical: "apply_patch"},
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
