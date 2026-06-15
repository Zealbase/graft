package cli

import (
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/spf13/cobra"
)

// newSkillCommand builds the `graft skill` group: list / status / install / sync.
// All route through the gateway (EntryGate) only.
func (c *DefaultCli) newSkillCommand() *cobra.Command {
	skillCmd := &cobra.Command{
		Use:   "skill",
		Short: "Manage canonical skills symlinked into supporting providers",
	}
	skillCmd.AddCommand(c.newSkillListCommand())
	skillCmd.AddCommand(c.newSkillStatusCommand())
	skillCmd.AddCommand(c.newSkillInstallCommand())
	skillCmd.AddCommand(c.newSkillSyncCommand())
	return skillCmd
}

// newSkillListCommand builds `graft skill list`.
func (c *DefaultCli) newSkillListCommand() *cobra.Command {
	flags := ProvisionSkillFlags()
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List canonical skills",
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
			skills, err := gate.SkillList()
			if err != nil {
				return err
			}
			return printOutput(cmd.OutOrStdout(), "skill.list", resolved.Output, skills)
		},
	}
	cmd.Flags().StringP("output", "o", flags.Output, "Output format: json|yaml|table")
	return cmd
}

// newSkillStatusCommand builds `graft skill status`.
func (c *DefaultCli) newSkillStatusCommand() *cobra.Command {
	flags := ProvisionSkillFlags()
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show per-provider link state for each skill",
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
			states, err := gate.SkillStatus(skillOptsFrom(cmd, resolved))
			if err != nil {
				return err
			}
			return printOutput(cmd.OutOrStdout(), "skill.status", resolved.Output, states)
		},
	}
	addSkillScopeFlags(cmd, flags)
	return cmd
}

// newSkillInstallCommand builds `graft skill install <name|path>`.
func (c *DefaultCli) newSkillInstallCommand() *cobra.Command {
	flags := ProvisionSkillFlags()
	cmd := &cobra.Command{
		Use:   "install <name|path>",
		Short: "Copy a skill into .agents/skills (if absent) and symlink it into supporting providers",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			gate, err := c.requireGate()
			if err != nil {
				return err
			}
			resolved := flags
			if err := loadFlags(cmd, &resolved); err != nil {
				return err
			}
			states, err := gate.SkillInstall(args[0], skillOptsFrom(cmd, resolved))
			if err != nil {
				return err
			}
			return printOutput(cmd.OutOrStdout(), "skill.status", resolved.Output, states)
		},
	}
	addSkillScopeFlags(cmd, flags)
	return cmd
}

// newSkillSyncCommand builds `graft skill sync`.
func (c *DefaultCli) newSkillSyncCommand() *cobra.Command {
	flags := ProvisionSkillFlags()
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Symlink all canonical skills into all supporting providers",
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
			states, err := gate.SkillSync(skillOptsFrom(cmd, resolved))
			if err != nil {
				return err
			}
			return printOutput(cmd.OutOrStdout(), "skill.status", resolved.Output, states)
		},
	}
	addSkillScopeFlags(cmd, flags)
	return cmd
}

// addSkillScopeFlags registers the shared skill scoping flags (--override,
// --provider, --yes/--install) plus -o.
func addSkillScopeFlags(cmd *cobra.Command, flags SkillFlags) {
	cmd.Flags().Bool("override", flags.Override, "Replace a non-symlink entry with a symlink")
	cmd.Flags().StringP("provider", "p", flags.Provider, "Limit to a single supporting provider")
	cmd.Flags().Bool("yes", flags.Yes, "Non-interactive: auto-install missing referenced skills")
	cmd.Flags().Bool("install", flags.Yes, "Non-interactive: auto-install missing referenced skills (alias of --yes)")
	cmd.Flags().StringP("output", "o", flags.Output, "Output format: json|yaml|table")
}

// skillOptsFrom maps resolved flags + the cobra command onto contract.SkillOpts.
// --install is an alias for --yes (not in the koanf struct), so it is read from
// the command directly and OR-ed into Yes.
func skillOptsFrom(cmd *cobra.Command, f SkillFlags) contract.SkillOpts {
	yes := f.Yes
	if cmd.Flags().Lookup("install") != nil {
		if v, _ := cmd.Flags().GetBool("install"); v {
			yes = true
		}
	}
	return contract.SkillOpts{
		Override: f.Override,
		Provider: f.Provider,
		Yes:      yes,
	}
}
