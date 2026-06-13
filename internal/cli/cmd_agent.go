package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newAgentCommand builds the `graft agent` group. It exposes `agent list` as a
// subcommand and `agent <name> status` directly on the group (per plan 03's
// `graft agent <x> status` surface — the agent name precedes the verb).
func (c *DefaultCli) newAgentCommand() *cobra.Command {
	flags := ProvisionAgentStatusFlags()
	agentCmd := &cobra.Command{
		Use:   "agent <name> status",
		Short: "Inspect a single agent (list | <name> status)",
		// Accept `<name> status`; `list` is dispatched as a subcommand by cobra.
		// Zero args is allowed so the bare `graft agent` shows help (exit 0).
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return nil
			}
			if len(args) != 2 || args[1] != "status" {
				return fmt.Errorf("usage: graft agent <name> status  (or: graft agent list)")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// No `<name> status` given: render help instead of erroring out.
			if len(args) == 0 {
				return cmd.Help()
			}
			gate, err := c.requireGate()
			if err != nil {
				return err
			}
			resolved := flags
			if err := loadFlags(cmd, &resolved); err != nil {
				return err
			}
			name := args[0]
			rep, err := gate.Status(&name)
			if err != nil {
				return err
			}
			return printOutput(cmd.OutOrStdout(), "status", resolved.Output, rep)
		},
	}
	agentCmd.Flags().StringP("output", "o", flags.Output, "Output format: json|yaml|table")
	agentCmd.AddCommand(c.newAgentListCommand())
	agentCmd.AddCommand(c.newAgentInitCommand())
	agentCmd.AddCommand(c.newAgentModelCommand())
	return agentCmd
}

// newAgentInitCommand builds `graft agent init <name> [prompt]` (plan-sync
// task 2): scaffold a default canonical agent and print a next-step hint.
func (c *DefaultCli) newAgentInitCommand() *cobra.Command {
	flags := ProvisionAgentListFlags()
	cmd := &cobra.Command{
		Use:   "init <name> [prompt]",
		Short: "Scaffold a new canonical agent (fans out to providers on next sync)",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			gate, err := c.requireGate()
			if err != nil {
				return err
			}
			resolved := flags
			if err := loadFlags(cmd, &resolved); err != nil {
				return err
			}
			name := args[0]
			var prompt string
			if len(args) == 2 {
				prompt = args[1]
			}
			a, err := gate.CreateAgent(name, prompt)
			if err != nil {
				return err
			}
			if err := printOutput(cmd.OutOrStdout(), "agent.create", resolved.Output, a); err != nil {
				return err
			}
			// Next-step hint (table mode only — keep machine output clean).
			if resolved.Output == "table" {
				fmt.Fprintf(cmd.OutOrStdout(),
					"\nCreated agent %q. Run `graft sync agent %s` to fan it out to your providers.\n",
					a.Name, a.Name)
			}
			return nil
		},
	}
	cmd.Flags().StringP("output", "o", flags.Output, "Output format: json|yaml|table")
	return cmd
}

// newAgentModelCommand builds `graft agent model <name> --provider <p> --model
// <m> [--clear]` (v0.0.3 task 3): set or clear a per-provider model override and
// print any warning-only validation findings.
func (c *DefaultCli) newAgentModelCommand() *cobra.Command {
	flags := ProvisionAgentModelFlags()
	cmd := &cobra.Command{
		Use:   "model <name> --provider <p> --model <m> [--clear]",
		Short: "Set or clear a per-provider model override on an agent",
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
			if resolved.Provider == "" {
				return fmt.Errorf("--provider is required")
			}
			if resolved.Clear && resolved.Model != "" {
				return fmt.Errorf("--clear and --model are mutually exclusive")
			}
			if !resolved.Clear && resolved.Model == "" {
				return fmt.Errorf("--model is required (or use --clear to remove the override)")
			}
			model := resolved.Model
			if resolved.Clear {
				model = "" // clearing: SetAgentModel removes the key on empty model
			}
			findings, err := gate.SetAgentModel(args[0], resolved.Provider, model)
			if err != nil {
				return err
			}
			// Warn-only: model findings never block; surface them but still exit 0.
			return printOutput(cmd.OutOrStdout(), "validate", resolved.Output, findings)
		},
	}
	cmd.Flags().StringP("output", "o", flags.Output, "Output format: json|yaml|table")
	cmd.Flags().String("provider", "", "Target provider id (required)")
	cmd.Flags().String("model", "", "Model id to set for the provider")
	cmd.Flags().Bool("clear", false, "Remove the provider's model override")
	return cmd
}

// newAgentListCommand builds `graft agent list`.
func (c *DefaultCli) newAgentListCommand() *cobra.Command {
	flags := ProvisionAgentListFlags()
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List canonical agents and per-provider coverage",
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
			agents, err := gate.List()
			if err != nil {
				return err
			}
			return printOutput(cmd.OutOrStdout(), "agent.list", resolved.Output, agents)
		},
	}
	cmd.Flags().StringP("output", "o", flags.Output, "Output format: json|yaml|table")
	return cmd
}
