package opencode

import (
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolmapper"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolset"
)

// knownTools is the set of native tool names this provider understands on disk.
// Implements contract.ToolSupporter. Native names are lowercase for opencode.
// Sources:
//   - Core set (confirmed): provider-model-tool-sources.md opencode row +
//     https://opencode.ai/docs/agents/ tool permission reference.
//   - "question", "todowrite", "todoread", "apply_patch": confirmed in opencode
//     tool registry. Marked provisional — opencode evolves quickly.
//
// Source: internal/catalog/data/opencode/tools.json
var knownTools = toolset.New(
	"read", "edit", "glob", "grep", "list", "bash", "task",
	"external_directory", "lsp", "skill", "webfetch", "websearch",
	"todowrite", "todoread", "apply_patch", "question",
)

// SupportsTool reports whether the provider understands the given native tool name.
// Implements contract.ToolSupporter.
func (Provider) SupportsTool(tool string) bool { return knownTools.Contains(tool) }

// toolMap is the bidirectional native↔canonical mapping for opencode.
// Source: internal/catalog/data/opencode/tools.json
var toolMap = toolmapper.New([]toolmapper.Entry{
	{Native: "read", Canonical: "read_file"},
	{Native: "edit", Canonical: "file_edit"},
	{Native: "glob", Canonical: "glob"},
	{Native: "grep", Canonical: "grep"},
	{Native: "list", Canonical: "list_directory"},
	{Native: "bash", Canonical: "bash"},
	{Native: "task", Canonical: "task"},
	{Native: "external_directory", Canonical: "external_directory"},
	{Native: "lsp", Canonical: "lsp"},
	{Native: "skill", Canonical: "skill"},
	{Native: "webfetch", Canonical: "web_fetch"},
	{Native: "websearch", Canonical: "web_search"},
	{Native: "todowrite", Canonical: "todo_write"},
	{Native: "todoread", Canonical: "todo_read"},
	{Native: "apply_patch", Canonical: "apply_patch"},
	{Native: "question", Canonical: "ask_user_question"},
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
