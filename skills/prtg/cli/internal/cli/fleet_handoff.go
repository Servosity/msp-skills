// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: fleet handoff lists what changed across cores in a time window.
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

func newNovelFleetHandoffCmd(flags *rootFlags) *cobra.Command {
	var since string
	var events []string
	var scope fleetScope

	cmd := &cobra.Command{
		Use:   "handoff",
		Short: "What changed across all cores since a point in time: new alarms, recoveries, acknowledgements, pauses",
		Long: `What changed across all cores since a point in time: new alarms, recoveries, and new acknowledgements or pauses with who and why.

Use this command for what changed across cores since a point in time, such as a shift handoff. Do NOT use this command for the current list of broken sensors; use 'fleet alarms' instead.

Reads every sensor on each core (GET /sensors, paged) and buckets it by its
timestamps inside --since: new_alarm (in alarm now and status_since inside the
window), recovered (up now, status_since, the switch to up, inside, and a recorded
last_down at or before it, whatever interrupted the way back: via=direct when the
down sample was the scan before the switch, via=after_gap when the sensor was
paused and resumed, acknowledged, or rescheduled in between), acknowledged or paused (status_info time stamp inside, with the
user and message; auto=true marks an acknowledgement a trigger made). Each
sensor reports its current state only, so a sensor that went down and
recovered twice in the window appears once.`,
		Example: strings.Trim(`
  prtg-cli fleet handoff --since 12h --agent
  prtg-cli fleet handoff --since 2d --event recovered,new_alarm --json`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "live",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet handoff")
			}
			if strings.TrimSpace(since) == "" {
				since = "12h"
			}
			win, err := parseWindow("since", since)
			if err != nil {
				return err
			}
			if win == 0 {
				return usageErr(fmt.Errorf("--since must be longer than zero, e.g. 12h"))
			}
			keep := map[string]bool{}
			for _, e := range events {
				e = strings.ToLower(strings.TrimSpace(e))
				switch e {
				case "new_alarm", "recovered", "acknowledged", "paused":
					keep[e] = true
				case "":
				default:
					return usageErr(fmt.Errorf("--event %q: use new_alarm, recovered, acknowledged or paused", e))
				}
			}
			cores, done, err := scope.resolveOrEmpty(cmd, flags)
			if err != nil || done {
				return err
			}
			now := time.Now().UTC()
			from := now.Add(-win)
			req := fleet.Request{Method: fleet.MethodGetAll, Path: "/sensors"}
			rows, failed := collect(cmd, &scope, flags, cores, []fleet.Request{req},
				func(c fleet.Core, bodies []json.RawMessage) ([]fleet.HandoffRow, error) {
					body, err := bodyOrErr(bodies[0])
					if err != nil {
						return nil, err
					}
					nodes, err := fleet.DecodeNodes(body)
					if err != nil {
						return nil, fmt.Errorf("unexpected /sensors body: %v", err)
					}
					all := fleet.Handoff(c.Name, nodes, from, now)
					if len(keep) == 0 {
						return all, nil
					}
					out := make([]fleet.HandoffRow, 0, len(all))
					for _, r := range all {
						if keep[r.Event] {
							out = append(out, r)
						}
					}
					return out, nil
				},
				func(c fleet.Core, err error) fleet.HandoffRow {
					return fleet.HandoffRow{Core: c.Name, Event: "error", Error: err.Error()}
				})
			fleet.SortHandoff(rows)
			if err := render(cmd, flags, rows, []string{"core", "event", "at", "name", "device", "status", "via", "by", "auto", "last_down", "message", "error"}, "Nothing changed on any core in scope during the window."); err != nil {
				return err
			}
			return allFailed(failed, len(cores))
		},
	}
	cmd.Flags().StringVar(&since, "since", "12h", "Window to look back over, e.g. 8h, 12h, 2d")
	cmd.Flags().StringSliceVar(&events, "event", nil, "Only these events: new_alarm, recovered, acknowledged, paused")
	addFleetScopeFlags(cmd, &scope)
	return cmd
}
