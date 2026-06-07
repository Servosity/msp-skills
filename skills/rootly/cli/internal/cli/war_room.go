// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Transcendence feature (hand-authored, Phase 3): one-screen incident war room.
// Joins incident + timeline + action items + services + current on-call from the
// local mirror. No single JSON:API call returns this composite.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelWarRoomCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "war-room <incident-id>",
		Short: "One screen for an active incident: header, timeline, action items, on-call.",
		Long: `Assemble everything you need for an active incident into one view from the
local mirror: header (severity, status, duration), affected services and teams,
the timeline, open action items, and who is currently on call for the affected
services. Offline join — no single API call returns this composite.`,
		Example: `  rootly-cli war-room INC-1234
  rootly-cli war-room INC-1234 --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		Args:        cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			target := args[0]

			db, err := novelOpenStoreChecked(cmd, flags, dbPath, "incidents")
			if err != nil {
				return err
			}
			defer db.Close()

			incidents, err := novelLoad(db, novelResolveType(db, "incidents"))
			if err != nil {
				return err
			}
			var inc *record
			for i := range incidents {
				if incidents[i].ID == target ||
					recStr(incidents[i].Attrs, "slug") == target ||
					recStr(incidents[i].Attrs, "sequential_id") == target {
					inc = &incidents[i]
					break
				}
			}
			if inc == nil {
				return notFoundErr(fmt.Errorf("incident %q not found in the local mirror (run 'rootly-cli sync')", target))
			}

			// Timeline: prefer the synced sub-resource table, fall back to none.
			timeline := novelLoadChildTable(db, "incidents_events", "incidents_id", inc.ID)
			type tlEvent struct {
				At    string `json:"at,omitempty"`
				Event string `json:"event"`
			}
			var events []tlEvent
			for _, e := range timeline {
				events = append(events, tlEvent{
					At:    recStr(e.Attrs, "occurred_at", "created_at"),
					Event: strings.TrimSpace(recStr(e.Attrs, "event", "summary", "description")),
				})
			}
			sort.Slice(events, func(i, j int) bool { return events[i].At < events[j].At })

			// Open action items: union of the sub-resource table and any
			// top-level action-items that reference this incident.
			openAIs := collectOpenActionItems(db, inc.ID)

			// Current on-call for the affected services.
			services := incidentServiceNames(*inc)
			oncall := oncallForServices(db, services)

			start, hasStart := incidentStart(*inc)
			dur := ""
			if hasStart {
				end := time.Now()
				if r, ok := incidentResolved(*inc); ok {
					end = r
				}
				dur = humanDuration(end.Sub(start))
			}

			out := struct {
				ID          string    `json:"id"`
				Title       string    `json:"title"`
				Severity    string    `json:"severity,omitempty"`
				Status      string    `json:"status,omitempty"`
				Open        bool      `json:"open"`
				StartedAt   string    `json:"started_at,omitempty"`
				Duration    string    `json:"duration,omitempty"`
				Services    []string  `json:"services"`
				Teams       []string  `json:"teams"`
				URL         string    `json:"url,omitempty"`
				Timeline    []tlEvent `json:"timeline"`
				ActionItems []string  `json:"open_action_items"`
				OnCall      []string  `json:"current_oncall"`
			}{
				ID:          inc.ID,
				Title:       incidentTitle(*inc),
				Severity:    incidentSeverity(*inc),
				Status:      recStr(inc.Attrs, "status"),
				Open:        incidentOpen(*inc),
				StartedAt:   recStr(inc.Attrs, "started_at", "created_at"),
				Duration:    dur,
				Services:    services,
				Teams:       incidentTeamNames(*inc),
				URL:         recStr(inc.Attrs, "url", "short_url"),
				Timeline:    events,
				ActionItems: openAIs,
				OnCall:      oncall,
			}
			if out.Services == nil {
				out.Services = []string{}
			}
			if out.Teams == nil {
				out.Teams = []string{}
			}
			if out.Timeline == nil {
				out.Timeline = []tlEvent{}
			}

			return novelEmit(cmd, flags, out, func() {
				w := cmd.OutOrStdout()
				fmt.Fprintf(w, "WAR ROOM — %s  %s\n", out.ID, out.Title)
				fmt.Fprintf(w, "  severity: %s   status: %s   open: %v   duration: %s\n", dash(out.Severity), dash(out.Status), out.Open, dash(out.Duration))
				fmt.Fprintf(w, "  services: %s\n", dash(strings.Join(out.Services, ", ")))
				fmt.Fprintf(w, "  teams:    %s\n", dash(strings.Join(out.Teams, ", ")))
				if out.URL != "" {
					fmt.Fprintf(w, "  url:      %s\n", out.URL)
				}
				fmt.Fprintf(w, "\n  current on-call: %s\n", dash(strings.Join(out.OnCall, ", ")))
				fmt.Fprintf(w, "\n  open action items (%d):\n", len(out.ActionItems))
				for _, a := range out.ActionItems {
					fmt.Fprintf(w, "    - %s\n", a)
				}
				fmt.Fprintf(w, "\n  timeline (%d events):\n", len(out.Timeline))
				for _, e := range out.Timeline {
					fmt.Fprintf(w, "    %s  %s\n", e.At, truncate(e.Event, 80))
				}
			})
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/rootly-cli/data.db)")
	return cmd
}

func dash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
