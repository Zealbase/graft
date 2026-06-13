package cli

import (
	"fmt"

	"github.com/Shaik-Sirajuddin/graft/internal/catalog"
	"github.com/spf13/cobra"
)

// newCatalogCommand builds the `graft catalog` group (v0.0.3 task 10). It does
// not touch the gateway/store — the catalog is embedded, offline data — so the
// command runs anywhere.
func (c *DefaultCli) newCatalogCommand() *cobra.Command {
	catalogCmd := &cobra.Command{
		Use:   "catalog",
		Short: "Inspect the embedded provider catalog",
	}
	catalogCmd.AddCommand(c.newCatalogVerifyCommand())
	return catalogCmd
}

// catalogVerifyResult is the machine shape of `catalog verify -o json|yaml`.
type catalogVerifyResult struct {
	OK        bool     `json:"ok" yaml:"ok"`
	Verified  []string `json:"verified,omitempty" yaml:"verified,omitempty"`
	Providers int      `json:"providers" yaml:"providers"`
	Error     string   `json:"error,omitempty" yaml:"error,omitempty"`
}

// newCatalogVerifyCommand builds `graft catalog verify`: recompute the embedded
// catalog hashes and compare against the manifest. Exits non-zero on mismatch.
func (c *DefaultCli) newCatalogVerifyCommand() *cobra.Command {
	flags := ProvisionCatalogFlags()
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify embedded catalog hashes match the manifest",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			resolved := flags
			if err := loadFlags(cmd, &resolved); err != nil {
				return err
			}
			cat, err := catalog.Load()
			if err != nil {
				return fmt.Errorf("catalog: load: %w", err)
			}
			verr := cat.Verify()
			res := catalogVerifyResult{
				OK:        verr == nil,
				Providers: len(catalog.Providers),
			}
			if verr == nil {
				res.Verified = append([]string(nil), catalog.Providers...)
			} else {
				res.Error = verr.Error()
			}

			switch resolved.Output {
			case "json":
				if perr := printJSON(cmd.OutOrStdout(), res); perr != nil {
					return perr
				}
			case "yaml", "yml":
				if perr := printYAML(cmd.OutOrStdout(), res); perr != nil {
					return perr
				}
			default:
				if verr == nil {
					fmt.Fprintf(cmd.OutOrStdout(), "catalog OK  (%d/%d providers verified)\n",
						len(catalog.Providers), len(catalog.Providers))
				}
			}
			// Non-zero exit on mismatch (after rendering the result/details).
			if verr != nil {
				return verr
			}
			return nil
		},
	}
	cmd.Flags().StringP("output", "o", flags.Output, "Output format: json|yaml|table")
	return cmd
}

// CatalogFlags is the flag schema for `graft catalog verify`.
type CatalogFlags struct {
	Output string `koanf:"output" json:"output"`
}

// ProvisionCatalogFlags returns catalog-command defaults.
func ProvisionCatalogFlags() CatalogFlags { return CatalogFlags{Output: "table"} }
