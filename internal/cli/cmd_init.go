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
			// First-run: detect providers + (TUI or auto) select, persist config.
			// Branding/prompts go to stderr so the result stream stays clean.
			yes, _ := cmd.Flags().GetBool("yes")
			if ferr := c.maybeRunFirstRun(cmd.ErrOrStderr(), yes); ferr != nil {
				return ferr
			}
			res, err := gate.Init()
			if err != nil {
				return err
			}
			return printOutput(cmd.OutOrStdout(), "init", resolved.Output, res)
		},
	}
	cmd.Flags().StringP("output", "o", flags.Output, "Output format: json|yaml|table")
	cmd.Flags().Bool("yes", false, "Non-interactive: auto-select detected providers (skip the first-run checklist)")
	return cmd
}
