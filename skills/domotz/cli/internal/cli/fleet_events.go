// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature. Fleet event timeline: per-agent activity-log
// history merged into one chronological fleet-wide feed. Events are exposed
// per agent only; the cross-site "what happened recently" view is a live
// fan-out merged and ordered locally.

package cli

import (
	"context"
	"domotz-pp-cli/internal/cliutil"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"time"

	"github.com/spf13/cobra"
)

// activityLogItem mirrors ActivityLog: one event on one agent.
type activityLogItem struct {
	DeviceID    json.RawMessage `json:"device_id"`
	Type        string          `json:"type"`
	Description string          `json:"description"`
	User        string          `json:"user"`
	Timestamp   string          `json:"timestamp"`
}

// fleetEventRow is one event labeled with its site, carrying the parsed
// timestamp in an unexported field so ordering never falls back to raw
// string comparison across mixed formats.
type fleetEventRow struct {
	Site        string `json:"site"`
	AgentID     string `json:"agent_id"`
	DeviceID    string `json:"device_id,omitempty"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	User        string `json:"user,omitempty"`
	Timestamp   string `json:"timestamp"`
	parsedAt    time.Time
}

// parseEventTime parses the activity-log timestamp shapes (RFC3339 with or
// without fractional seconds, and the bare ISO form), returning the zero time
// when unparseable so unknown rows sort last instead of corrupting the order.
func parseEventTime(s string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// pp:data-source live
func newNovelFleetEventsCmd(flags *rootFlags) *cobra.Command {
	var flagSince string
	var flagType string
	var limit int

	cmd := &cobra.Command{
		Use:   "events",
		Short: "Per-agent activity logs merged into one chronological fleet timeline",
		Long: "Fan out to every agent's activity log and merge the events into one fleet-wide " +
			"timeline, newest first. Use this command for a fleet-wide chronological event/activity " +
			"feed across all sites. Do NOT use this command for newly-appeared devices; use 'fleet new'. " +
			"Do NOT use it for currently-down devices; use 'fleet offline'. " +
			"Fetches live from the API (needs DOMOTZ_API_KEY).",
		Example:     "  domotz-cli fleet events --since 24h --json --select site,type,timestamp",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := novelRequireLive(flags); err != nil {
				return err
			}
			cutoff, err := parseSinceCutoff(flagSince, time.Now().UTC())
			if err != nil {
				_ = cmd.Usage()
				return usageErr(err)
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			agents, err := fleetAgentsLive(cmd.Context(), c)
			if err != nil {
				return classifyAPIError(err, flags)
			}

			params := map[string]string{"from": cutoff.Format(time.RFC3339)}
			if flagType != "" {
				params["type"] = flagType
			}
			results, errs := cliutil.FanoutRun(cmd.Context(), agents,
				func(a fleetAgentRef) string { return a.Name },
				func(ctx context.Context, a fleetAgentRef) ([]fleetEventRow, error) {
					data, err := c.Get(ctx, fmt.Sprintf("/agent/%s/activity-log", url.PathEscape(a.ID)), params)
					if err != nil {
						return nil, err
					}
					var items []activityLogItem
					if err := json.Unmarshal(data, &items); err != nil {
						return nil, fmt.Errorf("parsing activity log for agent %s: %w", a.ID, err)
					}
					out := make([]fleetEventRow, 0, len(items))
					for _, it := range items {
						out = append(out, fleetEventRow{
							Site:        a.site(),
							AgentID:     a.ID,
							DeviceID:    trimJSONString(it.DeviceID),
							Type:        it.Type,
							Description: it.Description,
							User:        it.User,
							Timestamp:   it.Timestamp,
							parsedAt:    parseEventTime(it.Timestamp),
						})
					}
					return out, nil
				},
				cliutil.WithConcurrency(6),
			)
			cliutil.FanoutReportErrors(cmd.ErrOrStderr(), errs)

			rows := make([]fleetEventRow, 0)
			for _, r := range results {
				rows = append(rows, r.Value...)
			}
			// Newest first on the parsed time; zero (unparseable) times last.
			sort.SliceStable(rows, func(i, j int) bool {
				if rows[i].parsedAt.IsZero() != rows[j].parsedAt.IsZero() {
					return !rows[i].parsedAt.IsZero()
				}
				return rows[i].parsedAt.After(rows[j].parsedAt)
			})
			if limit > 0 && len(rows) > limit {
				rows = rows[:limit]
			}
			return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "24h", "Time window for events (forms like 24h, 90m, or 7d)")
	cmd.Flags().StringVar(&flagType, "type", "", "Filter to one activity-log event type (passed to the API)")
	cmd.Flags().IntVar(&limit, "limit", 100, "Maximum events to return after merging (0 = no limit)")
	return cmd
}
