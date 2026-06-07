// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature. Fleet health rollup: one status board across
// every Domotz Collector. No API call returns status across all agents — this
// joins the synced agent and device tables in the local store.

package cli

import (
	"sort"

	"github.com/spf13/cobra"
)

// fleetHealthSite is one site's (agent's) health summary.
type fleetHealthSite struct {
	Site           string `json:"site"`
	AgentID        string `json:"agent_id"`
	AgentStatus    string `json:"agent_status"`
	Devices        int    `json:"devices"`
	DevicesOffline int    `json:"devices_offline"`
}

// fleetHealthReport is the full rollup payload.
type fleetHealthReport struct {
	AgentsTotal             int               `json:"agents_total"`
	AgentsOnline            int               `json:"agents_online"`
	AgentsOffline           int               `json:"agents_offline"`
	DevicesTotal            int               `json:"devices_total"`
	DevicesOffline          int               `json:"devices_offline"`
	SitesWithOfflineDevices int               `json:"sites_with_offline_devices"`
	Sites                   []fleetHealthSite `json:"sites"`
}

const fleetHealthAgentsQuery = `
SELECT
  id                                                       AS agent_id,
  COALESCE(NULLIF(display_name, ''), id)                   AS site,
  COALESCE(json_extract(data, '$.status.value'), '')       AS status
FROM "agent"`

const fleetHealthDevicesQuery = `
SELECT
  agent_id,
  COUNT(*)                                                              AS total,
  SUM(CASE WHEN json_extract(data, '$.status') IN ('OFFLINE', 'DOWN') THEN 1 ELSE 0 END) AS offline
FROM "device"
GROUP BY agent_id`

// pp:data-source local
func newNovelFleetHealthCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "health",
		Short: "One status board across every site: agent status and down-device counts",
		Long: "Roll up health across every Domotz Collector — online/offline agents and the count " +
			"of down devices per site — in one local query. Reads the local store; run " +
			"'domotz-cli sync --full' first.",
		Example:     "  domotz-cli fleet health --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			db, err := novelOpenStoreChecked(cmd, flags, dbPath, "")
			if err != nil {
				return err
			}
			defer db.Close()

			agentRows, err := queryFleetRows(cmd.Context(), db, fleetHealthAgentsQuery)
			if err != nil {
				return err
			}
			deviceRows, err := queryFleetRows(cmd.Context(), db, fleetHealthDevicesQuery)
			if err != nil {
				return err
			}

			devByAgent := make(map[string][2]int, len(deviceRows)) // [total, offline]
			for _, r := range deviceRows {
				id := asString(r["agent_id"])
				devByAgent[id] = [2]int{asInt(r["total"]), asInt(r["offline"])}
			}

			report := fleetHealthReport{Sites: make([]fleetHealthSite, 0, len(agentRows))}
			for _, r := range agentRows {
				id := asString(r["agent_id"])
				status := asString(r["status"])
				counts := devByAgent[id]
				site := fleetHealthSite{
					Site:           asString(r["site"]),
					AgentID:        id,
					AgentStatus:    status,
					Devices:        counts[0],
					DevicesOffline: counts[1],
				}
				report.AgentsTotal++
				if status == "ONLINE" {
					report.AgentsOnline++
				} else {
					report.AgentsOffline++
				}
				report.DevicesTotal += counts[0]
				report.DevicesOffline += counts[1]
				if counts[1] > 0 {
					report.SitesWithOfflineDevices++
				}
				report.Sites = append(report.Sites, site)
			}
			sort.Slice(report.Sites, func(i, j int) bool {
				if report.Sites[i].DevicesOffline != report.Sites[j].DevicesOffline {
					return report.Sites[i].DevicesOffline > report.Sites[j].DevicesOffline
				}
				return report.Sites[i].Site < report.Sites[j].Site
			})

			return printJSONFiltered(cmd.OutOrStdout(), report, flags)
		},
	}
	addFleetDBFlag(cmd, &dbPath)
	return cmd
}
