package opencode

import (
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolset"
)

// knownTools is the set of tool names this provider understands on disk.
// Implements contract.ToolSupporter.
// Sources:
//   - Core set (confirmed): provider-model-tool-sources.md opencode row +
//     https://opencode.ai/docs/agents/ tool permission reference.
//   - "question", "doom_loop", "todowrite": listed in the opencode row of the
//     research table; confirmed present in opencode's internal tool registry.
//     Marked provisional — opencode evolves quickly and these may be renamed
//     in future releases.
var knownTools = toolset.New(
	"read",
	"edit",
	"glob",
	"grep",
	"list",
	"bash",
	"task",
	"external_directory",
	"lsp",
	"skill",
	"webfetch",
	"websearch",
	// Provisional — confirmed in opencode tool registry but subject to rename:
	"question",
	"doom_loop",
	"todowrite",
)

// SupportsTool reports whether the provider understands the given tool name.
// Implements contract.ToolSupporter. Tools not in this set are NOT written to
// the provider's file by the transformer but remain in the canonical form.
func (Provider) SupportsTool(tool string) bool { return knownTools.Contains(tool) }
