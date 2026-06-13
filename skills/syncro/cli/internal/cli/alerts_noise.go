// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"
	"syncro-pp-cli/internal/store"
)

// rmmAlertResourceTypes lists the resource_type strings under which RMM alerts
// may be stored. Sync writes "rmm-alerts"; the underlying endpoint is
// /rmm_alerts, so the underscore variant is matched too for robustness.
var rmmAlertResourceTypes = []string{"rmm-alerts", "rmm_alerts"}

// rmmAlertRow is a decoded RMM alert from the resources table.
type rmmAlertRow struct {
	id         string
	customerID string
	assetID    string
	created    time.Time
	hasCreated bool
}

// loadRMMAlerts reads every RMM alert from the resources table and decodes the
// fields the alerts analytics commands need.
func loadRMMAlerts(db *store.Store) []rmmAlertRow {
	var out []rmmAlertRow
	rows, err := db.Query(
		`SELECT id, data FROM resources WHERE resource_type IN (?, ?)`,
		rmmAlertResourceTypes[0], rmmAlertResourceTypes[1],
	)
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
		r := rmmAlertRow{
			id:         id,
			customerID: novelJSONString(obj, "customer_id", "customerId"),
			assetID:    novelJSONString(obj, "asset_id", "assetId", "customer_asset_id"),
		}
		raw := novelJSONString(obj, "created_at", "timestamp", "created", "createdAt")
		if t, ok := novelParseTimestamp(raw); ok {
			r.created = t
			r.hasCreated = true
		}
		out = append(out, r)
	}
	return out
}

// pp:data-source local
func newNovelAlertsNoiseCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit int
	var flagWindow string

	cmd := &cobra.Command{
		Use:   "noise",
		Short: "Rank customers by RMM alert volume over a time window.",
		Long: `Count RMM alerts per customer over the lookback window using each alert's
created timestamp, resolving customer names from synced customers when present.
Customers are ranked by alert count, descending.`,
		Example:     "  syncro-cli alerts noise --window 30d --limit 10",
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
			windowDur := 30 * 24 * time.Hour
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
			alerts := loadRMMAlerts(db)

			counts := map[string]int{}
			total := 0
			for _, a := range alerts {
				if a.hasCreated && a.created.Before(windowStart) {
					continue
				}
				counts[a.customerID]++
				total++
			}

			type itemOut struct {
				CustomerID   string `json:"customer_id"`
				CustomerName string `json:"customer_name"`
				AlertCount   int    `json:"alert_count"`
			}
			items := make([]itemOut, 0, len(counts))
			for id, c := range counts {
				name := custNames[id]
				if name == "" {
					name = id
				}
				items = append(items, itemOut{CustomerID: id, CustomerName: name, AlertCount: c})
			}
			sort.Slice(items, func(i, j int) bool {
				if items[i].AlertCount != items[j].AlertCount {
					return items[i].AlertCount > items[j].AlertCount
				}
				return items[i].CustomerID < items[j].CustomerID
			})
			if limit > 0 && len(items) > limit {
				items = items[:limit]
			}

			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{
					"window":       flagWindowOrDefaultN(flagWindow, "30d"),
					"items":        items,
					"total_alerts": total,
				})
			}

			if len(items) == 0 {
				novelSyncHint(cmd.ErrOrStderr(), "No RMM alerts found. If alerts are not synced, run 'syncro-cli sync' first.")
				fmt.Fprintln(cmd.OutOrStdout(), "No alerts in window.")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-14s %-30s %s\n", "CUSTOMER_ID", "CUSTOMER", "ALERTS")
			for _, it := range items {
				fmt.Fprintf(cmd.OutOrStdout(), "%-14s %-30s %d\n", it.CustomerID, truncate(it.CustomerName, 30), it.AlertCount)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n%d alert(s) across %d customer(s).\n", total, len(items))
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/syncro-cli/data.db)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum customers to show")
	cmd.Flags().StringVar(&flagWindow, "window", "30d", "Lookback window (e.g. 30d, 168h)")
	return cmd
}

func flagWindowOrDefaultN(w, def string) string {
	if w == "" {
		return def
	}
	return w
}
