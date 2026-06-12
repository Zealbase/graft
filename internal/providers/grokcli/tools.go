package grokcli

import (
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolset"
)

// knownTools is the set of tool names this provider understands on disk.
// Implements contract.ToolSupporter. Source: provider-model-tool-sources.md grok-cli row.
var knownTools = toolset.New(
		"bash",
		"file_edit",
		"web_search",
		"computer_use",)

// SupportsTool reports whether the provider understands the given tool name.
// Implements contract.ToolSupporter. Tools not in this set are NOT written to
// the provider's file by the transformer but remain in the canonical form.
func (Provider) SupportsTool(tool string) bool { return knownTools.Contains(tool) }
