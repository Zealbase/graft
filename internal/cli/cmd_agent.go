package cli

import (
	"fmt"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/gateway"
	"github.com/spf13/cobra"
)

// omniFlagDefault is the sentinel value pflag assigns when --omni-agent is given
// WITHOUT an explicit value (NoOptDefVal). It is never a real ref: when the
// resolved flag value equals it, the omni ref defaults to the positional agent
// name. A user passing --omni-agent=<ref> overrides it with <ref>.
const omniFlagDefault = "\x00graft-omni-default\x00"

// newAgentCommand builds the `graft agent` group. It exposes `agent list` as a
// subcommand and `agent <name> status` directly on the group (per plan 03's
// `graft agent <x> status` surface — the agent name precedes the verb).
func (c *DefaultCli) newAgentCommand() *cobra.Command {
	flags := ProvisionAgentStatusFlags()
	agentCmd := &cobra.Command{
		Use:   "agent",
		Short: "Inspect a single agent by name",
		// Accept `<name> status` and `<name> omni`; `list` is dispatched as a
		// subcommand by cobra. Zero args shows help (exit 0).
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return nil
			}
			if len(args) != 2 || (args[1] != "status" && args[1] != "omni") {
				return fmt.Errorf("usage: graft agent <name> status | omni --refresh  (or: graft agent list)")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// No `<name> <verb>` given: render help instead of erroring out.
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
			if args[1] == "omni" {
				return c.runAgentOmni(cmd, gate, name, resolved.Output)
			}
			rep, err := gate.Status(&name)
			if err != nil {
				return err
			}
			// Additive hydrate block (plan-c): expose the resolved model/tools/
			// skills/mcp and a provider-scoped sandbox for the named agent. The
			// existing StatusReport keys are preserved (statusView embeds it), so
			// machine consumers parsing contract.StatusReport are unaffected.
			provider, _ := cmd.Flags().GetString("provider")
			view := c.statusViewFor(gate, rep, name, provider)
			return printOutput(cmd.OutOrStdout(), "status", resolved.Output, view)
		},
	}
	agentCmd.Flags().StringP("output", "o", flags.Output, "Output format: json|yaml|table")
	agentCmd.Flags().String("provider", "",
		"Scope the hydrate view's sandbox to this provider (e.g. codex sandbox_mode)")
	// --refresh applies to the `<name> omni` form: re-run the omni resolver and
	// replace the header in place (no-op + warning when unsupported).
	agentCmd.Flags().Bool("refresh", false, "With `<name> omni`: re-run the omni resolver and replace the header in place")
	agentCmd.AddCommand(c.newAgentListCommand())
	agentCmd.AddCommand(c.newAgentInitCommand())
	agentCmd.AddCommand(c.newAgentModelCommand())
	agentCmd.AddCommand(c.newAgentSyncCommand())
	return agentCmd
}

// runAgentOmni implements `graft agent <name> omni --refresh`: re-run the omni
// resolver for the agent and replace its header in place. Without --refresh it
// errors (nothing to do). Unsupported resolver ⇒ clean no-op + warning, exit 0.
func (c *DefaultCli) runAgentOmni(cmd *cobra.Command, gate contract.EntryGate, name, output string) error {
	refresh, _ := cmd.Flags().GetBool("refresh")
	if !refresh {
		return fmt.Errorf("nothing to do: pass --refresh to re-run the omni resolver")
	}
	oc, ok := gate.(gateway.AgentOmniCapable)
	if !ok {
		return fmt.Errorf("this build does not support omni refresh")
	}
	res, err := oc.RefreshOmni(name)
	if err != nil {
		return err
	}
	// Unsupported ⇒ clean no-op + warning to stderr, exit 0.
	if res.Warning != "" {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", res.Warning)
	}
	return printOutput(cmd.OutOrStdout(), "agent.omni", output, res)
}

// statusView wraps a StatusReport with an additive hydrate block. It embeds the
// report anonymously so the JSON/YAML output keeps the report's own top-level
// keys (agents, out_of_sync_providers) and merely GAINS a "hydrate" key —
// machine consumers parsing contract.StatusReport are unaffected (back-compat).
type statusView struct {
	contract.StatusReport
	Hydrate *contract.HydrateView `json:"hydrate,omitempty"`
}

// statusViewFor builds the statusView for a named agent, attaching its hydrate
// block when the gate supports hydration and the agent resolves. A hydration
// failure (e.g. agent has no canonical) is non-fatal: the report is returned
// without a hydrate block rather than failing the status command.
func (c *DefaultCli) statusViewFor(gate contract.EntryGate, rep contract.StatusReport, name, provider string) statusView {
	view := statusView{StatusReport: rep}
	if hc, ok := gate.(gateway.HydrateCapable); ok {
		if h, err := hc.Hydrate(name, provider); err == nil {
			view.Hydrate = &h
		}
	}
	return view
}

// newAgentSyncCommand builds `graft agent sync [<name>]`, an alias for
// `graft sync agents` / `graft sync agent <name>`. It shares the same runSync
// implementation so behavior and output match exactly (v0.0.4 verify); the two
// surfaces are kept side by side.
func (c *DefaultCli) newAgentSyncCommand() *cobra.Command {
	flags := ProvisionSyncFlags()
	cmd := &cobra.Command{
		Use:   "sync [<name>]",
		Short: "Sync agents across providers (alias for graft sync)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resolved := flags
			if err := loadFlags(cmd, &resolved); err != nil {
				return err
			}
			var names []string
			if len(args) == 1 {
				names = []string{args[0]}
			}
			return c.runSync(cmd, names, resolved)
		},
	}
	addSyncFlags(cmd, flags)
	return cmd
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

			// Resolve --omni-agent: absent ⇒ "", bare ⇒ default to <name>,
			// --omni-agent=<ref> ⇒ <ref>.
			omniRef, omniGiven, err := resolveOmniFlag(cmd, name)
			if err != nil {
				return err
			}

			var (
				a       contract.CanonicalAgent
				omniRes gateway.OmniResult
			)
			if omniGiven {
				oc, ok := gate.(gateway.AgentOmniCapable)
				if !ok {
					return fmt.Errorf("this build does not support --omni-agent")
				}
				a, omniRes, err = oc.CreateAgentWithOmni(name, prompt, omniRef)
			} else {
				a, err = gate.CreateAgent(name, prompt)
			}
			if err != nil {
				return err
			}

			// An unsupported omni ref is recorded but not applied: warn on stderr so
			// machine stdout stays clean, and never error out.
			if omniGiven && omniRes.Warning != "" {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", omniRes.Warning)
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
	// --omni-agent is an OPTIONAL-VALUE flag: bare `--omni-agent` records the
	// positional <name> as the omni ref; `--omni-agent=<ref>` records <ref>.
	// NoOptDefVal makes the bare form valid; the sentinel is resolved to <name>
	// in resolveOmniFlag.
	omniF := cmd.Flags().String("omni-agent", "",
		"Record (and apply when supported) an omni-agent header; bare = use <name>, or =<ref>")
	_ = omniF
	cmd.Flag("omni-agent").NoOptDefVal = omniFlagDefault
	return cmd
}

// resolveOmniFlag reads the optional-value --omni-agent flag. It returns the
// resolved ref, whether the flag was given at all, and any error. Resolution:
//   - flag not present       -> ("", false, nil)
//   - bare --omni-agent      -> (name, true, nil)   [pflag set the sentinel]
//   - --omni-agent=<ref>     -> (<ref>, true, nil)
//   - --omni-agent= (empty)  -> error (explicit empty ref is ambiguous)
func resolveOmniFlag(cmd *cobra.Command, name string) (ref string, given bool, err error) {
	f := cmd.Flag("omni-agent")
	if f == nil || !f.Changed {
		return "", false, nil
	}
	val := f.Value.String()
	if val == omniFlagDefault {
		return name, true, nil
	}
	if val == "" {
		return "", false, fmt.Errorf("--omni-agent given an empty ref; use bare --omni-agent to default to the agent name")
	}
	return val, true, nil
}

// newAgentModelCommand builds `graft agent model <name> --provider <p> --model
// <m> [--clear]` (v0.0.3 task 3): set or clear a per-provider model override and
// print any warning-only validation findings.
func (c *DefaultCli) newAgentModelCommand() *cobra.Command {
	flags := ProvisionAgentModelFlags()
	cmd := &cobra.Command{
		Use:   "model <name>",
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
