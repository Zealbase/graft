package continueprov

import (
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolmapper"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolset"
)

var knownTools = toolset.New(
	"Read", "Write", "Edit", "MultiEdit", "Bash", "Glob", "Search", "List",
	"Fetch", "fetch_url", "web_search", "codebase_search",
	"AskQuestion", "Diff", "Status", "Checklist",
)

func (Provider) SupportsTool(tool string) bool { return knownTools.Contains(tool) }

var toolMap = toolmapper.New([]toolmapper.Entry{
	{Native: "Read", Canonical: "read_file"},
	{Native: "Write", Canonical: "file_write"},
	{Native: "Edit", Canonical: "file_edit"},
	{Native: "MultiEdit", Canonical: "file_edit"},
	{Native: "Bash", Canonical: "bash"},
	{Native: "Glob", Canonical: "glob"},
	{Native: "Search", Canonical: "grep"},
	{Native: "List", Canonical: "list_directory"},
	{Native: "Fetch", Canonical: "web_fetch"},
	{Native: "fetch_url", Canonical: "web_fetch"},
	{Native: "web_search", Canonical: "web_search"},
	{Native: "codebase_search", Canonical: "codebase_search"},
	{Native: "AskQuestion", Canonical: "ask_user_question"},
	{Native: "Diff", Canonical: "view_diff"},
	{Native: "Status", Canonical: "status"},
	{Native: "Checklist", Canonical: "checklist"},
})

func (Provider) CanonicalTool(native string) (string, bool) { return toolMap.CanonicalTool(native) }
func (Provider) NativeTool(canonical string) (string, bool) { return toolMap.NativeTool(canonical) }
func (Provider) Tools() []string                             { return toolMap.Tools() }
