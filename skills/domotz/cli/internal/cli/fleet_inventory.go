// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature. Unified fleet inventory: one asset table across
// every agent and device, assembled from the local store. The API only returns
// inventory per-device, one agent at a time.

package cli

import (
	"github.com/spf13/cobra"
)

// fleetInventoryQuery assembles a single asset row per device across the fleet.
// IP/MAC are not present in the device-list payload (use 'agent device get' for
// those); vendor/model/type/os/serial come from the device's user_data, type,
// os, and details objects.
const fleetInventoryQuery = `
SELECT
  COALESCE(NULLIF(a.display_name, ''), d.agent_id)        AS site,
  d.agent_id                                              AS agent_id,
  d.id                                                    AS device_id,
  json_extract(d.data, '$.display_name')                 AS display_name,
  json_extract(d.data, '$.type.label')                   AS type,
  json_extract(d.data, '$.user_data.vendor')             AS vendor,
  json_extract(d.data, '$.user_data.model')              AS model,
  json_extract(d.data, '$.os.name')                      AS os,
  json_extract(d.data, '$.details.serial')               AS serial,
  json_extract(d.data, '$.importance')                   AS importance,
  json_extract(d.data, '$.status')                       AS status,
  json_extract(d.data, '$.first_seen_on')                AS first_seen_on
FROM "device" d
LEFT JOIN "agent" a ON a.id = d.agent_id
ORDER BY site, type, display_name`

// pp:data-source local
func newNovelFleetInventoryCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "inventory",
		Short: "One asset table across every device and site (vendor, model, type, OS, serial)",
		Long: "Export a single asset inventory across the whole fleet for client reports or a " +
			"fleet-wide CMDB. Reads the local store; run 'domotz-cli sync --full' first. " +
			"Pairs well with --csv.",
		Example:     "  domotz-cli fleet inventory --csv > assets.csv",
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

			rows, err := queryFleetRows(cmd.Context(), db, fleetInventoryQuery)
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
