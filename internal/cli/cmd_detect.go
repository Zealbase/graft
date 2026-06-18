package cli

import (
	"os"
	"path/filepath"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/spf13/cobra"
)

// newDetectCommand builds `graft detect`: a side-effect-free probe that answers
// "is this dir a graft workspace, and is it initialized?" without mutating
// anything. It deliberately does NOT construct the gateway (which would create
// .graft/ and seed git) — the report is computed from the filesystem alone, so a
// host can safely call it before deciding to consume graft.
//
// Contract (contract.DetectReport):
//   - non-graft dir         -> {isWorkspace:false, initialized:false, hint:"run graft init first"}
//   - .graft/ present, raw  -> {isWorkspace:true,  initialized:false, hint:"run graft init first"}
//   - .graft/agents present -> {isWorkspace:true,  initialized:true}
func (c *DefaultCli) newDetectCommand() *cobra.Command {
	flags := ProvisionDetectFlags()
	cmd := &cobra.Command{
		Use:   "detect",
		Short: "Report graft workspace/init state (side-effect-free; no writes)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			resolved := flags
			if err := loadFlags(cmd, &resolved); err != nil {
				return err
			}
			root, err := os.Getwd()
			if err != nil {
				return err
			}
			return printOutput(cmd.OutOrStdout(), "detect", resolved.Output, detectReport(root))
		},
	}
	cmd.Flags().StringP("output", "o", flags.Output, "Output format: json|yaml|table")
	return cmd
}

// detectReport computes the DetectReport for root from the filesystem only (no
// mutation). IsWorkspace is true when a .graft/ directory exists; Initialized is
// true once the canonical store (.graft/agents) has been created by `graft init`.
// The friendly "run graft init first" hint is reused (not a raw git error) for
// any not-yet-initialized state.
func detectReport(root string) contract.DetectReport {
	rep := contract.DetectReport{Root: root}

	graftPath := filepath.Join(root, ".graft")
	if fi, err := os.Stat(graftPath); err != nil || !fi.IsDir() {
		// No .graft/ at all: not a graft workspace.
		rep.Hint = "run graft init first"
		return rep
	}
	rep.IsWorkspace = true

	// Initialized == the canonical store dir exists (created by `graft init`).
	if fi, err := os.Stat(filepath.Join(graftPath, "agents")); err == nil && fi.IsDir() {
		rep.Initialized = true
		return rep
	}
	rep.Hint = "run graft init first"
	return rep
}
