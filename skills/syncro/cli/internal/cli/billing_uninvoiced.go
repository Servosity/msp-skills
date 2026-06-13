// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"syncro-pp-cli/internal/store"
)

// pp:data-source local
func newNovelBillingUninvoicedCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit int
	var customer string

	cmd := &cobra.Command{
		Use:   "uninvoiced",
		Short: "Find logged-but-unbilled labor across every customer so no billable time slips.",
		Long: `Aggregate timer entries from the local store that have not yet been invoiced
(no invoice_id and not explicitly marked non-chargeable), join each entry's
ticket to its customer, and group uninvoiced hours per customer.`,
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

			custNames := loadCustomerNames(db)
			ticketCustomer := loadTicketCustomerMap(db)

			type agg struct {
				customerID string
				name       string
				hours      float64
				entries    int
			}
			byCustomer := map[string]*agg{}
			var totalHours float64

			rows, err := db.Query(`SELECT tickets_id, data FROM timer_entry`)
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var ticketID *string
					var data []byte
					if err := rows.Scan(&ticketID, &data); err != nil {
						continue
					}
					var obj map[string]any
					if json.Unmarshal(data, &obj) != nil {
						continue
					}
					if !timerEntryUninvoiced(obj) {
						continue
					}
					hrs := timerEntryHours(obj)
					tid := ""
					if ticketID != nil {
						tid = *ticketID
					}
					custID := ticketCustomer[tid]
					if customer != "" && custID != customer {
						continue
					}
					a := byCustomer[custID]
					if a == nil {
						a = &agg{customerID: custID, name: custNames[custID]}
						byCustomer[custID] = a
					}
					a.hours += hrs
					a.entries++
					totalHours += hrs
				}
			}

			type itemOut struct {
				CustomerID      string  `json:"customer_id"`
				CustomerName    string  `json:"customer_name"`
				UninvoicedHours float64 `json:"uninvoiced_hours"`
				UninvoicedCount int     `json:"uninvoiced_entries"`
			}
			items := make([]itemOut, 0, len(byCustomer))
			for _, a := range byCustomer {
				items = append(items, itemOut{
					CustomerID:      a.customerID,
					CustomerName:    a.name,
					UninvoicedHours: a.hours,
					UninvoicedCount: a.entries,
				})
			}
			sort.Slice(items, func(i, j int) bool {
				return items[i].UninvoicedHours > items[j].UninvoicedHours
			})
			if limit > 0 && len(items) > limit {
				items = items[:limit]
			}

			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{
					"items":                  items,
					"total_uninvoiced_hours": totalHours,
				})
			}

			if len(items) == 0 {
				novelSyncHint(cmd.ErrOrStderr(), "No uninvoiced timer entries found. Timer entries may not be synced; run 'syncro-cli sync' first.")
				fmt.Fprintln(cmd.OutOrStdout(), "No uninvoiced labor found.")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-14s %-30s %-10s %s\n", "CUSTOMER_ID", "CUSTOMER", "HOURS", "ENTRIES")
			for _, it := range items {
				name := it.CustomerName
				if name == "" {
					name = "(unknown)"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-14s %-30s %-10.2f %d\n", it.CustomerID, truncate(name, 30), it.UninvoicedHours, it.UninvoicedCount)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\nTotal uninvoiced hours: %.2f\n", totalHours)
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/syncro-cli/data.db)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum customers to show")
	cmd.Flags().StringVar(&customer, "customer", "", "Filter to a single customer id")
	return cmd
}

// timerEntryUninvoiced reports whether a timer entry's JSON indicates it has
// not yet been billed: no invoice_id present, and not explicitly flagged
// non-chargeable. Missing invoice_id + chargeable != false == uninvoiced.
func timerEntryUninvoiced(obj map[string]any) bool {
	for _, k := range []string{"invoice_id", "invoiceId", "invoiced", "is_invoiced"} {
		if v, ok := obj[k]; ok && v != nil {
			switch t := v.(type) {
			case bool:
				if t {
					return false
				}
			case string:
				if t != "" && t != "0" && t != "false" {
					return false
				}
			case float64:
				if t != 0 {
					return false
				}
			}
		}
	}
	// Explicit non-chargeable flags exclude the entry.
	if v, ok := obj["chargeable"]; ok {
		if b, ok := v.(bool); ok && !b {
			return false
		}
	}
	if v, ok := obj["charged"]; ok {
		if b, ok := v.(bool); ok && b {
			return false
		}
	}
	if v, ok := obj["billed"]; ok {
		if b, ok := v.(bool); ok && b {
			return false
		}
	}
	return true
}

// timerEntryHours derives hours from a timer entry's JSON, preferring an
// explicit duration (seconds) then falling back to start/end timestamps.
func timerEntryHours(obj map[string]any) float64 {
	for _, k := range []string{"duration", "duration_seconds", "seconds", "billed_duration"} {
		if v, ok := obj[k]; ok {
			if f, ok := v.(float64); ok && f > 0 {
				return f / 3600.0
			}
		}
	}
	if v, ok := obj["hours"]; ok {
		if f, ok := v.(float64); ok && f > 0 {
			return f
		}
	}
	start := novelJSONString(obj, "start_at", "started_at", "start")
	end := novelJSONString(obj, "end_at", "ended_at", "end")
	if start != "" && end != "" {
		if st, ok := novelParseTimestamp(start); ok {
			if et, ok := novelParseTimestamp(end); ok && et.After(st) {
				return et.Sub(st).Hours()
			}
		}
	}
	return 0
}

// loadCustomerNames returns a map of customer id -> display name from the
// resources table (resource_type=customers). Empty on an unsynced store.
func loadCustomerNames(db *store.Store) map[string]string {
	out := map[string]string{}
	rows, err := db.Query(`SELECT id, data FROM resources WHERE resource_type = 'customers'`)
	if err != nil {
		return out
	}
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
		name := novelJSONString(obj, "business_then_name", "business_name", "fullname", "full_name", "name", "firstname")
		out[id] = name
	}
	return out
}

// loadTicketCustomerMap returns a map of ticket id -> customer id from the
// resources table (resource_type=tickets). Empty on an unsynced store.
func loadTicketCustomerMap(db *store.Store) map[string]string {
	out := map[string]string{}
	rows, err := db.Query(`SELECT id, data FROM resources WHERE resource_type = 'tickets'`)
	if err != nil {
		return out
	}
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
		out[id] = novelJSONString(obj, "customer_id", "customerId")
	}
	return out
}
