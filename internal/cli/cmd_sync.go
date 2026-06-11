package cli

import (
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
	cmd.Flags().StringP("provider", "p", flags.Provider, "Limit to a single provider")
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
	return printOutput(cmd.OutOrStdout(), "sync", resolved.Output, res)
}
