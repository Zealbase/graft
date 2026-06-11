package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

// newValidateCommand builds `graft validate [--provider p | --all]`. It returns
// a non-zero error when any error-severity finding is present.
func (c *DefaultCli) newValidateCommand() *cobra.Command {
	flags := ProvisionValidateFlags()
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Schema + semantic validation of canonical agents",
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
			if resolved.Provider != "" && resolved.All {
				return errors.New("--provider and --all are mutually exclusive")
			}
			scope := resolved.Provider
			if resolved.All {
				scope = "all"
			}

			findings, err := gate.Validate(scope)
			if err != nil {
				return err
			}
			if perr := printOutput(cmd.OutOrStdout(), "validate", resolved.Output, findings); perr != nil {
				return perr
			}

			// Non-zero exit when any error-severity finding exists.
			n := 0
			for _, f := range findings {
				if f.Severity == "error" {
					n++
				}
			}
			if n > 0 {
				return fmt.Errorf("validation failed: %d error finding(s)", n)
			}
			return nil
		},
	}
	cmd.Flags().StringP("output", "o", flags.Output, "Output format: json|yaml|table")
	cmd.Flags().StringP("provider", "p", flags.Provider, "Validate only agents this provider has on disk")
	cmd.Flags().Bool("all", flags.All, "Validate all tracked agents")
	return cmd
}
