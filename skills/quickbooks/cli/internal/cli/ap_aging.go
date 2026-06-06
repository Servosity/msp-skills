// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"time"

	"github.com/spf13/cobra"

	"quickbooks-pp-cli/internal/analytics"
)

// pp:data-source local
func newNovelApAgingCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "ap-aging",
		Short: "Age open payables and roll them up by vendor (offline)",
		Long: "The payables mirror of ar-aging: bucket open bill balances by age\n" +
			"(0-30 / 31-60 / 61-90 / 90+) and roll them up by vendor, computed from the\n" +
			"local store. Run `sync` first to populate the store.",
		Example:     "  quickbooks-cli ap-aging --agent\n  quickbooks-cli ap-aging --json | jq '.by_entity'",
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
			if !hintIfUnsynced(cmd, db, "bills") {
				hintIfStale(cmd, db, "bills", flags.maxAge)
			}
			bills, err := loadResources(ctx, db, "bills")
			if err != nil {
				return err
			}
			report := analytics.Aging(bills, "DueDate", "TxnDate", "Balance", "VendorRef", time.Now())
			return flags.printJSON(cmd, report)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Path to the local SQLite store (default: per-CLI data dir)")
	return cmd
}
