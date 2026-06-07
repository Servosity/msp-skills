// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature. Stale-agent / sync-freshness check: Collectors
// whose local snapshot has gone quiet. The API has no sync cursor; only the
// local store knows when each agent's data was last refreshed, so staleness
// is a purely local question.

package cli

import (
	"sort"
	"time"

	"github.com/spf13/cobra"
)

// fleetStaleQuery reports, per agent, the freshest local snapshot timestamp
// across the agent row itself and every device row synced under it.
const fleetStaleQuery = `
SELECT
  COALESCE(NULLIF(a.display_name, ''), a.id) AS site,
  a.id                                       AS agent_id,
  COALESCE(MAX(d.synced_at), a.synced_at)    AS last_synced,
  COUNT(d.id)                                AS devices
FROM "agent" a
LEFT JOIN "device" d ON d.agent_id = a.id
GROUP BY a.id`

// fleetStaleRow is the JSON row for one quiet (or measured) Collector.
type fleetStaleRow struct {
	Site       string  `json:"site"`
	AgentID    string  `json:"agent_id"`
	LastSynced string  `json:"last_synced"`
	AgeHours   float64 `json:"age_hours"`
	Devices    int     `json:"devices"`
}

// parseStoreTimestamp parses the SQLite DATETIME formats the store writes
// (CURRENT_TIMESTAMP "2006-01-02 15:04:05" and RFC3339), returning the zero
// time when the value is empty or unparseable.
func parseStoreTimestamp(s string) time.Time {
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339, "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// pp:data-source local
func newNovelFleetStaleCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "stale",
		Short: "Collectors whose local snapshot has gone quiet (sync-freshness check)",
		Long: "Report each agent's freshest local snapshot age and list the Collectors whose data " +
			"is older than the --max-age threshold — a Collector-offline or sync-gap signal. " +
			"Use this command to find agents that have gone quiet (no fresh data). " +
			"Do NOT use this command for devices that are reachable but unhealthy; use 'fleet offline' " +
			"instead. Pass --max-age 0 to list every agent with its age instead of filtering. " +
			"Reads the local store; run 'domotz-cli sync --full' first to populate it.",
		Example:     "  domotz-cli fleet stale --max-age 24h --json",
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

			raw, err := queryFleetRows(cmd.Context(), db, fleetStaleQuery)
			if err != nil {
				return err
			}

			now := time.Now().UTC()
			rows := make([]fleetStaleRow, 0, len(raw))
			for _, r := range raw {
				lastSynced := asString(r["last_synced"])
				ts := parseStoreTimestamp(lastSynced)
				age := 0.0
				if !ts.IsZero() {
					age = now.Sub(ts).Hours()
				}
				if flags.maxAge > 0 && !ts.IsZero() && now.Sub(ts) < flags.maxAge {
					continue
				}
				rows = append(rows, fleetStaleRow{
					Site:       asString(r["site"]),
					AgentID:    asString(r["agent_id"]),
					LastSynced: lastSynced,
					AgeHours:   float64(int(age*10)) / 10,
					Devices:    asInt(r["devices"]),
				})
			}
			// Oldest snapshot first: parsed-time order, never raw string compare.
			sort.Slice(rows, func(i, j int) bool {
				return parseStoreTimestamp(rows[i].LastSynced).Before(parseStoreTimestamp(rows[j].LastSynced))
			})
			return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
		},
	}
	addFleetDBFlag(cmd, &dbPath)
	return cmd
}
