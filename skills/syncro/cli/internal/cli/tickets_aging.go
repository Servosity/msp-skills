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
func newNovelTicketsAgingCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit int
	var flagNoComment string
	var flagStatus string

	cmd := &cobra.Command{
		Use:   "aging",
		Short: "List open tickets past SLA with no comment in the last N hours.",
		Long: `List open tickets (status not Resolved/Closed) whose last activity is older than
--no-comment. Last activity is the most recent comment time when comments are
synced, otherwise the ticket's own updated/last-response timestamp.`,
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
			noCommentDur := 48 * time.Hour
			if flagNoComment != "" {
				d, err := parseAgeDuration(flagNoComment)
				if err != nil {
					return usageErr(fmt.Errorf("invalid --no-comment: %w", err))
				}
				noCommentDur = d
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
			cutoff := now.Add(-noCommentDur)

			lastComment := loadLastCommentTimes(db)
			custNames := loadCustomerNames(db)

			type itemOut struct {
				TicketID     string `json:"ticket_id"`
				Number       string `json:"number"`
				CustomerName string `json:"customer_name"`
				Status       string `json:"status"`
				LastActivity string `json:"last_activity"`
				HoursSince   int    `json:"hours_since"`
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
					if ls == "resolved" || ls == "closed" {
						continue
					}
					if flagStatus != "" && !strings.EqualFold(status, flagStatus) {
						continue
					}

					var lastActivity time.Time
					if t, ok := lastComment[id]; ok {
						lastActivity = t
					} else {
						raw := novelJSONString(obj, "last_response_at", "updated_at", "last_communication", "created_at")
						if t, ok := novelParseTimestamp(raw); ok {
							lastActivity = t
						}
					}
					if lastActivity.IsZero() {
						continue
					}
					if lastActivity.After(cutoff) {
						continue
					}
					custID := novelJSONString(obj, "customer_id", "customerId")
					items = append(items, itemOut{
						TicketID:     id,
						Number:       novelJSONString(obj, "number", "ticket_number"),
						CustomerName: custNames[custID],
						Status:       status,
						LastActivity: lastActivity.Format(time.RFC3339),
						HoursSince:   int(now.Sub(lastActivity).Hours()),
					})
				}
			}

			sort.Slice(items, func(i, j int) bool {
				return items[i].HoursSince > items[j].HoursSince
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
				novelSyncHint(cmd.ErrOrStderr(), "No aging tickets found. If tickets are not synced, run 'syncro-cli sync' first.")
				fmt.Fprintln(cmd.OutOrStdout(), "No aging open tickets found.")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-10s %-12s %-24s %-12s %s\n", "TICKET", "NUMBER", "CUSTOMER", "STATUS", "HOURS")
			for _, it := range items {
				fmt.Fprintf(cmd.OutOrStdout(), "%-10s %-12s %-24s %-12s %d\n", it.TicketID, it.Number, truncate(it.CustomerName, 24), it.Status, it.HoursSince)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n%d open ticket(s) with no recent activity.\n", totalCount)
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/syncro-cli/data.db)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum tickets to show")
	cmd.Flags().StringVar(&flagNoComment, "no-comment", "48h", "Flag tickets with no activity for at least this long (e.g. 48h, 3d)")
	cmd.Flags().StringVar(&flagStatus, "status", "", "Filter to an exact ticket status")
	return cmd
}

// loadLastCommentTimes returns a map of ticket id -> most recent comment time
// from the comments table. Empty when comments are not synced.
func loadLastCommentTimes(db *store.Store) map[string]time.Time {
	out := map[string]time.Time{}
	rows, err := db.Query(`SELECT tickets_id, data FROM comments`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var ticketID *string
		var data []byte
		if err := rows.Scan(&ticketID, &data); err != nil {
			continue
		}
		if ticketID == nil {
			continue
		}
		var obj map[string]any
		if json.Unmarshal(data, &obj) != nil {
			continue
		}
		raw := novelJSONString(obj, "created_at", "updated_at", "timestamp")
		t, ok := novelParseTimestamp(raw)
		if !ok {
			continue
		}
		if cur, exists := out[*ticketID]; !exists || t.After(cur) {
			out[*ticketID] = t
		}
	}
	return out
}
