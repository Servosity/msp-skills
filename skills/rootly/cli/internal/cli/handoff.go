// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Transcendence feature (hand-authored, Phase 3): end-of-shift handoff summary.
// Windows incidents + action items to a time range from the local mirror.
// Matches Rootly's remote get_oncall_handoff_summary, offline.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelHandoffCmd(flags *rootFlags) *cobra.Command {
	var hours int
	var scheduleFilter string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "handoff",
		Short: "End-of-shift summary: what opened, closed, and is still open.",
		Long: `Summarize the outgoing shift for the next on-call: incidents opened, closed,
and still open during the window, the severity mix, and open action items. The
window defaults to the last --hours (24); when --schedule is given and shifts are
synced, the most recent past shift for that schedule sets the window. Offline
extraction over the local mirror.`,
		Example: `  rootly-cli handoff
  rootly-cli handoff --hours 12 --json
  rootly-cli handoff --schedule primary`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if hours <= 0 {
				hours = 24
			}

			db, err := novelOpenStoreChecked(cmd, flags, dbPath, "shifts")
			if err != nil {
				return err
			}
			defer db.Close()

			now := time.Now()
			winStart := now.Add(-time.Duration(hours) * time.Hour)
			winEnd := now

			// If a schedule is named and shifts exist, use the most recent past
			// shift's bounds for that schedule as the window.
			if scheduleFilter != "" {
				schedNames := map[string]string{}
				scheds, _ := novelLoad(db, novelResolveType(db, "schedules"))
				for _, s := range scheds {
					schedNames[s.ID] = recStr(s.Attrs, "name")
				}
				shifts, _ := novelLoad(db, novelResolveType(db, "shifts"))
				var best *struct{ start, end time.Time }
				for _, sh := range shifts {
					sid := firstNonEmpty(recStr(sh.Attrs, "schedule_id"), relID(sh, "schedule"))
					if !strings.Contains(strings.ToLower(schedNames[sid]), strings.ToLower(scheduleFilter)) {
						continue
					}
					st, ok1 := recTime(sh.Attrs, "starts_at")
					en, ok2 := recTime(sh.Attrs, "ends_at")
					if !ok1 || !ok2 || en.After(now) {
						continue // want a shift that has ended
					}
					if best == nil || st.After(best.start) {
						best = &struct{ start, end time.Time }{st, en}
					}
				}
				if best != nil {
					winStart, winEnd = best.start, best.end
				}
			}

			inWindow := func(t time.Time, ok bool) bool {
				return ok && !t.Before(winStart) && !t.After(winEnd)
			}

			incidents, err := novelLoad(db, novelResolveType(db, "incidents"))
			if err != nil {
				return err
			}

			type incRef struct {
				ID       string `json:"id"`
				Title    string `json:"title"`
				Severity string `json:"severity,omitempty"`
				Status   string `json:"status,omitempty"`
			}
			var opened, closed, stillOpen []incRef
			severityMix := map[string]int{}
			var openActions []string
			for _, r := range incidents {
				start, hasStart := incidentStart(r)
				ref := incRef{ID: r.ID, Title: incidentTitle(r), Severity: incidentSeverity(r), Status: recStr(r.Attrs, "status")}
				openedInWin := inWindow(start, hasStart)
				resolved, hasRes := incidentResolved(r)
				closedInWin := inWindow(resolved, hasRes)
				if openedInWin {
					opened = append(opened, ref)
					sev := incidentSeverity(r)
					if sev == "" {
						sev = "(none)"
					}
					severityMix[sev]++
				}
				if closedInWin {
					closed = append(closed, ref)
				}
				// still open: opened on/before window end and not resolved by window end
				if hasStart && !start.After(winEnd) && incidentOpen(r) {
					stillOpen = append(stillOpen, ref)
					openActions = append(openActions, collectOpenActionItems(db, r.ID)...)
				}
			}
			sort.Slice(stillOpen, func(i, j int) bool { return stillOpen[i].ID < stillOpen[j].ID })
			// dedup action items
			seen := map[string]bool{}
			dedupActions := []string{}
			for _, a := range openActions {
				if !seen[a] {
					seen[a] = true
					dedupActions = append(dedupActions, a)
				}
			}

			out := struct {
				Window      map[string]string `json:"window"`
				Opened      []incRef          `json:"opened"`
				Closed      []incRef          `json:"closed"`
				StillOpen   []incRef          `json:"still_open"`
				SeverityMix map[string]int    `json:"severity_mix"`
				OpenActions []string          `json:"open_action_items"`
			}{
				Window:      map[string]string{"from": winStart.Format(time.RFC3339), "to": winEnd.Format(time.RFC3339)},
				Opened:      nonNilRefs(opened),
				Closed:      nonNilRefs(closed),
				StillOpen:   nonNilRefs(stillOpen),
				SeverityMix: severityMix,
				OpenActions: dedupActions,
			}
			if out.OpenActions == nil {
				out.OpenActions = []string{}
			}

			return novelEmit(cmd, flags, out, func() {
				w := cmd.OutOrStdout()
				fmt.Fprintf(w, "Shift handoff — %s → %s\n\n", winStart.Format("Jan 2 15:04"), winEnd.Format("Jan 2 15:04"))
				if len(incidents) == 0 {
					fmt.Fprintln(w, "No incidents synced — run 'rootly-cli sync'.")
					return
				}
				fmt.Fprintf(w, "  opened:     %d\n", len(out.Opened))
				fmt.Fprintf(w, "  closed:     %d\n", len(out.Closed))
				fmt.Fprintf(w, "  still open: %d\n", len(out.StillOpen))
				if len(out.SeverityMix) > 0 {
					var parts []string
					for k, v := range out.SeverityMix {
						parts = append(parts, fmt.Sprintf("%s=%d", k, v))
					}
					sort.Strings(parts)
					fmt.Fprintf(w, "  opened severity mix: %s\n", strings.Join(parts, " "))
				}
				if len(out.StillOpen) > 0 {
					fmt.Fprintln(w, "\n  still-open incidents:")
					for _, r := range out.StillOpen {
						fmt.Fprintf(w, "    %s  [%s]  %s\n", r.ID, dash(r.Severity), truncate(r.Title, 60))
					}
				}
				if len(out.OpenActions) > 0 {
					fmt.Fprintf(w, "\n  open action items (%d):\n", len(out.OpenActions))
					for _, a := range out.OpenActions {
						fmt.Fprintf(w, "    - %s\n", truncate(a, 100))
					}
				}
			})
		},
	}
	cmd.Flags().IntVar(&hours, "hours", 24, "Window length in hours (used when --schedule is not set)")
	cmd.Flags().StringVar(&scheduleFilter, "schedule", "", "Use the most recent past shift of the matching schedule as the window")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/rootly-cli/data.db)")
	return cmd
}

// nonNilRefs guarantees a non-nil slice for stable JSON ([] not null).
func nonNilRefs[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
