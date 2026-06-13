// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// profileTicket is one ticket row surfaced on the customer card.
type profileTicket struct {
	TicketID  string `json:"ticket_id"`
	Number    string `json:"number"`
	Subject   string `json:"subject"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at,omitempty"`
}

// pp:data-source local
func newNovelCustomersProfileCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var alertWindow string

	cmd := &cobra.Command{
		Use:   "profile [customer-id]",
		Short: "One-shot cross-entity customer snapshot: tickets, assets, AR balance, contracts, and latest RMM alerts.",
		Long: `Use this command for a single-customer cross-entity snapshot built from the
local store: open ticket load, asset count, unpaid invoice balance, contracts
on file, and recent RMM alert pressure in one card.
Do NOT use it for cross-customer rankings; use 'alerts noise' or
'billing ar-aging' instead.`,
		Example: strings.Trim(`
  # Full customer card as JSON for an agent
  syncro-cli customers profile 12345 --json

  # Narrow the card to money fields only
  syncro-cli customers profile 12345 --json --select customer_id,unpaid_invoice_total,unpaid_invoice_count
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would build a cross-entity profile card from the local store")
				return nil
			}
			if len(args) < 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("customer-id is required"))
			}
			customerID := strings.TrimSpace(args[0])
			if customerID == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("customer-id is required"))
			}
			window := 30 * 24 * time.Hour
			windowLabel := "30d"
			if alertWindow != "" {
				d, err := parseAgeDuration(alertWindow)
				if err != nil {
					return usageErr(fmt.Errorf("invalid --alert-window: %w", err))
				}
				window = d
				windowLabel = alertWindow
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

			// Customer record.
			customerName := ""
			customerFound := false
			if row := db.DB().QueryRow(
				`SELECT data FROM resources WHERE resource_type = 'customers' AND id = ?`,
				customerID,
			); row != nil {
				var data []byte
				if err := row.Scan(&data); err == nil {
					var obj map[string]any
					if json.Unmarshal(data, &obj) == nil {
						customerFound = true
						customerName = novelJSONString(obj, "business_then_name", "business_name", "fullname", "full_name", "name", "firstname")
					}
				}
			}

			// Tickets: totals, open count, newest open tickets.
			ticketTotal := 0
			ticketOpen := 0
			var openTickets []profileTicket
			if rows, err := db.Query(`SELECT id, data FROM resources WHERE resource_type = 'tickets'`); err == nil {
				func() {
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
						if novelJSONString(obj, "customer_id", "customerId") != customerID {
							continue
						}
						ticketTotal++
						status := novelJSONString(obj, "status", "state")
						ls := strings.ToLower(strings.TrimSpace(status))
						if ls == "resolved" || ls == "closed" {
							continue
						}
						ticketOpen++
						openTickets = append(openTickets, profileTicket{
							TicketID:  id,
							Number:    novelJSONString(obj, "number", "ticket_number"),
							Subject:   novelJSONString(obj, "subject", "title"),
							Status:    status,
							CreatedAt: novelJSONString(obj, "created_at", "createdAt"),
						})
					}
				}()
			}
			sort.Slice(openTickets, func(i, j int) bool {
				// Prefer chronological comparison: stored created_at strings can
				// mix RFC3339 Z/offset and SQLite space forms, where lexical
				// order diverges from time order. Fall back to string compare
				// only when either side fails to parse.
				ti, oki := novelParseTimestamp(openTickets[i].CreatedAt)
				tj, okj := novelParseTimestamp(openTickets[j].CreatedAt)
				if oki && okj {
					return ti.After(tj)
				}
				return openTickets[i].CreatedAt > openTickets[j].CreatedAt
			})
			if len(openTickets) > 5 {
				openTickets = openTickets[:5]
			}

			// Assets.
			assetCount := 0
			if rows, err := db.Query(`SELECT data FROM customer_assets`); err == nil {
				func() {
					defer rows.Close()
					for rows.Next() {
						var data []byte
						if err := rows.Scan(&data); err != nil {
							continue
						}
						var obj map[string]any
						if json.Unmarshal(data, &obj) != nil {
							continue
						}
						if novelJSONString(obj, "customer_id", "customerId") == customerID {
							assetCount++
						}
					}
				}()
			}

			// Invoices: unpaid balance over the typed ledger.
			unpaidTotal := 0.0
			unpaidCount := 0
			if rows, err := db.Query(
				`SELECT CAST(total AS REAL) FROM invoices
				 WHERE COALESCE(is_paid, 0) = 0
				   AND json_extract(data, '$.customer_id') = ?`,
				customerID,
			); err == nil {
				func() {
					defer rows.Close()
					for rows.Next() {
						var amount float64
						if err := rows.Scan(&amount); err != nil {
							continue
						}
						unpaidTotal += amount
						unpaidCount++
					}
				}()
			}

			// Contracts on file.
			contractCount := 0
			var contractNames []string
			if rows, err := db.Query(`SELECT data FROM resources WHERE resource_type = 'contracts'`); err == nil {
				func() {
					defer rows.Close()
					for rows.Next() {
						var data []byte
						if err := rows.Scan(&data); err != nil {
							continue
						}
						var obj map[string]any
						if json.Unmarshal(data, &obj) != nil {
							continue
						}
						if novelJSONString(obj, "customer_id", "customerId") != customerID {
							continue
						}
						contractCount++
						if name := novelJSONString(obj, "name", "title", "contract_name"); name != "" {
							contractNames = append(contractNames, name)
						}
					}
				}()
			}
			sort.Strings(contractNames)
			if len(contractNames) > 5 {
				contractNames = contractNames[:5]
			}
			if contractNames == nil {
				contractNames = []string{}
			}

			// RMM alerts inside the window, newest timestamp surfaced.
			alertCount := 0
			latestAlert := ""
			cutoff := time.Now().Add(-window)
			for _, a := range loadRMMAlerts(db) {
				if a.customerID != customerID {
					continue
				}
				if a.hasCreated && a.created.Before(cutoff) {
					continue
				}
				alertCount++
				if a.hasCreated {
					ts := a.created.UTC().Format(time.RFC3339)
					if ts > latestAlert {
						latestAlert = ts
					}
				}
			}

			if openTickets == nil {
				openTickets = []profileTicket{}
			}
			card := map[string]any{
				"customer_id":          customerID,
				"customer_name":        customerName,
				"customer_synced":      customerFound,
				"ticket_total":         ticketTotal,
				"ticket_open":          ticketOpen,
				"open_tickets":         openTickets,
				"asset_count":          assetCount,
				"unpaid_invoice_total": unpaidTotal,
				"unpaid_invoice_count": unpaidCount,
				"contract_count":       contractCount,
				"contracts":            contractNames,
				"alert_count_window":   alertCount,
				"alert_window":         windowLabel,
				"latest_alert_at":      latestAlert,
			}

			if flags.asJSON {
				return printJSONFiltered(cmd.OutOrStdout(), card, flags)
			}

			name := customerName
			if name == "" {
				name = "(unknown)"
			}
			if !customerFound {
				novelSyncHint(cmd.ErrOrStderr(), fmt.Sprintf("Customer %s not found in the local store. Customers may not be synced; run 'syncro-cli sync' first.", customerID))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Customer %s — %s\n", customerID, truncate(name, 40))
			fmt.Fprintf(cmd.OutOrStdout(), "Tickets:   %d open / %d total\n", ticketOpen, ticketTotal)
			for _, t := range openTickets {
				fmt.Fprintf(cmd.OutOrStdout(), "  #%-10s %-12s %s\n", t.Number, t.Status, truncate(t.Subject, 50))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Assets:    %d\n", assetCount)
			fmt.Fprintf(cmd.OutOrStdout(), "AR:        %.2f unpaid across %d invoice(s)\n", unpaidTotal, unpaidCount)
			fmt.Fprintf(cmd.OutOrStdout(), "Contracts: %d", contractCount)
			if len(contractNames) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), " (%s)", strings.Join(contractNames, ", "))
			}
			fmt.Fprintln(cmd.OutOrStdout())
			fmt.Fprintf(cmd.OutOrStdout(), "Alerts:    %d in last %s", alertCount, windowLabel)
			if latestAlert != "" {
				fmt.Fprintf(cmd.OutOrStdout(), " (latest %s)", latestAlert)
			}
			fmt.Fprintln(cmd.OutOrStdout())
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/syncro-cli/data.db)")
	cmd.Flags().StringVar(&alertWindow, "alert-window", "30d", "Lookback window for the RMM alert count (e.g. 7d, 48h)")
	return cmd
}
