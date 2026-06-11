package cli

import "github.com/spf13/cobra"

// newAgentsCommand builds the `graft agents` group with the `status` aggregate.
func (c *DefaultCli) newAgentsCommand() *cobra.Command {
	agentsCmd := &cobra.Command{
		Use:   "agents",
		Short: "Operate across all agents",
	}
	agentsCmd.AddCommand(c.newAgentsStatusCommand())
	return agentsCmd
}

// newAgentsStatusCommand builds `graft agents status`: aggregated drift.
func (c *DefaultCli) newAgentsStatusCommand() *cobra.Command {
	flags := ProvisionAgentsStatusFlags()
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Aggregated drift across all agents and providers",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			gate, err := c.requireGate()
			if err != nil {
				return err
			}
			resolved := flags
			if err := loadFlags(cmd, &resolved); err != nil {
				return err
			}
			rep, err := gate.Status(nil) // nil = all agents (aggregated)
			if err != nil {
				return err
			}
			return printOutput(cmd.OutOrStdout(), "status", resolved.Output, rep)
		},
	}
	cmd.Flags().StringP("output", "o", flags.Output, "Output format: json|yaml|table")
	return cmd
}
