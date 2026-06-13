package cli

import (
	"errors"
	"log"
	"os"

	"github.com/Shaik-Sirajuddin/graft/internal/cli/config"
	"github.com/Shaik-Sirajuddin/graft/internal/cli/theme"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/gateway"
	"github.com/spf13/cobra"
)

// DefaultCli is the shared deps struct. Every command is a method on this
// receiver so they all share the same gateway + config resolver.
type DefaultCli struct {
	root            *cobra.Command
	gate            contract.EntryGate
	configResolver  config.Resolver
	projectResolver config.ProjectResolver
	version         string
}

// Entrypoint builds the CLI root command with version "dev".
func Entrypoint(gate contract.EntryGate, resolver config.Resolver) *DefaultCli {
	return EntrypointWithVersion(gate, resolver, "dev")
}

// EntrypointWithVersion builds the CLI root command with release metadata. gate
// may be nil for commands that do not need it (config get/set); such commands
// nil-guard before use.
func EntrypointWithVersion(gate contract.EntryGate, resolver config.Resolver, version string) *DefaultCli {
	if version == "" {
		version = "dev"
	}
	if resolver == nil {
		resolver = &config.DefaultResolver{}
	}
	c := &DefaultCli{
		gate:            gate,
		configResolver:  resolver,
		projectResolver: defaultProjectResolver(),
		version:         version,
	}

	root := &cobra.Command{
		Use:           "graft",
		Short:         "graft — canonical agent definitions synced across providers",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetVersionTemplate("{{.Version}}\n")
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		renderHelp(cmd, cmd.OutOrStdout())
	})

	root.AddCommand(c.newInitCommand())
	root.AddCommand(c.newAgentCommand())
	root.AddCommand(c.newAgentsCommand())
	root.AddCommand(c.newSyncCommand())
	root.AddCommand(c.newValidateCommand())
	root.AddCommand(c.newSkillCommand())
	root.AddCommand(c.newConfigCommand())
	root.AddCommand(c.newUpdateCommand())
	root.AddCommand(c.newDestroyCommand())
	root.AddCommand(c.newCatalogCommand())

	c.root = root
	// Attach `completion install` onto cobra's auto-generated `completion` cmd.
	c.attachCompletionInstall()
	return c
}

// Root exposes the constructed cobra root (test seam).
func (c *DefaultCli) Root() *cobra.Command { return c.root }

// SetProjectResolver overrides the per-project config resolver (test seam). It
// must be called before Install/Execute. A nil resolver is ignored.
func (c *DefaultCli) SetProjectResolver(r config.ProjectResolver) {
	if r != nil {
		c.projectResolver = r
	}
}

// defaultProjectResolver builds a project resolver rooted at the current working
// directory (the workspace root for CLI invocations). A cwd-resolution failure
// yields a resolver rooted at "." so reads simply fall back to global config.
func defaultProjectResolver() config.ProjectResolver {
	root, err := os.Getwd()
	if err != nil {
		root = "."
	}
	return &config.DefaultProjectResolver{WorkspaceRoot: root}
}

// Install activates the theme, wires log output to stderr, pushes the resolved
// skills hook config into the gateway, and executes the root.
func (c *DefaultCli) Install() error {
	if c == nil || c.root == nil {
		return errors.New("cli is not initialized")
	}
	c.activateTheme()
	c.configureSkillsHook()
	c.configureEnabledProviders()
	return c.root.Execute()
}

// configureEnabledProviders pushes the effective enabled-provider set into the
// gateway (if it supports the capability) so real-time model validation only
// runs against enabled providers. Non-fatal: a config read error leaves the
// gateway's default (all providers).
func (c *DefaultCli) configureEnabledProviders() {
	if c.gate == nil {
		return
	}
	configurable, ok := c.gate.(gateway.EnabledProvidersConfigurable)
	if !ok {
		return
	}
	configurable.SetEnabledProviders(c.effectiveProviders())
}

// configureSkillsHook reads the global XDG skills config and pushes it into the
// gateway (if it supports the hook capability) so the implicit init/sync
// skill-apply pass is gated/scoped per config. Failures are non-fatal: a config
// read error leaves the gateway's zero-value hook config (disabled).
func (c *DefaultCli) configureSkillsHook() {
	if c.gate == nil || c.configResolver == nil {
		return
	}
	hookable, ok := c.gate.(gateway.SkillHookConfigurable)
	if !ok {
		return
	}
	cfg, err := ResolveConfig(c.configResolver)
	if err != nil || cfg == nil {
		// Config unreadable: apply the documented default (hook ENABLED) rather
		// than leaving the gate's zero value, which would silently disable skills.
		hookable.SetSkillHookConfig(gateway.SkillHookConfig{Enabled: true})
		return
	}
	hookable.SetSkillHookConfig(gateway.SkillHookConfig{
		Enabled:     cfg.Skills.EnabledOrDefault(),
		AutoInstall: cfg.Skills.AutoInstall,
		Providers:   cfg.Skills.Providers,
	})
}

// activateTheme resolves the active colour theme: env GRAFT_THEME -> config ->
// default, and routes the standard logger to a level-colourising stderr writer.
func (c *DefaultCli) activateTheme() {
	name := os.Getenv("GRAFT_THEME")
	if name == "" && c.configResolver != nil {
		if cfg, err := ResolveConfig(c.configResolver); err == nil && cfg != nil {
			name = cfg.Theme
		}
	}
	theme.Activate(name)
	log.SetOutput(newLogWriter(os.Stderr))
	log.SetFlags(0)
}

// requireGate returns the gateway or an error if it was not constructed.
func (c *DefaultCli) requireGate() (contract.EntryGate, error) {
	if c.gate == nil {
		return nil, errors.New("gateway is required for this command (is this a graft workspace?)")
	}
	return c.gate, nil
}
