// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Transcendence feature (hand-authored, Phase 3): the cross-incident
// follow-through backlog. Aggregates every open/overdue action item across all
// incidents, grouped by owner or team — Rootly never aggregates follow-through
// outside a single incident.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelActionItemsOverdueCmd(flags *rootFlags) *cobra.Command {
	var groupBy string
	var limit int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "action-items-overdue",
		Short: "List every open or overdue incident action item across all incidents.",
		Long: `The weekly follow-through review in one command. Unions top-level action
items with per-incident action-item rows from the local mirror, keeps the
open ones, marks past-due items, joins each back to its incident (title +
severity), and groups counts by owner or team. Offline — run
'rootly-cli sync --resources action-items,incidents' first.`,
		Example: `  rootly-cli action-items-overdue --group-by owner
  rootly-cli action-items-overdue --group-by team --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if groupBy != "owner" && groupBy != "team" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--group-by must be owner or team, got %q", groupBy))
			}
			db, err := novelOpenStoreChecked(cmd, flags, dbPath, "incidents")
			if err != nil {
				return err
			}
			defer db.Close()

			// Incident context for joining.
			incidents, err := novelLoad(db, novelResolveType(db, "incidents"))
			if err != nil {
				return err
			}
			type incCtx struct {
				title, severity string
				teams           []string
			}
			incByID := map[string]incCtx{}
			for _, r := range incidents {
				incByID[r.ID] = incCtx{title: incidentTitle(r), severity: incidentSeverity(r), teams: incidentTeamNames(r)}
			}

			now := time.Now()
			type item struct {
				Summary       string  `json:"summary"`
				Owner         string  `json:"owner,omitempty"`
				Team          string  `json:"team,omitempty"`
				IncidentID    string  `json:"incident_id,omitempty"`
				IncidentTitle string  `json:"incident_title,omitempty"`
				Severity      string  `json:"severity,omitempty"`
				AgeDays       float64 `json:"age_days"`
				Due           string  `json:"due,omitempty"`
				Overdue       bool    `json:"overdue"`
			}
			seen := map[string]bool{}
			var items []item
			add := func(r record, incidentID string) {
				if !actionItemOpen(r) {
					return
				}
				summary := strings.TrimSpace(recStr(r.Attrs, "summary", "description", "title"))
				if summary == "" {
					return
				}
				key := incidentID + "\x00" + summary
				if seen[key] {
					return
				}
				seen[key] = true
				it := item{Summary: summary, IncidentID: incidentID}
				it.Owner = firstNonEmpty(recName(r.Attrs["assigned_user"]), recName(r.Rels["assigned_user"]), recName(r.Attrs["user"]), recName(r.Rels["user"]), recName(r.Attrs["owner"]))
				if g := recNames(r.Attrs["groups"]); len(g) > 0 {
					it.Team = g[0]
				}
				if ctx, ok := incByID[incidentID]; ok {
					it.IncidentTitle = ctx.title
					it.Severity = ctx.severity
					if it.Team == "" && len(ctx.teams) > 0 {
						it.Team = ctx.teams[0]
					}
				}
				if created, ok := recTime(r.Attrs, "created_at"); ok {
					it.AgeDays = float64(int(now.Sub(created).Hours()/24*10)) / 10
				}
				if due, ok := recTime(r.Attrs, "due_date", "due_at", "deadline"); ok {
					it.Due = due.Format("2006-01-02")
					it.Overdue = due.Before(now)
				}
				items = append(items, it)
			}

			// Per-incident sub-resource rows.
			for _, cr := range novelLoadChildTableAll(db, "incidents_action_items", "incidents_id") {
				add(cr.record, cr.FK)
			}
			// Top-level action items (linked back via relationships when possible).
			if topLevel, err := novelLoad(db, novelResolveType(db, "action-items", "action_items")); err == nil {
				for _, r := range topLevel {
					incidentID := relID(r, "incident")
					if incidentID == "" {
						for id := range incByID {
							if recRefersTo(r, id) {
								incidentID = id
								break
							}
						}
					}
					add(r, incidentID)
				}
			}

			// Oldest first; overdue items lead.
			sort.Slice(items, func(i, j int) bool {
				if items[i].Overdue != items[j].Overdue {
					return items[i].Overdue
				}
				if items[i].AgeDays != items[j].AgeDays {
					return items[i].AgeDays > items[j].AgeDays
				}
				return items[i].Summary < items[j].Summary
			})
			groups := map[string]int{}
			for _, it := range items {
				k := it.Owner
				if groupBy == "team" {
					k = it.Team
				}
				if k == "" {
					k = "(unassigned)"
				}
				groups[k]++
			}
			// Totals reflect the FULL open set (consistent with groups);
			// --limit only truncates the items shown.
			openTotal := len(items)
			overdueCount := 0
			for _, it := range items {
				if it.Overdue {
					overdueCount++
				}
			}
			if limit > 0 && len(items) > limit {
				items = items[:limit]
			}
			if items == nil {
				items = []item{}
			}
			out := struct {
				GroupBy string         `json:"group_by"`
				Groups  map[string]int `json:"groups"`
				Open    int            `json:"open_count"`
				Overdue int            `json:"overdue_count"`
				Items   []item         `json:"items"`
				Note    string         `json:"note,omitempty"`
			}{GroupBy: groupBy, Groups: groups, Open: openTotal, Overdue: overdueCount, Items: items}
			if openTotal == 0 {
				out.Note = "no open action items found in the local store; run 'rootly-cli sync --resources action-items,incidents' first"
			}

			return novelEmit(cmd, flags, out, func() {
				w := cmd.OutOrStdout()
				fmt.Fprintf(w, "open action items: %d (%d overdue), grouped by %s\n", out.Open, out.Overdue, groupBy)
				keys := make([]string, 0, len(groups))
				for k := range groups {
					keys = append(keys, k)
				}
				sort.Slice(keys, func(i, j int) bool { return groups[keys[i]] > groups[keys[j]] })
				for _, k := range keys {
					fmt.Fprintf(w, "  %-28s %d\n", k, groups[k])
				}
				for _, it := range items {
					marker := " "
					if it.Overdue {
						marker = "!"
					}
					fmt.Fprintf(w, "  %s %-50.50s %-20.20s %s\n", marker, it.Summary, firstNonEmpty(it.Owner, it.Team, "(unassigned)"), it.IncidentTitle)
				}
				if out.Note != "" {
					fmt.Fprintf(w, "  note: %s\n", out.Note)
				}
			})
		},
	}
	cmd.Flags().StringVar(&groupBy, "group-by", "owner", "Group counts by: owner or team")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum items to return (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/rootly-cli/data.db)")
	return cmd
}
