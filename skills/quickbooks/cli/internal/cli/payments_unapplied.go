// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"github.com/spf13/cobra"

	"quickbooks-pp-cli/internal/analytics"
)

// pp:data-source local
func newNovelPaymentsUnappliedCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "unapplied",
		Short: "Payments with money received but not applied to an invoice",
		Long: "List payments where UnappliedAmt > 0 — cash received but not applied to any\n" +
			"invoice, a reconciliation leak that quietly distorts AR. Computed offline from\n" +
			"the local store, largest unapplied amount first. Run `sync` first.",
		Example:     "  quickbooks-cli payments unapplied --agent\n  quickbooks-cli payments unapplied --json | jq '.[].unapplied_amt'",
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
			if !hintIfUnsynced(cmd, db, "payments") {
				hintIfStale(cmd, db, "payments", flags.maxAge)
			}
			payments, err := loadResources(ctx, db, "payments")
			if err != nil {
				return err
			}
			rows := analytics.UnappliedPayments(payments)
			return flags.printJSON(cmd, rows)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Path to the local SQLite store (default: per-CLI data dir)")
	return cmd
}
