// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"quickbooks-pp-cli/internal/analytics"
)

// pp:data-source local
func newNovelDsoCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var days int

	cmd := &cobra.Command{
		Use:   "dso",
		Short: "Days Sales Outstanding plus per-customer average days-to-pay",
		Long: "Compute DSO — (open AR balance / credit sales over the trailing --days window)\n" +
			"x window days — the headline collections-efficiency KPI, plus realized average\n" +
			"days-to-pay per customer from payment-to-invoice links, slowest payers first.\n" +
			"Computed offline from the local store. Run `sync` first.",
		Example:     "  quickbooks-cli dso --days 90 --agent\n  quickbooks-cli dso --json | jq '.dso_days'",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if days <= 0 {
				return usageErr(fmt.Errorf("--days must be a positive window, got %d", days))
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
			if !hintIfUnsynced(cmd, db, "invoices") {
				hintIfStale(cmd, db, "invoices", flags.maxAge)
			}
			invoices, err := loadResources(ctx, db, "invoices")
			if err != nil {
				return err
			}
			payments, err := loadResources(ctx, db, "payments")
			if err != nil {
				return err
			}
			rep := analytics.DSO(invoices, payments, days, time.Now())
			return flags.printJSON(cmd, rep)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Path to the local SQLite store (default: per-CLI data dir)")
	cmd.Flags().IntVar(&days, "days", 90, "Trailing window in days for credit sales")
	return cmd
}
