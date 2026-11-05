// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: fleet alarms merges every core's alarm list into one ranked table.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"prtg-pp-cli/internal/fleet"
)

func newNovelFleetAlarmsCmd(flags *rootFlags) *cobra.Command {
	var statuses []string
	var unacked bool
	var minAge, filter, rollup string
	var limit int
	var scope fleetScope

	cmd := &cobra.Command{
		Use:   "alarms",
		Short: "See every down, warning and unusual sensor across all of your clients' PRTG cores in one list",
		Long: `See every down, warning and unusual sensor across all of your clients' PRTG cores in one list.

Use this command to list sensors that are down, warning, or unusual right now across every registered core. Do NOT use this command for what changed or recovered during a time window; use 'fleet handoff' instead. Do NOT use it for alarms someone paused or acknowledged and left that way; use 'fleet silenced' instead. Do NOT use it for per-core counts; use 'fleet status' instead.

Reads each core's alarm list (GET /sensors/alarms, paged at 3000 rows) and
sorts worst first: down, then warning, unusual and acknowledged; higher
priority first; then the oldest status_since. --rollup device collapses the
sensors on one device into a single row. A core that cannot be read becomes a
row with an error instead of silently shrinking the list.`,
		Example: strings.Trim(`
  prtg-cli fleet alarms --status down --unacked --agent
  prtg-cli fleet alarms --status down,warning --min-age 15m --rollup device --json
  prtg-cli fleet alarms --cores acme,globex --filter 'basic.tags contains "firewall"' --agent`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "live",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet alarms")
			}
			set, err := fleet.ParseAlarmStatuses(statuses)
			if err != nil {
				return usageErr(err)
			}
			age, err := parseWindow("min-age", minAge)
			if err != nil {
				return err
			}
			switch strings.ToLower(strings.TrimSpace(rollup)) {
			case "", "none", "device":
			default:
				return usageErr(fmt.Errorf("--rollup %q: use device (or omit it)", rollup))
			}
			if limit < 0 {
				return usageErr(fmt.Errorf("--limit must be zero (no limit) or positive"))
			}
			cores, done, err := scope.resolveOrEmpty(cmd, flags)
			if err != nil || done {
				return err
			}
			now := time.Now().UTC()
			opts := fleet.AlarmOptions{Statuses: set, Unacked: unacked, MinAge: age, Now: now}
			rows, failed := collect(cmd, &scope, flags, cores, []fleet.Request{alarmListRequest(filter)},
				func(c fleet.Core, bodies []json.RawMessage) ([]fleet.AlarmRow, error) {
					body, err := bodyOrErr(bodies[0])
					if err != nil {
						return nil, err
					}
					nodes, err := fleet.DecodeNodes(body)
					if err != nil {
						return nil, fmt.Errorf("unexpected /sensors/alarms body: %v", err)
					}
					return fleet.AlarmRows(c.Name, nodes, opts), nil
				},
				func(c fleet.Core, err error) fleet.AlarmRow {
					return fleet.AlarmRow{Core: c.Name, Error: err.Error()}
				})
			if strings.EqualFold(strings.TrimSpace(rollup), "device") {
				rows = fleet.RollupByDevice(rows)
			}
			fleet.SortAlarms(rows)
			rows = limitAlarms(rows, limit)
			if err := render(cmd, flags, rows, []string{"core", "status", "priority", "age_minutes", "device", "name", "sensors", "status_since", "message", "acked_by", "ack_message", "group", "probe", "error"}, "No alarms on any core in scope."); err != nil {
				return err
			}
			return allFailed(failed, len(cores))
		},
	}
	cmd.Flags().StringSliceVar(&statuses, "status", nil, "Alarm states to include: down, warning, unusual, acknowledged (default all)")
	cmd.Flags().BoolVar(&unacked, "unacked", false, "Leave out alarms that are already acknowledged")
	cmd.Flags().StringVar(&minAge, "min-age", "", "Only alarms at least this old, e.g. 15m, 2h, 1d")
	cmd.Flags().StringVar(&filter, "filter", "", "PRTG filter expression sent to each core, e.g. 'basic.tags contains \"firewall\"'")
	cmd.Flags().StringVar(&rollup, "rollup", "", "Collapse sensors into one row per device: device")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum alarm rows to return after sorting; error rows are always kept (0 = all)")
	addFleetScopeFlags(cmd, &scope)
	return cmd
}

// limitAlarms keeps the first limit alarm rows and every error row, so a
// limit never hides that a core could not be read.
func limitAlarms(rows []fleet.AlarmRow, limit int) []fleet.AlarmRow {
	if limit <= 0 {
		return rows
	}
	out := make([]fleet.AlarmRow, 0, limit)
	kept := 0
	for _, r := range rows {
		if r.Error != "" {
			out = append(out, r)
			continue
		}
		if kept < limit {
			out = append(out, r)
			kept++
		}
	}
	return out
}
