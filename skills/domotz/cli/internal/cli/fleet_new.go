// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature. Fleet-wide new/rogue-device detection: devices
// first-seen anywhere in the fleet within a time window — a security signal the
// API has no single endpoint for.

package cli

import (
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"
)

// fleetNewQuery selects every device with a first_seen_on, labeled with site;
// newFleetDevices keeps those first seen at or after the cutoff, newest first.
var fleetNewQuery = `
SELECT
  COALESCE(NULLIF(a.display_name, ''), d.agent_id)        AS site,
  d.agent_id                                              AS agent_id,
  ` + deviceField("d", "display_name") + `  AS display_name,
  ` + deviceField("d", "type") + `          AS type,
  ` + deviceField("d", "vendor") + `        AS vendor,
  ` + deviceField("d", "status") + `        AS status,
  ` + deviceField("d", "first_seen_on") + ` AS first_seen_on
FROM ` + fleetDeviceSource + ` d
LEFT JOIN "agent" a ON a.id = d.agent_id
WHERE ` + deviceField("d", "first_seen_on") + ` IS NOT NULL
ORDER BY site, display_name`

// newFleetDevices keeps the rows first seen at or after cutoff, compared as
// instants by the one shared parser (parseDomotzTime; Codex r1, domotz 4.34.2,
// class time-compared-as-text: the SQL compared the strings, so a device
// first seen 3h ago at +05:00 passed a 1h window and one first seen 30m ago at
// -05:00 did not). A first_seen_on that does not parse cannot be shown to be
// older than the window, so the device is kept (a rogue-device sweep fails
// open toward reporting), sorted last; the rest sort newest first.
func newFleetDevices(rows []map[string]any, cutoff time.Time) []map[string]any {
	type seen struct {
		row map[string]any
		at  time.Time
		ok  bool
	}
	kept := make([]seen, 0, len(rows))
	for _, r := range rows {
		at, ok := parseDomotzTime(asString(r["first_seen_on"]))
		if ok && at.Before(cutoff) {
			continue
		}
		kept = append(kept, seen{r, at, ok})
	}
	sort.SliceStable(kept, func(i, j int) bool {
		if kept[i].ok != kept[j].ok {
			return kept[i].ok
		}
		return kept[i].at.After(kept[j].at)
	})
	out := make([]map[string]any, 0, len(kept))
	for _, k := range kept {
		out = append(out, k.row)
	}
	return out
}

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
			cutoffStr := cutoff.Format(time.RFC3339)

			db, err := novelOpenStoreChecked(cmd, flags, dbPath, "agent_device", "agent", "agent_device")
			if err != nil {
				return err
			}
			defer db.Close()

			rows, err := queryFleetRows(cmd.Context(), db, fleetNewQuery)
			if err != nil {
				return err
			}
			rows = newFleetDevices(rows, cutoff)
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
