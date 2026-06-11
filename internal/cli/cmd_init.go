package cli

import "github.com/spf13/cobra"

// newInitCommand builds `graft init`: create .graft/ + the workspace row.
func (c *DefaultCli) newInitCommand() *cobra.Command {
	flags := ProvisionInitFlags()
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a graft workspace (.graft/, store, git mode)",
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
			res, err := gate.Init()
			if err != nil {
				return err
			}
			return printOutput(cmd.OutOrStdout(), "init", resolved.Output, res)
		},
	}
	cmd.Flags().StringP("output", "o", flags.Output, "Output format: json|yaml|table")
	return cmd
}
