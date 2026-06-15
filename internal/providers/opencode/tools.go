package opencode

import (
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolmapper"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolset"
)

// knownTools is the set of native tool names this provider understands on disk.
// Implements contract.ToolSupporter. Native names are lowercase for opencode.
// Sources:
//   - Core set (confirmed): sst/opencode registry.ts (packages/opencode/src/tool/registry.ts).
//   - "question", "todowrite", "apply_patch": confirmed in opencode tool registry.
//     Marked provisional — opencode evolves quickly.
//
// Source: internal/catalog/data/opencode/tools.json
var knownTools = toolset.New(
	"read", "edit", "glob", "grep", "bash", "task",
	"lsp", "skill", "webfetch", "websearch",
	"todowrite", "apply_patch", "question", "write",
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
	{Native: "bash", Canonical: "bash"},
	{Native: "task", Canonical: "task"},
	{Native: "lsp", Canonical: "lsp"},
	{Native: "skill", Canonical: "skill"},
	{Native: "webfetch", Canonical: "web_fetch"},
	{Native: "websearch", Canonical: "web_search"},
	{Native: "todowrite", Canonical: "todo_write"},
	{Native: "apply_patch", Canonical: "apply_patch"},
	{Native: "question", Canonical: "ask_user_question"},
	{Native: "write", Canonical: "file_write"},
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
