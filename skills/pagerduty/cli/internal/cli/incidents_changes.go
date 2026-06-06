// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// incidents changes <id>: "what shipped right before this broke." Joins the
// synced change events to one incident by service and time proximity — every
// change event recorded on the same service (plus service-less account-wide
// events) inside the window before the incident triggered, ranked closest
// first. PagerDuty exposes change events but reserves incident correlation
// for its premium AIOps tier; this reconstructs the high-value part offline.
// Registered as a subcommand of the generated `incidents` parent.
package cli

import (
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/spf13/cobra"
)

type pdIncidentChange struct {
	At            string `json:"at"`
	BeforeTrigger string `json:"before_trigger"`
	Summary       string `json:"summary"`
	Source        string `json:"source,omitempty"`
	Service       string `json:"service,omitempty"`
	ServiceID     string `json:"service_id,omitempty"`
	AccountWide   bool   `json:"account_wide,omitempty"`
}

type pdIncidentChangesResult struct {
	IncidentID    string             `json:"incident_id"`
	Service       string             `json:"service,omitempty"`
	ServiceID     string             `json:"service_id,omitempty"`
	TriggeredAt   string             `json:"triggered_at,omitempty"`
	WindowMinutes int                `json:"window_minutes"`
	Changes       []pdIncidentChange `json:"changes"`
	Note          string             `json:"note,omitempty"`
}

// pp:data-source local
func newNovelIncidentsChangesCmd(flags *rootFlags) *cobra.Command {
	var flagWindow string

	cmd := &cobra.Command{
		Use:   "changes [incident-id]",
		Short: "Change events that shipped on the incident's service right before it triggered",
		Long: `Finds what changed right before an incident broke: every synced change event
recorded on the incident's service (plus account-wide change events that carry
no service reference) inside the --window before the trigger time, ranked
closest-to-trigger first.

Use this command to find what changed right before an incident on the same
service. Do NOT use it for the incident's own internal event sequence; use
'incidents timeline' instead.

Run ` + "`sync --resources incidents,change-events`" + ` first; exits 0 with an empty
result when nothing relevant is synced.`,
		Example:     "  pagerduty-cli incidents changes PT4KHLK\n  pagerduty-cli incidents changes PT4KHLK --window 240m\n  pagerduty-cli incidents changes PT4KHLK --window 12h --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			pdHintSync(cmd, flags, "change-events")
			id := args[0]
			window, ok := pdParseRelative(flagWindow)
			if !ok || window <= 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("invalid --window %q: use a relative duration like 120m, 4h, 1d", flagWindow))
			}
			incidents, err := pdLoadResource(cmd.Context(), "incidents")
			if err != nil {
				return fmt.Errorf("reading incidents from local store: %w", err)
			}
			changes, err := pdLoadResource(cmd.Context(), "change_events")
			if err != nil {
				return fmt.Errorf("reading change_events from local store: %w", err)
			}
			res := buildIncidentChanges(incidents, changes, id, window)
			return pdEmit(cmd, flags, res, func(w io.Writer) {
				if res.TriggeredAt == "" {
					fmt.Fprintf(w, "Incident %q not found in the local store (run `pagerduty-cli sync --resources incidents,change-events` first).\n", id)
					return
				}
				if len(res.Changes) == 0 {
					fmt.Fprintf(w, "No change events on %s in the %dm before incident %s triggered (%s).\n", dash(res.Service), res.WindowMinutes, id, res.TriggeredAt)
					return
				}
				fmt.Fprintf(w, "Change events in the %dm before incident %s (%s) triggered at %s\n\n", res.WindowMinutes, id, dash(res.Service), res.TriggeredAt)
				for _, c := range res.Changes {
					scope := c.Service
					if c.AccountWide {
						scope = "account-wide"
					}
					line := fmt.Sprintf("  -%s  %s  [%s]  %s", c.BeforeTrigger, c.At, dash(scope), c.Summary)
					if c.Source != "" {
						line += " (" + c.Source + ")"
					}
					fmt.Fprintln(w, line)
				}
			})
		},
	}
	cmd.Flags().StringVar(&flagWindow, "window", "120m", "Look-back window before the trigger: relative duration like 120m, 4h, 1d")
	return cmd
}

// buildIncidentChanges is the pure split-out for tests.
func buildIncidentChanges(incidents, changes []map[string]any, incidentID string, window time.Duration) pdIncidentChangesResult {
	res := pdIncidentChangesResult{
		IncidentID:    incidentID,
		WindowMinutes: int(window.Minutes()),
		Changes:       []pdIncidentChange{},
	}

	var incident map[string]any
	for _, in := range incidents {
		if pdString(in, "id") == incidentID {
			incident = in
			break
		}
	}
	if incident == nil {
		return res
	}
	svcRef := pdMap(incident, "service")
	res.ServiceID = pdString(svcRef, "id")
	res.Service = pdRefLabel(svcRef)
	triggered, ok := pdParseTime(pdString(incident, "created_at"))
	if !ok {
		return res
	}
	res.TriggeredAt = triggered.UTC().Format(time.RFC3339)
	since := triggered.Add(-window)

	for _, ce := range changes {
		at, ok := pdParseTime(pdString(ce, "timestamp"))
		if !ok {
			// Some payloads carry created_at instead of timestamp.
			at, ok = pdParseTime(pdString(ce, "created_at"))
			if !ok {
				continue
			}
		}
		if at.Before(since) || at.After(triggered) {
			continue
		}
		svcs := pdSlice(ce, "services")
		matched := false
		accountWide := len(svcs) == 0
		var matchedSvc map[string]any
		if accountWide {
			matched = true // account-wide change event: relevant to every service
		} else {
			for _, s := range svcs {
				if pdString(s, "id") == res.ServiceID {
					matched = true
					matchedSvc = s
					break
				}
			}
		}
		if !matched {
			continue
		}
		c := pdIncidentChange{
			At:            at.UTC().Format(time.RFC3339),
			BeforeTrigger: pdHumanDur(triggered.Sub(at)),
			Summary:       pdString(ce, "summary"),
			Source:        pdString(ce, "source"),
			AccountWide:   accountWide,
		}
		if matchedSvc != nil {
			c.Service = pdRefLabel(matchedSvc)
			c.ServiceID = pdString(matchedSvc, "id")
		}
		res.Changes = append(res.Changes, c)
	}

	// Closest to the trigger first. Lexicographic comparison is safe here only
	// because every At is normalized via UTC().Format(time.RFC3339) (uniform Z
	// suffix, second precision), so string order equals chronological order.
	sort.SliceStable(res.Changes, func(i, j int) bool {
		return res.Changes[i].At > res.Changes[j].At
	})
	if len(res.Changes) == 0 {
		res.Note = fmt.Sprintf("no change events within %dm before trigger; widen with --window to look further back", res.WindowMinutes)
	}
	return res
}
