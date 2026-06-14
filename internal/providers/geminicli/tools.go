package geminicli

import (
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolmapper"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolset"
)

// knownTools is the set of native tool names accepted by SupportsTool for
// gemini-cli. The transformer translates canonical names to native via ToolMapper
// before calling SupportsTool, so this set uses gemini-cli's native names.
// Implements contract.ToolSupporter.
// Source: provider-model-tool-sources.md gemini-cli row.
var knownTools = toolset.New(
	"run_shell_command",
	"read_file",
	"write_file",
	"google_web_search",
	"web_fetch",
)

// SupportsTool reports whether the provider understands the given tool name as
// stored in CanonicalAgent.Tools. Implements contract.ToolSupporter.
func (Provider) SupportsTool(tool string) bool { return knownTools.Contains(tool) }

// toolMap is the bidirectional native↔canonical mapping for gemini-cli.
// Note: both "edit" and "replace" map to canonical "file_edit"; "edit" wins
// for the canonical→native direction (first entry).
// Source: internal/catalog/data/gemini-cli/tools.json
var toolMap = toolmapper.New([]toolmapper.Entry{
	{Native: "read_file", Canonical: "read_file"},
	{Native: "write_file", Canonical: "file_write"},
	{Native: "edit", Canonical: "file_edit"},
	{Native: "read_many_files", Canonical: "read_many_files"},
	{Native: "list_directory", Canonical: "list_directory"},
	{Native: "glob", Canonical: "glob"},
	{Native: "search_file_content", Canonical: "grep"},
	{Native: "replace", Canonical: "file_edit"}, // alias: edit wins for reverse lookup
	{Native: "run_shell_command", Canonical: "bash"},
	{Native: "web_fetch", Canonical: "web_fetch"},
	{Native: "google_web_search", Canonical: "web_search"},
	{Native: "save_memory", Canonical: "save_memory"},
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
