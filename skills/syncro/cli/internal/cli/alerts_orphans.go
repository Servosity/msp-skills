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

// ticketLink carries the linkage fields used to match an alert to a ticket.
type ticketLink struct {
	customerID string
	assetID    string
	created    time.Time
	hasCreated bool
}

// loadTicketLinks reads tickets from resources with the fields needed to match
// an alert to a converted ticket.
func loadTicketLinks(db *store.Store) []ticketLink {
	var out []ticketLink
	rows, err := db.Query(`SELECT data FROM resources WHERE resource_type = 'tickets'`)
	if err != nil {
		return out
	}
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
		l := ticketLink{
			customerID: novelJSONString(obj, "customer_id", "customerId"),
			assetID:    novelJSONString(obj, "asset_id", "assetId", "customer_asset_id"),
		}
		raw := novelJSONString(obj, "created_at", "createdAt", "created")
		if t, ok := novelParseTimestamp(raw); ok {
			l.created = t
			l.hasCreated = true
		}
		out = append(out, l)
	}
	return out
}

// pp:data-source local
func newNovelAlertsOrphansCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit int
	var flagWindow string
	var flagProximity string

	cmd := &cobra.Command{
		Use:   "orphans",
		Short: "Surface RMM alerts that never became a ticket within a window.",
		Long: `List RMM alerts in the window that have no matching ticket. An alert is
considered converted when a ticket exists for the same asset (or customer when
no asset id is present) created within the proximity window of the alert.`,
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
			windowDur := 7 * 24 * time.Hour
			if flagWindow != "" {
				d, err := parseAgeDuration(flagWindow)
				if err != nil {
					return usageErr(fmt.Errorf("invalid --window: %w", err))
				}
				windowDur = d
			}
			proximityDur := 24 * time.Hour
			if flagProximity != "" {
				d, err := parseAgeDuration(flagProximity)
				if err != nil {
					return usageErr(fmt.Errorf("invalid --proximity: %w", err))
				}
				proximityDur = d
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
			tickets := loadTicketLinks(db)

			type itemOut struct {
				AlertID      string `json:"alert_id"`
				CustomerName string `json:"customer_name"`
				AssetID      string `json:"asset_id"`
				CreatedAt    string `json:"created_at"`
				Reason       string `json:"reason"`
			}
			items := []itemOut{}

			for _, a := range alerts {
				if a.hasCreated && a.created.Before(windowStart) {
					continue
				}
				if alertConverted(a, tickets, proximityDur) {
					continue
				}
				name := custNames[a.customerID]
				if name == "" {
					name = a.customerID
				}
				created := ""
				if a.hasCreated {
					created = a.created.Format(time.RFC3339)
				}
				items = append(items, itemOut{
					AlertID:      a.id,
					CustomerName: name,
					AssetID:      a.assetID,
					CreatedAt:    created,
					Reason:       "no matching ticket",
				})
			}

			sort.Slice(items, func(i, j int) bool {
				return items[i].CreatedAt > items[j].CreatedAt
			})
			totalOrphans := len(items)
			if limit > 0 && len(items) > limit {
				items = items[:limit]
			}

			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{
					"items":         items,
					"total_orphans": totalOrphans,
				})
			}

			if totalOrphans == 0 {
				novelSyncHint(cmd.ErrOrStderr(), "No orphan alerts found. If alerts/tickets are not synced, run 'syncro-cli sync' first.")
				fmt.Fprintln(cmd.OutOrStdout(), "No orphan alerts found.")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-12s %-28s %-12s %s\n", "ALERT_ID", "CUSTOMER", "ASSET_ID", "CREATED")
			for _, it := range items {
				fmt.Fprintf(cmd.OutOrStdout(), "%-12s %-28s %-12s %s\n", it.AlertID, truncate(it.CustomerName, 28), it.AssetID, it.CreatedAt)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n%d orphan alert(s).\n", totalOrphans)
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/syncro-cli/data.db)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum orphans to show")
	cmd.Flags().StringVar(&flagWindow, "window", "7d", "Lookback window for alerts (e.g. 7d, 48h)")
	cmd.Flags().StringVar(&flagProximity, "proximity", "24h", "Time proximity for matching an alert to a ticket (e.g. 24h)")
	return cmd
}

// alertConverted reports whether any ticket plausibly converted from this
// alert: same asset id (or same customer when the alert carries no asset id),
// created within proximity of the alert. When the alert has no timestamp, the
// proximity test is skipped and any matching ticket counts as a conversion.
func alertConverted(a rmmAlertRow, tickets []ticketLink, proximity time.Duration) bool {
	for _, t := range tickets {
		matchKey := false
		if a.assetID != "" && t.assetID != "" {
			matchKey = a.assetID == t.assetID
		} else if a.customerID != "" {
			matchKey = a.customerID == t.customerID
		}
		if !matchKey {
			continue
		}
		if !a.hasCreated || !t.hasCreated {
			return true
		}
		delta := t.created.Sub(a.created)
		if delta < 0 {
			delta = -delta
		}
		if delta <= proximity {
			return true
		}
	}
	return false
}
