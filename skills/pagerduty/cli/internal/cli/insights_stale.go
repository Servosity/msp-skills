// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// insights stale: open incidents with no log activity past a threshold — the
// ones quietly rotting. Joins the synced incidents to their log entries and
// computes a last-activity age per open incident; `pulse` ranks the open queue
// by SLA risk (age since trigger), this ranks by inactivity (age since the
// last human or system touch), which is the shift-handoff "did we forget
// something" sweep. No API endpoint reports "no movement since".
package cli

import (
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/spf13/cobra"
)

type pdStaleIncident struct {
	IncidentID   string `json:"incident_id"`
	Title        string `json:"title"`
	Status       string `json:"status"`
	Urgency      string `json:"urgency,omitempty"`
	Service      string `json:"service,omitempty"`
	AssignedTo   string `json:"assigned_to,omitempty"`
	LastActivity string `json:"last_activity"`
	IdleSeconds  int64  `json:"idle_seconds"`
	Idle         string `json:"idle"`
}

type pdStaleResult struct {
	ThresholdHours float64           `json:"threshold_hours"`
	OpenIncidents  int               `json:"open_incidents"`
	Stale          []pdStaleIncident `json:"stale"`
	Note           string            `json:"note,omitempty"`
}

// pp:data-source local
func newNovelInsightsStaleCmd(flags *rootFlags) *cobra.Command {
	var flagHours float64
	var flagLimit int

	cmd := &cobra.Command{
		Use:   "stale",
		Short: "Open incidents with no log activity past a threshold — the ones quietly rotting",
		Long: `Sweeps the open (triggered or acknowledged) incidents whose latest synced log
entry is older than --hours, sorted longest-idle first. The last-activity time
is the newest log entry for the incident, falling back to the incident's
creation time when no log entries are synced.

Use this command to find open incidents with no recent activity. Do NOT use it
for the full open-queue SLA-risk picture; use 'pulse' instead.

Run ` + "`sync --resources incidents,log-entries`" + ` first; exits 0 with an empty
result when nothing is synced.`,
		Example:     "  pagerduty-cli insights stale\n  pagerduty-cli insights stale --hours 4 --agent\n  pagerduty-cli insights stale --hours 48 --limit 10",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			pdHintSync(cmd, flags, "incidents")
			if flagHours <= 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--hours must be a positive number of hours, got %v", flagHours))
			}
			incidents, err := pdLoadResource(cmd.Context(), "incidents")
			if err != nil {
				return fmt.Errorf("reading incidents from local store: %w", err)
			}
			logs, err := pdLoadResource(cmd.Context(), "log_entries")
			if err != nil {
				return fmt.Errorf("reading log_entries from local store: %w", err)
			}
			res := buildStaleIncidents(incidents, logs, time.Duration(flagHours*float64(time.Hour)), time.Now())
			if flagLimit > 0 && len(res.Stale) > flagLimit {
				res.Stale = res.Stale[:flagLimit]
			}
			return pdEmit(cmd, flags, res, func(w io.Writer) {
				if res.OpenIncidents == 0 {
					fmt.Fprintln(w, "No open incidents in the local store (run `pagerduty-cli sync --resources incidents,log-entries` first).")
					return
				}
				if len(res.Stale) == 0 {
					fmt.Fprintf(w, "All %d open incidents have activity within the last %.0fh.\n", res.OpenIncidents, res.ThresholdHours)
					return
				}
				fmt.Fprintf(w, "%d of %d open incidents idle longer than %.0fh\n\n", len(res.Stale), res.OpenIncidents, res.ThresholdHours)
				for _, s := range res.Stale {
					fmt.Fprintf(w, "  %s  idle %s  [%s/%s]  %s — %s (assigned: %s)\n", s.IncidentID, s.Idle, s.Status, dash(s.Urgency), dash(s.Service), s.Title, dash(s.AssignedTo))
				}
			})
		},
	}
	cmd.Flags().Float64Var(&flagHours, "hours", 24, "Idle threshold in hours")
	cmd.Flags().IntVar(&flagLimit, "limit", 0, "Maximum stale incidents to return (0 = all)")
	return cmd
}

// buildStaleIncidents is the pure split-out for tests.
func buildStaleIncidents(incidents, logs []map[string]any, threshold time.Duration, now time.Time) pdStaleResult {
	res := pdStaleResult{
		ThresholdHours: pdRound2(threshold.Hours()),
		Stale:          []pdStaleIncident{},
	}

	// Newest log entry per incident.
	lastTouch := map[string]time.Time{}
	for _, le := range logs {
		iid := pdString(pdMap(le, "incident"), "id")
		if iid == "" {
			continue
		}
		t, ok := pdParseTime(pdString(le, "created_at"))
		if !ok {
			continue
		}
		if t.After(lastTouch[iid]) {
			lastTouch[iid] = t
		}
	}

	for _, in := range incidents {
		status := pdString(in, "status")
		if status != "triggered" && status != "acknowledged" {
			continue
		}
		res.OpenIncidents++
		id := pdString(in, "id")
		last, hasTouch := lastTouch[id]
		if !hasTouch {
			created, ok := pdParseTime(pdString(in, "created_at"))
			if !ok {
				continue
			}
			last = created
		}
		idle := now.Sub(last)
		if idle <= threshold {
			continue
		}
		row := pdStaleIncident{
			IncidentID:   id,
			Title:        pdString(in, "title"),
			Status:       status,
			Urgency:      pdString(in, "urgency"),
			Service:      pdRefLabel(pdMap(in, "service")),
			LastActivity: last.UTC().Format(time.RFC3339),
			IdleSeconds:  int64(idle.Seconds()),
			Idle:         pdHumanDur(idle),
		}
		if assignments := pdSlice(in, "assignments"); len(assignments) > 0 {
			row.AssignedTo = pdRefLabel(pdMap(assignments[0], "assignee"))
		}
		res.Stale = append(res.Stale, row)
	}

	sort.SliceStable(res.Stale, func(i, j int) bool {
		return res.Stale[i].IdleSeconds > res.Stale[j].IdleSeconds
	})
	if res.OpenIncidents > 0 && len(res.Stale) == 0 {
		res.Note = fmt.Sprintf("all open incidents touched within %.0fh; lower --hours to tighten the sweep", res.ThresholdHours)
	}
	return res
}
