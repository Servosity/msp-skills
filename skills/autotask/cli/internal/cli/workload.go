// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// newNovelWorkloadCmd joins open tickets and tasks to their assigned resource —
// the team-capacity read Autotask has no endpoint for.
// pp:data-source local
func newNovelWorkloadCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:   "workload",
		Short: "See which technicians are overloaded by open ticket and task hours before you assign more.",
		Long:  "Join open tickets and tasks to their assigned resource and rank resources by open-item count and estimated hours. Run `sync` first.",
		Example: strings.Trim(`
  autotask-cli workload
  autotask-cli workload --agent
  autotask-cli workload --json --select resourceID,openItems`, "\n"),
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
			tasks, _ := listEntity(db, "tasks")

			type load struct {
				ResourceID  string  `json:"resourceID"`
				OpenTickets int     `json:"openTickets"`
				OpenTasks   int     `json:"openTasks"`
				OpenItems   int     `json:"openItems"`
				EstHours    float64 `json:"estHours"`
			}
			byRes := map[string]*load{}
			get := func(id string) *load {
				if id == "" {
					id = "(unassigned)"
				}
				if byRes[id] == nil {
					byRes[id] = &load{ResourceID: id}
				}
				return byRes[id]
			}
			for _, t := range tickets {
				if !isTicketOpen(t) {
					continue
				}
				l := get(strAt(t, "assignedResourceID", "assignedResourceId"))
				l.OpenTickets++
				l.OpenItems++
				if h, ok := numAt(t, "estimatedHours", "hoursToBeScheduled"); ok {
					l.EstHours += h
				}
			}
			for _, tk := range tasks {
				if pct, ok := numAt(tk, "percentComplete"); ok && pct >= 100 {
					continue
				}
				l := get(strAt(tk, "assignedResourceID", "assignedResourceId"))
				l.OpenTasks++
				l.OpenItems++
				if h, ok := numAt(tk, "estimatedHours"); ok {
					l.EstHours += h
				}
			}
			rows := make([]load, 0, len(byRes))
			for _, l := range byRes {
				rows = append(rows, *l)
			}
			sort.Slice(rows, func(i, j int) bool {
				if rows[i].OpenItems != rows[j].OpenItems {
					return rows[i].OpenItems > rows[j].OpenItems
				}
				return rows[i].EstHours > rows[j].EstHours
			})
			return flags.printJSON(cmd, rows)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/autotask-cli/data.db)")
	return cmd
}
