// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Transcendence feature (hand-authored, Phase 3): render a paste-ready
// post-mortem markdown skeleton from synced incident + timeline + action items.
// Pure local extraction, zero API round-trips.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelPostmortemSkeletonCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "postmortem-skeleton <incident-id>",
		Short: "Emit a paste-ready post-mortem markdown skeleton for an incident.",
		Long: `Render a retrospective skeleton from the local mirror: header metadata
(severity, status, duration, affected services), the timeline, action items, and
empty analysis sections (Summary, Impact, Root Cause, Resolution, Lessons) ready
for you to fill in. Pure offline extraction — no API round-trips.`,
		Example: `  rootly-cli postmortem-skeleton INC-1234 > postmortem.md
  rootly-cli postmortem-skeleton INC-1234 --json`,
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

			start, hasStart := incidentStart(*inc)
			duration := ""
			if hasStart {
				end := time.Now()
				if r, ok := incidentResolved(*inc); ok {
					end = r
				}
				duration = humanDuration(end.Sub(start))
			}

			events := novelLoadChildTable(db, "incidents_events", "incidents_id", inc.ID)
			type ev struct {
				at   string
				text string
			}
			var tl []ev
			for _, e := range events {
				tl = append(tl, ev{
					at:   recStr(e.Attrs, "occurred_at", "created_at"),
					text: strings.TrimSpace(recStr(e.Attrs, "event", "summary", "description")),
				})
			}
			sort.Slice(tl, func(i, j int) bool { return tl[i].at < tl[j].at })
			actions := collectAllActionItems(db, inc.ID)

			var b strings.Builder
			fmt.Fprintf(&b, "# Post-Mortem: %s\n\n", incidentTitle(*inc))
			fmt.Fprintf(&b, "- **Incident:** %s\n", inc.ID)
			fmt.Fprintf(&b, "- **Severity:** %s\n", dash(incidentSeverity(*inc)))
			fmt.Fprintf(&b, "- **Status:** %s\n", dash(recStr(inc.Attrs, "status")))
			fmt.Fprintf(&b, "- **Started:** %s\n", dash(recStr(inc.Attrs, "started_at", "created_at")))
			fmt.Fprintf(&b, "- **Resolved:** %s\n", dash(recStr(inc.Attrs, "resolved_at", "mitigated_at")))
			fmt.Fprintf(&b, "- **Duration:** %s\n", dash(duration))
			fmt.Fprintf(&b, "- **Services:** %s\n", dash(strings.Join(incidentServiceNames(*inc), ", ")))
			fmt.Fprintf(&b, "- **Teams:** %s\n\n", dash(strings.Join(incidentTeamNames(*inc), ", ")))

			b.WriteString("## Summary\n\n")
			if s := strings.TrimSpace(recStr(inc.Attrs, "summary")); s != "" {
				b.WriteString(s + "\n\n")
			} else {
				b.WriteString("_TODO: one-paragraph summary of what happened._\n\n")
			}
			b.WriteString("## Impact\n\n_TODO: who and what was affected, for how long._\n\n")
			b.WriteString("## Timeline\n\n")
			if len(tl) == 0 {
				b.WriteString("_No timeline events synced. Run `rootly-cli sync` or add events in Rootly._\n\n")
			} else {
				for _, e := range tl {
					at := e.at
					if at == "" {
						at = "—"
					}
					fmt.Fprintf(&b, "- **%s** — %s\n", at, e.text)
				}
				b.WriteString("\n")
			}
			b.WriteString("## Root Cause\n\n_TODO: the underlying cause._\n\n")
			b.WriteString("## Resolution\n\n")
			if res := strings.TrimSpace(recStr(inc.Attrs, "resolution_message", "mitigation_message")); res != "" {
				b.WriteString(res + "\n\n")
			} else {
				b.WriteString("_TODO: how the incident was resolved._\n\n")
			}
			b.WriteString("## Action Items\n\n")
			if len(actions) == 0 {
				b.WriteString("_No action items synced._\n\n")
			} else {
				for _, a := range actions {
					fmt.Fprintf(&b, "- [ ] %s\n", a)
				}
				b.WriteString("\n")
			}
			b.WriteString("## Lessons Learned\n\n_TODO: what went well, what didn't, what we'll change._\n")

			md := b.String()

			if flags.asJSON || flags.agent {
				return flags.printJSON(cmd, map[string]any{
					"incident_id": inc.ID,
					"title":       incidentTitle(*inc),
					"markdown":    md,
				})
			}
			fmt.Fprint(cmd.OutOrStdout(), md)
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/rootly-cli/data.db)")
	return cmd
}
