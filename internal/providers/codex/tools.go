package codex

import (
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolmapper"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolset"
)

// knownTools is the set of native tool names this provider understands on disk.
// Implements contract.ToolSupporter. Native names are lowercase_snake_case for codex.
// Source: internal/catalog/data/codex/tools.json
var knownTools = toolset.New(
	"shell", "web_search", "apply_patch", "image_generation",
	"computer_use", "code_review", "tool_search", "spawn_agent", "view_image",
)

// SupportsTool reports whether the provider understands the given native tool name.
// Implements contract.ToolSupporter.
func (Provider) SupportsTool(tool string) bool { return knownTools.Contains(tool) }

// toolMap is the bidirectional native↔canonical mapping for codex.
// Source: internal/catalog/data/codex/tools.json
var toolMap = toolmapper.New([]toolmapper.Entry{
	{Native: "shell", Canonical: "bash"},
	{Native: "web_search", Canonical: "web_search"},
	{Native: "apply_patch", Canonical: "apply_patch"},
	{Native: "image_generation", Canonical: "image_generation"},
	{Native: "computer_use", Canonical: "computer_use"},
	{Native: "code_review", Canonical: "code_review"},
	{Native: "tool_search", Canonical: "tool_search"},
	{Native: "spawn_agent", Canonical: "spawn_agent"},
	{Native: "view_image", Canonical: "view_image"},
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
