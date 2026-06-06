// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"time"

	"github.com/spf13/cobra"

	"quickbooks-pp-cli/internal/analytics"
)

// pp:data-source local
func newNovelBalancesCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "balances",
		Short: "Cash & balance snapshot across accounts, customers, and vendors (offline)",
		Long: "One view of bank/AR/AP account balances (rolled up by account type) plus total\n" +
			"customer receivables and total vendor payables. Spans three entity types, so it\n" +
			"is only possible once they coexist in the local store. Run `sync` first.",
		Example:     "  quickbooks-cli balances --agent\n  quickbooks-cli balances --json | jq '.bank_total'",
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
			if !hintIfUnsynced(cmd, db, "accounts") {
				hintIfStale(cmd, db, "accounts", flags.maxAge)
			}
			accounts, err := loadResources(ctx, db, "accounts")
			if err != nil {
				return err
			}
			customers, err := loadResources(ctx, db, "customers")
			if err != nil {
				return err
			}
			vendors, err := loadResources(ctx, db, "vendors")
			if err != nil {
				return err
			}
			report := analytics.Balances(accounts, customers, vendors, time.Now())
			return flags.printJSON(cmd, report)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Path to the local SQLite store (default: per-CLI data dir)")
	return cmd
}
