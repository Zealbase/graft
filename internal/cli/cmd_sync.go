package cli

import (
	"fmt"

	"github.com/Shaik-Sirajuddin/graft/internal/cli/config"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/gateway"
	"github.com/spf13/cobra"
)

// newSyncCommand builds the `graft sync` group: `agent <x>` and `agents`.
func (c *DefaultCli) newSyncCommand() *cobra.Command {
	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync agents across providers",
	}
	syncCmd.AddCommand(c.newSyncAgentCommand())
	syncCmd.AddCommand(c.newSyncAgentsCommand())
	return syncCmd
}

// newSyncAgentCommand builds `graft sync agent <name>`.
func (c *DefaultCli) newSyncAgentCommand() *cobra.Command {
	flags := ProvisionSyncFlags()
	cmd := &cobra.Command{
		Use:   "agent <name>",
		Short: "Sync one agent across providers",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resolved := flags
			if err := loadFlags(cmd, &resolved); err != nil {
				return err
			}
			return c.runSync(cmd, []string{args[0]}, resolved)
		},
	}
	addSyncFlags(cmd, flags)
	return cmd
}

// newSyncAgentsCommand builds `graft sync agents`.
func (c *DefaultCli) newSyncAgentsCommand() *cobra.Command {
	flags := ProvisionSyncFlags()
	cmd := &cobra.Command{
		Use:   "agents",
		Short: "Sync all changed agents",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			resolved := flags
			if err := loadFlags(cmd, &resolved); err != nil {
				return err
			}
			return c.runSync(cmd, nil, resolved)
		},
	}
	addSyncFlags(cmd, flags)
	return cmd
}

// addSyncFlags registers the shared sync flags.
func addSyncFlags(cmd *cobra.Command, flags SyncFlags) {
	cmd.Flags().StringP("output", "o", flags.Output, "Output format: json|yaml|table")
	cmd.Flags().Bool("continue", flags.Continue, "Resume an interrupted conflict run")
	cmd.Flags().Bool("ingest", flags.Ingest, "Canonicalize provider-only agents and fan them out (default true; --ingest=false to suppress)")
	cmd.Flags().Bool("dry-run", flags.DryRun, "Report what would change (incl. agents pending deletion) without mutating any files or db rows")
	cmd.Flags().Bool("abort", flags.Abort, "Abort a halted conflict run: prune its temp branches + worktrees and mark it terminated (no-op if none)")
	// --provider does NOT scope the sync itself (the engine has no per-provider
	// sync scoping yet); it scopes ONLY the additive hydrate block's sandbox on a
	// single-agent `sync agent <name>` (e.g. --provider codex surfaces sandbox_mode).
	cmd.Flags().String("provider", flags.Provider, "Scope the hydrate sandbox to this provider (e.g. codex); does not scope the sync")
}

// syncView wraps a RunResult with the count of enabled providers so the sync
// output can render "{y} agents in sync with {x} providers" (plan-revise task 2).
// JSON/YAML output unwraps to the raw RunResult so machine consumers are
// unaffected.
type syncView struct {
	Result        contract.RunResult
	ProviderCount int
	// SkillCount is the number of canonical skills under .agents/skills, used to
	// claim "K skills" in the in-sync summary. It is set (>= 0) only when skills
	// are enabled and there is at least one canonical skill; otherwise -1 so the
	// summary makes no skill claim (v0.0.4 verify).
	SkillCount int
	// Hydrate is the additive hydrate block attached on a single-agent
	// `sync agent <name>` (plan-c). Nil on a multi-agent `sync agents`.
	Hydrate *contract.HydrateView
}

// syncMachineView is the machine (json/yaml) shape for a single-agent sync: the
// RunResult embedded so its existing keys are preserved (back-compat — consumers
// still parse contract.RunResult), plus an additive "hydrate" key.
type syncMachineView struct {
	contract.RunResult
	Hydrate *contract.HydrateView `json:"hydrate,omitempty"`
}

// effectiveProviders resolves the active provider set, layering the per-project
// config (.graft/config.json) over the global config (v0.0.3 task 6): a project
// that sets providers wins; otherwise the global effective set applies. Falls
// back to the full supported set when neither is readable.
func (c *DefaultCli) effectiveProviders() []string {
	var global *config.Config
	if c.configResolver != nil {
		if cfg, err := ResolveConfig(c.configResolver); err == nil {
			global = cfg
		}
	}
	var project *config.ProjectConfig
	if c.projectResolver != nil {
		if pc, err := c.projectResolver.Get(); err == nil {
			project = pc
		}
	}
	if global == nil && project == nil {
		return config.SupportedProviders()
	}
	return config.EffectiveProviders(global, project)
}

// enabledProviderCount returns x for the sync summary: the number of providers
// in the effective set.
func (c *DefaultCli) enabledProviderCount() int {
	return len(c.effectiveProviders())
}

// canonicalSkillCount returns the number of canonical skills (.agents/skills) for
// the in-sync summary, or -1 when skills are disabled or there are none — so the
// summary only claims "K skills" when skills are enabled and present (v0.0.4
// verify). Errors are non-fatal: a read failure yields -1 (no skill claim).
func (c *DefaultCli) canonicalSkillCount(gate contract.EntryGate) int {
	if !c.skillsEnabled() {
		return -1
	}
	skills, err := gate.SkillList()
	if err != nil || len(skills) == 0 {
		return -1
	}
	return len(skills)
}

// skillsEnabled reports whether the skills hook is enabled per the resolved
// config (default true when config is unreadable, matching the gateway hook).
func (c *DefaultCli) skillsEnabled() bool {
	if c.configResolver == nil {
		return true
	}
	cfg, err := ResolveConfig(c.configResolver)
	if err != nil || cfg == nil {
		return true
	}
	return cfg.Skills.EnabledOrDefault()
}

// runSync is the shared sync body: build opts, call the gateway, render result.
// A blocking validation gate surfaces as a non-zero error.
func (c *DefaultCli) runSync(cmd *cobra.Command, names []string, resolved SyncFlags) error {
	gate, err := c.requireGate()
	if err != nil {
		return err
	}
	// --abort short-circuits the sync: clean up a halted conflict run instead of
	// running one. It ignores the target names (abort is workspace-scoped).
	if resolved.Abort {
		return c.runAbort(cmd, gate, resolved)
	}
	enabled := c.effectiveProviders()
	res, err := gate.Sync(contract.SyncOpts{
		Names:     names,
		Continue:  resolved.Continue,
		Providers: enabled,
		Ingest:    resolved.Ingest,
		DryRun:    resolved.DryRun,
	})
	if err != nil {
		return err
	}
	view := syncView{Result: res, ProviderCount: len(enabled), SkillCount: c.canonicalSkillCount(gate)}
	// Additive hydrate block on a single-agent sync (plan-c): expose the resolved
	// model/tools/skills/mcp + provider-scoped sandbox for the host consumer.
	if len(names) == 1 {
		if hc, ok := gate.(gateway.HydrateCapable); ok {
			if h, herr := hc.Hydrate(names[0], resolved.Provider); herr == nil {
				view.Hydrate = &h
			}
		}
	}
	if err := printOutput(cmd.OutOrStdout(), "sync", resolved.Output, view); err != nil {
		return err
	}
	// A surfaced merge conflict is a non-zero outcome: the user must resolve the
	// markers and re-run. The result is already rendered above.
	if res.Status == contract.RunConflict {
		return fmt.Errorf("merge conflict — resolve the markers in the listed file(s), then re-run `graft sync`")
	}
	return nil
}

// runAbort handles `graft sync ... --abort`: it cleans up a halted conflict run
// (prune temp branches + worktrees, mark the run terminated) and prints a clear
// confirmation. When there is no in-progress run it is a friendly no-op (exit 0).
func (c *DefaultCli) runAbort(cmd *cobra.Command, gate contract.EntryGate, resolved SyncFlags) error {
	res, err := gate.AbortSync()
	if err != nil {
		return err
	}
	return printOutput(cmd.OutOrStdout(), "abort", resolved.Output, res)
}
