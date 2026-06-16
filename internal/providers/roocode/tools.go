package roocode

import (
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolmapper"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolset"
)

// TODO(follow-up): .roo/commands/ slash-command discovery (GAP4) and
// .roo/rules / .roorules rules-dir support (GAP5) are out of scope for this
// change; tracked as follow-up work.

// knownTools is the set of native tool names this provider understands on disk.
// Implements contract.ToolSupporter. Native names are lowercase for roo-code.
// Note: the "browser" permission group was removed — it is listed in
// deprecatedToolGroups in packages/types/src/tool.ts (RooCodeInc/Roo-Code).
// Source: internal/catalog/data/roo-code/tools.json
var knownTools = toolset.New(
	"read", "edit", "command", "mcp", "modes",
)

// SupportsTool reports whether the provider understands the given native tool name.
// Implements contract.ToolSupporter.
func (Provider) SupportsTool(tool string) bool { return knownTools.Contains(tool) }

// toolMap is the bidirectional native↔canonical mapping for roo-code.
// Source: internal/catalog/data/roo-code/tools.json
var toolMap = toolmapper.New([]toolmapper.Entry{
	{Native: "read", Canonical: "read_file"},
	{Native: "edit", Canonical: "file_edit"},
	{Native: "command", Canonical: "bash"},
	{Native: "mcp", Canonical: "mcp"},
	{Native: "modes", Canonical: "task"},
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
