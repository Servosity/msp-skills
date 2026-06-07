// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature. Fleet-wide new/rogue-device detection: devices
// first-seen anywhere in the fleet within a time window — a security signal the
// API has no single endpoint for.

package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

// fleetNewQuery selects devices whose first_seen_on is at or after the cutoff,
// newest first, labeled with site.
const fleetNewQuery = `
SELECT
  COALESCE(NULLIF(a.display_name, ''), d.agent_id)        AS site,
  d.agent_id                                              AS agent_id,
  json_extract(d.data, '$.display_name')                 AS display_name,
  json_extract(d.data, '$.type.label')                   AS type,
  json_extract(d.data, '$.user_data.vendor')             AS vendor,
  json_extract(d.data, '$.status')                       AS status,
  json_extract(d.data, '$.first_seen_on')                AS first_seen_on
FROM "device" d
LEFT JOIN "agent" a ON a.id = d.agent_id
WHERE json_extract(d.data, '$.first_seen_on') IS NOT NULL
  AND json_extract(d.data, '$.first_seen_on') >= ?
ORDER BY first_seen_on DESC`

// pp:data-source local
func newNovelFleetNewCmd(flags *rootFlags) *cobra.Command {
	var flagSince string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "new",
		Short: "Devices first-seen anywhere in the fleet within a time window (rogue-device signal)",
		Long: "Surface devices whose first_seen_on falls within --since across all synced sites — " +
			"useful for overnight rogue-device security sweeps. Reads the local store; run " +
			"'domotz-cli sync --full' first.",
		Example:     "  domotz-cli fleet new --since 24h --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			cutoff, err := parseSinceCutoff(flagSince, time.Now().UTC())
			if err != nil {
				return err
			}
			// Domotz first_seen_on is an ISO-8601 / RFC3339 string; lexical
			// comparison is correct for that format.
			cutoffStr := cutoff.Format(time.RFC3339)

			db, err := novelOpenStoreChecked(cmd, flags, dbPath, "device")
			if err != nil {
				return err
			}
			defer db.Close()

			rows, err := queryFleetRows(cmd.Context(), db, fleetNewQuery, cutoffStr)
			if err != nil {
				return err
			}
			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				if len(rows) == 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "No devices first-seen since %s.\n", cutoffStr)
					return nil
				}
				return printAutoTable(cmd.OutOrStdout(), rows)
			}
			return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "24h", "Time window for first-seen devices (forms like 24h, 90m, or 7d)")
	addFleetDBFlag(cmd, &dbPath)
	return cmd
}
