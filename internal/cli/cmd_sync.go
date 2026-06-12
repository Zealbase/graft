package cli

import (
	"fmt"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/spf13/cobra"
)

// newSyncCommand builds the `graft sync` group: `agent <x>` and `agents`.
func (c *DefaultCli) newSyncCommand() *cobra.Command {
	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Run a sync (agent <name> | agents)",
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
	// NOTE: no --provider flag here — the sync engine has no per-provider scoping
	// yet, so exposing it would be a silent no-op. Re-add when SyncOpts supports it.
}

// syncView wraps a RunResult with the count of enabled providers so the sync
// output can render "{y} agents in sync with {x} providers" (plan-revise task 2).
// JSON/YAML output unwraps to the raw RunResult so machine consumers are
// unaffected.
type syncView struct {
	Result        contract.RunResult
	ProviderCount int
}

// totalSupportedProviders is the count of providers graft supports (the ten
// transform-registered providers). Used when no explicit enabled subset is set.
const totalSupportedProviders = 10

// enabledProviderCount returns x for the sync summary: the number of enabled
// providers from global config, or the full supported set when none is pinned.
func (c *DefaultCli) enabledProviderCount() int {
	if c.configResolver != nil {
		if cfg, err := ResolveConfig(c.configResolver); err == nil && cfg != nil {
			if n := len(cfg.Providers.Enabled); n > 0 {
				return n
			}
		}
	}
	return totalSupportedProviders
}

// runSync is the shared sync body: build opts, call the gateway, render result.
// A blocking validation gate surfaces as a non-zero error.
func (c *DefaultCli) runSync(cmd *cobra.Command, names []string, resolved SyncFlags) error {
	gate, err := c.requireGate()
	if err != nil {
		return err
	}
	res, err := gate.Sync(contract.SyncOpts{
		Names:    names,
		Continue: resolved.Continue,
	})
	if err != nil {
		return err
	}
	view := syncView{Result: res, ProviderCount: c.enabledProviderCount()}
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
