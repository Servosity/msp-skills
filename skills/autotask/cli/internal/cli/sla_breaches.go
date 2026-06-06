// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
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
		Long:  "List open tickets whose dueDateTime is in the past, ranked by how overdue they are. Run `sync` first.",
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
				OverdueHrs float64 `json:"overdueHours"`
				Priority   string  `json:"priority,omitempty"`
			}
			var rows []row
			for _, t := range tickets {
				if !isTicketOpen(t) {
					continue
				}
				due, ok := timeAt(t, "dueDateTime", "dueDate")
				if !ok || !due.Before(now) {
					continue
				}
				id, _ := intAt(t, "id")
				rows = append(rows, row{
					ID:         id,
					Title:      strAt(t, "title"),
					CompanyID:  strAt(t, "companyID", "companyId"),
					DueDate:    strAt(t, "dueDateTime", "dueDate"),
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
