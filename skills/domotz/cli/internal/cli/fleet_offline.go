// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature. Cross-site offline-device rollup: a single
// prioritized list of every unreachable device across all sites, which the
// agent-scoped Domotz API can only answer one agent at a time.

package cli

import (
	"github.com/spf13/cobra"
)

// fleetOfflineQuery selects every device whose status is OFFLINE or DOWN,
// labeled with its site (agent display name) and ordered VITAL-first.
const fleetOfflineQuery = `
SELECT
  COALESCE(NULLIF(a.display_name, ''), d.agent_id)        AS site,
  d.agent_id                                              AS agent_id,
  json_extract(d.data, '$.display_name')                 AS display_name,
  json_extract(d.data, '$.status')                       AS status,
  json_extract(d.data, '$.importance')                   AS importance,
  json_extract(d.data, '$.type.label')                   AS type,
  json_extract(d.data, '$.last_status_change')           AS last_status_change
FROM "device" d
LEFT JOIN "agent" a ON a.id = d.agent_id
WHERE json_extract(d.data, '$.status') IN ('OFFLINE', 'DOWN')
ORDER BY
  CASE json_extract(d.data, '$.importance') WHEN 'VITAL' THEN 0 ELSE 1 END,
  site,
  display_name`

// pp:data-source local
func newNovelFleetOfflineCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "offline",
		Short: "Every offline or unreachable device across all sites, prioritized",
		Long: "List every device whose status is OFFLINE or DOWN across all synced sites, " +
			"labeled with its site and ordered VITAL-first. Reads the local store; run " +
			"'domotz-cli sync --full' first to populate it.",
		Example:     "  domotz-cli fleet offline --json --select site,display_name,importance",
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

			rows, err := queryFleetRows(cmd.Context(), db, fleetOfflineQuery)
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
