package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Shaik-Sirajuddin/graft/internal/cli/config"
	"github.com/Shaik-Sirajuddin/graft/internal/cli/theme"
	"github.com/spf13/cobra"
)

// newConfigCommand builds the `graft config` group: `get` and `set`. By default
// both operate on the PER-PROJECT config (.graft/config.json); -g/--global
// targets the global XDG config. `get` (project) resolves project-over-global;
// `get -g` shows global only. These commands bypass the gateway.
func (c *DefaultCli) newConfigCommand() *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Manage graft config (project by default; -g for global)",
	}
	configCmd.AddCommand(c.newConfigGetCommand())
	configCmd.AddCommand(c.newConfigSetCommand())
	return configCmd
}

// newConfigGetCommand builds `graft config get`. Default prints the resolved
// project-over-global view; -g/--global prints the global config only.
func (c *DefaultCli) newConfigGetCommand() *cobra.Command {
	flags := ProvisionConfigGetFlags()
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Print config (resolved project-over-global; -g for global only)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			resolved := flags
			if err := loadFlags(cmd, &resolved); err != nil {
				return err
			}
			global, err := ResolveConfig(c.configResolver)
			if err != nil {
				return err
			}
			if resolved.Global {
				return printOutput(cmd.OutOrStdout(), "config", resolved.Output, global)
			}
			// Default: resolved view. Layer the project providers/scope over global
			// so `get` reflects exactly what a sync would use (project→global
			// fallback).
			project, perr := c.projectConfig()
			if perr != nil {
				return perr
			}
			resolvedCfg := layerProjectOverGlobal(global, project)
			return printOutput(cmd.OutOrStdout(), "config", resolved.Output, resolvedCfg)
		},
	}
	cmd.Flags().StringP("output", "o", flags.Output, "Output format: json|yaml|table")
	cmd.Flags().BoolP("global", "g", flags.Global, "Operate on the global config instead of the project config")
	return cmd
}

// newConfigSetCommand builds `graft config set`. Default writes the per-project
// config (.graft/config.json) — only the project-overridable keys (providers.*,
// scope) are accepted there; global-only keys (theme, skills.*, sync.gitAuto)
// require -g/--global. Empty flag values leave the corresponding field
// unchanged.
func (c *DefaultCli) newConfigSetCommand() *cobra.Command {
	flags := ProvisionConfigSetFlags()
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Update config (project by default; -g for global). Empty flag = unchanged",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			f := cmd.Flags()
			global, _ := f.GetBool("global")
			if global {
				return c.runConfigSetGlobal(cmd)
			}
			return c.runConfigSetProject(cmd)
		},
	}
	cmd.Flags().String("sync.gitAuto", flags.GitAuto, "Auto-commit tracking branches (true|false); GLOBAL only")
	cmd.Flags().String("scope", flags.Scope, "Synced capability: agents|skills|slash; empty leaves unchanged")
	cmd.Flags().String("providers.mode", "", "Provider selection mode: all|specific; empty leaves unchanged")
	cmd.Flags().String("providers.enabled", flags.Enabled, "Comma-separated active providers (mode=specific)")
	cmd.Flags().String("providers.disabled", "", "Comma-separated excluded providers (mode=all)")
	cmd.Flags().String("theme", flags.Theme, "Colour theme: dark|dark-dim|light|colorblind; GLOBAL only")
	cmd.Flags().String("skills.enabled", "", "Master switch for the init/sync skill hook (true|false); GLOBAL only")
	cmd.Flags().String("skills.autoInstall", "", "Install missing referenced skills without prompting (true|false); GLOBAL only")
	cmd.Flags().String("skills.providers", "", "Comma-separated subset of supporting providers to link; GLOBAL only")
	cmd.Flags().StringP("output", "o", flags.Output, "Output format: json|yaml|table")
	cmd.Flags().BoolP("global", "g", false, "Write to the global config instead of the project config")
	return cmd
}

// runConfigSetGlobal applies the set to the global XDG config (the historical
// behavior). Read flags directly: the dotted flag names collide with koanf's "."
// nesting, so we bypass loadFlags and apply the empty=unchanged rule here.
func (c *DefaultCli) runConfigSetGlobal(cmd *cobra.Command) error {
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
		ids := splitCSV(raw)
		if err := validateProviderIDs("--providers.enabled", ids); err != nil {
			return err
		}
		cfg.Providers.Enabled = ids
	}
	if f.Changed("providers.disabled") {
		raw, _ := f.GetString("providers.disabled")
		ids := splitCSV(raw)
		if err := validateProviderIDs("--providers.disabled", ids); err != nil {
			return err
		}
		cfg.Providers.Disabled = ids
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
}

// runConfigSetProject applies the set to the per-project config
// (.graft/config.json). Only provider selection and scope are project-overridable;
// any global-only key (theme/skills/sync.gitAuto) is rejected with a hint to use
// -g/--global.
func (c *DefaultCli) runConfigSetProject(cmd *cobra.Command) error {
	if c.projectResolver == nil {
		return fmt.Errorf("project config is unavailable (not a graft workspace?); use -g/--global")
	}
	f := cmd.Flags()

	// Reject global-only keys at the project scope.
	for _, key := range []string{"sync.gitAuto", "theme", "skills.enabled", "skills.autoInstall", "skills.providers"} {
		if f.Changed(key) {
			return fmt.Errorf("--%s is a global-only setting; use `graft config set -g --%s ...`", key, key)
		}
	}

	pc, err := c.projectResolver.Get()
	if err != nil {
		return err
	}
	if pc.Providers == nil {
		pc.Providers = &config.ProvidersConfig{Mode: config.DefaultProviderMode}
	}

	if f.Changed("scope") {
		scope, _ := f.GetString("scope")
		if !contains(config.ValidScopes(), scope) {
			return fmt.Errorf("invalid --scope %q (valid: %s)", scope, strings.Join(config.ValidScopes(), ", "))
		}
		pc.Scope = scope
	}
	if f.Changed("providers.mode") {
		mode, _ := f.GetString("providers.mode")
		if !contains(config.ValidProviderModes(), mode) {
			return fmt.Errorf("invalid --providers.mode %q (valid: %s)", mode, strings.Join(config.ValidProviderModes(), ", "))
		}
		pc.Providers.Mode = mode
	}
	if f.Changed("providers.enabled") {
		raw, _ := f.GetString("providers.enabled")
		ids := splitCSV(raw)
		if err := validateProviderIDs("--providers.enabled", ids); err != nil {
			return err
		}
		pc.Providers.Enabled = ids
	}
	if f.Changed("providers.disabled") {
		raw, _ := f.GetString("providers.disabled")
		ids := splitCSV(raw)
		if err := validateProviderIDs("--providers.disabled", ids); err != nil {
			return err
		}
		pc.Providers.Disabled = ids
	}

	if err := c.projectResolver.Save(pc); err != nil {
		return err
	}
	// Show the resolved project-over-global view after the write.
	global, gerr := ResolveConfig(c.configResolver)
	if gerr != nil {
		return gerr
	}
	out, _ := f.GetString("output")
	return printOutput(cmd.OutOrStdout(), "config", out, layerProjectOverGlobal(global, pc))
}

// projectConfig reads the per-project config (empty when unavailable).
func (c *DefaultCli) projectConfig() (*config.ProjectConfig, error) {
	if c.projectResolver == nil {
		return &config.ProjectConfig{}, nil
	}
	return c.projectResolver.Get()
}

// layerProjectOverGlobal returns a *config.Config whose provider selection and
// scope reflect the project override (when set), for display. Global-only fields
// are carried through unchanged.
func layerProjectOverGlobal(global *config.Config, project *config.ProjectConfig) *config.Config {
	merged := *global // shallow copy is fine: we replace the selection wholesale
	if project != nil {
		if project.Providers != nil {
			merged.Providers = *project.Providers
		}
		if project.Scope != "" {
			merged.Scope = project.Scope
		}
	}
	return config.ApplyDefaults(&merged)
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// validateProviderIDs rejects any id that is not a supported provider, so an
// unrecognised id is surfaced at set time rather than silently dropped later by
// EffectiveProviders. flag is the flag name used in the error message.
func validateProviderIDs(flag string, ids []string) error {
	var unknown []string
	for _, id := range ids {
		if !config.IsSupportedProvider(id) {
			unknown = append(unknown, id)
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("invalid %s: unknown provider(s) %s (valid: %s)",
			flag, strings.Join(unknown, ", "), strings.Join(config.SupportedProviders(), ", "))
	}
	return nil
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
