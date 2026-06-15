package antigravity

import (
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolmapper"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolset"
)

// knownTools is the set of native tool names this provider understands on disk.
// Implements contract.ToolSupporter. Native names are lowercase_snake_case for antigravity.
// Source: provider-model-tool-sources.md antigravity row.
// Note: antigravity (gemini-cli fork / Google AI Studio agent) shares most of
// gemini-cli's toolset but exposes a limited subset in its agent files.
var knownTools = toolset.New(
	"bash", "edit_file", "list_dir", "web_search",
)

// SupportsTool reports whether the provider understands the given native tool name.
// Implements contract.ToolSupporter.
func (Provider) SupportsTool(tool string) bool { return knownTools.Contains(tool) }

// toolMap is the bidirectional native↔canonical mapping for antigravity.
// Source: provider-model-tool-sources.md antigravity row.
var toolMap = toolmapper.New([]toolmapper.Entry{
	{Native: "bash", Canonical: "bash"},
	{Native: "edit_file", Canonical: "file_edit"},
	{Native: "list_dir", Canonical: "list_directory"},
	{Native: "web_search", Canonical: "web_search"},
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
