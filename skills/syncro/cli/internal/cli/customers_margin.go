// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelCustomersMarginCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit int
	var flagWindow string

	cmd := &cobra.Command{
		Use:   "margin",
		Short: "Compare logged labor against invoiced revenue per customer over a window.",
		Long: `For each customer, sum invoiced revenue and logged labor hours over the window
and compute revenue per labor hour. Revenue comes from the invoices table;
labor hours come from timer entries joined to tickets. Sorted by revenue.`,
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
			windowDur := 90 * 24 * time.Hour
			if flagWindow != "" {
				d, err := parseAgeDuration(flagWindow)
				if err != nil {
					return usageErr(fmt.Errorf("invalid --window: %w", err))
				}
				windowDur = d
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
			windowStart := now.Add(-windowDur)

			custNames := loadCustomerNames(db)
			ticketCustomer := loadTicketCustomerMap(db)

			type agg struct {
				revenue float64
				hours   float64
			}
			byCustomer := map[string]*agg{}
			ensure := func(id string) *agg {
				a := byCustomer[id]
				if a == nil {
					a = &agg{}
					byCustomer[id] = a
				}
				return a
			}

			// Revenue: invoices in window.
			invRows, err := db.Query(`SELECT
				json_extract(data, '$.customer_id') AS customer_id,
				COALESCE(due_date, date, created_at) AS effective_date,
				CAST(total AS REAL) AS amount
			FROM invoices`)
			if err == nil {
				for invRows.Next() {
					var custID *string
					var eff *string
					var amt *float64
					if err := invRows.Scan(&custID, &eff, &amt); err != nil {
						continue
					}
					if eff != nil {
						if t, ok := novelParseTimestamp(*eff); ok && t.Before(windowStart) {
							continue
						}
					}
					id := ""
					if custID != nil {
						id = *custID
					}
					a := ensure(id)
					if amt != nil {
						a.revenue += *amt
					}
				}
				_ = invRows.Close()
			}

			// Labor: timer entries in window, joined to ticket->customer.
			tRows, err := db.Query(`SELECT tickets_id, data FROM timer_entry`)
			if err == nil {
				for tRows.Next() {
					var ticketID *string
					var data []byte
					if err := tRows.Scan(&ticketID, &data); err != nil {
						continue
					}
					var obj map[string]any
					if json.Unmarshal(data, &obj) != nil {
						continue
					}
					if ts := novelJSONString(obj, "start_at", "created_at", "started_at"); ts != "" {
						if t, ok := novelParseTimestamp(ts); ok && t.Before(windowStart) {
							continue
						}
					}
					tid := ""
					if ticketID != nil {
						tid = *ticketID
					}
					custID := ticketCustomer[tid]
					ensure(custID).hours += timerEntryHours(obj)
				}
				_ = tRows.Close()
			}

			type itemOut struct {
				CustomerID     string   `json:"customer_id"`
				CustomerName   string   `json:"customer_name"`
				Revenue        float64  `json:"revenue"`
				LaborHours     float64  `json:"labor_hours"`
				RevenuePerHour *float64 `json:"revenue_per_hour"`
			}
			items := make([]itemOut, 0, len(byCustomer))
			for id, a := range byCustomer {
				it := itemOut{
					CustomerID:   id,
					CustomerName: custNames[id],
					Revenue:      a.revenue,
					LaborHours:   a.hours,
				}
				if a.hours > 0 {
					rph := a.revenue / a.hours
					it.RevenuePerHour = &rph
				}
				items = append(items, it)
			}
			sort.Slice(items, func(i, j int) bool {
				return items[i].Revenue > items[j].Revenue
			})
			if limit > 0 && len(items) > limit {
				items = items[:limit]
			}

			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{
					"window": flagWindowOrDefault(flagWindow),
					"items":  items,
				})
			}

			if len(items) == 0 {
				novelSyncHint(cmd.ErrOrStderr(), "No customer margin data. If invoices/timer entries are not synced, run 'syncro-cli sync' first.")
				fmt.Fprintln(cmd.OutOrStdout(), "No margin data found.")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-14s %-26s %-12s %-10s %s\n", "CUSTOMER_ID", "CUSTOMER", "REVENUE", "HOURS", "REV/HR")
			for _, it := range items {
				rph := "n/a"
				if it.RevenuePerHour != nil {
					rph = fmt.Sprintf("%.2f", *it.RevenuePerHour)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-14s %-26s %-12.2f %-10.2f %s\n", it.CustomerID, truncate(it.CustomerName, 26), it.Revenue, it.LaborHours, rph)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/syncro-cli/data.db)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum customers to show")
	cmd.Flags().StringVar(&flagWindow, "window", "90d", "Lookback window (e.g. 90d, 720h)")
	return cmd
}

func flagWindowOrDefault(w string) string {
	if w == "" {
		return "90d"
	}
	return w
}
