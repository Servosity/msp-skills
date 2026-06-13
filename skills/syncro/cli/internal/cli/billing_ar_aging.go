// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelBillingArAgingCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit int
	var customer string

	cmd := &cobra.Command{
		Use:   "ar-aging",
		Short: "Bucket unpaid invoices into 0-30/30-60/60-90/90+ day aging tiers.",
		Long: `Bucket unpaid (is_paid=0) invoices from the local store into accounts-receivable
aging tiers based on the age of each invoice's due date (falling back to invoice
date, then created_at). Sums the outstanding total per bucket.`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return nil
			}
			if dbPath == "" {
				dbPath = defaultDBPath("syncro-cli")
			}
			db, err := syncroOpenStore(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w", err)
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, "") {
				hintIfStale(cmd, db, "", flags.maxAge)
			}

			now := time.Now()

			query := `SELECT
				COALESCE(due_date, date, created_at) AS effective_date,
				CAST(total AS REAL) AS amount
			FROM invoices
			WHERE COALESCE(is_paid, 0) = 0`
			qArgs := []any{}
			if customer != "" {
				query += ` AND json_extract(data, '$.customer_id') = ?`
				qArgs = append(qArgs, customer)
			}

			rows, err := db.Query(query, qArgs...)
			if err != nil {
				// Table may not exist yet on an unsynced store.
				return emitArAging(cmd, flags, now, 0, 0, nil)
			}
			defer rows.Close()

			type arRow struct {
				effDate string
				amount  float64
			}
			var collected []arRow
			for rows.Next() {
				var eff *string
				var amt *float64
				if err := rows.Scan(&eff, &amt); err != nil {
					continue
				}
				r := arRow{}
				if eff != nil {
					r.effDate = *eff
				}
				if amt != nil {
					r.amount = *amt
				}
				collected = append(collected, r)
			}

			type bucketAgg struct {
				count int
				total float64
			}
			buckets := map[string]*bucketAgg{
				"0-30":  {},
				"31-60": {},
				"61-90": {},
				"90+":   {},
			}
			var totalOutstanding float64
			var totalCount int
			for _, r := range collected {
				totalCount++
				totalOutstanding += r.amount
				ageDays := 0
				if t, ok := novelParseTimestamp(r.effDate); ok {
					ageDays = int(now.Sub(t).Hours() / 24)
				}
				var key string
				switch {
				case ageDays <= 30:
					key = "0-30"
				case ageDays <= 60:
					key = "31-60"
				case ageDays <= 90:
					key = "61-90"
				default:
					key = "90+"
				}
				buckets[key].count++
				buckets[key].total += r.amount
			}

			ordered := []string{"0-30", "31-60", "61-90", "90+"}
			out := make([]arBucketOut, 0, len(ordered))
			for _, k := range ordered {
				out = append(out, arBucketOut{Bucket: k, Count: buckets[k].count, Total: buckets[k].total})
			}

			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{
					"as_of":             now.Format(time.RFC3339),
					"buckets":           out,
					"total_outstanding": totalOutstanding,
					"total_count":       totalCount,
				})
			}

			if totalCount == 0 {
				novelSyncHint(cmd.ErrOrStderr(), "No unpaid invoices found. If you have not synced, run 'syncro-cli sync' first.")
				fmt.Fprintln(cmd.OutOrStdout(), "No outstanding receivables.")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "AR aging as of %s\n\n", now.Format("2006-01-02"))
			fmt.Fprintf(cmd.OutOrStdout(), "%-8s %-8s %s\n", "BUCKET", "COUNT", "TOTAL")
			fmt.Fprintf(cmd.OutOrStdout(), "%-8s %-8s %s\n", "------", "-----", "-----")
			for _, b := range out {
				fmt.Fprintf(cmd.OutOrStdout(), "%-8s %-8d %.2f\n", b.Bucket, b.Count, b.Total)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\nTotal outstanding: %.2f across %d invoices\n", totalOutstanding, totalCount)
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/syncro-cli/data.db)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Reserved for parity with other analytics commands")
	cmd.Flags().StringVar(&customer, "customer", "", "Filter to a single customer id")
	_ = limit
	return cmd
}

type arBucketOut struct {
	Bucket string  `json:"bucket"`
	Count  int     `json:"count"`
	Total  float64 `json:"total"`
}

// emitArAging emits an empty AR-aging result (used when the invoices table is
// absent on an unsynced store).
func emitArAging(cmd *cobra.Command, flags *rootFlags, now time.Time, totalOutstanding float64, totalCount int, _ []arBucketOut) error {
	out := []arBucketOut{
		{Bucket: "0-30"}, {Bucket: "31-60"}, {Bucket: "61-90"}, {Bucket: "90+"},
	}
	if flags.asJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{
			"as_of":             now.Format(time.RFC3339),
			"buckets":           out,
			"total_outstanding": totalOutstanding,
			"total_count":       totalCount,
		})
	}
	novelSyncHint(cmd.ErrOrStderr(), "No invoices in local store. Run 'syncro-cli sync' first.")
	fmt.Fprintln(cmd.OutOrStdout(), "No outstanding receivables.")
	return nil
}
