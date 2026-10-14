// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"cisco-meraki-pp-cli/internal/client"
)

type deviceCounts struct {
	Online   int `json:"online"`
	Alerting int `json:"alerting"`
	Offline  int `json:"offline"`
	Dormant  int `json:"dormant"`
}

type uplinkCounts struct {
	Active       int `json:"active"`
	Ready        int `json:"ready"`
	Failed       int `json:"failed"`
	NotConnected int `json:"not_connected"`
	Connecting   int `json:"connecting"`
}

type fleetHealthRow struct {
	OrganizationID   string        `json:"organization_id"`
	OrganizationName string        `json:"organization_name"`
	LicensingModel   string        `json:"licensing_model"`
	Reachable        bool          `json:"reachable"`
	Devices          *deviceCounts `json:"devices"`
	Uplinks          *uplinkCounts `json:"uplinks"`
	AlertsActive     *int          `json:"alerts_active"`
	AlertsCritical   *int          `json:"alerts_critical"`
	Score            int           `json:"score"`
	Complete         bool          `json:"complete"`
	Errors           []string      `json:"errors,omitempty"`
	ObservedAt       string        `json:"observed_at"`
}

// healthScore ranks how bad an org looks right now: offline gear and failed
// WAN uplinks weigh most, then alerting devices and critical alerts.
func healthScore(r fleetHealthRow) int {
	s := 0
	if r.Devices != nil {
		s += r.Devices.Offline*3 + r.Devices.Alerting*2
	}
	if r.Uplinks != nil {
		s += r.Uplinks.Failed * 3
	}
	if r.AlertsCritical != nil {
		s += *r.AlertsCritical * 2
	}
	return s
}

func sortHealthRows(rows []fleetHealthRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Reachable != rows[j].Reachable {
			return !rows[i].Reachable // unreachable orgs first: nothing is known about them
		}
		if rows[i].Score != rows[j].Score {
			return rows[i].Score > rows[j].Score
		}
		return strings.ToLower(rows[i].OrganizationName) < strings.ToLower(rows[j].OrganizationName)
	})
}

func fetchOrgHealth(ctx context.Context, c *client.Client, org fleetOrg) fleetHealthRow {
	row := fleetHealthRow{
		OrganizationID:   org.ID,
		OrganizationName: org.Name,
		LicensingModel:   org.LicensingModel,
		ObservedAt:       time.Now().UTC().Format(time.RFC3339),
	}
	if !org.APIEnabled {
		row.Errors = []string{errOrgAPIDisabled.Error()}
		return row
	}
	get := func(path string, v any) error {
		data, err := c.GetNoCache(ctx, orgPath(path, org.ID), nil)
		if err != nil {
			return err
		}
		return json.Unmarshal(data, v)
	}

	var dev struct {
		Counts struct {
			ByStatus deviceCounts `json:"byStatus"`
		} `json:"counts"`
	}
	if err := get("/organizations/{organizationId}/devices/statuses/overview", &dev); err != nil {
		row.Errors = append(row.Errors, "devices: "+err.Error())
	} else {
		d := dev.Counts.ByStatus
		row.Devices = &d
		row.Reachable = true
	}

	var up struct {
		Counts struct {
			ByStatus struct {
				Active       int `json:"active"`
				Ready        int `json:"ready"`
				Failed       int `json:"failed"`
				NotConnected int `json:"notConnected"`
				Connecting   int `json:"connecting"`
			} `json:"byStatus"`
		} `json:"counts"`
	}
	if err := get("/organizations/{organizationId}/appliance/uplinks/statuses/overview", &up); err != nil {
		row.Errors = append(row.Errors, "uplinks: "+err.Error())
	} else {
		b := up.Counts.ByStatus
		row.Uplinks = &uplinkCounts{Active: b.Active, Ready: b.Ready, Failed: b.Failed, NotConnected: b.NotConnected, Connecting: b.Connecting}
		row.Reachable = true
	}

	var alerts struct {
		Counts struct {
			Total      int `json:"total"`
			BySeverity []struct {
				Type  string `json:"type"`
				Count int    `json:"count"`
			} `json:"bySeverity"`
		} `json:"counts"`
	}
	if err := get("/organizations/{organizationId}/assurance/alerts/overview", &alerts); err != nil {
		row.Errors = append(row.Errors, "alerts: "+err.Error())
	} else {
		total, critical := alerts.Counts.Total, 0
		for _, s := range alerts.Counts.BySeverity {
			if strings.EqualFold(s.Type, "critical") {
				critical += s.Count
			}
		}
		row.AlertsActive, row.AlertsCritical = &total, &critical
		row.Reachable = true
	}

	row.Complete = len(row.Errors) == 0
	row.Score = healthScore(row)
	return row
}

func newNovelFleetHealthCmd(flags *rootFlags) *cobra.Command {
	var orgFilter []string
	var limit int
	cmd := &cobra.Command{
		Use:   "health",
		Short: "Rank client orgs worst-first, one row each: device status counts, failed WAN uplinks, active alerts, reachability",
		Long: strings.TrimSpace(`
One worst-first row per organization the API key can see, built from the
Dashboard's own overview endpoints (device statuses, appliance uplink statuses,
assurance alerts). Orgs whose API access is disabled or that fail to answer are
listed first with reachable=false and null counts, never as zeros.

Use this command for per-org counts that show which client org is worst right now.
Do NOT use this command to list the individual offline devices; use 'fleet down' instead.`),
		Example: strings.Trim(`
  cisco-meraki-cli fleet health --agent
  cisco-meraki-cli fleet health --org "Acme Dental" --json
  cisco-meraki-cli fleet health --limit 10 --select organization_name,score,devices.offline`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet health")
			}
			if limit < 0 {
				return usageErr(fmt.Errorf("--limit must be 0 (all) or positive"))
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, orgs, err := fleetClientOrgs(ctx, cmd, flags, orgFilter)
			if err != nil {
				return err
			}
			outcomes := fanOrgs(ctx, orgs, func(ctx context.Context, o fleetOrg) (fleetHealthRow, error) {
				return fetchOrgHealth(ctx, c, o), nil
			})
			rows := make([]fleetHealthRow, 0, len(outcomes))
			unreachable := 0
			for _, oc := range outcomes {
				row := oc.Value
				if oc.Err != nil {
					row = fleetHealthRow{OrganizationID: oc.Org.ID, OrganizationName: oc.Org.Name, Errors: []string{oc.Err.Error()}, ObservedAt: time.Now().UTC().Format(time.RFC3339)}
				}
				if !row.Reachable {
					unreachable++
				}
				rows = append(rows, row)
			}
			sortHealthRows(rows)
			if unreachable > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d organizations unreachable; their counts are null, not zero\n", unreachable, len(rows))
				for _, r := range rows {
					if !r.Reachable {
						fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s (%s): %s\n", r.OrganizationName, r.OrganizationID, strings.Join(r.Errors, "; "))
					}
				}
			}
			if limit > 0 && len(rows) > limit {
				rows = rows[:limit]
			}
			return printFleetRows(cmd, flags, rows, []string{"organization_name", "score", "devices", "uplinks", "alerts_active", "reachable"}, "No organizations visible to this API key.")
		},
	}
	addOrgFlag(cmd, &orgFilter)
	cmd.Flags().IntVar(&limit, "limit", 0, "Return only the N worst organizations (0 = all)")
	return cmd
}
