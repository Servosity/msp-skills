// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"github.com/spf13/cobra"

	"quickbooks-pp-cli/internal/analytics"
)

// pp:data-source local
func newNovelDupesCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dupes",
		Short: "Find duplicate name-list records (customers or vendors)",
		Long: "Fuzzy-match customers or vendors by normalized display name and email to catch\n" +
			"duplicate records (\"Acme Inc\" vs \"Acme, Inc.\") before they fragment AR/AP.\n" +
			"Cross-record comparison over the full synced set. Run `sync` first.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newNovelDupesCustomersCmd(flags))
	cmd.AddCommand(newNovelDupesVendorsCmd(flags))
	return cmd
}

// dupesRunE is the shared implementation for both `dupes customers` and
// `dupes vendors`; only the resource_type differs.
func dupesRunE(flags *rootFlags, dbPath *string, resourceType string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if dryRunOK(flags) {
			return nil
		}
		if err := validateDataSourceStrategy(flags, "local"); err != nil {
			return err
		}
		ctx := cmd.Context()
		db, err := openLocalStore(ctx, *dbPath)
		if err != nil {
			return err
		}
		defer db.Close()
		if !hintIfUnsynced(cmd, db, resourceType) {
			hintIfStale(cmd, db, resourceType, flags.maxAge)
		}
		items, err := loadResources(ctx, db, resourceType)
		if err != nil {
			return err
		}
		groups := analytics.Dupes(items)
		return flags.printJSON(cmd, groups)
	}
}
