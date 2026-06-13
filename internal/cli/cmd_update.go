package cli

import (
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/gateway"
	"github.com/spf13/cobra"
)

// newUpdateCommand builds `graft update [--check]` (plan-sync task 6). It calls
// gateway.RunUpdate DIRECTLY — self-update needs no workspace, store, or lock —
// so the command works outside an initialized repo and is intentionally kept out
// of commandRequiresGateway gating in cmd/graft.
func (c *DefaultCli) newUpdateCommand() *cobra.Command {
	flags := ProvisionUpdateFlags()
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Check for / install a newer graft release",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			resolved := flags
			if err := loadFlags(cmd, &resolved); err != nil {
				return err
			}
			res, err := gateway.RunUpdate(contract.UpdateOpts{CheckOnly: resolved.Check})
			if err != nil {
				return err
			}
			return printOutput(cmd.OutOrStdout(), "update", resolved.Output, res)
		},
	}
	cmd.Flags().StringP("output", "o", flags.Output, "Output format: json|yaml|table")
	cmd.Flags().Bool("check", flags.Check, "Report current vs latest without installing")
	return cmd
}

// UpdateFlags is the flag schema for `graft update`.
type UpdateFlags struct {
	Output string `koanf:"output" json:"output"`
	Check  bool   `koanf:"check" json:"check"`
}

// ProvisionUpdateFlags returns update defaults.
func ProvisionUpdateFlags() UpdateFlags { return UpdateFlags{Output: "table"} }
