package kilocode

import (
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolmapper"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolset"
)

// knownTools is the set of native tool names kilo-code understands in the
// permission.allow / permission.deny / permission.ask arrays.
// Source: internal/catalog/data/kilo-code/tools.json
var knownTools = toolset.New(
	"read", "edit", "bash", "glob", "grep", "task", "webfetch", "websearch", "todowrite", "todoread",
)

// SupportsTool reports whether the provider understands the given native tool name.
// Implements contract.ToolSupporter.
func (Provider) SupportsTool(tool string) bool { return knownTools.Contains(tool) }

// toolMap is the bidirectional native↔canonical mapping for kilo-code.
// Source: internal/catalog/data/kilo-code/tools.json
var toolMap = toolmapper.New([]toolmapper.Entry{
	{Native: "read", Canonical: "read_file"},
	{Native: "edit", Canonical: "file_edit"},
	{Native: "bash", Canonical: "bash"},
	{Native: "glob", Canonical: "glob"},
	{Native: "grep", Canonical: "grep"},
	{Native: "task", Canonical: "task_create"},
	{Native: "webfetch", Canonical: "web_fetch"},
	{Native: "websearch", Canonical: "web_search"},
	{Native: "todowrite", Canonical: "todo_write"},
	{Native: "todoread", Canonical: "todo_read"},
})

// CanonicalTool translates a native tool name to its canonical equivalent.
// Implements contract.ToolMapper.
func (Provider) CanonicalTool(native string) (string, bool) { return toolMap.CanonicalTool(native) }

// NativeTool translates a canonical tool name to this provider's native name.
// Implements contract.ToolMapper.
func (Provider) NativeTool(canonical string) (string, bool) { return toolMap.NativeTool(canonical) }

// Tools returns the sorted canonical names of all tools this provider supports.
// Implements contract.ToolMapper.
func (Provider) Tools() []string { return toolMap.Tools() }
