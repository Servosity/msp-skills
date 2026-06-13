// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"syncro-pp-cli/internal/store"
)

// pp:data-source local
func newNovelBillingDriftCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit int
	var flagClosedBefore string

	cmd := &cobra.Command{
		Use:   "drift",
		Short: "Surface tickets closed long ago that have logged time but were never invoiced.",
		Long: `List tickets in the local store whose status is Resolved or Closed and whose
close/resolve timestamp is older than --closed-before, but which have no linked
invoice (no invoices row references their ticket id).`,
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
			cutoffDur := 14 * 24 * time.Hour
			if flagClosedBefore != "" {
				d, err := parseAgeDuration(flagClosedBefore)
				if err != nil {
					return usageErr(fmt.Errorf("invalid --closed-before: %w", err))
				}
				cutoffDur = d
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
			cutoff := now.Add(-cutoffDur)

			invoicedTickets := loadInvoicedTicketIDs(db)
			custNames := loadCustomerNames(db)

			type itemOut struct {
				TicketID     string `json:"ticket_id"`
				TicketNumber string `json:"ticket_number"`
				CustomerName string `json:"customer_name"`
				Status       string `json:"status"`
				ClosedAt     string `json:"closed_at"`
				DaysSince    int    `json:"days_since"`
			}
			items := []itemOut{}

			rows, err := db.Query(`SELECT id, data FROM resources WHERE resource_type = 'tickets'`)
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var id string
					var data []byte
					if err := rows.Scan(&id, &data); err != nil {
						continue
					}
					var obj map[string]any
					if json.Unmarshal(data, &obj) != nil {
						continue
					}
					status := novelJSONString(obj, "status", "state")
					ls := strings.ToLower(strings.TrimSpace(status))
					if ls != "resolved" && ls != "closed" {
						continue
					}
					if _, has := invoicedTickets[id]; has {
						continue
					}
					closedRaw := novelJSONString(obj, "resolved_at", "closed_at", "updated_at", "date_resolved")
					closedT, ok := novelParseTimestamp(closedRaw)
					if !ok {
						// Without a usable timestamp we cannot prove it's older
						// than the cutoff, so skip it.
						continue
					}
					if closedT.After(cutoff) {
						continue
					}
					custID := novelJSONString(obj, "customer_id", "customerId")
					items = append(items, itemOut{
						TicketID:     id,
						TicketNumber: novelJSONString(obj, "number", "ticket_number"),
						CustomerName: custNames[custID],
						Status:       status,
						ClosedAt:     closedT.Format(time.RFC3339),
						DaysSince:    int(now.Sub(closedT).Hours() / 24),
					})
				}
			}

			sort.Slice(items, func(i, j int) bool {
				return items[i].DaysSince > items[j].DaysSince
			})
			totalCount := len(items)
			if limit > 0 && len(items) > limit {
				items = items[:limit]
			}

			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{
					"items":       items,
					"total_count": totalCount,
				})
			}

			if totalCount == 0 {
				novelSyncHint(cmd.ErrOrStderr(), "No drift found. If tickets/invoices are not synced, run 'syncro-cli sync' first.")
				fmt.Fprintln(cmd.OutOrStdout(), "No uninvoiced closed tickets found.")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-10s %-12s %-26s %-10s %s\n", "TICKET", "NUMBER", "CUSTOMER", "STATUS", "DAYS")
			for _, it := range items {
				fmt.Fprintf(cmd.OutOrStdout(), "%-10s %-12s %-26s %-10s %d\n", it.TicketID, it.TicketNumber, truncate(it.CustomerName, 26), it.Status, it.DaysSince)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n%d closed ticket(s) with no invoice.\n", totalCount)
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/syncro-cli/data.db)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum tickets to show")
	cmd.Flags().StringVar(&flagClosedBefore, "closed-before", "14d", "Only tickets closed/resolved at least this long ago (e.g. 14d, 48h)")
	return cmd
}

// loadInvoicedTicketIDs returns the set of ticket ids referenced by any
// invoices row (via the extracted ticket_id column). Empty on an unsynced
// store.
func loadInvoicedTicketIDs(db *store.Store) map[string]struct{} {
	out := map[string]struct{}{}
	rows, err := db.Query(`SELECT DISTINCT ticket_id FROM invoices WHERE ticket_id IS NOT NULL`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var tid *int64
		if err := rows.Scan(&tid); err != nil {
			continue
		}
		if tid != nil {
			out[fmt.Sprintf("%d", *tid)] = struct{}{}
		}
	}
	return out
}
