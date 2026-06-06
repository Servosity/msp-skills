// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"time"

	"github.com/spf13/cobra"

	"quickbooks-pp-cli/internal/analytics"
)

// pp:data-source local
func newNovelArAgingCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "ar-aging",
		Short: "Age open receivables and roll them up by customer (offline)",
		Long: "Bucket open invoice balances by age (0-30 / 31-60 / 61-90 / 90+) and roll them\n" +
			"up by customer, computed from the local store. No single QuickBooks API call\n" +
			"returns aging — this joins open invoices to customers and does the date math\n" +
			"offline. Run `sync` first to populate the store.",
		Example:     "  quickbooks-cli ar-aging --agent\n  quickbooks-cli ar-aging --json | jq '.buckets'",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
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
			if !hintIfUnsynced(cmd, db, "invoices") {
				hintIfStale(cmd, db, "invoices", flags.maxAge)
			}
			invoices, err := loadResources(ctx, db, "invoices")
			if err != nil {
				return err
			}
			report := analytics.Aging(invoices, "DueDate", "TxnDate", "Balance", "CustomerRef", time.Now())
			return flags.printJSON(cmd, report)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Path to the local SQLite store (default: per-CLI data dir)")
	return cmd
}
