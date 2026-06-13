package cli

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/spf13/cobra"
)

// newDestroyCommand builds `graft destroy [--yes|--ci] [--keep-store]` (v0.0.3
// task 1): tear down this workspace's graft-managed state. It removes the in-repo
// .graft/ (or only its non-store parts with --keep-store), the global-db
// workspace rows, and the lock — but NEVER touches provider agent files
// (.claude/…, .codex/…, etc.). A confirmation prompt guards the destructive op
// unless --yes or --ci is passed.
func (c *DefaultCli) newDestroyCommand() *cobra.Command {
	flags := ProvisionDestroyFlags()
	cmd := &cobra.Command{
		Use:   "destroy",
		Short: "Remove graft state for this workspace (provider files are kept)",
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
			autoYes := resolved.Yes || resolved.CI
			if !autoYes {
				ok, cerr := confirmDestroy(cmd, resolved.KeepStore)
				if cerr != nil {
					return cerr
				}
				if !ok {
					fmt.Fprintln(cmd.OutOrStdout(), "aborted")
					return nil
				}
			}
			res, err := gate.Destroy(contract.DestroyOpts{KeepStore: resolved.KeepStore})
			if err != nil {
				return err
			}
			if perr := printOutput(cmd.OutOrStdout(), "destroy", resolved.Output, res); perr != nil {
				return perr
			}
			// Surface kept-store explicitly so `removed_dir: false` is not read as
			// "nothing happened". (DestroyResult is a frozen contract, so this is
			// a CLI-side annotation for the text view rather than a result field.)
			if resolved.KeepStore && resolved.Output != "json" && resolved.Output != "yaml" && resolved.Output != "yml" {
				fmt.Fprintln(cmd.OutOrStdout(), "kept_store: true  (.graft/agents canonical store retained)")
			}
			return nil
		},
	}
	cmd.Flags().StringP("output", "o", flags.Output, "Output format: json|yaml|table")
	cmd.Flags().Bool("yes", false, "Skip the confirmation prompt")
	cmd.Flags().Bool("ci", false, "Non-interactive: skip the confirmation prompt (alias of --yes)")
	cmd.Flags().Bool("keep-store", false, "Retain the canonical store (.graft/agents); drop config/db/lock")
	return cmd
}

// confirmDestroy prompts on stderr and reads a y/N answer from stdin. A non-"y"
// answer (or EOF) is treated as "no".
func confirmDestroy(cmd *cobra.Command, keepStore bool) (bool, error) {
	target := ".graft and forget this workspace"
	if keepStore {
		target = ".graft config/db/lock (keeping the canonical store) and forget this workspace"
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "Destroy %s? Provider files are kept. [y/N] ", target)
	reader := bufio.NewReader(cmd.InOrStdin())
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return false, nil // EOF / no input -> treat as no
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

// DestroyFlags is the flag schema for `graft destroy`.
type DestroyFlags struct {
	Output    string `koanf:"output" json:"output"`
	Yes       bool   `koanf:"yes" json:"yes"`
	CI        bool   `koanf:"ci" json:"ci"`
	KeepStore bool   `koanf:"keep-store" json:"keep-store"`
}

// ProvisionDestroyFlags returns destroy defaults.
func ProvisionDestroyFlags() DestroyFlags { return DestroyFlags{Output: "table"} }
