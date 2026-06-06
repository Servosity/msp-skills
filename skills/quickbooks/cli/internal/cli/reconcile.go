// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"time"

	"github.com/spf13/cobra"

	"quickbooks-pp-cli/internal/analytics"
)

// pp:data-source local
func newNovelReconcileCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "reconcile",
		Short: "Month-end hygiene sweep: every close check in one findings list",
		Long: "Run the composed close-hygiene sweep over the synced store: unapplied payments,\n" +
			"duplicate customers and vendors, unbalanced or suspense-touching journal\n" +
			"entries, and inactive records still referenced by open transactions — one\n" +
			"findings list with a clean/not-clean verdict.\n" +
			"Use this command for the full month-end hygiene sweep across all checks. Do NOT\n" +
			"use it when you only need unapplied payments; use 'payments unapplied' instead.\n" +
			"Run `sync` first.",
		Example:     "  quickbooks-cli reconcile --agent\n  quickbooks-cli reconcile --json | jq '.findings[].category'",
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
			if !hintIfUnsynced(cmd, db, "") {
				hintIfStale(cmd, db, "", flags.maxAge)
			}
			payments, err := loadResources(ctx, db, "payments")
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
			journalEntries, err := loadResources(ctx, db, "journal-entries")
			if err != nil {
				return err
			}
			invoices, err := loadResources(ctx, db, "invoices")
			if err != nil {
				return err
			}
			bills, err := loadResources(ctx, db, "bills")
			if err != nil {
				return err
			}
			items, err := loadResources(ctx, db, "items")
			if err != nil {
				return err
			}
			rep := analytics.Reconcile(payments, customers, vendors, journalEntries, invoices, bills, items, time.Now())
			return flags.printJSON(cmd, rep)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Path to the local SQLite store (default: per-CLI data dir)")
	return cmd
}
