// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: fleet status shows one row per core with health and sensor counts.
// pp:data-source live

package cli

import (
	"encoding/json"
	"strings"

	"github.com/spf13/cobra"

	"prtg-pp-cli/internal/fleet"
)

func newNovelFleetStatusCmd(flags *rootFlags) *cobra.Command {
	var scope fleetScope

	cmd := &cobra.Command{
		Use:   "status",
		Short: "One row per client core with reachability and sensor counts by state, worst first",
		Long: `One row per client core with reachability and sensor counts by state, worst first.

Use this command for one row per core with reachability and sensor counts by state. Do NOT use it for individual sensors; use 'fleet alarms' instead. Do NOT use it for version, license, or probe hygiene; use 'fleet drift' instead.

Reads GET /health (ok, or the PRTG error code such as SERVICE_UNAVAILABLE or
LICENSE_INACTIVE) and GET /sensor-status-summary on every core. Cores that
cannot be read sort first; the rest sort by down count, then warning count.`,
		Example: strings.Trim(`
  prtg-cli fleet status --agent
  prtg-cli fleet status --tag gold --json`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "live",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet status")
			}
			cores, done, err := scope.resolveOrEmpty(cmd, flags)
			if err != nil || done {
				return err
			}
			reqs := []fleet.Request{{Path: "/health"}, {Path: "/sensor-status-summary"}}
			rows, _ := collect(cmd, &scope, flags, cores, reqs,
				func(c fleet.Core, bodies []json.RawMessage) ([]fleet.StatusRow, error) {
					return []fleet.StatusRow{fleet.StatusRowFor(c, bodies[0], bodies[1])}, nil
				},
				func(c fleet.Core, err error) fleet.StatusRow {
					return fleet.StatusRow{Core: c.Name, Server: c.Server, Health: "unreachable", Error: err.Error()}
				})
			fleet.SortStatus(rows)
			if err := render(cmd, flags, rows, []string{"core", "health", "down", "down_acknowledged", "warning", "unusual", "unknown", "paused", "probe_disconnected", "up", "total", "error"}, "No cores in scope."); err != nil {
				return err
			}
			// /health is public, so a reachable core with a rejected key still
			// answers it; a core counts as failed when its counts were unreadable.
			unread := 0
			for _, r := range rows {
				if r.Error != "" {
					unread++
				}
			}
			return allFailed(unread, len(cores))
		},
	}
	addFleetScopeFlags(cmd, &scope)
	return cmd
}
