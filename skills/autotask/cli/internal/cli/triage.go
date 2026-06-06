// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// newNovelTriageCmd ranks open, unassigned tickets by priority-weighted age —
// the dispatcher's morning queue, computed locally.
// pp:data-source local
func newNovelTriageCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit int
	cmd := &cobra.Command{
		Use:   "triage",
		Short: "Rank open unassigned tickets by priority and age into a workable queue.",
		Long:  "Rank open, unassigned tickets by a priority-weighted age score so a dispatcher gets a workable queue. Run `sync` first.",
		Example: strings.Trim(`
  autotask-cli triage
  autotask-cli triage --agent
  autotask-cli triage --json --limit 25`, "\n"),
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
				ID        int64   `json:"id"`
				Title     string  `json:"title,omitempty"`
				Priority  string  `json:"priority,omitempty"`
				CompanyID string  `json:"companyID,omitempty"`
				AgeHours  float64 `json:"ageHours"`
				Score     float64 `json:"score"`
			}
			var rows []row
			for _, t := range tickets {
				if !isTicketOpen(t) {
					continue
				}
				if rid := strAt(t, "assignedResourceID", "assignedResourceId"); rid != "" && rid != "0" {
					continue
				}
				created, ok := ticketCreated(t)
				if !ok {
					created = now
				}
				ageHours := now.Sub(created).Hours()
				weight := 1.0
				if prio, ok := numAt(t, "priority"); ok && prio > 0 {
					weight = 4.0 / prio // priority 1 -> 4x, priority 4 -> 1x
				}
				id, _ := intAt(t, "id")
				rows = append(rows, row{
					ID:        id,
					Title:     strAt(t, "title"),
					Priority:  strAt(t, "priority"),
					CompanyID: strAt(t, "companyID", "companyId"),
					AgeHours:  ageHours,
					Score:     ageHours * weight,
				})
			}
			sort.Slice(rows, func(i, j int) bool { return rows[i].Score > rows[j].Score })
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
