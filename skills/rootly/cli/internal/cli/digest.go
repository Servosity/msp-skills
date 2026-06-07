// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Transcendence feature (hand-authored, Phase 3): the "what did I miss" view.
// Windows incidents, timeline events, action items, and alerts to a --since
// bound in one local query — a portfolio-wide rollup no single API call returns.

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelDigestCmd(flags *rootFlags) *cobra.Command {
	var since string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "digest",
		Short: "Time-windowed rollup of everything that moved: incidents, action items, alerts.",
		Long: `Use this command for a portfolio-wide "what moved since <time>" rollup across
all incidents and alerts — incidents opened and resolved, severity-change
timeline events, new action items, and alerts fired since the given bound.
Reads only the local mirror; run 'rootly-cli sync' first for fresh data.

Do NOT use this command for an outgoing on-call's end-of-shift summary scoped
to one schedule; use 'handoff' instead.`,
		Example: `  rootly-cli digest --since 24h
  rootly-cli digest --since 7d --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			window, ok := parseWindowDuration(since)
			if !ok || window <= 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--since must be a duration like 24h, 7d, 90m (got %q)", since))
			}
			db, err := novelOpenStoreChecked(cmd, flags, dbPath, "incidents")
			if err != nil {
				return err
			}
			defer db.Close()

			now := time.Now()
			cutoff := now.Add(-window)

			type incRef struct {
				ID       string `json:"id"`
				Title    string `json:"title"`
				Severity string `json:"severity,omitempty"`
			}
			var opened, resolved []incRef
			incidents, err := novelLoad(db, novelResolveType(db, "incidents"))
			if err != nil {
				return err
			}
			for _, r := range incidents {
				ref := incRef{ID: r.ID, Title: incidentTitle(r), Severity: incidentSeverity(r)}
				if start, ok := incidentStart(r); ok && start.After(cutoff) {
					opened = append(opened, ref)
				}
				if res, ok := incidentResolved(r); ok && res.After(cutoff) {
					resolved = append(resolved, ref)
				}
			}
			if opened == nil {
				opened = []incRef{}
			}
			if resolved == nil {
				resolved = []incRef{}
			}

			// Severity-change timeline events (tolerate the events table being absent).
			severityChanges := 0
			timelineEvents := 0
			for _, ev := range novelLoadChildTableAll(db, "incidents_events", "incidents_id") {
				created, ok := recTime(ev.Attrs, "created_at", "occurred_at")
				if !ok || !created.After(cutoff) {
					continue
				}
				timelineEvents++
				text := strings.ToLower(recStr(ev.Attrs, "event", "kind", "summary", "description"))
				if strings.Contains(text, "severity") {
					severityChanges++
				}
			}

			// New action items: top-level plus per-incident rows, deduped by summary.
			newActionItems := 0
			seenAI := map[string]bool{}
			countAI := func(r record, incidentID string) {
				created, ok := recTime(r.Attrs, "created_at")
				if !ok || !created.After(cutoff) {
					return
				}
				summary := strings.TrimSpace(recStr(r.Attrs, "summary", "description", "title"))
				if summary == "" {
					return
				}
				// Scope the dedup key to the incident so distinct incidents
				// sharing a generic summary ("Update runbook") both count —
				// same keying as action-items-overdue.
				key := incidentID + "\x00" + summary
				if seenAI[key] {
					return
				}
				seenAI[key] = true
				newActionItems++
			}
			if topLevel, err := novelLoad(db, novelResolveType(db, "action-items", "action_items")); err == nil {
				for _, r := range topLevel {
					// Same fallback as action-items-overdue: when the item has
					// no parseable incident relationship, resolve it via
					// recRefersTo so the key matches its child-row twin and
					// the item is not double-counted.
					incID := relID(r, "incident")
					if incID == "" {
						for _, inc := range incidents {
							if recRefersTo(r, inc.ID) {
								incID = inc.ID
								break
							}
						}
					}
					countAI(r, incID)
				}
			}
			for _, cr := range novelLoadChildTableAll(db, "incidents_action_items", "incidents_id") {
				countAI(cr.record, cr.FK)
			}

			// Alerts fired in the window.
			alertsFired := 0
			if alerts, err := novelLoad(db, novelResolveType(db, "alerts")); err == nil {
				for _, a := range alerts {
					if created, ok := recTime(a.Attrs, "created_at", "started_at", "triggered_at"); ok && created.After(cutoff) {
						alertsFired++
					}
				}
			}

			note := ""
			if len(incidents) == 0 {
				note = "no incidents synced; run 'rootly-cli sync' first"
			}
			out := struct {
				Since           string   `json:"since"`
				WindowStart     string   `json:"window_start"`
				IncidentsOpened []incRef `json:"incidents_opened"`
				IncidentsClosed []incRef `json:"incidents_resolved"`
				TimelineEvents  int      `json:"timeline_events"`
				SeverityChanges int      `json:"severity_changes"`
				NewActionItems  int      `json:"new_action_items"`
				AlertsFired     int      `json:"alerts_fired"`
				Note            string   `json:"note,omitempty"`
			}{
				Since: since, WindowStart: cutoff.Format(time.RFC3339),
				IncidentsOpened: opened, IncidentsClosed: resolved,
				TimelineEvents: timelineEvents, SeverityChanges: severityChanges,
				NewActionItems: newActionItems, AlertsFired: alertsFired, Note: note,
			}

			return novelEmit(cmd, flags, out, func() {
				w := cmd.OutOrStdout()
				fmt.Fprintf(w, "digest: what moved in the last %s (since %s)\n", since, out.WindowStart)
				fmt.Fprintf(w, "  incidents opened:   %d\n", len(opened))
				for _, r := range opened {
					fmt.Fprintf(w, "    - [%s] %s\n", firstNonEmpty(r.Severity, "?"), r.Title)
				}
				fmt.Fprintf(w, "  incidents resolved: %d\n", len(resolved))
				for _, r := range resolved {
					fmt.Fprintf(w, "    - [%s] %s\n", firstNonEmpty(r.Severity, "?"), r.Title)
				}
				fmt.Fprintf(w, "  timeline events: %d (severity changes: %d)\n", timelineEvents, severityChanges)
				fmt.Fprintf(w, "  new action items: %d\n", newActionItems)
				fmt.Fprintf(w, "  alerts fired: %d\n", alertsFired)
				if note != "" {
					fmt.Fprintf(w, "  note: %s\n", note)
				}
			})
		},
	}
	cmd.Flags().StringVar(&since, "since", "24h", "Lookback window, e.g. 24h, 7d, 90m")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/rootly-cli/data.db)")
	return cmd
}
