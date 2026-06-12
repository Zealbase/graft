package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Shaik-Sirajuddin/graft/internal/cli/config"
	"github.com/Shaik-Sirajuddin/graft/internal/cli/theme"
	"github.com/spf13/cobra"
)

// newConfigCommand builds the `graft config` group: `get` and `set`. These
// operate on the global XDG config directly and bypass the gateway.
func (c *DefaultCli) newConfigCommand() *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Manage graft global config",
	}
	configCmd.AddCommand(c.newConfigGetCommand())
	configCmd.AddCommand(c.newConfigSetCommand())
	return configCmd
}

// newConfigGetCommand builds `graft config get`.
func (c *DefaultCli) newConfigGetCommand() *cobra.Command {
	flags := ProvisionConfigGetFlags()
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Print resolved global config",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			resolved := flags
			if err := loadFlags(cmd, &resolved); err != nil {
				return err
			}
			cfg, err := ResolveConfig(c.configResolver)
			if err != nil {
				return err
			}
			return printOutput(cmd.OutOrStdout(), "config", resolved.Output, cfg)
		},
	}
	cmd.Flags().StringP("output", "o", flags.Output, "Output format: json|yaml|table")
	return cmd
}

// newConfigSetCommand builds `graft config set`. Empty flag values leave the
// corresponding field unchanged.
func (c *DefaultCli) newConfigSetCommand() *cobra.Command {
	flags := ProvisionConfigSetFlags()
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Update global config (empty flag = leave unchanged)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Read flags directly: the dotted flag names (sync.gitAuto,
			// providers.enabled) collide with koanf's "." nesting, so we bypass
			// loadFlags for config set and apply the empty=unchanged rule here.
			cfg, err := ResolveConfig(c.configResolver)
			if err != nil {
				return err
			}
			f := cmd.Flags()

			if f.Changed("sync.gitAuto") {
				raw, _ := f.GetString("sync.gitAuto")
				v, perr := strconv.ParseBool(raw)
				if perr != nil {
					return fmt.Errorf("invalid --sync.gitAuto value %q: %w", raw, perr)
				}
				cfg.Sync.GitAuto = v
			}
			if f.Changed("scope") {
				scope, _ := f.GetString("scope")
				if !contains(config.ValidScopes(), scope) {
					return fmt.Errorf("invalid --scope %q (valid: %s)", scope, strings.Join(config.ValidScopes(), ", "))
				}
				cfg.Scope = scope
			}
			if f.Changed("theme") {
				th, _ := f.GetString("theme")
				if !theme.IsValidName(th) {
					return fmt.Errorf("invalid --theme %q (valid: %s)", th, strings.Join(theme.Names(), ", "))
				}
				cfg.Theme = th
			}
			if f.Changed("providers.mode") {
				mode, _ := f.GetString("providers.mode")
				if !contains(config.ValidProviderModes(), mode) {
					return fmt.Errorf("invalid --providers.mode %q (valid: %s)", mode, strings.Join(config.ValidProviderModes(), ", "))
				}
				cfg.Providers.Mode = mode
			}
			if f.Changed("providers.enabled") {
				raw, _ := f.GetString("providers.enabled")
				cfg.Providers.Enabled = splitCSV(raw)
			}
			if f.Changed("providers.disabled") {
				raw, _ := f.GetString("providers.disabled")
				cfg.Providers.Disabled = splitCSV(raw)
			}
			if f.Changed("skills.enabled") {
				raw, _ := f.GetString("skills.enabled")
				v, perr := strconv.ParseBool(raw)
				if perr != nil {
					return fmt.Errorf("invalid --skills.enabled value %q: %w", raw, perr)
				}
				cfg.Skills.Enabled = &v
			}
			if f.Changed("skills.autoInstall") {
				raw, _ := f.GetString("skills.autoInstall")
				v, perr := strconv.ParseBool(raw)
				if perr != nil {
					return fmt.Errorf("invalid --skills.autoInstall value %q: %w", raw, perr)
				}
				cfg.Skills.AutoInstall = v
			}
			if f.Changed("skills.providers") {
				raw, _ := f.GetString("skills.providers")
				cfg.Skills.Providers = splitCSV(raw)
			}

			if err := SaveConfig(c.configResolver, cfg); err != nil {
				return err
			}
			out, _ := f.GetString("output")
			return printOutput(cmd.OutOrStdout(), "config", out, cfg)
		},
	}
	cmd.Flags().String("sync.gitAuto", flags.GitAuto, "Auto-commit tracking branches (true|false); empty leaves unchanged")
	cmd.Flags().String("scope", flags.Scope, "Synced capability: agents|skills|slash; empty leaves unchanged")
	cmd.Flags().String("providers.mode", "", "Provider selection mode: all|specific; empty leaves unchanged")
	cmd.Flags().String("providers.enabled", flags.Enabled, "Comma-separated active providers (mode=specific)")
	cmd.Flags().String("providers.disabled", "", "Comma-separated excluded providers (mode=all)")
	cmd.Flags().String("theme", flags.Theme, "Colour theme: dark|dark-dim|light|colorblind; empty leaves unchanged")
	cmd.Flags().String("skills.enabled", "", "Master switch for the init/sync skill hook (true|false); empty leaves unchanged")
	cmd.Flags().String("skills.autoInstall", "", "Install missing referenced skills without prompting (true|false); empty leaves unchanged")
	cmd.Flags().String("skills.providers", "", "Comma-separated subset of supporting providers to link")
	cmd.Flags().StringP("output", "o", flags.Output, "Output format: json|yaml|table")
	return cmd
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// splitCSV splits a comma-separated flag value into a trimmed, non-empty slice.
func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
