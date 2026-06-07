// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature. Fleet degraded-agent sweep: only the sites
// (Collectors) that are offline or degraded right now, with location,
// organization, and version — the filtered triage list the raw agent list
// endpoint doesn't give you.

package cli

import (
	"github.com/spf13/cobra"
)

// fleetAgentsQuery selects every agent whose status value is not ONLINE,
// labeled with the human site name and the operational context a NOC
// technician triages with (status age, version, location, organization).
const fleetAgentsQuery = `
SELECT
  COALESCE(NULLIF(a.display_name, ''), a.id)                  AS site,
  a.id                                                        AS agent_id,
  COALESCE(json_extract(a.data, '$.status.value'), 'UNKNOWN') AS status,
  json_extract(a.data, '$.status.last_change')                AS status_since,
  json_extract(a.data, '$.version')                           AS version,
  json_extract(a.data, '$.location.name')                     AS location,
  json_extract(a.data, '$.organization.name')                 AS organization
FROM "agent" a
WHERE COALESCE(json_extract(a.data, '$.status.value'), 'UNKNOWN') != 'ONLINE'
ORDER BY status, site`

// pp:data-source local
func newNovelFleetAgentsCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "agents",
		Short: "Only the sites (Collectors) that are offline or degraded right now",
		Long: "List every agent (Collector) whose status is not ONLINE, with location, organization, " +
			"version, and how long it has been in that state — the problem-sites-only triage list. " +
			"Use this command when you want only the SITES (agents) that are down or degraded. " +
			"Do NOT use this command for individual offline DEVICES; use 'fleet offline' instead. " +
			"For the full board including healthy sites, use 'fleet health'. Reads the local store; " +
			"run 'domotz-cli sync --full' first to populate it.",
		Example:     "  domotz-cli fleet agents --json --select site,status,status_since",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			db, err := novelOpenStoreChecked(cmd, flags, dbPath, "agent")
			if err != nil {
				return err
			}
			defer db.Close()

			rows, err := queryFleetRows(cmd.Context(), db, fleetAgentsQuery)
			if err != nil {
				return err
			}
			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printAutoTable(cmd.OutOrStdout(), rows)
			}
			return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
		},
	}
	addFleetDBFlag(cmd, &dbPath)
	return cmd
}
