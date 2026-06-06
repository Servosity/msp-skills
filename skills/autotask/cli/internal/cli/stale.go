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

// newNovelStaleCmd lists tickets or projects with no activity in N days —
// a lastActivityDate window over the local store.
// pp:data-source local
func newNovelStaleCmd(flags *rootFlags) *cobra.Command {
	var flagDays string
	var flagEntity string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "stale",
		Short: "Find tickets or projects with no activity in N days.",
		Long:  "List open tickets (or projects) whose lastActivityDate is older than --days. Run `sync` first.",
		Example: strings.Trim(`
  autotask-cli stale --days 14
  autotask-cli stale --days 30 --entity projects --agent
  autotask-cli stale --days 7 --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			days := 14
			if strings.TrimSpace(flagDays) != "" {
				n, err := strconv.Atoi(strings.TrimSpace(flagDays))
				if err != nil || n < 0 {
					return usageErr(fmt.Errorf("invalid --days %q: must be a non-negative integer", flagDays))
				}
				days = n
			}
			entity := flagEntity
			if entity == "" {
				entity = "tickets"
			}
			if entity != "tickets" && entity != "projects" {
				return usageErr(fmt.Errorf("invalid --entity %q: must be tickets or projects", entity))
			}
			db, err := openNovelStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, entity) {
				hintIfStale(cmd, db, entity, flags.maxAge)
			}
			recs, err := listEntity(db, entity)
			if err != nil {
				return apiErr(err)
			}
			cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
			type row struct {
				ID           int64   `json:"id"`
				Title        string  `json:"title,omitempty"`
				LastActivity string  `json:"lastActivityDate,omitempty"`
				IdleDays     float64 `json:"idleDays"`
			}
			var rows []row
			for _, r := range recs {
				if entity == "tickets" && !isTicketOpen(r) {
					continue
				}
				if entity == "projects" {
					if s, ok := intAt(r, "status"); ok && s == 5 {
						continue
					}
				}
				la, ok := timeAt(r, "lastActivityDate", "lastActivityDateTime")
				if !ok {
					la, ok = ticketCreated(r)
				}
				if !ok || !la.Before(cutoff) {
					continue
				}
				id, _ := intAt(r, "id")
				rows = append(rows, row{
					ID:           id,
					Title:        strAt(r, "title", "projectName"),
					LastActivity: strAt(r, "lastActivityDate", "lastActivityDateTime"),
					IdleDays:     time.Since(la).Hours() / 24,
				})
			}
			sort.Slice(rows, func(i, j int) bool { return rows[i].IdleDays > rows[j].IdleDays })
			return flags.printJSON(cmd, rows)
		},
	}
	cmd.Flags().StringVar(&flagDays, "days", "", "consider items with no activity in this many days stale (default 14)")
	cmd.Flags().StringVar(&flagEntity, "entity", "", "which entity to scan: tickets (default) or projects")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/autotask-cli/data.db)")
	return cmd
}
