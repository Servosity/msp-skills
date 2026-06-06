// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source local

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"microsoft-graph-pp-cli/internal/insights"
)

func newNovelLicensesMapCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:   "map [sku]",
		Short: "List every consumer of one SKU with account state, to plan reassignment",
		Long: strings.Trim(`
Use this command to list every user consuming one specific SKU, with their
account state, to plan reassignment. Do NOT use it to rank tenant-wide
over-provisioned SKUs; use 'licenses waste' instead. Do NOT use it to find
disabled/guest holders across all SKUs; use 'licenses orphans' instead.

The SKU may be given as a part number (ENTERPRISEPACK) or a skuId, matched
case-insensitively against the locally synced subscribedSkus. Consumers flagged
disabled or guest sort first — they are the reclaimable seats. Reads the local
store; run 'microsoft-graph-cli pull' first.`, "\n"),
		Example: strings.Trim(`
  microsoft-graph-cli licenses map ENTERPRISEPACK --agent
  microsoft-graph-cli licenses map SPE_E5 --json --select consumers.userPrincipalName,reclaimableSeats`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "sku=ENTERPRISEPACK",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			if len(args) < 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a SKU part number or skuId is required"))
			}
			users, err := loadDomainRows(dbPath, `SELECT data FROM users WHERE user_principal_name IS NOT NULL`)
			if err != nil {
				return apiErr(fmt.Errorf("reading local store: %w", err))
			}
			skus, err := loadDomainRows(dbPath, `SELECT data FROM licenses`)
			if err != nil {
				return apiErr(fmt.Errorf("reading local store: %w", err))
			}
			result, ok := insights.LicenseMap(users, skus, args[0])
			if !ok {
				if len(skus) == 0 {
					// Empty store is an honest empty result, not bad input.
					result.Note = "local store has no subscribedSkus; run 'microsoft-graph-cli pull --only licenses' first"
					hintUnsyncedIfEmpty(cmd, dbPath, true)
					return printJSONFiltered(cmd.OutOrStdout(), result, flags)
				}
				return notFoundErr(fmt.Errorf("no subscribed SKU matches %q; see 'microsoft-graph-cli licenses skus' for available SKUs", args[0]))
			}
			hintUnsyncedIfEmpty(cmd, dbPath, len(result.Consumers) == 0)
			return printJSONFiltered(cmd.OutOrStdout(), result, flags)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: ~/.local/share/microsoft-graph-cli/data.db)")
	return cmd
}
