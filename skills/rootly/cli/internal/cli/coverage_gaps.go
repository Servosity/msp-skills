// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Transcendence feature (hand-authored, Phase 3): detect unstaffed on-call
// windows across all schedules from synced shifts. No Rootly endpoint returns
// "coverage gaps".

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelCoverageGapsCmd(flags *rootFlags) *cobra.Command {
	var days int
	var scheduleFilter string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "coverage-gaps",
		Short: "Scan on-call schedules for unstaffed windows over the next N days.",
		Long: `Find on-call coverage gaps before a page is missed. Reads synced shifts,
groups them by schedule, and reports time windows in the next N days with no
shift assigned. Interval math over the local mirror — no Rootly endpoint returns
unstaffed windows, and this spans every schedule at once.`,
		Example: `  rootly-cli coverage-gaps --days 14
  rootly-cli coverage-gaps --schedule primary --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if days <= 0 {
				days = 14
			}

			db, err := novelOpenStoreChecked(cmd, flags, dbPath, "shifts")
			if err != nil {
				return err
			}
			defer db.Close()

			shifts, err := novelLoad(db, novelResolveType(db, "shifts"))
			if err != nil {
				return err
			}
			// schedule id -> name
			schedNames := map[string]string{}
			scheds, _ := novelLoad(db, novelResolveType(db, "schedules"))
			for _, s := range scheds {
				schedNames[s.ID] = recStr(s.Attrs, "name")
			}

			now := time.Now()
			windowEnd := now.AddDate(0, 0, days)

			type interval struct{ start, end time.Time }
			bySchedule := map[string][]interval{}
			for _, sh := range shifts {
				st, ok1 := recTime(sh.Attrs, "starts_at")
				en, ok2 := recTime(sh.Attrs, "ends_at")
				if !ok1 || !ok2 {
					continue
				}
				sid := firstNonEmpty(recStr(sh.Attrs, "schedule_id"), relID(sh, "schedule"))
				if sid == "" {
					sid = "(unknown schedule)"
				}
				if scheduleFilter != "" {
					nm := strings.ToLower(schedNames[sid])
					if !strings.Contains(nm, strings.ToLower(scheduleFilter)) && !strings.Contains(strings.ToLower(sid), strings.ToLower(scheduleFilter)) {
						continue
					}
				}
				// clamp to [now, windowEnd]
				if en.Before(now) || st.After(windowEnd) {
					continue
				}
				if st.Before(now) {
					st = now
				}
				if en.After(windowEnd) {
					en = windowEnd
				}
				bySchedule[sid] = append(bySchedule[sid], interval{st, en})
			}

			type gap struct {
				From     string `json:"from"`
				To       string `json:"to"`
				Duration string `json:"duration"`
				minutes  int
			}
			type schedGaps struct {
				ScheduleID   string `json:"schedule_id"`
				ScheduleName string `json:"schedule_name,omitempty"`
				Gaps         []gap  `json:"gaps"`
			}
			var result []schedGaps
			totalGapMin := 0
			for sid, ivs := range bySchedule {
				sort.Slice(ivs, func(i, j int) bool { return ivs[i].start.Before(ivs[j].start) })
				// merge overlapping, then find gaps within [now, windowEnd]
				merged := []interval{}
				for _, iv := range ivs {
					if len(merged) == 0 || iv.start.After(merged[len(merged)-1].end) {
						merged = append(merged, iv)
					} else if iv.end.After(merged[len(merged)-1].end) {
						merged[len(merged)-1].end = iv.end
					}
				}
				var gaps []gap
				cursor := now
				for _, iv := range merged {
					if iv.start.After(cursor) {
						d := iv.start.Sub(cursor)
						gaps = append(gaps, gap{cursor.Format(time.RFC3339), iv.start.Format(time.RFC3339), humanDuration(d), roundMinutes(d)})
						totalGapMin += roundMinutes(d)
					}
					if iv.end.After(cursor) {
						cursor = iv.end
					}
				}
				if cursor.Before(windowEnd) {
					d := windowEnd.Sub(cursor)
					gaps = append(gaps, gap{cursor.Format(time.RFC3339), windowEnd.Format(time.RFC3339), humanDuration(d), roundMinutes(d)})
					totalGapMin += roundMinutes(d)
				}
				if len(gaps) == 0 {
					continue
				}
				result = append(result, schedGaps{ScheduleID: sid, ScheduleName: schedNames[sid], Gaps: gaps})
			}
			sort.Slice(result, func(i, j int) bool { return result[i].ScheduleName < result[j].ScheduleName })

			out := struct {
				Window        map[string]any `json:"window"`
				TotalGapMins  int            `json:"total_gap_minutes"`
				SchedulesWith int            `json:"schedules_with_gaps"`
				Schedules     []schedGaps    `json:"schedules"`
			}{
				Window:        map[string]any{"from": now.Format(time.RFC3339), "to": windowEnd.Format(time.RFC3339), "days": days},
				TotalGapMins:  totalGapMin,
				SchedulesWith: len(result),
				Schedules:     result,
			}
			if out.Schedules == nil {
				out.Schedules = []schedGaps{}
			}

			return novelEmit(cmd, flags, out, func() {
				w := cmd.OutOrStdout()
				if len(shifts) == 0 {
					fmt.Fprintln(w, "No shifts synced — run 'rootly-cli sync' to evaluate coverage.")
					return
				}
				if len(result) == 0 {
					fmt.Fprintf(w, "No coverage gaps in the next %d days. ✓\n", days)
					return
				}
				fmt.Fprintf(w, "Coverage gaps in the next %d days (%d schedule(s) affected):\n\n", days, len(result))
				for _, sg := range result {
					name := dash(sg.ScheduleName)
					fmt.Fprintf(w, "  %s [%s]\n", name, sg.ScheduleID)
					for _, g := range sg.Gaps {
						fmt.Fprintf(w, "    GAP %s  →  %s   (%s)\n", g.From, g.To, g.Duration)
					}
				}
			})
		},
	}
	cmd.Flags().IntVar(&days, "days", 14, "Days ahead to scan for coverage gaps")
	cmd.Flags().StringVar(&scheduleFilter, "schedule", "", "Only scan schedules whose name or id contains this string")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/rootly-cli/data.db)")
	return cmd
}
