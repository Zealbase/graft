package clineprov

import (
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolmapper"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolset"
)

// knownTools is the set of native tool names cline understands.
var knownTools = toolset.New(
	"read_file", "write_to_file", "replace_in_file", "execute_command",
	"search_files", "list_files", "list_code_definition_names",
	"browser_action", "use_mcp_tool", "access_mcp_resource",
	"load_mcp_documentation", "new_task", "plan_mode_respond", "act_mode_respond",
	"focus_chain", "web_fetch", "web_search", "condense", "summarize_task",
	"report_bug", "new_rule", "apply_patch", "generate_explanation",
	"use_skill", "use_subagents", "ask_followup_question", "attempt_completion",
)

// SupportsTool reports whether the provider understands the given native tool name.
func (Provider) SupportsTool(tool string) bool { return knownTools.Contains(tool) }

// toolMap is the bidirectional native↔canonical mapping for cline.
var toolMap = toolmapper.New([]toolmapper.Entry{
	{Native: "read_file",           Canonical: "read_file"},
	{Native: "write_to_file",       Canonical: "file_write"},
	{Native: "replace_in_file",     Canonical: "file_edit"},
	{Native: "execute_command",     Canonical: "bash"},
	{Native: "search_files",        Canonical: "grep"},
	{Native: "list_files",          Canonical: "list_directory"},
	{Native: "browser_action",      Canonical: "browser"},
	{Native: "use_mcp_tool",        Canonical: "mcp"},
	{Native: "access_mcp_resource", Canonical: "mcp"},
	{Native: "web_fetch",           Canonical: "web_fetch"},
	{Native: "web_search",          Canonical: "web_search"},
	{Native: "new_task",            Canonical: "task"},
	{Native: "apply_patch",         Canonical: "apply_patch"},
	{Native: "use_skill",           Canonical: "skill"},
})

// CanonicalTool translates a native tool name to its canonical equivalent.
func (Provider) CanonicalTool(native string) (string, bool) { return toolMap.CanonicalTool(native) }

// NativeTool translates a canonical tool name to this provider's native name.
func (Provider) NativeTool(canonical string) (string, bool) { return toolMap.NativeTool(canonical) }

// Tools returns the sorted canonical names of all tools this provider supports.
func (Provider) Tools() []string { return toolMap.Tools() }
