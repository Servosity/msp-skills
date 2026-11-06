// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"quickbooks-pp-cli/internal/analytics"
)

// pp:data-source local
func newNovelBillsDueCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var days int

	cmd := &cobra.Command{
		Use:   "due",
		Short: "Open bills due in the next N days (and any already overdue), sorted by due date with a per-vendor total",
		Long: "The AP payment-run list: every open bill due within --days (overdue bills\n" +
			"included), earliest due date first, plus what each vendor is owed inside that\n" +
			"window. Bills whose due date cannot be read are listed under \"undated\" rather\n" +
			"than guessed. Amounts are in home currency; a foreign-currency bill also shows\n" +
			"its currency and foreign_balance. Computed offline from the local store. Run\n" +
			"`sync` first.",
		Example:     "  quickbooks-cli bills due --days 7 --agent\n  quickbooks-cli bills due --days 14 --json --select by_vendor",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if days < 0 {
				return usageErr(fmt.Errorf("--days must be 0 or more, got %d", days))
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
			return flags.printJSON(cmd, analytics.BillsDue(bills, days, time.Now()))
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Path to the local SQLite store (default: per-CLI data dir)")
	cmd.Flags().IntVar(&days, "days", 7, "Window in days from today; overdue bills are always included")
	return cmd
}
