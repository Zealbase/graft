package cursor

import (
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolmapper"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolset"
)

// knownTools is the set of native tool names this provider understands on disk.
// Implements contract.ToolSupporter. Native names are lowercase_snake_case for cursor.
// Source: internal/catalog/data/cursor/tools.json
var knownTools = toolset.New(
	"list_dir", "codebase_search", "read_file", "run_terminal_command",
	"grep_search", "file_search", "edit_file", "delete_file",
	"web_search", "browser", "image_generation", "ask_questions",
)

// SupportsTool reports whether the provider understands the given native tool name.
// Implements contract.ToolSupporter.
func (Provider) SupportsTool(tool string) bool { return knownTools.Contains(tool) }

// toolMap is the bidirectional native↔canonical mapping for cursor.
// Source: internal/catalog/data/cursor/tools.json
var toolMap = toolmapper.New([]toolmapper.Entry{
	{Native: "list_dir", Canonical: "list_directory"},
	{Native: "codebase_search", Canonical: "semantic_search"},
	{Native: "read_file", Canonical: "read_file"},
	{Native: "run_terminal_command", Canonical: "bash"},
	{Native: "grep_search", Canonical: "grep"},
	{Native: "file_search", Canonical: "file_search"},
	{Native: "edit_file", Canonical: "file_edit"},
	{Native: "delete_file", Canonical: "delete_file"},
	{Native: "web_search", Canonical: "web_search"},
	{Native: "browser", Canonical: "browser"},
	{Native: "image_generation", Canonical: "image_generation"},
	{Native: "ask_questions", Canonical: "ask_user_question"},
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
