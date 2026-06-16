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

	// Re-init preservation (v0.0.6 issue #1): when this is NOT a true first run we
	// already have persisted config. The interactive flow must PREFILL from it, not
	// reset to detected/global defaults.
	//
	// Step 1 (GLOBAL): seed the pre-check from the EXISTING global effective set so
	// confirming the form preserves the user's prior global selection. On a true
	// first run there is no prior selection, so seed from detected (a sensible
	// default for a fresh machine). We only auto-show the global form when the
	// seed differs from the current effective set (first run with detected
	// providers); on re-init they match by construction, so the global step is
	// skipped and the prior global config is preserved untouched.
	globalEnabled := cfg.EffectiveProviders()
	globalSeed := detected
	if !firstRun {
		globalSeed = globalEnabled
	}
	if differs(globalSeed, globalEnabled) {
		selected, serr := runChecklist(out,
			"Enable these AI coding tools globally (detected ones pre-checked)",
			config.SupportedProviders(), globalSeed)
		if serr == nil {
			cfg.Providers.Mode = config.ProviderModeSpecific
			cfg.Providers.Enabled = selected
			if err := SaveConfig(c.configResolver, cfg); err != nil {
				return err
			}
			fmt.Fprintf(out, "Global: enabled %d provider(s).\n", len(selected))
		}
	}

	// Step 2 (PROJECT): prefill from the EXISTING project config when present so a
	// re-init preserves this project's prior provider selection (issue #1). Only
	// when the project has no override yet (true first project init) do we seed
	// from the global effective set. The form is still shown so the user can
	// adjust, but the pre-checked boxes reflect what is already configured rather
	// than clobbering it.
	globalNow := cfg.EffectiveProviders()
	var existingProj *config.ProjectConfig
	if c.projectResolver != nil {
		ep, gerr := c.projectResolver.Get()
		if gerr != nil {
			return gerr
		}
		existingProj = ep
	}
	projSeed, preserved := projectSeed(existingProj, globalNow)
	title := "Select the providers to sync in THIS project (seeded from global)"
	if preserved {
		title = "Select the providers to sync in THIS project (prefilled from existing config)"
	}
	projSelected, perr := runChecklist(out, title, config.SupportedProviders(), projSeed)
	if perr == nil && c.projectResolver != nil {
		pc := existingProj
		if pc == nil {
			pc = &config.ProjectConfig{}
		}
		pc.Providers = &config.ProvidersConfig{
			Mode:    config.ProviderModeSpecific,
			Enabled: projSelected,
		}
		if err := c.projectResolver.Save(pc); err != nil {
			return err
		}
		if preserved {
			fmt.Fprintf(out, "Project: preserved + syncing to %d provider(s). Run `graft sync agents`.\n", len(projSelected))
		} else {
			fmt.Fprintf(out, "Project: syncing to %d provider(s). Run `graft sync agents`.\n", len(projSelected))
		}
	}
	return nil
}

// projectSeed computes the pre-check seed for the PROJECT provider checklist and
// reports whether it was prefilled from an existing project override (issue #1).
// When the project already has a providers override it is preserved (prefilled);
// otherwise we seed from the global effective set (true first project init).
func projectSeed(existing *config.ProjectConfig, globalNow []string) (seed []string, preserved bool) {
	if existing != nil && existing.Providers != nil {
		return existing.Providers.EffectiveProviders(), true
	}
	return globalNow, false
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

// isInteractive reports whether stdin and stderr are both a TTY. huh renders to
// stderr (the results stream on stdout stays clean), so the form can only run
// without hanging when stdin AND stderr are terminals — checking stdout would
// wrongly enable the form when stdout is piped but stderr is a TTY.
func isInteractive() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stderr.Fd())
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
