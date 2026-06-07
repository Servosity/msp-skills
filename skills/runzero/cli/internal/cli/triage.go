// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"runzero-pp-cli/internal/inventory"
)

// pp:data-source local
func newNovelTriageCmd(flags *rootFlags) *cobra.Command {
	var flagInternetFacing bool
	var dbPath string

	cmd := &cobra.Command{
		Use:   "triage",
		Short: "Rank assets by criticality joined to their exposed services and high-severity vulnerability findings.",
		Long: strings.Trim(`
Rank locally-synced assets by criticality, then by their highest vulnerability
severity, then by vulnerability count. Joins assets, services, and
vulnerabilities in one pass — something the live runZero API cannot return in a
single call. Reads the local store; run 'inventory sync' first.`, "\n"),
		Example: strings.Trim(`
  # Rank everything by exposure
  runzero-cli triage --agent

  # Only externally-exposed assets
  runzero-cli triage --internet-facing --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			st, err := openInvStore(cmd, dbPath)
			if err != nil {
				return err
			}
			defer st.Close()
			rows, err := inventory.Triage(cmd.Context(), st.DB(), flagInternetFacing)
			if err != nil {
				return err
			}
			return flags.printJSON(cmd, rows)
		},
	}
	cmd.Flags().BoolVar(&flagInternetFacing, "internet-facing", false, "Only rank externally-exposed (internet-facing) assets")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/runzero-cli/data.db)")
	return cmd
}
