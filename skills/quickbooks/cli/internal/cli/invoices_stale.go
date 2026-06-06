// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"time"

	"github.com/spf13/cobra"

	"quickbooks-pp-cli/internal/analytics"
)

// pp:data-source local
func newNovelInvoicesStaleCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var days int

	cmd := &cobra.Command{
		Use:   "stale",
		Short: "Overdue invoices ranked by collection priority (age × balance)",
		Long: "List open invoices more than --days past due, ranked by age times balance so the\n" +
			"biggest, oldest debts surface first — a ready-made collections call list. Computed\n" +
			"offline from the local store. Run `sync` first.",
		Example:     "  quickbooks-cli invoices stale --days 60 --agent\n  quickbooks-cli invoices stale --days 30 --json --select id,doc_number,balance,customer",
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
			rows := analytics.StaleInvoices(invoices, days, time.Now())
			return flags.printJSON(cmd, rows)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Path to the local SQLite store (default: per-CLI data dir)")
	cmd.Flags().IntVar(&days, "days", 30, "Minimum days past due to count as stale")
	return cmd
}
