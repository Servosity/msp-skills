// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"quickbooks-pp-cli/internal/analytics"
)

// pp:data-source local
func newNovelVendorsSpendCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var since string
	var limit int

	cmd := &cobra.Command{
		Use:   "spend",
		Short: "Total billed per vendor over a date window, ranked",
		Long: "Sum bill totals per vendor (optionally since --since YYYY-MM-DD), ranked highest\n" +
			"first — see where the money goes for spend review or vendor negotiation. A local\n" +
			"join+aggregate across bills and vendors. Run `sync` first.",
		Example:     "  quickbooks-cli vendors spend --since 2026-01-01 --agent\n  quickbooks-cli vendors spend --limit 10 --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			var sinceT time.Time
			hasSince := false
			if since != "" {
				t, ok := analytics.ParseDate(since)
				if !ok {
					return usageErr(fmt.Errorf("--since must be a date like 2026-01-01, got %q", since))
				}
				sinceT, hasSince = t, true
			}
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			ctx := cmd.Context()
			db, err := openLocalStore(ctx, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "bills") {
				hintIfStale(cmd, db, "bills", flags.maxAge)
			}
			bills, err := loadResources(ctx, db, "bills")
			if err != nil {
				return err
			}
			rows := analytics.SumByRef(bills, "TotalAmt", "VendorRef", "TxnDate", sinceT, hasSince)
			if limit > 0 && len(rows) > limit {
				rows = rows[:limit]
			}
			return flags.printJSON(cmd, rows)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Path to the local SQLite store (default: per-CLI data dir)")
	cmd.Flags().StringVar(&since, "since", "", "Only count bills on/after this date (YYYY-MM-DD)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum vendors to return (0 = all)")
	return cmd
}
