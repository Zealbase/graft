package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// completionTarget describes where a shell's completion script should be written
// and, when there is no auto-sourced dir, how to wire it up via the shell rc.
type completionTarget struct {
	Shell string
	// AutoPath is the standard auto-sourced completion file for the shell, when
	// one exists (bash-completion dir, a zsh fpath dir, fish completions dir).
	// Empty means "no standard auto dir — fall back to an rc source line".
	AutoPath string
	// RCFile + SourceLine are the fallback: append SourceLine to RCFile so the
	// shell sources `graft completion <shell>` on startup.
	RCFile     string
	SourceLine string
}

// resolveCompletionTarget computes the install target for a shell given the
// user's home dir and an XDG data home (both injectable for tests). It is the
// testable core of `graft completion install`.
func resolveCompletionTarget(shell, home, xdgData string) (completionTarget, error) {
	if xdgData == "" {
		xdgData = filepath.Join(home, ".local", "share")
	}
	switch shell {
	case "bash":
		// bash-completion v2 auto-sources ~/.local/share/bash-completion/completions/<name>.
		return completionTarget{
			Shell:      "bash",
			AutoPath:   filepath.Join(xdgData, "bash-completion", "completions", "graft"),
			RCFile:     filepath.Join(home, ".bashrc"),
			SourceLine: "source <(graft completion bash)",
		}, nil
	case "zsh":
		// A conventional user fpath dir; the file must be named _graft.
		return completionTarget{
			Shell:      "zsh",
			AutoPath:   filepath.Join(home, ".zsh", "completions", "_graft"),
			RCFile:     filepath.Join(home, ".zshrc"),
			SourceLine: "source <(graft completion zsh)",
		}, nil
	case "fish":
		return completionTarget{
			Shell:      "fish",
			AutoPath:   filepath.Join(home, ".config", "fish", "completions", "graft.fish"),
			RCFile:     filepath.Join(home, ".config", "fish", "config.fish"),
			SourceLine: "graft completion fish | source",
		}, nil
	default:
		return completionTarget{}, fmt.Errorf("unsupported shell %q (use: bash|zsh|fish)", shell)
	}
}

// detectShell guesses the user's shell from $SHELL (basename).
func detectShell() string {
	sh := os.Getenv("SHELL")
	if sh == "" {
		return ""
	}
	base := filepath.Base(sh)
	switch base {
	case "bash", "zsh", "fish":
		return base
	default:
		// Unknown shell: let the caller emit the clearer "pass one explicitly".
		return ""
	}
}

// newCompletionInstallCommand builds `graft completion install [bash|zsh|fish]`.
// It is attached to cobra's auto-generated `completion` command (see
// attachCompletionInstall). The raw `graft completion <shell>` stays available
// for manual piping.
func (c *DefaultCli) newCompletionInstallCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install [bash|zsh|fish]",
		Short: "Install graft shell completion into the shell's auto-sourced dir",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			shell := ""
			if len(args) == 1 {
				shell = args[0]
			} else {
				shell = detectShell()
			}
			if shell == "" {
				return fmt.Errorf("could not detect shell; pass one explicitly: graft completion install [bash|zsh|fish]")
			}
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("resolve home dir: %w", err)
			}
			target, err := resolveCompletionTarget(shell, home, os.Getenv("XDG_DATA_HOME"))
			if err != nil {
				return err
			}
			assumeYes, _ := cmd.Flags().GetBool("yes")
			return c.installCompletion(cmd, target, assumeYes)
		},
	}
	cmd.Flags().Bool("yes", false, "Assume yes for the rc-file fallback consent prompt")
	return cmd
}

// installCompletion writes the completion script to the target's auto dir, or
// (when there is none / it is not writable) appends a source line to the rc file
// with consent.
func (c *DefaultCli) installCompletion(cmd *cobra.Command, target completionTarget, assumeYes bool) error {
	out := cmd.OutOrStdout()

	// Preferred path: write the script into the shell's auto-sourced dir.
	if target.AutoPath != "" {
		if err := os.MkdirAll(filepath.Dir(target.AutoPath), 0o755); err == nil {
			if werr := c.writeCompletionScript(target.Shell, target.AutoPath); werr == nil {
				fmt.Fprintf(out, "Installed %s completion to %s\n", target.Shell, target.AutoPath)
				if target.Shell == "zsh" {
					fmt.Fprintf(out, "Ensure your ~/.zshrc has: fpath=(%s $fpath) before compinit\n", filepath.Dir(target.AutoPath))
				}
				return nil
			}
		}
	}

	// Fallback: append a source line to the rc file (with consent).
	if !assumeYes && !c.confirmRCEdit(cmd, target.RCFile, target.SourceLine) {
		fmt.Fprintf(out, "Skipped. To enable completion manually, add to %s:\n  %s\n", target.RCFile, target.SourceLine)
		return nil
	}
	if err := appendSourceLine(target.RCFile, target.SourceLine); err != nil {
		return fmt.Errorf("append to %s: %w", target.RCFile, err)
	}
	fmt.Fprintf(out, "Added completion source line to %s. Restart your shell.\n", target.RCFile)
	return nil
}

// writeCompletionScript generates the shell's completion script to path.
func (c *DefaultCli) writeCompletionScript(shell, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	switch shell {
	case "bash":
		return c.root.GenBashCompletionV2(f, true)
	case "zsh":
		return c.root.GenZshCompletion(f)
	case "fish":
		return c.root.GenFishCompletion(f, true)
	default:
		return fmt.Errorf("unsupported shell %q", shell)
	}
}

// confirmRCEdit asks the user to confirm appending a source line to their rc
// file. Non-interactive input (no TTY) declines (returns false) so we never hang.
func (c *DefaultCli) confirmRCEdit(cmd *cobra.Command, rcFile, line string) bool {
	if !isInteractive() {
		return false
	}
	fmt.Fprintf(cmd.OutOrStdout(), "No auto-completion dir found. Append this line to %s?\n  %s\n[y/N]: ", rcFile, line)
	var resp string
	fmt.Fscanln(cmd.InOrStdin(), &resp)
	resp = strings.ToLower(strings.TrimSpace(resp))
	return resp == "y" || resp == "yes"
}

// appendSourceLine appends line to rcFile (creating it) unless already present.
func appendSourceLine(rcFile, line string) error {
	if data, err := os.ReadFile(rcFile); err == nil && strings.Contains(string(data), line) {
		return nil // already present — idempotent
	}
	if err := os.MkdirAll(filepath.Dir(rcFile), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(rcFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "\n# graft shell completion\n%s\n", line)
	return err
}

// attachCompletionInstall wires the `install` subcommand onto cobra's
// auto-generated `completion` command so `graft completion install` works
// alongside the raw `graft completion <shell>`.
func (c *DefaultCli) attachCompletionInstall() {
	if c.root == nil {
		return
	}
	// cobra adds the `completion` command lazily during Execute; materialize it
	// now so we can attach the `install` subcommand to it.
	c.root.InitDefaultCompletionCmd()
	for _, sub := range c.root.Commands() {
		if sub.Name() == "completion" {
			sub.AddCommand(c.newCompletionInstallCommand())
			return
		}
	}
}
