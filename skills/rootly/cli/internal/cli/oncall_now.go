// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Transcendence feature (hand-authored, Phase 3): who is on call right now across
// every schedule, from the synced on-call snapshot. Cross-portfolio view in one
// command.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelOncallNowCmd(flags *rootFlags) *cobra.Command {
	var scheduleFilter string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "oncall-now",
		Short: "Show who is on call right now across every schedule and service.",
		Long: `List the current on-call roster across all schedules in one table from the
synced 'oncalls' snapshot. If no on-call snapshot is present, falls back to the
shifts that cover the current moment. Filter to one schedule with --schedule.`,
		Example: `  rootly-cli oncall-now
  rootly-cli oncall-now --schedule primary --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			db, err := novelOpenStoreChecked(cmd, flags, dbPath, "shifts")
			if err != nil {
				return err
			}
			defer db.Close()

			type entry struct {
				User             string `json:"user"`
				Schedule         string `json:"schedule,omitempty"`
				EscalationPolicy string `json:"escalation_policy,omitempty"`
				Source           string `json:"source"`
			}
			var entries []entry

			for _, e := range currentOncallEntries(db) {
				if scheduleFilter != "" && !strings.Contains(strings.ToLower(e.Schedule), strings.ToLower(scheduleFilter)) {
					continue
				}
				entries = append(entries, entry{User: e.User, Schedule: e.Schedule, EscalationPolicy: e.EscalationPolicy, Source: "oncalls"})
			}

			// Fallback: derive from shifts covering now.
			if len(entries) == 0 {
				now := time.Now()
				schedNames := map[string]string{}
				scheds, _ := novelLoad(db, novelResolveType(db, "schedules"))
				for _, s := range scheds {
					schedNames[s.ID] = recStr(s.Attrs, "name")
				}
				users := map[string]string{}
				us, _ := novelLoad(db, novelResolveType(db, "users"))
				for _, u := range us {
					users[u.ID] = firstNonEmpty(recStr(u.Attrs, "full_name", "name"), recStr(u.Attrs, "email"))
				}
				shifts, _ := novelLoad(db, novelResolveType(db, "shifts"))
				for _, sh := range shifts {
					st, ok1 := recTime(sh.Attrs, "starts_at")
					en, ok2 := recTime(sh.Attrs, "ends_at")
					if !ok1 || !ok2 || now.Before(st) || now.After(en) {
						continue
					}
					sid := firstNonEmpty(recStr(sh.Attrs, "schedule_id"), relID(sh, "schedule"))
					schedName := schedNames[sid]
					if scheduleFilter != "" && !strings.Contains(strings.ToLower(schedName), strings.ToLower(scheduleFilter)) {
						continue
					}
					uid := firstNonEmpty(recStr(sh.Attrs, "user_id"), relID(sh, "user"))
					entries = append(entries, entry{User: firstNonEmpty(users[uid], uid), Schedule: schedName, Source: "shifts"})
				}
			}

			sort.Slice(entries, func(i, j int) bool {
				if entries[i].Schedule == entries[j].Schedule {
					return entries[i].User < entries[j].User
				}
				return entries[i].Schedule < entries[j].Schedule
			})

			out := struct {
				Count  int     `json:"count"`
				OnCall []entry `json:"oncall"`
			}{Count: len(entries), OnCall: entries}
			if out.OnCall == nil {
				out.OnCall = []entry{}
			}

			return novelEmit(cmd, flags, out, func() {
				w := cmd.OutOrStdout()
				if len(entries) == 0 {
					fmt.Fprintln(w, "No current on-call found. Run 'rootly-cli sync' (syncs the 'oncalls' and 'shifts' resources).")
					return
				}
				tw := newTabWriter(w)
				fmt.Fprintln(tw, "USER\tSCHEDULE\tESCALATION POLICY\tSOURCE")
				for _, e := range entries {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", dash(e.User), dash(e.Schedule), dash(e.EscalationPolicy), e.Source)
				}
				flushHuman(cmd, tw)
			})
		},
	}
	cmd.Flags().StringVar(&scheduleFilter, "schedule", "", "Only show on-call for schedules whose name contains this string")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/rootly-cli/data.db)")
	return cmd
}
