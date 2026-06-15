package goose

import (
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolmapper"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolset"
)

// knownTools is the set of native tool names this provider understands on disk.
// Implements contract.ToolSupporter. Native names are lowercase_snake_case for goose.
// Source: internal/catalog/data/goose/tools.json
var knownTools = toolset.New(
	"shell", "edit", "write", "tree", "read_image",
)

// SupportsTool reports whether the provider understands the given native tool name.
// Implements contract.ToolSupporter.
func (Provider) SupportsTool(tool string) bool { return knownTools.Contains(tool) }

// toolMap is the bidirectional native↔canonical mapping for goose.
// Source: internal/catalog/data/goose/tools.json
var toolMap = toolmapper.New([]toolmapper.Entry{
	{Native: "shell", Canonical: "bash"},
	{Native: "edit", Canonical: "file_edit"},
	{Native: "write", Canonical: "file_write"},
	{Native: "tree", Canonical: "tree"},
	{Native: "read_image", Canonical: "read_image"},
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
