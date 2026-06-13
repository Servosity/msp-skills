// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: workload.

// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type workloadRow struct {
	Assignee    string         `json:"assignee"`
	OpenTickets int            `json:"open_tickets"`
	ByPriority  map[string]int `json:"by_priority"`
	HoursLogged float64        `json:"hours_logged_window"`
}

type workloadView struct {
	Items          []workloadRow `json:"items"`
	TotalOpen      int           `json:"total_open"`
	Unassigned     int           `json:"unassigned"`
	WindowDays     int           `json:"window_days"`
	AvgOpenPerTech float64       `json:"avg_open_per_tech"`
	Note           string        `json:"note,omitempty"`
}

// aggregateWorkload joins open-ticket load per assignee with hours logged in
// the recent window, keying on user IDs with a normalized-name fallback.
// Pure function for table-driven tests.
func aggregateWorkload(ticketRows, timelogRows []map[string]any, windowDays int, unit string, now time.Time) workloadView {
	view := workloadView{WindowDays: windowDays, Items: []workloadRow{}}
	byID := map[string]*workloadRow{}
	byName := map[string]*workloadRow{}
	var rows []*workloadRow

	// lookup finds-or-creates a row, joining on the numeric user/assignee ID
	// first and falling back to the normalized display name. Joining on the
	// name alone produced phantom duplicate rows whenever AssigneeName and
	// FirstName+LastName disagreed ("Smith, Jane", middle names, spacing).
	lookup := func(id, name string) *workloadRow {
		norm := kbmsNormName(name)
		if id != "" {
			if row, ok := byID[id]; ok {
				if row.Assignee == "" && name != "" {
					row.Assignee = name
				}
				if norm != "" {
					byName[norm] = row
				}
				return row
			}
		}
		if norm != "" {
			if row, ok := byName[norm]; ok {
				if id != "" {
					byID[id] = row
				}
				return row
			}
		}
		if id == "" && norm == "" {
			return nil
		}
		row := &workloadRow{Assignee: name, ByPriority: map[string]int{}}
		if id != "" {
			byID[id] = row
		}
		if norm != "" {
			byName[norm] = row
		}
		rows = append(rows, row)
		return row
	}

	for _, m := range ticketRows {
		if !kbmsTicketOpen(m) {
			continue
		}
		view.TotalOpen++
		assignee := kbmsStr(m, "AssigneeName")
		assigneeID := kbmsIDString(m, "AssigneeId")
		if assignee == "" && assigneeID == "" {
			view.Unassigned++
			continue
		}
		row := lookup(assigneeID, assignee)
		if row == nil {
			view.Unassigned++
			continue
		}
		row.OpenTickets++
		if p := kbmsStr(m, "PriorityName"); p != "" {
			row.ByPriority[p]++
		}
	}

	cutoff := now.AddDate(0, 0, -windowDays)
	for _, m := range timelogRows {
		started, ok := kbmsTime(m, "StartDate")
		if !ok {
			started, ok = kbmsTime(m, "CreatedOn")
		}
		if !ok || started.Before(cutoff) {
			continue
		}
		name := strings.TrimSpace(kbmsStr(m, "FirstName") + " " + kbmsStr(m, "LastName"))
		userID := kbmsIDString(m, "UserId")
		raw, ok := kbmsNum(m, "Timespent")
		if !ok {
			continue
		}
		row := lookup(userID, name)
		if row == nil {
			continue
		}
		row.HoursLogged += kbmsHoursFromTimespent(raw, unit)
	}

	techsWithOpen := 0
	for _, row := range rows {
		row.HoursLogged = kbmsRound2(row.HoursLogged)
		if row.Assignee == "" {
			row.Assignee = "(unknown)"
		}
		if row.OpenTickets > 0 {
			techsWithOpen++
		}
		view.Items = append(view.Items, *row)
	}
	sort.Slice(view.Items, func(i, j int) bool {
		if view.Items[i].OpenTickets != view.Items[j].OpenTickets {
			return view.Items[i].OpenTickets > view.Items[j].OpenTickets
		}
		return view.Items[i].Assignee < view.Items[j].Assignee
	})
	if techsWithOpen > 0 {
		assigned := view.TotalOpen - view.Unassigned
		view.AvgOpenPerTech = kbmsRound2(float64(assigned) / float64(techsWithOpen))
	}
	if view.TotalOpen == 0 && len(view.Items) == 0 {
		view.Note = "no open tickets or recent time logs in the local mirror; run 'sync --resources servicedesk,timelogs' to refresh"
	} else if view.TotalOpen == 0 {
		view.Note = "no open tickets in the local mirror; hours columns reflect recent time logs only"
	}
	return view
}

func newNovelWorkloadCmd(flags *rootFlags) *cobra.Command {
	var windowDays int
	var unit string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "workload",
		Short: "Open and in-progress ticket load per assignee, flagging who is overloaded and who has slack before you dispatch the next ticket.",
		Long: strings.Trim(`
Joins open tickets with recently logged time per technician from the local
mirror - a cross-entity rollup no single BMS API call returns. Unassigned open
tickets are counted separately so dispatchers see them immediately.`, "\n"),
		Example: strings.Trim(`
  # Who can take the next ticket?
  kaseya-bms-cli workload --agent

  # Load plus hours logged in the last 14 days
  kaseya-bms-cli workload --window-days 14 --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would aggregate open tickets and recent hours per assignee from the local mirror")
				return nil
			}
			if err := kbmsRejectLiveSource(flags, "workload"); err != nil {
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
			ticketRows, err := kbmsRows(cmd.Context(), db, "servicedesk")
			if err != nil {
				return fmt.Errorf("querying tickets: %w", err)
			}
			timelogRows, err := kbmsRows(cmd.Context(), db, "timelogs")
			if err != nil {
				return fmt.Errorf("querying time logs: %w", err)
			}
			view := aggregateWorkload(ticketRows, timelogRows, windowDays, unit, time.Now().UTC())
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().IntVar(&windowDays, "window-days", 7, "Window for the hours-logged column")
	cmd.Flags().StringVar(&unit, "timespent-unit", "minutes", "Unit BMS uses for Timespent values: minutes, hours, or seconds")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default ~/.local/share/kaseya-bms-cli/data.db)")
	return cmd
}
