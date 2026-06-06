// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// newNovelProjectHealthCmd scans open projects for risk signals by joining
// projects to their overdue tasks — a portfolio read with no single endpoint.
// pp:data-source local
func newNovelProjectHealthCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:   "project-health",
		Short: "Flag projects with overdue tasks or past end dates.",
		Long:  "Scan open projects for risk signals: a past endDate, or open tasks whose due dates have passed. Run `sync` first.",
		Example: strings.Trim(`
  autotask-cli project-health
  autotask-cli project-health --agent
  autotask-cli project-health --json`, "\n"),
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
			if !hintIfUnsynced(cmd, db, "projects") {
				hintIfStale(cmd, db, "projects", flags.maxAge)
			}
			projects, err := listEntity(db, "projects")
			if err != nil {
				return apiErr(err)
			}
			tasks, _ := listEntity(db, "tasks")
			now := time.Now()

			overdueTasksByProject := map[string]int{}
			for _, tk := range tasks {
				if pct, ok := numAt(tk, "percentComplete"); ok && pct >= 100 {
					continue
				}
				due, ok := timeAt(tk, "endDateTime", "dueDate")
				if !ok || !due.Before(now) {
					continue
				}
				overdueTasksByProject[strAt(tk, "projectID", "projectId")]++
			}

			type row struct {
				ID           int64    `json:"id"`
				Name         string   `json:"projectName,omitempty"`
				EndDate      string   `json:"endDate,omitempty"`
				OverdueTasks int      `json:"overdueTasks"`
				Risks        []string `json:"risks"`
			}
			var rows []row
			for _, p := range projects {
				if s, ok := intAt(p, "status"); ok && s == 5 {
					continue
				}
				var risks []string
				if end, ok := timeAt(p, "endDate", "endDateTime"); ok && end.Before(now) {
					risks = append(risks, "past end date")
				}
				id, _ := intAt(p, "id")
				ot := overdueTasksByProject[strconv.FormatInt(id, 10)]
				if ot > 0 {
					risks = append(risks, fmt.Sprintf("%d overdue task(s)", ot))
				}
				if len(risks) == 0 {
					continue
				}
				rows = append(rows, row{
					ID:           id,
					Name:         strAt(p, "projectName", "title"),
					EndDate:      strAt(p, "endDate", "endDateTime"),
					OverdueTasks: ot,
					Risks:        risks,
				})
			}
			sort.Slice(rows, func(i, j int) bool { return rows[i].OverdueTasks > rows[j].OverdueTasks })
			return flags.printJSON(cmd, rows)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/autotask-cli/data.db)")
	return cmd
}
