// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type deviceHealth struct {
	ComputerID       string `json:"computerId"`
	ComputerName     string `json:"computerName"`
	OrganizationID   string `json:"organizationId"`
	OrganizationName string `json:"organizationName"`
	LastCheckin      string `json:"lastCheckin"`
	HealthClass      string `json:"healthClass"`
}

type orgRollup struct {
	OrganizationID   string `json:"organizationId"`
	OrganizationName string `json:"organizationName"`
	Total            int    `json:"total"`
	Online           int    `json:"online"`
	Stale            int    `json:"stale"`
	Offline          int    `json:"offline"`
	Isolated         int    `json:"isolated"`
}

func newNovelDevicesHealthCmd(flags *rootFlags) *cobra.Command {
	var flagAllTenants bool
	var flagDetail bool
	var flagStaleAfter time.Duration
	var dbPath string

	cmd := &cobra.Command{
		Use:         "health",
		Short:       "Joins computers, online-devices, and last-checkin data to classify every endpoint healthy / offline / stale / isolated",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Health joins the locally synced computers and online-devices tables and
classifies every endpoint as online, stale, offline, or isolated, rolled up per
tenant — the cross-tenant device-health view the Portal API has no single
endpoint for. Use --detail to list individual devices.

Sync first: threatlocker-cli sync --resources computers,online_devices`,
		Example: strings.Trim(`
  threatlocker-cli devices health --all-tenants --agent
  threatlocker-cli devices health --all-tenants --detail --select computerName,healthClass,lastCheckin
`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			out := cmd.OutOrStdout()
			if dbPath == "" {
				dbPath = defaultDBPath("threatlocker-cli")
			}
			db, err := tlOpenStore(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w\nRun 'threatlocker-cli sync --resources computers,online_devices' first.", err)
			}
			defer db.Close()

			query := `SELECT c.computer_id, c.computer_name, c.organization_id, c.organization_name,
				c.last_checkin, c.is_isolated, c.is_online, od.computer_id
				FROM computers c
				LEFT JOIN online_devices od ON c.computer_id = od.computer_id`
			var sqlArgs []any
			if !flagAllTenants && flags.orgID != "" {
				query += ` WHERE c.organization_id = ?`
				sqlArgs = append(sqlArgs, flags.orgID)
			}
			rows, err := db.DB().QueryContext(cmd.Context(), query, sqlArgs...)
			if err != nil {
				return fmt.Errorf("querying device health: %w", err)
			}
			defer rows.Close()

			now := time.Now()
			var devices []deviceHealth
			rollups := map[string]*orgRollup{}
			for rows.Next() {
				var compID, compName, orgID, orgName, lastCheckin, odMatch *string
				var isIsolated, isOnline *int
				if err := rows.Scan(&compID, &compName, &orgID, &orgName, &lastCheckin, &isIsolated, &isOnline, &odMatch); err != nil {
					return fmt.Errorf("scanning device: %w", err)
				}
				online := (isOnline != nil && *isOnline == 1) || odMatch != nil
				isolated := isIsolated != nil && *isIsolated == 1
				class := "offline"
				switch {
				case isolated:
					class = "isolated"
				case online:
					class = "online"
				default:
					if t, ok := parseTLTime(tlString(lastCheckin)); ok && now.Sub(t) <= flagStaleAfter {
						class = "stale"
					}
				}
				dh := deviceHealth{
					ComputerID: tlString(compID), ComputerName: tlString(compName),
					OrganizationID: tlString(orgID), OrganizationName: tlString(orgName),
					LastCheckin: tlString(lastCheckin), HealthClass: class,
				}
				devices = append(devices, dh)

				key := dh.OrganizationID
				r := rollups[key]
				if r == nil {
					r = &orgRollup{OrganizationID: dh.OrganizationID, OrganizationName: dh.OrganizationName}
					rollups[key] = r
				}
				r.Total++
				switch class {
				case "online":
					r.Online++
				case "stale":
					r.Stale++
				case "offline":
					r.Offline++
				case "isolated":
					r.Isolated++
				}
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("iterating devices: %w", err)
			}

			summary := make([]orgRollup, 0, len(rollups))
			for _, r := range rollups {
				summary = append(summary, *r)
			}
			// Most "unhealthy" (offline+stale+isolated) tenants first.
			sort.Slice(summary, func(i, j int) bool {
				ui := summary[i].Offline + summary[i].Stale + summary[i].Isolated
				uj := summary[j].Offline + summary[j].Stale + summary[j].Isolated
				if ui != uj {
					return ui > uj
				}
				return summary[i].OrganizationName < summary[j].OrganizationName
			})

			if flags.asJSON {
				if flagDetail {
					return flags.printJSON(cmd, devices)
				}
				return flags.printJSON(cmd, map[string]any{"summary": summary, "deviceCount": len(devices)})
			}
			if len(devices) == 0 {
				fmt.Fprintln(out, "No synced computers. Run: threatlocker-cli sync --resources computers,online_devices")
				return nil
			}
			if flagDetail {
				headers := []string{"HEALTH", "ORG", "COMPUTER", "LAST_CHECKIN"}
				tableRows := make([][]string, 0, len(devices))
				for _, d := range devices {
					org := d.OrganizationName
					if org == "" {
						org = d.OrganizationID
					}
					tableRows = append(tableRows, []string{d.HealthClass, org, d.ComputerName, d.LastCheckin})
				}
				return flags.printTable(cmd, headers, tableRows)
			}
			headers := []string{"ORG", "TOTAL", "ONLINE", "STALE", "OFFLINE", "ISOLATED"}
			tableRows := make([][]string, 0, len(summary))
			for _, r := range summary {
				org := r.OrganizationName
				if org == "" {
					org = r.OrganizationID
				}
				tableRows = append(tableRows, []string{org, fmt.Sprintf("%d", r.Total), fmt.Sprintf("%d", r.Online), fmt.Sprintf("%d", r.Stale), fmt.Sprintf("%d", r.Offline), fmt.Sprintf("%d", r.Isolated)})
			}
			return flags.printTable(cmd, headers, tableRows)
		},
	}
	cmd.Flags().BoolVar(&flagAllTenants, "all-tenants", false, "Include devices from every synced organization (default scopes to --org when set)")
	cmd.Flags().BoolVar(&flagDetail, "detail", false, "List individual devices instead of the per-tenant rollup")
	cmd.Flags().DurationVar(&flagStaleAfter, "stale-after", 24*time.Hour, "A non-online device that last checked in within this window is 'stale'; older is 'offline'")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
