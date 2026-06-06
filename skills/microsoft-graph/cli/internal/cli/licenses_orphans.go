// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source local

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"microsoft-graph-pp-cli/internal/insights"
)

func newNovelLicensesOrphansCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:   "orphans",
		Short: "List disabled or guest accounts that still hold paid licenses",
		Long: strings.Trim(`
Joins the locally synced users with their assigned licenses to find accounts
that are disabled (accountEnabled=false) or guests but still hold a paid SKU —
licenses an MSP is paying for that no active member is using.

Requires users synced with assignedLicenses: run 'microsoft-graph-cli pull'
first (pull requests assignedLicenses via $select).`, "\n"),
		Example: strings.Trim(`
  microsoft-graph-cli licenses orphans --json
  microsoft-graph-cli licenses orphans --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			users, err := loadDomainRows(dbPath, `SELECT data FROM users WHERE user_principal_name IS NOT NULL`)
			if err != nil {
				return apiErr(fmt.Errorf("reading local store: %w", err))
			}
			skus, err := loadDomainRows(dbPath, `SELECT data FROM licenses`)
			if err != nil {
				return apiErr(fmt.Errorf("reading local store: %w", err))
			}
			result := insights.LicensedOrphans(users, insights.SkuNameMap(skus))
			hintUnsyncedIfEmpty(cmd, dbPath, len(result) == 0)
			return printJSONFiltered(cmd.OutOrStdout(), result, flags)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: ~/.local/share/microsoft-graph-cli/data.db)")
	return cmd
}
