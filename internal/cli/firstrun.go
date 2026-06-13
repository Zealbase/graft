package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/Shaik-Sirajuddin/graft/internal/cli/config"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
)

// tagline is graft's one-line description shown in first-run branding.
const tagline = "sync one agent definition across every AI coding tool"

// firstRunNeeded reports whether this is a first run: no persisted global config
// file yet. A read error (other than not-exist) is treated as "not first run" so
// we never re-prompt over an existing-but-unreadable config.
func (c *DefaultCli) firstRunNeeded() bool {
	if c.configResolver == nil {
		return false
	}
	path, err := c.configResolver.Path()
	if err != nil {
		return false
	}
	if _, err := os.Stat(path); err == nil {
		return false // config exists
	} else if !os.IsNotExist(err) {
		return false
	}
	return true
}

// maybeRunFirstRun runs the first-run provider-selection flow (v0.0.3 task 5).
// autoYes forces the non-interactive path (used by --yes / --ci). Branding +
// prompts go to stderr (results stream stays clean) and never hang.
//
// Flow (interactive):
//  1. Detect installed providers. If they differ from the GLOBAL enabled set,
//     show a checklist (pre-checked with detected) to set the global enabled set,
//     and persist it globally.
//  2. Show a PROJECT checklist seeded from the (now-current) global set, and
//     persist the selection to .graft/config.json.
//
// NON-INTERACTIVE (no TTY / --yes / --ci): skip all prompts. The global config
// is seeded to mode=all (every supported provider) on a true first run, and the
// project inherits the effective global set (no project override written).
func (c *DefaultCli) maybeRunFirstRun(out io.Writer, autoYes bool) error {
	firstRun := c.firstRunNeeded()

	cfg, err := ResolveConfig(c.configResolver)
	if err != nil {
		return err
	}

	interactive := !autoYes && isInteractive()
	if !interactive {
		// Non-interactive: seed a missing global config to mode=all; the project
		// inherits the effective global set (no project override written).
		if firstRun {
			renderBranding(out)
			cfg.Providers.Mode = config.ProviderModeAll
			cfg.Providers.Disabled = []string{}
			if err := SaveConfig(c.configResolver, cfg); err != nil {
				return err
			}
			fmt.Fprintf(out, "Enabled all %d providers (mode=all). Narrow with `graft config set`.\n",
				len(config.SupportedProviders()))
		}
		return nil
	}

	renderBranding(out)
	home := userHome()
	detected := detectInstalledProviders(home)

	// Step 1: reconcile GLOBAL enabled set with detected providers when they
	// differ. The pre-check seeds from detected so a brand-new machine gets a
	// sensible default.
	globalEnabled := cfg.EffectiveProviders()
	if differs(detected, globalEnabled) {
		selected, serr := runChecklist(out,
			"Enable these AI coding tools globally (detected ones pre-checked)",
			config.SupportedProviders(), detected)
		if serr == nil {
			cfg.Providers.Mode = config.ProviderModeSpecific
			cfg.Providers.Enabled = selected
			if err := SaveConfig(c.configResolver, cfg); err != nil {
				return err
			}
			fmt.Fprintf(out, "Global: enabled %d provider(s).\n", len(selected))
		}
	}

	// Step 2: PROJECT checklist seeded from the current global effective set.
	globalNow := cfg.EffectiveProviders()
	projSelected, perr := runChecklist(out,
		"Select the providers to sync in THIS project (seeded from global)",
		globalNow, globalNow)
	if perr == nil && c.projectResolver != nil {
		pc, gerr := c.projectResolver.Get()
		if gerr != nil {
			return gerr
		}
		pc.Providers = &config.ProvidersConfig{
			Mode:    config.ProviderModeSpecific,
			Enabled: projSelected,
		}
		if err := c.projectResolver.Save(pc); err != nil {
			return err
		}
		fmt.Fprintf(out, "Project: syncing to %d provider(s). Run `graft sync agents`.\n", len(projSelected))
	}
	return nil
}

// differs reports whether two provider id sets differ (order-insensitive).
func differs(a, b []string) bool {
	if len(a) != len(b) {
		return true
	}
	set := map[string]bool{}
	for _, x := range a {
		set[x] = true
	}
	for _, y := range b {
		if !set[y] {
			return true
		}
	}
	return false
}

// isInteractive reports whether stdin and stdout are both a TTY (so a huh form
// can run without hanging).
func isInteractive() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd())
}

// runChecklist renders a huh [x] multi-select over options, pre-checking the
// preChecked ids, and returns the user's selection.
func runChecklist(out io.Writer, title string, options, preChecked []string) ([]string, error) {
	checked := map[string]bool{}
	for _, d := range preChecked {
		checked[d] = true
	}

	opts := make([]huh.Option[string], 0, len(options))
	for _, id := range options {
		opts = append(opts, huh.NewOption(id, id).Selected(checked[id]))
	}

	selected := append([]string(nil), preChecked...)
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title(title).
				Description("Space toggles, Enter confirms.").
				Options(opts...).
				Value(&selected),
		),
	).WithOutput(out)

	if err := form.Run(); err != nil {
		return nil, err
	}
	return selected, nil
}

// renderBranding prints the lipgloss-styled Graft header + tagline to out.
func renderBranding(out io.Writer) {
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("63")).
		Render("Graft")
	line := lipgloss.NewStyle().
		Faint(true).
		Render(tagline)
	fmt.Fprintf(out, "\n%s — %s\n\n", header, line)
}
