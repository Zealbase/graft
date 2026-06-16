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
	// search group
	"search",
	// execute group
	"execute",
	// web group
	"web",
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
//	search      → canonical: grep
//	execute     → canonical: bash
//	web         → canonical: web_search
//	agent       → canonical: task
//	todo        → canonical: todo_write
//	apply_patch → canonical: apply_patch
//
// Source: internal/catalog/data/github-copilot/tools.json
var toolMap = toolmapper.New([]toolmapper.Entry{
	{Native: "read",        Canonical: "read_file"},
	{Native: "edit",        Canonical: "file_edit"},
	{Native: "search",      Canonical: "grep"},
	{Native: "execute",     Canonical: "bash"},
	{Native: "web",         Canonical: "web_search"},
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
