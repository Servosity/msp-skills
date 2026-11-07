// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// newNovelSlaBreachesCmd ranks open tickets by how far past their due date
// they are — the time-windowed SLA read Autotask makes you build a grid for.
// pp:data-source local
func newNovelSlaBreachesCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit int
	cmd := &cobra.Command{
		Use:   "sla-breaches",
		Short: "List open tickets past their due date or overdue for first response.",
		Long: "List open tickets with an overdue pending deadline, ranked by how overdue they are: the first response " +
			"(firstResponseDueDateTime, met once firstResponseDateTime is set), the resolution plan (resolutionPlanDueDateTime / " +
			"resolutionPlanDateTime), the resolution (resolvedDueDateTime / resolvedDateTime) and the ticket due date " +
			"(dueDateTime). `breach` names the most overdue one. Run `sync` first.",
		Example: strings.Trim(`
  autotask-cli sla-breaches
  autotask-cli sla-breaches --agent
  autotask-cli sla-breaches --json --limit 20`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openNovelStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "tickets") {
				hintIfStale(cmd, db, "tickets", flags.maxAge)
			}
			tickets, err := listEntity(db, "tickets")
			if err != nil {
				return apiErr(err)
			}
			now := time.Now()
			type row struct {
				ID         int64   `json:"id"`
				Title      string  `json:"title,omitempty"`
				CompanyID  string  `json:"companyID,omitempty"`
				DueDate    string  `json:"dueDateTime,omitempty"`
				Breach     string  `json:"breach"`
				BreachDue  string  `json:"breachDue"`
				OverdueHrs float64 `json:"overdueHours"`
				Priority   string  `json:"priority,omitempty"`
			}
			var rows []row
			for _, t := range tickets {
				if !isTicketOpen(t) {
					continue
				}
				breach, due, ok := mostOverdueDeadline(t, now)
				if !ok {
					continue
				}
				id, _ := intAt(t, "id")
				rows = append(rows, row{
					ID:         id,
					Title:      strAt(t, "title"),
					CompanyID:  strAt(t, "companyID", "companyId"),
					DueDate:    strAt(t, "dueDateTime"),
					Breach:     breach,
					BreachDue:  due.UTC().Format(time.RFC3339),
					OverdueHrs: now.Sub(due).Hours(),
					Priority:   strAt(t, "priority"),
				})
			}
			sort.Slice(rows, func(i, j int) bool { return rows[i].OverdueHrs > rows[j].OverdueHrs })
			if limit > 0 && len(rows) > limit {
				rows = rows[:limit]
			}
			return flags.printJSON(cmd, rows)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/autotask-cli/data.db)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum rows to return (0 = all)")
	return cmd
}

// slaDeadlines are the documented TicketModel deadlines, each with the field
// that records it was met (spec.json). An empty met field means pending.
var slaDeadlines = []struct{ name, due, met string }{
	{"first-response", "firstResponseDueDateTime", "firstResponseDateTime"},
	{"resolution-plan", "resolutionPlanDueDateTime", "resolutionPlanDateTime"},
	{"resolution", "resolvedDueDateTime", "resolvedDateTime"},
	{"due-date", "dueDateTime", ""},
}

// mostOverdueDeadline returns the pending deadline furthest past now, so a
// ticket overdue for first response is listed even when its due date is ahead.
func mostOverdueDeadline(t map[string]any, now time.Time) (name string, due time.Time, ok bool) {
	for _, d := range slaDeadlines {
		at, has := timeAt(t, d.due)
		if !has || !at.Before(now) || (d.met != "" && strAt(t, d.met) != "") {
			continue
		}
		if !ok || at.Before(due) {
			name, due, ok = d.name, at, true
		}
	}
	return name, due, ok
}
