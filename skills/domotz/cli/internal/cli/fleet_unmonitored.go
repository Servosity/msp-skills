// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature. Monitoring-coverage audit: devices Domotz
// cannot fully monitor — authentication required/wrong or SNMP not
// authenticated — surfaced fleet-wide. The per-device fields exist in the
// API, but no endpoint reports coverage across the whole fleet.

package cli

import (
	"github.com/spf13/cobra"
)

// fleetUnmonitoredQuery selects devices whose monitoring is incomplete:
// authentication is REQUIRED/PENDING/WRONG_CREDENTIALS, or the SNMP service
// was found but Domotz could not authenticate to it. NO_AUTHENTICATION and
// NOT_FOUND mean the device needs no extended discovery — those are healthy.
const fleetUnmonitoredQuery = `
SELECT
  COALESCE(NULLIF(a.display_name, ''), d.agent_id) AS site,
  d.agent_id                                       AS agent_id,
  json_extract(d.data, '$.display_name')           AS display_name,
  json_extract(d.data, '$.authentication_status')  AS authentication_status,
  json_extract(d.data, '$.snmp_status')            AS snmp_status,
  json_extract(d.data, '$.importance')             AS importance,
  json_extract(d.data, '$.type.label')             AS type
FROM "device" d
LEFT JOIN "agent" a ON a.id = d.agent_id
WHERE json_extract(d.data, '$.authentication_status') IN ('REQUIRED', 'PENDING', 'WRONG_CREDENTIALS')
   OR json_extract(d.data, '$.snmp_status') = 'NOT_AUTHENTICATED'
ORDER BY
  CASE json_extract(d.data, '$.importance') WHEN 'VITAL' THEN 0 ELSE 1 END,
  site,
  display_name`

// pp:data-source local
func newNovelFleetUnmonitoredCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "unmonitored",
		Short: "Devices Domotz can't fully monitor (auth/SNMP coverage gaps) fleet-wide",
		Long: "Audit monitoring coverage across the fleet: devices whose authentication status is " +
			"REQUIRED, PENDING, or WRONG_CREDENTIALS, or whose SNMP service exists but is not " +
			"authenticated. Use this command to find devices Domotz is monitoring incompletely " +
			"(failed/missing auth or SNMP) across the fleet — a coverage-gap audit. " +
			"Do NOT use this command for devices that are simply offline; use 'fleet offline' instead. " +
			"Reads the local store; run 'domotz-cli sync --full' first to populate it.",
		Example:     "  domotz-cli fleet unmonitored --json --select site,display_name,authentication_status",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			db, err := novelOpenStoreChecked(cmd, flags, dbPath, "device")
			if err != nil {
				return err
			}
			defer db.Close()

			rows, err := queryFleetRows(cmd.Context(), db, fleetUnmonitoredQuery)
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
