// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"github.com/spf13/cobra"

	"quickbooks-pp-cli/internal/analytics"
)

// pp:data-source local
func newNovelCustomerProfitabilityCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit int

	cmd := &cobra.Command{
		Use:   "customer-profitability",
		Short: "Per-customer net position: invoiced minus paid, open balance, days-to-pay",
		Long: "Join invoices, payments, and customers across the whole synced book into one\n" +
			"per-customer view: lifetime invoiced, payments received, net outstanding, open\n" +
			"balance, and realized average days-to-pay from payment-to-invoice links.\n" +
			"Use this command for per-customer net position (invoiced minus paid) and\n" +
			"days-to-pay. Do NOT use it for a single-metric balance ranking; use\n" +
			"'customers top' instead. Run `sync` first.",
		Example:     "  quickbooks-cli customer-profitability --agent\n  quickbooks-cli customer-profitability --limit 10 --json",
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
			payments, err := loadResources(ctx, db, "payments")
			if err != nil {
				return err
			}
			customers, err := loadResources(ctx, db, "customers")
			if err != nil {
				return err
			}
			rows := analytics.CustomerProfitability(invoices, payments, customers, limit)
			return flags.printJSON(cmd, rows)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Path to the local SQLite store (default: per-CLI data dir)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum customers to return (0 = all)")
	return cmd
}
