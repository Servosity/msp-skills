// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// audit schedule-gaps: future time windows where a schedule has nobody on
// call. Merges the synced on-call intervals per schedule across a look-ahead
// window and reports the uncovered slots — the temporal complement of
// `audit coverage` (which audits the structural escalation chain). The
// `oncalls` endpoint answers "who is on at time T" but never reports the gap;
// this finds the hole before an incident does. Registered as a subcommand of
// the promoted `audit` parent in root.go.
package cli

import (
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/spf13/cobra"
)

type pdScheduleGap struct {
	From     string `json:"from"`
	Until    string `json:"until"`
	Duration string `json:"duration"`
}

type pdScheduleGapRow struct {
	ScheduleID   string          `json:"schedule_id"`
	Schedule     string          `json:"schedule"`
	Covered      bool            `json:"covered"`
	NoSyncedData bool            `json:"no_synced_data,omitempty"`
	Gaps         []pdScheduleGap `json:"gaps"`
}

type pdScheduleGapsResult struct {
	Window struct {
		From  string `json:"from"`
		Until string `json:"until"`
	} `json:"window"`
	Schedules []pdScheduleGapRow `json:"schedules"`
	Note      string             `json:"note,omitempty"`
}

// pp:data-source local
func newNovelAuditScheduleGapsCmd(flags *rootFlags) *cobra.Command {
	var flagDays int
	var flagSchedule string

	cmd := &cobra.Command{
		Use:   "schedule-gaps",
		Short: "Future time windows where a schedule has nobody on call, from synced on-call entries",
		Long: `Finds the uncovered slots on every schedule across the next --days: merges the
synced on-call intervals per schedule and reports any window with nobody on
call, plus schedules with no synced future coverage at all.

Use this command to find time windows with nobody on a schedule. Do NOT use it
for whole-service escalation-chain holes (missing policies, empty tiers,
single points of failure); use 'audit coverage' instead.

Coverage derives from the synced on-call entries, so the horizon is bounded by
what the last sync captured. Run ` + "`sync --resources oncalls,schedules`" + ` first;
exits 0 and marks schedules "no synced data" when nothing relevant is synced.`,
		Example:     "  pagerduty-cli audit schedule-gaps\n  pagerduty-cli audit schedule-gaps --days 7 --agent\n  pagerduty-cli audit schedule-gaps --schedule PSCHED1",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			// Hint on oncalls: the gap math is bounded by synced on-call entries,
			// so a missing/stale oncalls sync is what produces empty/uncovered rows.
			pdHintSync(cmd, flags, "oncalls")
			if flagDays <= 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--days must be a positive number of days, got %d", flagDays))
			}
			schedules, err := pdLoadResource(cmd.Context(), "schedules")
			if err != nil {
				return fmt.Errorf("reading schedules from local store: %w", err)
			}
			oncalls, err := pdLoadResource(cmd.Context(), "oncalls")
			if err != nil {
				return fmt.Errorf("reading oncalls from local store: %w", err)
			}
			now := time.Now()
			win := pdWindow{Since: now, Until: now.AddDate(0, 0, flagDays)}
			res := buildScheduleGaps(schedules, oncalls, win, flagSchedule)
			return pdEmit(cmd, flags, res, func(w io.Writer) {
				if len(res.Schedules) == 0 {
					fmt.Fprintln(w, "No schedules in the local store (run `pagerduty-cli sync --resources oncalls,schedules` first).")
					return
				}
				fmt.Fprintf(w, "Schedule coverage %s → %s\n\n", res.Window.From, res.Window.Until)
				for _, s := range res.Schedules {
					switch {
					case s.NoSyncedData:
						fmt.Fprintf(w, "  %s (%s): NO SYNCED COVERAGE — sync a wider on-call window to audit this schedule\n", s.Schedule, s.ScheduleID)
					case s.Covered:
						fmt.Fprintf(w, "  %s (%s): fully covered\n", s.Schedule, s.ScheduleID)
					default:
						fmt.Fprintf(w, "  %s (%s): %d gap(s)\n", s.Schedule, s.ScheduleID, len(s.Gaps))
						for _, g := range s.Gaps {
							fmt.Fprintf(w, "      %s → %s (%s uncovered)\n", g.From, g.Until, g.Duration)
						}
					}
				}
			})
		},
	}
	cmd.Flags().IntVar(&flagDays, "days", 14, "Look-ahead window in days")
	cmd.Flags().StringVar(&flagSchedule, "schedule", "", "Limit to a single schedule ID or name")
	return cmd
}

// buildScheduleGaps is the pure split-out for tests.
func buildScheduleGaps(schedules, oncalls []map[string]any, win pdWindow, scheduleFilter string) pdScheduleGapsResult {
	var res pdScheduleGapsResult
	res.Window.From = win.Since.UTC().Format(time.RFC3339)
	res.Window.Until = win.Until.UTC().Format(time.RFC3339)
	res.Schedules = []pdScheduleGapRow{}

	type interval struct{ start, end time.Time }
	bySchedule := map[string][]interval{}
	for _, oc := range oncalls {
		schedRef := pdMap(oc, "schedule")
		sid := pdString(schedRef, "id")
		if sid == "" {
			continue // EP-level on-call with no schedule backing
		}
		start, hasStart := pdParseTime(pdString(oc, "start"))
		end, hasEnd := pdParseTime(pdString(oc, "end"))
		// Clip to the window; open ends cover the whole window side.
		if !hasStart || start.Before(win.Since) {
			start = win.Since
		}
		if !hasEnd || end.After(win.Until) {
			end = win.Until
		}
		if !end.After(start) {
			continue
		}
		bySchedule[sid] = append(bySchedule[sid], interval{start, end})
	}

	for _, sc := range schedules {
		sid := pdString(sc, "id")
		name := pdRefLabel(sc)
		if scheduleFilter != "" && sid != scheduleFilter && name != scheduleFilter {
			continue
		}
		row := pdScheduleGapRow{ScheduleID: sid, Schedule: name, Gaps: []pdScheduleGap{}}
		ivs := bySchedule[sid]
		if len(ivs) == 0 {
			row.NoSyncedData = true
			res.Schedules = append(res.Schedules, row)
			continue
		}
		sort.Slice(ivs, func(i, j int) bool { return ivs[i].start.Before(ivs[j].start) })
		// Merge and walk for gaps.
		cursor := win.Since
		for _, iv := range ivs {
			if iv.start.After(cursor) {
				row.Gaps = append(row.Gaps, pdScheduleGap{
					From:     cursor.UTC().Format(time.RFC3339),
					Until:    iv.start.UTC().Format(time.RFC3339),
					Duration: pdHumanDur(iv.start.Sub(cursor)),
				})
			}
			if iv.end.After(cursor) {
				cursor = iv.end
			}
		}
		if cursor.Before(win.Until) {
			row.Gaps = append(row.Gaps, pdScheduleGap{
				From:     cursor.UTC().Format(time.RFC3339),
				Until:    win.Until.UTC().Format(time.RFC3339),
				Duration: pdHumanDur(win.Until.Sub(cursor)),
			})
		}
		row.Covered = len(row.Gaps) == 0
		res.Schedules = append(res.Schedules, row)
	}

	// Gappy schedules first, fully-covered last; unsynced in between.
	sort.SliceStable(res.Schedules, func(i, j int) bool {
		return scheduleGapRank(res.Schedules[i]) < scheduleGapRank(res.Schedules[j])
	})
	if len(res.Schedules) == 0 {
		res.Note = "no schedules matched; run `pagerduty-cli sync --resources oncalls,schedules` or widen --schedule"
	}
	return res
}

func scheduleGapRank(r pdScheduleGapRow) int {
	switch {
	case len(r.Gaps) > 0:
		return 0
	case r.NoSyncedData:
		return 1
	default:
		return 2
	}
}
