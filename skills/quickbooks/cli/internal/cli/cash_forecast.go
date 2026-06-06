// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"quickbooks-pp-cli/internal/analytics"
)

// pp:data-source local
func newNovelCashForecastCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var weeks int

	cmd := &cobra.Command{
		Use:   "cash-forecast",
		Short: "Scheduled cash movement by week: invoice inflows minus bill outflows",
		Long: "Project net cash movement over the next --weeks weeks by bucketing open invoice\n" +
			"balances (inflows) and open bill balances (outflows) by DueDate — scheduled, not\n" +
			"predicted. Overdue amounts and amounts beyond the window are reported separately.\n" +
			"Use this command to project net cash movement forward by week from due dates.\n" +
			"Do NOT use it for the current static cash position; use 'balances' instead.\n" +
			"Run `sync` first.",
		Example:     "  quickbooks-cli cash-forecast --weeks 4 --agent\n  quickbooks-cli cash-forecast --json | jq '.by_week'",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if weeks <= 0 || weeks > 52 {
				return usageErr(fmt.Errorf("--weeks must be between 1 and 52, got %d", weeks))
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
			bills, err := loadResources(ctx, db, "bills")
			if err != nil {
				return err
			}
			rep := analytics.CashForecast(invoices, bills, weeks, time.Now())
			return flags.printJSON(cmd, rep)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Path to the local SQLite store (default: per-CLI data dir)")
	cmd.Flags().IntVar(&weeks, "weeks", 4, "Number of forward weeks to project")
	return cmd
}
