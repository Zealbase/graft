package cli

import (
	"fmt"

	"github.com/knadh/koanf/providers/posflag"
	"github.com/knadh/koanf/v2"
	"github.com/spf13/cobra"
)

// loadFlags parses a command's pflag values into a koanf-tagged target struct.
// It is the single flag-resolution helper shared by every command's RunE.
func loadFlags(cmd *cobra.Command, target any) error {
	k := koanf.New(".")
	if err := k.Load(posflag.Provider(cmd.Flags(), ".", k), nil); err != nil {
		return fmt.Errorf("load command flags: %w", err)
	}
	if err := k.Unmarshal("", target); err != nil {
		return fmt.Errorf("resolve command flags: %w", err)
	}
	return nil
}

// --- per-command koanf flag schemas + provision defaults -----------------

// InitFlags is the flag schema for `graft init`.
type InitFlags struct {
	Output string `koanf:"output" json:"output"`
}

// ProvisionInitFlags returns init defaults.
func ProvisionInitFlags() InitFlags { return InitFlags{Output: "table"} }

// DetectFlags is the flag schema for `graft detect`.
type DetectFlags struct {
	Output string `koanf:"output" json:"output"`
}

// ProvisionDetectFlags returns detect defaults (table, like other read cmds).
func ProvisionDetectFlags() DetectFlags { return DetectFlags{Output: "table"} }

// AgentListFlags is the flag schema for `graft agent list`.
type AgentListFlags struct {
	Output string `koanf:"output" json:"output"`
}

// ProvisionAgentListFlags returns agent-list defaults.
func ProvisionAgentListFlags() AgentListFlags { return AgentListFlags{Output: "table"} }

// AgentStatusFlags is the flag schema for `graft agent <x> status`.
type AgentStatusFlags struct {
	Output string `koanf:"output" json:"output"`
}

// ProvisionAgentStatusFlags returns agent-status defaults.
func ProvisionAgentStatusFlags() AgentStatusFlags { return AgentStatusFlags{Output: "table"} }

// AgentsStatusFlags is the flag schema for `graft agents status`.
type AgentsStatusFlags struct {
	Output string `koanf:"output" json:"output"`
}

// ProvisionAgentsStatusFlags returns agents-status defaults.
func ProvisionAgentsStatusFlags() AgentsStatusFlags { return AgentsStatusFlags{Output: "table"} }

// SyncFlags is the flag schema for `graft sync agent <x>` / `sync agents`.
type SyncFlags struct {
	Output   string `koanf:"output" json:"output"`
	Continue bool   `koanf:"continue" json:"continue"`
	Provider string `koanf:"provider" json:"provider"`
	Ingest   bool   `koanf:"ingest" json:"ingest"`
	DryRun   bool   `koanf:"dry-run" json:"dry_run"`
	// Abort cleans up a halted conflict run (prune temp branches + worktrees,
	// mark the run terminated) instead of running a sync.
	Abort bool `koanf:"abort" json:"abort"`
}

// ProvisionSyncFlags returns sync defaults. Ingest defaults TRUE (plan-sync
// task 5 / v0.0.3 task 9): a normal sync canonicalizes provider-only agents and
// fans them out; pass --ingest=false to suppress.
func ProvisionSyncFlags() SyncFlags { return SyncFlags{Output: "table", Ingest: true} }

// AgentModelFlags is the flag schema for `graft agent model <name>`.
type AgentModelFlags struct {
	Output   string `koanf:"output" json:"output"`
	Provider string `koanf:"provider" json:"provider"`
	Model    string `koanf:"model" json:"model"`
	Clear    bool   `koanf:"clear" json:"clear"`
}

// ProvisionAgentModelFlags returns agent-model defaults.
func ProvisionAgentModelFlags() AgentModelFlags { return AgentModelFlags{Output: "table"} }

// SkillFlags is the flag schema shared by the `graft skill` commands.
type SkillFlags struct {
	Output   string `koanf:"output" json:"output"`
	Override bool   `koanf:"override" json:"override"`
	Provider string `koanf:"provider" json:"provider"`
	Yes      bool   `koanf:"yes" json:"yes"`
}

// ProvisionSkillFlags returns skill-command defaults.
func ProvisionSkillFlags() SkillFlags { return SkillFlags{Output: "table"} }

// ValidateFlags is the flag schema for `graft validate`.
type ValidateFlags struct {
	Output   string `koanf:"output" json:"output"`
	Provider string `koanf:"provider" json:"provider"`
	All      bool   `koanf:"all" json:"all"`
}

// ProvisionValidateFlags returns validate defaults.
func ProvisionValidateFlags() ValidateFlags { return ValidateFlags{Output: "table"} }

// ConfigGetFlags is the flag schema for `graft config get`.
type ConfigGetFlags struct {
	Output string `koanf:"output" json:"output"`
	Global bool   `koanf:"global" json:"global"`
}

// ProvisionConfigGetFlags returns config-get defaults.
func ProvisionConfigGetFlags() ConfigGetFlags { return ConfigGetFlags{Output: "yaml"} }

// ConfigSetFlags is the flag schema for `graft config set`. Empty string fields
// mean "leave unchanged"; the gitAuto tri-state uses "" / "true" / "false".
type ConfigSetFlags struct {
	GitAuto string `koanf:"sync.gitAuto" json:"sync.gitAuto"`
	Scope   string `koanf:"scope" json:"scope"`
	Enabled string `koanf:"providers.enabled" json:"providers.enabled"`
	Theme   string `koanf:"theme" json:"theme"`
	Output  string `koanf:"output" json:"output"`
}

// ProvisionConfigSetFlags returns config-set defaults (all empty = unchanged).
func ProvisionConfigSetFlags() ConfigSetFlags { return ConfigSetFlags{Output: "yaml"} }
