package grokcli

import (
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolmapper"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolset"
)

// knownTools is the set of native tool names this provider understands on disk.
// Implements contract.ToolSupporter. Native names are lowercase_snake_case for grok-cli.
// Source: internal/catalog/data/grok-cli/tools.json
var knownTools = toolset.New(
	"search_x", "search_web", "generate_image", "generate_video",
	"task", "delegate", "computer",
)

// SupportsTool reports whether the provider understands the given native tool name.
// Implements contract.ToolSupporter.
func (Provider) SupportsTool(tool string) bool { return knownTools.Contains(tool) }

// toolMap is the bidirectional native↔canonical mapping for grok-cli.
// Note: "search_web" maps to canonical "web_search" (same logical operation).
// "generate_image" maps to canonical "image_generation".
// "computer" maps to canonical "computer_use".
// Source: internal/catalog/data/grok-cli/tools.json
var toolMap = toolmapper.New([]toolmapper.Entry{
	{Native: "search_x", Canonical: "search_x"},
	{Native: "search_web", Canonical: "web_search"},
	{Native: "generate_image", Canonical: "image_generation"},
	{Native: "generate_video", Canonical: "generate_video"},
	{Native: "task", Canonical: "task"},
	{Native: "delegate", Canonical: "delegate"},
	{Native: "computer", Canonical: "computer_use"},
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
