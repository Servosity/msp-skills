// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: stale-tickets.

// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type staleTicketItem struct {
	TicketNumber string `json:"ticket_number"`
	Title        string `json:"title"`
	Account      string `json:"account"`
	Queue        string `json:"queue"`
	Status       string `json:"status"`
	Priority     string `json:"priority"`
	Assignee     string `json:"assignee"`
	LastActivity string `json:"last_activity"`
	AgeDays      int    `json:"age_days"`
	DueDate      string `json:"due_date,omitempty"`
}

type staleTicketsView struct {
	Items []staleTicketItem `json:"items"`
	Count int               `json:"count"`
	Days  int               `json:"days"`
	Note  string            `json:"note,omitempty"`
}

// filterStaleTickets selects open tickets whose last activity is older than
// the cutoff, oldest first. Pure function for table-driven tests.
func filterStaleTickets(rows []map[string]any, days int, queue, assignee string, limit int, now time.Time) staleTicketsView {
	cutoff := now.AddDate(0, 0, -days)
	view := staleTicketsView{Days: days, Items: []staleTicketItem{}}

	type aged struct {
		item     staleTicketItem
		activity time.Time
	}
	var matched []aged
	for _, m := range rows {
		if !kbmsTicketOpen(m) {
			continue
		}
		if queue != "" && !strings.EqualFold(kbmsStr(m, "QueueName"), queue) {
			continue
		}
		if assignee != "" && !strings.EqualFold(kbmsStr(m, "AssigneeName"), assignee) {
			continue
		}
		activity, ok := kbmsTicketActivity(m)
		if !ok || !activity.Before(cutoff) {
			continue
		}
		item := staleTicketItem{
			TicketNumber: kbmsStr(m, "TicketNumber"),
			Title:        kbmsStr(m, "Title"),
			Account:      kbmsStr(m, "AccountName"),
			Queue:        kbmsStr(m, "QueueName"),
			Status:       kbmsStr(m, "StatusName"),
			Priority:     kbmsStr(m, "PriorityName"),
			Assignee:     kbmsStr(m, "AssigneeName"),
			LastActivity: activity.Format(time.RFC3339),
			AgeDays:      int(now.Sub(activity).Hours() / 24),
		}
		if due, ok := kbmsTime(m, "DueDate"); ok {
			item.DueDate = due.Format(time.RFC3339)
		}
		matched = append(matched, aged{item: item, activity: activity})
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].activity.Before(matched[j].activity) })
	for i, a := range matched {
		if limit > 0 && i >= limit {
			break
		}
		view.Items = append(view.Items, a.item)
	}
	view.Count = len(view.Items)
	if view.Count == 0 {
		view.Note = fmt.Sprintf("no open tickets older than %d days in the local mirror; lower --days or run 'sync --resources servicedesk' to refresh", days)
	}
	return view
}

func newNovelStaleTicketsCmd(flags *rootFlags) *cobra.Command {
	var days int
	var queue string
	var assignee string
	var limit int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "stale-tickets",
		Short: "List open tickets that have not been touched in N days, oldest first, with account and assignee so nothing rots in the queue.",
		Long: strings.Trim(`
Use this command for the specific tickets that are aging or SLA-at-risk.
Do NOT use this command for aggregate counts per queue; use 'queue-health' instead.

Staleness is measured against LastActivityUpdate (falling back to ModifiedOn,
then CreatedOn) on the synced local mirror.`, "\n"),
		Example: strings.Trim(`
  # Tickets nobody touched in a week
  kaseya-bms-cli stale-tickets --days 7 --agent

  # One tech's aging tickets
  kaseya-bms-cli stale-tickets --days 3 --assignee "Jane Smith" --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would list open tickets with no activity inside the staleness window")
				return nil
			}
			if err := kbmsRejectLiveSource(flags, "stale-tickets"); err != nil {
				return err
			}
			if dbPath == "" {
				dbPath = defaultDBPath("kaseya-bms-cli")
			}
			db, err := kbmsOpenStore(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "servicedesk") {
				hintIfStale(cmd, db, "servicedesk", flags.maxAge)
			}
			rows, err := kbmsRows(cmd.Context(), db, "servicedesk")
			if err != nil {
				return fmt.Errorf("querying tickets: %w", err)
			}
			view := filterStaleTickets(rows, days, queue, assignee, limit, time.Now().UTC())
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().IntVar(&days, "days", 7, "Days without activity before an open ticket is reported")
	cmd.Flags().StringVar(&queue, "queue", "", "Only report tickets in the named queue (case-insensitive)")
	cmd.Flags().StringVar(&assignee, "assignee", "", "Only report tickets assigned to the named person (case-insensitive)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum tickets to return (0 = no limit)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default ~/.local/share/kaseya-bms-cli/data.db)")
	return cmd
}
