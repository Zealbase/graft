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

// maybeRunFirstRun runs the first-run provider-selection flow when needed,
// persisting the result. autoYes forces the non-interactive path (used by
// --yes / CI). It writes branding + prompts to stderr (results stream stays
// clean) and never hangs.
//
// INTERACTIVE: the user confirms a [x] checklist -> persist mode=specific with
// the explicit selection.
// NON-INTERACTIVE (no TTY / --yes): do NOT silently restrict an unconfirmed
// machine -> persist mode=all (sync to every supported provider). This keeps
// scripted/CI runs predictable; the user can later narrow via `config set`.
func (c *DefaultCli) maybeRunFirstRun(out io.Writer, autoYes bool) error {
	if !c.firstRunNeeded() {
		return nil
	}
	home := userHome()
	detected := detectInstalledProviders(home)

	cfg, err := ResolveConfig(c.configResolver)
	if err != nil {
		return err
	}

	interactive := !autoYes && isInteractive()
	if interactive {
		selected, serr := runProviderChecklist(out, detected)
		if serr == nil {
			cfg.Providers.Mode = config.ProviderModeSpecific
			cfg.Providers.Enabled = selected
			if err := SaveConfig(c.configResolver, cfg); err != nil {
				return err
			}
			fmt.Fprintf(out, "Enabled %d provider(s). Run `graft sync agents`.\n", len(selected))
			return nil
		}
		// Checklist failed (e.g. surprise non-TTY) — fall through to all-mode.
	}

	// Non-interactive (or checklist error): mode=all, every supported provider.
	renderBranding(out)
	cfg.Providers.Mode = config.ProviderModeAll
	cfg.Providers.Disabled = []string{}
	if err := SaveConfig(c.configResolver, cfg); err != nil {
		return err
	}
	fmt.Fprintf(out, "Enabled all %d providers (mode=all). Narrow with `graft config set`.\n", len(config.SupportedProviders()))
	return nil
}

// isInteractive reports whether stdin and stdout are both a TTY (so a huh form
// can run without hanging).
func isInteractive() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd())
}

// runProviderChecklist renders branding then a huh [x] multi-select pre-checking
// the detected providers, returning the user's selection.
func runProviderChecklist(out io.Writer, detected []string) ([]string, error) {
	renderBranding(out)

	detectedSet := map[string]bool{}
	for _, d := range detected {
		detectedSet[d] = true
	}

	opts := make([]huh.Option[string], 0, len(config.SupportedProviders()))
	for _, id := range config.SupportedProviders() {
		opts = append(opts, huh.NewOption(id, id).Selected(detectedSet[id]))
	}

	selected := append([]string(nil), detected...)
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select the AI coding tools graft should sync to").
				Description("Detected tools are pre-checked. Space toggles, Enter confirms.").
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
