// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"github.com/spf13/cobra"

	"quickbooks-pp-cli/internal/analytics"
)

// pp:data-source local
func newNovelJournalEntriesCheckCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Flag journal entries that don't balance or touch suspense accounts",
		Long: "Sum each synced journal entry's debit and credit lines and flag entries that\n" +
			"don't net to zero (beyond a half-cent tolerance) or that post to suspense,\n" +
			"uncategorized, or opening-balance-equity accounts — the entries that keep a GL\n" +
			"from tying out at close. Computed offline from the local store. Run `sync` first.",
		Example:     "  quickbooks-cli journal-entries check --agent\n  quickbooks-cli journal-entries check --json | jq '.findings'",
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
			if !hintIfUnsynced(cmd, db, "journal-entries") {
				hintIfStale(cmd, db, "journal-entries", flags.maxAge)
			}
			entries, err := loadResources(ctx, db, "journal-entries")
			if err != nil {
				return err
			}
			rep := analytics.CheckJournalEntries(entries)
			return flags.printJSON(cmd, rep)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Path to the local SQLite store (default: per-CLI data dir)")
	return cmd
}
