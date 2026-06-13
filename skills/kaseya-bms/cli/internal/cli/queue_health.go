// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: queue-health.

// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type queueHealthRow struct {
	Queue          string         `json:"queue"`
	Open           int            `json:"open"`
	Stale          int            `json:"stale"`
	ByPriority     map[string]int `json:"by_priority"`
	ByStatus       map[string]int `json:"by_status"`
	OldestOpenDays int            `json:"oldest_open_days"`
}

type queueHealthView struct {
	Queues     []queueHealthRow `json:"queues"`
	TotalOpen  int              `json:"total_open"`
	StaleTotal int              `json:"stale_total"`
	StaleDays  int              `json:"stale_days"`
	Note       string           `json:"note,omitempty"`
}

// aggregateQueueHealth rolls open tickets up by queue with priority/status
// breakdowns and staleness flags. Pure function for table-driven tests.
func aggregateQueueHealth(rows []map[string]any, staleDays int, queueFilter string, now time.Time) queueHealthView {
	staleCutoff := now.AddDate(0, 0, -staleDays)
	byQueue := map[string]*queueHealthRow{}
	oldest := map[string]time.Time{}
	view := queueHealthView{StaleDays: staleDays}

	for _, m := range rows {
		if !kbmsTicketOpen(m) {
			continue
		}
		queue := kbmsStr(m, "QueueName")
		if queue == "" {
			queue = "(no queue)"
		}
		if queueFilter != "" && !strings.EqualFold(queue, queueFilter) {
			continue
		}
		row, ok := byQueue[queue]
		if !ok {
			row = &queueHealthRow{Queue: queue, ByPriority: map[string]int{}, ByStatus: map[string]int{}}
			byQueue[queue] = row
		}
		row.Open++
		view.TotalOpen++
		if p := kbmsStr(m, "PriorityName"); p != "" {
			row.ByPriority[p]++
		}
		if s := kbmsStr(m, "StatusName"); s != "" {
			row.ByStatus[s]++
		}
		if activity, ok := kbmsTicketActivity(m); ok && activity.Before(staleCutoff) {
			row.Stale++
			view.StaleTotal++
		}
		if opened, ok := kbmsTime(m, "OpenDate"); ok {
			if cur, seen := oldest[queue]; !seen || opened.Before(cur) {
				oldest[queue] = opened
			}
		}
	}

	for _, queue := range kbmsSortedKeys(byQueue) {
		row := byQueue[queue]
		if opened, ok := oldest[queue]; ok {
			row.OldestOpenDays = int(now.Sub(opened).Hours() / 24)
		}
		view.Queues = append(view.Queues, *row)
	}
	sort.Slice(view.Queues, func(i, j int) bool {
		if view.Queues[i].Open != view.Queues[j].Open {
			return view.Queues[i].Open > view.Queues[j].Open
		}
		return view.Queues[i].Queue < view.Queues[j].Queue
	})
	if view.Queues == nil {
		view.Queues = []queueHealthRow{}
	}
	if view.TotalOpen == 0 {
		view.Note = "no open tickets in the local mirror; run 'sync --resources servicedesk' to refresh it"
	}
	return view
}

func newNovelQueueHealthCmd(flags *rootFlags) *cobra.Command {
	var staleDays int
	var queueFilter string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "queue-health",
		Short: "See open ticket volume by queue, priority, and status in one shot, with stale counts flagged before the morning standup.",
		Long: strings.Trim(`
Use this command for a snapshot of open ticket volume by queue/priority/status.
Do NOT use this command for which individual tickets are aging; use 'stale-tickets' instead.

Reads the synced local mirror (resource 'servicedesk'); it never calls the API,
so it costs nothing against the 1500/hour/endpoint rate limit.`, "\n"),
		Example: strings.Trim(`
  # The dispatcher's morning board
  kaseya-bms-cli queue-health --agent

  # One queue only, with a tighter staleness bar
  kaseya-bms-cli queue-health --queue "Service Desk" --stale-days 3 --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would aggregate open tickets by queue/priority/status from the local mirror")
				return nil
			}
			if err := kbmsRejectLiveSource(flags, "queue-health"); err != nil {
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
			view := aggregateQueueHealth(rows, staleDays, queueFilter, time.Now().UTC())
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().IntVar(&staleDays, "stale-days", 7, "Days without activity before an open ticket counts as stale")
	cmd.Flags().StringVar(&queueFilter, "queue", "", "Only report the named queue (case-insensitive)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default ~/.local/share/kaseya-bms-cli/data.db)")
	return cmd
}
