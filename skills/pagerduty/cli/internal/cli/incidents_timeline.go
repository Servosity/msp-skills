// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// incidents timeline <id>: reconstructs one incident's full chronology —
// trigger, every acknowledge, note, reassignment, escalation, and resolve —
// with elapsed deltas between events, ordered from the synced log entries. The
// API returns log entries as unordered paginated raw data; this assembles the
// ordered, agent-shaped story. Registered as a subcommand of the generated
// `incidents` parent.
package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

type pdTimelineEvent struct {
	At             string `json:"at"`
	ElapsedSeconds int64  `json:"elapsed_seconds"`
	Elapsed        string `json:"elapsed"`
	Type           string `json:"type"`
	By             string `json:"by,omitempty"`
	Detail         string `json:"detail,omitempty"`
}

type pdTimelineResult struct {
	IncidentID string            `json:"incident_id"`
	Events     []pdTimelineEvent `json:"events"`
}

// pp:data-source local
func newNovelIncidentsTimelineCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "timeline [incident-id]",
		Short:       "Reconstruct one incident's full chronology with elapsed deltas from synced log entries",
		Long:        "Orders every synced log entry for an incident (trigger, acknowledge, note, reassign, escalate, resolve) into a single chronology with the elapsed time from the first event. Run `sync` first; exits 0 with an empty timeline when the incident has no synced log entries.",
		Example:     "  pagerduty-cli incidents timeline PXXXXXX\n  pagerduty-cli incidents timeline PXXXXXX --agent",
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
			pdHintSync(cmd, flags, "log-entries")
			id := args[0]
			logs, err := pdLoadResource(cmd.Context(), "log_entries")
			if err != nil {
				return fmt.Errorf("reading log_entries from local store: %w", err)
			}
			res := buildTimeline(logs, id)
			return pdEmit(cmd, flags, res, func(w io.Writer) {
				if len(res.Events) == 0 {
					fmt.Fprintf(w, "No log entries for incident %q in the local store (run `pagerduty-cli sync` first).\n", id)
					return
				}
				fmt.Fprintf(w, "Timeline for incident %s (%d events)\n\n", id, len(res.Events))
				tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
				fmt.Fprintln(tw, "ELAPSED\tWHEN\tEVENT\tBY\tDETAIL")
				for _, e := range res.Events {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", e.Elapsed, e.At, e.Type, dash(e.By), dash(e.Detail))
				}
				_ = tw.Flush()
			})
		},
	}
	return cmd
}

// buildTimeline is the pure split-out for tests.
func buildTimeline(logs []map[string]any, incidentID string) pdTimelineResult {
	res := pdTimelineResult{IncidentID: incidentID, Events: []pdTimelineEvent{}}

	type ev struct {
		at  time.Time
		typ string
		by  string
		det string
	}
	var events []ev
	for _, le := range logs {
		if pdString(pdMap(le, "incident"), "id") != incidentID {
			continue
		}
		t, ok := pdParseTime(pdString(le, "created_at"))
		if !ok {
			continue
		}
		e := ev{
			at:  t,
			typ: pdTimelineType(pdString(le, "type")),
			by:  pdRefLabel(pdMap(le, "agent")),
		}
		if ch := pdMap(le, "channel"); ch != nil {
			e.det = pdString(ch, "summary")
			if e.det == "" {
				e.det = pdString(ch, "type")
			}
		}
		events = append(events, e)
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].at.Before(events[j].at) })

	if len(events) == 0 {
		return res
	}
	base := events[0].at
	for _, e := range events {
		elapsed := e.at.Sub(base)
		res.Events = append(res.Events, pdTimelineEvent{
			At:             e.at.UTC().Format(time.RFC3339),
			ElapsedSeconds: int64(elapsed.Seconds()),
			Elapsed:        pdHumanDur(elapsed),
			Type:           e.typ,
			By:             e.by,
			Detail:         e.det,
		})
	}
	return res
}

// pdTimelineType renders a raw "*_log_entry" type into a short verb.
func pdTimelineType(raw string) string {
	t := strings.TrimSuffix(raw, "_log_entry")
	if t == "" {
		return raw
	}
	return strings.ReplaceAll(t, "_", " ")
}
