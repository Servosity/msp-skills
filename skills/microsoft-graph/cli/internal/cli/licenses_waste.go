// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source local

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"microsoft-graph-pp-cli/internal/insights"
)

func newNovelLicensesWasteCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:   "waste",
		Short: "Find tenant SKUs with unused paid seats, ranked by waste",
		Long: strings.Trim(`
Surfaces every tenant subscription (SKU) where prepaid seats exceed consumed
seats — the recoverable license spend an MSP can reclaim at renewal. Reads the
locally synced subscribedSkus; run 'microsoft-graph-cli pull' first.

Unused = prepaidUnits.enabled - consumedUnits. SKUs that are fully consumed are
omitted. Results are ranked by unused seats, descending.`, "\n"),
		Example: strings.Trim(`
  microsoft-graph-cli licenses waste --agent
  microsoft-graph-cli licenses waste --csv`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			rows, err := loadDomainRows(dbPath, `SELECT data FROM licenses`)
			if err != nil {
				return apiErr(fmt.Errorf("reading local store: %w", err))
			}
			result := insights.LicenseWaste(rows)
			hintUnsyncedIfEmpty(cmd, dbPath, len(result) == 0)
			return printJSONFiltered(cmd.OutOrStdout(), result, flags)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: ~/.local/share/microsoft-graph-cli/data.db)")
	return cmd
}
