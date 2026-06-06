// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"quickbooks-pp-cli/internal/analytics"
)

// pp:data-source local
func newNovelCustomersTopCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var by string
	var limit int

	cmd := &cobra.Command{
		Use:   "top",
		Short: "Rank customers by outstanding balance or lifetime invoiced",
		Long: "Rank customers by outstanding balance (--by balance, default) or by lifetime\n" +
			"invoiced total (--by invoiced, summed from synced invoices). A sort/aggregate\n" +
			"over the whole customer set the API can't return in one call. Run `sync` first.",
		Example:     "  quickbooks-cli customers top --by balance --limit 10 --agent\n  quickbooks-cli customers top --by invoiced --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			switch by {
			case "balance", "invoiced":
			default:
				return usageErr(fmt.Errorf("--by must be 'balance' or 'invoiced', got %q", by))
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
			if !hintIfUnsynced(cmd, db, "customers") {
				hintIfStale(cmd, db, "customers", flags.maxAge)
			}

			if by == "invoiced" {
				invoices, err := loadResources(ctx, db, "invoices")
				if err != nil {
					return err
				}
				spend := analytics.SumByRef(invoices, "TotalAmt", "CustomerRef", "", time.Time{}, false)
				ranked := make([]analytics.RankedRow, 0, len(spend))
				for _, s := range spend {
					ranked = append(ranked, analytics.RankedRow{ID: s.ID, Name: s.Name, Value: s.Total})
				}
				if limit > 0 && len(ranked) > limit {
					ranked = ranked[:limit]
				}
				return flags.printJSON(cmd, ranked)
			}

			customers, err := loadResources(ctx, db, "customers")
			if err != nil {
				return err
			}
			ranked := analytics.TopByField(customers, "Balance", "DisplayName", limit)
			return flags.printJSON(cmd, ranked)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Path to the local SQLite store (default: per-CLI data dir)")
	cmd.Flags().StringVar(&by, "by", "balance", "Ranking metric: balance | invoiced")
	cmd.Flags().IntVar(&limit, "limit", 10, "Maximum customers to return (0 = all)")
	return cmd
}
