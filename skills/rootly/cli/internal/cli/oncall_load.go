// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Transcendence feature (hand-authored, Phase 3): per-person on-call burden over
// a window — hours on call, shift count, and pages received while on shift —
// across ALL schedules. No Rootly screen or endpoint totals this per person.

package cli

import (
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelOncallLoadCmd(flags *rootFlags) *cobra.Command {
	var days int
	var limit int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "oncall-load",
		Short: "Rank people by on-call hours and pages received across every schedule.",
		Long: `Surface uneven rotations and burnout risk before someone quits. Reads synced
shifts and incidents from the local mirror, clips each shift to the lookback
window, totals per-person on-call hours and shift counts across ALL schedules,
and counts incidents that started during each person's watch. Offline — run
'rootly-cli sync --resources shifts,incidents' first.`,
		Example: `  rootly-cli oncall-load --days 30
  rootly-cli oncall-load --days 7 --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if days <= 0 {
				days = 30
			}
			db, err := novelOpenStoreChecked(cmd, flags, dbPath, "shifts")
			if err != nil {
				return err
			}
			defer db.Close()

			now := time.Now()
			windowStart := now.Add(-time.Duration(days) * 24 * time.Hour)

			shifts, err := novelLoad(db, novelResolveType(db, "shifts"))
			if err != nil {
				return err
			}

			type window struct{ start, end time.Time }
			type loadRow struct {
				User   string  `json:"user"`
				Hours  float64 `json:"hours_on_call"`
				Shifts int     `json:"shifts"`
				Pages  int     `json:"pages_during_shifts"`
			}
			byUser := map[string]*loadRow{}
			watch := map[string][]window{}
			scanned := 0
			for _, s := range shifts {
				user := firstNonEmpty(recName(s.Attrs["user"]), recName(s.Rels["user"]), recStr(s.Attrs, "user_name"))
				if user == "" {
					continue
				}
				start, okS := recTime(s.Attrs, "starts_at", "start_time", "start", "begins_at")
				end, okE := recTime(s.Attrs, "ends_at", "end_time", "end", "finishes_at")
				if !okS || !okE || !end.After(start) {
					continue
				}
				// Clip to the lookback window; skip shifts entirely outside it.
				if end.Before(windowStart) || start.After(now) {
					continue
				}
				scanned++
				cs, ce := start, end
				if cs.Before(windowStart) {
					cs = windowStart
				}
				if ce.After(now) {
					ce = now
				}
				row := byUser[user]
				if row == nil {
					row = &loadRow{User: user}
					byUser[user] = row
				}
				row.Hours += ce.Sub(cs).Hours()
				row.Shifts++
				watch[user] = append(watch[user], window{start: start, end: end})
			}

			// Pages: incidents that started during a person's shift.
			incidents, _ := novelLoad(db, novelResolveType(db, "incidents"))
			for _, inc := range incidents {
				started, ok := incidentStart(inc)
				if !ok || started.Before(windowStart) || started.After(now) {
					continue
				}
				for user, wins := range watch {
					for _, w := range wins {
						if !started.Before(w.start) && started.Before(w.end) {
							byUser[user].Pages++
							break
						}
					}
				}
			}

			rows := make([]loadRow, 0, len(byUser))
			for _, r := range byUser {
				r.Hours = float64(int(r.Hours*10)) / 10
				rows = append(rows, *r)
			}
			sort.Slice(rows, func(i, j int) bool {
				if rows[i].Hours == rows[j].Hours {
					return rows[i].User < rows[j].User
				}
				return rows[i].Hours > rows[j].Hours
			})
			if limit > 0 && len(rows) > limit {
				rows = rows[:limit]
			}

			out := struct {
				WindowDays    int       `json:"window_days"`
				Items         []loadRow `json:"items"`
				ScannedShifts int       `json:"scanned_shifts"`
				Note          string    `json:"note,omitempty"`
			}{WindowDays: days, Items: rows, ScannedShifts: scanned}
			if len(rows) == 0 {
				out.Note = "no shifts with user + start/end found in the local store; run 'rootly-cli sync --resources shifts,incidents' first"
			}

			return novelEmit(cmd, flags, out, func() {
				w := cmd.OutOrStdout()
				fmt.Fprintf(w, "on-call load, last %d days (%d shifts scanned)\n", days, scanned)
				if len(rows) == 0 {
					fmt.Fprintf(w, "  %s\n", out.Note)
					return
				}
				for _, r := range rows {
					fmt.Fprintf(w, "  %-28s %7.1fh  %3d shifts  %3d pages\n", r.User, r.Hours, r.Shifts, r.Pages)
				}
			})
		},
	}
	cmd.Flags().IntVar(&days, "days", 30, "Lookback window in days")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum people to return (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/rootly-cli/data.db)")
	return cmd
}
