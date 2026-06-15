package githubcopilot

import (
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolmapper"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolset"
)

// knownTools is the set of native tool names this provider understands on disk.
// Implements contract.ToolSupporter. Native names are lowercase_snake_case for github-copilot.
// Source: internal/catalog/data/github-copilot/tools.json
var knownTools = toolset.New(
	"bash", "view", "grep", "glob", "web_fetch", "apply_patch", "task", "rg", "read", "write",
)

// SupportsTool reports whether the provider understands the given native tool name.
// Implements contract.ToolSupporter.
func (Provider) SupportsTool(tool string) bool { return knownTools.Contains(tool) }

// toolMap is the bidirectional native↔canonical mapping for github-copilot.
// Note: both "view" and "read" map to canonical "read_file"; "view" wins for
// the canonical→native direction (first entry). Similarly "grep" and "rg" both
// map to canonical "grep"; "grep" wins for reverse lookup.
// Source: internal/catalog/data/github-copilot/tools.json
var toolMap = toolmapper.New([]toolmapper.Entry{
	{Native: "bash", Canonical: "bash"},
	{Native: "view", Canonical: "read_file"},
	{Native: "grep", Canonical: "grep"},
	{Native: "glob", Canonical: "glob"},
	{Native: "web_fetch", Canonical: "web_fetch"},
	{Native: "apply_patch", Canonical: "apply_patch"},
	{Native: "task", Canonical: "task"},
	{Native: "rg", Canonical: "grep"},   // alias: grep wins for reverse lookup
	{Native: "read", Canonical: "read_file"}, // alias: view wins for reverse lookup
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
