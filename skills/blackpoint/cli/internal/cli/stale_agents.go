// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: stale-agents.
// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type staleAgentRow struct {
	Tenant         string   `json:"tenant"`
	TenantID       string   `json:"tenant_id"`
	DeviceID       string   `json:"device_id"`
	Device         string   `json:"device"`
	Platform       string   `json:"platform"`
	AgentLastSeen  string   `json:"agent_last_seen"`
	DaysSilent     *float64 `json:"days_silent"`
	Decommissioned bool     `json:"decommissioned"`
	WDStatus       string   `json:"wd_status"`
	Reason         string   `json:"reason"`
	silent         time.Duration
}

func newNovelStaleAgentsCmd(flags *rootFlags) *cobra.Command {
	var days, limit int
	var tenant, dbPath string
	var includeDecommissioned bool

	cmd := &cobra.Command{
		Use:   "stale-agents",
		Short: "Devices across every tenant whose agent has not reported in N days",
		Long: `Use this command for devices whose agent stopped reporting.
Do NOT use this command for tenant-level coverage gaps like missing scans or escalation contacts; use 'gaps' instead.

Reads the DEVICE assets fleet-sync stored for every tenant and lists those whose
agentLastSeenOn (falling back to lastSeenOn) is older than --days, plus devices
with no recorded check-in. Decommissioned devices are skipped unless
--include-decommissioned is set.`,
		Example: strings.Trim(`
  blackpoint-cli stale-agents --days 14 --agent
  blackpoint-cli stale-agents --tenant "Acme Dental" --days 7`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "stale-agents: read DEVICE assets from the local fleet store")
			}
			if days < 0 || limit < 0 {
				return usageErr(fmt.Errorf("--days and --limit must be zero or more"))
			}
			db, ok, err := openFleetForRead(cmd, flags, &dbPath)
			if err != nil {
				return err
			}
			if !ok {
				return emitRows(cmd, flags, []staleAgentRow{})
			}
			defer db.Close()
			idx, tenants, err := fleetTenantIndex(db.DB())
			if err != nil {
				return err
			}
			scopes, err := filterTenantScope(tenants, tenant)
			if err != nil {
				return err
			}
			if err := checkFleetKind(cmd, flags, db.DB(), fleetKindDevice, scopes...); err != nil {
				return err
			}
			recs, err := loadCleanFleetRows(db.DB(), fleetKindDevice, fleetKindDevice, scopes...)
			if err != nil {
				return err
			}
			rows := buildStaleAgentRows(recs, idx, time.Now().UTC(), days, includeDecommissioned)
			if limit > 0 && len(rows) > limit {
				rows = rows[:limit]
			}
			return emitRows(cmd, flags, rows, "tenant", "device", "platform", "days_silent", "agent_last_seen", "wd_status", "reason")
		},
	}
	cmd.Flags().IntVar(&days, "days", 14, "Report agents silent for at least this many days")
	cmd.Flags().StringVar(&tenant, "tenant", "", "Limit to one tenant (name or id)")
	cmd.Flags().BoolVar(&includeDecommissioned, "include-decommissioned", false, "Also list decommissioned devices")
	cmd.Flags().IntVar(&limit, "limit", 100, "Maximum rows to return (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database path (default: the CLI's local data.db)")
	return cmd
}

// deviceDecommissioned accepts a boolean or string `decommissioned`, a set
// decommissionDate, or a decommissioned status.
func deviceDecommissioned(o map[string]any) bool {
	if b, ok := fieldBool(o, "decommissioned"); ok {
		return b
	}
	return fieldStr(o, "decommissionDate", "decommissionedOn") != "" || strings.EqualFold(fieldStr(o, "status"), "decommissioned")
}

func buildStaleAgentRows(recs []fleetRecord, idx map[string]fleetTenant, now time.Time, days int, includeDecommissioned bool) []staleAgentRow {
	cutoff := time.Duration(days) * 24 * time.Hour
	rows := []staleAgentRow{}
	for _, r := range recs {
		o := r.Obj
		decom := deviceDecommissioned(o)
		if decom && !includeDecommissioned {
			continue
		}
		seen := fieldStr(o, "agentLastSeenOn", "lastSeenOn")
		row := staleAgentRow{
			Tenant: tenantName(idx, r.Scope), TenantID: r.Scope, DeviceID: r.ID,
			Device: fieldStr(o, "displayName", "name", "hostname"), Platform: fieldStr(o, "platform", "operatingSystem"),
			AgentLastSeen: seen, Decommissioned: decom, WDStatus: fieldStr(o, "wdStatus", "windowsDefenderStatus"),
		}
		t, ok := parseFleetTime(seen)
		if !ok {
			row.Reason = "no agent check-in recorded"
			rows = append(rows, row)
			continue
		}
		silent := now.Sub(t)
		if silent < cutoff {
			continue
		}
		d := float64(int(silent.Hours()/24*10)) / 10
		row.DaysSilent = &d
		row.silent = silent
		row.Reason = fmt.Sprintf("silent %s", humanAge(silent))
		rows = append(rows, row)
	}
	// Longest-silent first; devices with no check-in at all lead.
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i].DaysSilent, rows[j].DaysSilent
		switch {
		case a == nil && b == nil:
			return rows[i].Tenant < rows[j].Tenant
		case a == nil:
			return true
		case b == nil:
			return false
		}
		return rows[i].silent > rows[j].silent
	})
	return rows
}
