// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"cisco-meraki-pp-cli/internal/client"
)

type fleetDownRow struct {
	OrganizationID   string `json:"organization_id"`
	OrganizationName string `json:"organization_name"`
	NetworkID        string `json:"network_id"`
	NetworkName      string `json:"network_name"`
	Kind             string `json:"kind"` // device | uplink
	Serial           string `json:"serial"`
	Name             string `json:"name"`
	Model            string `json:"model"`
	ProductType      string `json:"product_type,omitempty"`
	Status           string `json:"status"`
	Interface        string `json:"interface,omitempty"`
	HARole           string `json:"ha_role,omitempty"`
	LastReportedAt   string `json:"last_reported_at,omitempty"`
	DownFor          string `json:"down_for,omitempty"`
	DownMinutes      *int64 `json:"down_minutes"`
	ObservedAt       string `json:"observed_at"`
}

type downStatus struct {
	Name           string `json:"name"`
	Serial         string `json:"serial"`
	NetworkID      string `json:"networkId"`
	Status         string `json:"status"`
	LastReportedAt string `json:"lastReportedAt"`
	ProductType    string `json:"productType"`
	Model          string `json:"model"`
	Network        *struct {
		ID string `json:"id"`
	} `json:"network"`
}

type downUplinkDevice struct {
	NetworkID        string `json:"networkId"`
	Serial           string `json:"serial"`
	Model            string `json:"model"`
	LastReportedAt   string `json:"lastReportedAt"`
	HighAvailability struct {
		Enabled bool   `json:"enabled"`
		Role    string `json:"role"`
	} `json:"highAvailability"`
	Uplinks []struct {
		Interface string `json:"interface"`
		Status    string `json:"status"`
	} `json:"uplinks"`
}

// downRowsFromStatuses keeps offline/alerting devices and computes how long
// each offline device has been silent from lastReportedAt.
func downRowsFromStatuses(org fleetOrg, nets map[string]string, statuses []downStatus, now time.Time) []fleetDownRow {
	rows := make([]fleetDownRow, 0)
	for _, s := range statuses {
		st := strings.ToLower(s.Status)
		if st != "offline" && st != "alerting" {
			continue
		}
		netID := s.NetworkID
		if netID == "" && s.Network != nil {
			netID = s.Network.ID
		}
		row := fleetDownRow{
			OrganizationID: org.ID, OrganizationName: org.Name,
			NetworkID: netID, NetworkName: nets[netID],
			Kind: "device", Serial: s.Serial, Name: s.Name, Model: s.Model, ProductType: s.ProductType,
			Status: st, LastReportedAt: s.LastReportedAt, ObservedAt: now.Format(time.RFC3339),
		}
		if t, ok := parseMerakiTime(s.LastReportedAt); ok && st == "offline" {
			d := now.Sub(t)
			mins := int64(d / time.Minute)
			row.DownMinutes = &mins
			row.DownFor = humanDuration(d)
		}
		rows = append(rows, row)
	}
	return rows
}

// downRowsFromUplinks lists failed WAN uplinks (and, when asked, uplinks
// that are not connected). A site whose wan1 failed while wan2 is active is
// running on its backup WAN even though the MX itself reports online.
func downRowsFromUplinks(org fleetOrg, nets map[string]string, devices []downUplinkDevice, includeNotConnected bool, now time.Time) []fleetDownRow {
	rows := make([]fleetDownRow, 0)
	for _, d := range devices {
		for _, u := range d.Uplinks {
			st := strings.ToLower(u.Status)
			if st != "failed" && !(includeNotConnected && st == "not connected") {
				continue
			}
			row := fleetDownRow{
				OrganizationID: org.ID, OrganizationName: org.Name,
				NetworkID: d.NetworkID, NetworkName: nets[d.NetworkID],
				Kind: "uplink", Serial: d.Serial, Model: d.Model, ProductType: "appliance",
				Status: st, Interface: u.Interface, LastReportedAt: d.LastReportedAt,
				ObservedAt: now.Format(time.RFC3339),
			}
			if d.HighAvailability.Enabled {
				row.HARole = d.HighAvailability.Role
			}
			rows = append(rows, row)
		}
	}
	return rows
}

func sortDownRows(rows []fleetDownRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		di, dj := int64(-1), int64(-1)
		if rows[i].DownMinutes != nil {
			di = *rows[i].DownMinutes
		}
		if rows[j].DownMinutes != nil {
			dj = *rows[j].DownMinutes
		}
		if di != dj {
			return di > dj
		}
		if rows[i].OrganizationName != rows[j].OrganizationName {
			return strings.ToLower(rows[i].OrganizationName) < strings.ToLower(rows[j].OrganizationName)
		}
		return rows[i].Serial < rows[j].Serial
	})
}

// fetchOrgDown returns the org's down rows of the requested kind ("" = both).
// When one read fails after another succeeded, the rows already read are kept
// and Partial names the gap. Network names are fetched only when something is
// down, so a healthy org costs one or two calls.
func fetchOrgDown(ctx context.Context, c *client.Client, org fleetOrg, kind string, includeNotConnected bool) (partialRows[fleetDownRow], error) {
	var out partialRows[fleetDownRow]
	if !org.APIEnabled {
		return out, errOrgAPIDisabled
	}
	now := time.Now().UTC()
	if kind != "uplink" {
		// devices/statuses carries lastReportedAt (how long a device has been
		// silent). It is deprecated in favour of devices/availabilities, which
		// lacks the timestamp, so fall back to it only when statuses is gone.
		filter := url.Values{"statuses[]": {"offline", "alerting"}}
		res, err := merakiGetAllValues(ctx, c, orgPath("/organizations/{organizationId}/devices/statuses", org.ID), filter, 0)
		if err != nil && (isHTTPStatus(err, http.StatusNotFound) || isHTTPStatus(err, http.StatusGone)) {
			res, err = merakiGetAllValues(ctx, c, orgPath("/organizations/{organizationId}/devices/availabilities", org.ID), filter, 0)
		}
		if err != nil {
			return out, fmt.Errorf("device statuses: %w", err)
		}
		out.Rows = downRowsFromStatuses(org, nil, decodeItems[downStatus](res.Items), now)
	}
	if kind != "device" {
		up, err := merakiGetAll(ctx, c, orgPath("/organizations/{organizationId}/appliance/uplink/statuses", org.ID), nil, 0)
		switch {
		case err != nil && kind == "uplink":
			return out, fmt.Errorf("uplink statuses: %w", err)
		case err != nil:
			out.Partial = "uplink statuses: " + err.Error()
		default:
			out.Rows = append(out.Rows, downRowsFromUplinks(org, nil, decodeItems[downUplinkDevice](up.Items), includeNotConnected, now)...)
		}
	}
	if len(out.Rows) == 0 {
		return out, nil
	}
	nets, err := fleetNetworks(ctx, c, org.ID)
	if err != nil {
		// The rows stand; only their network names are missing.
		out.Partial = strings.TrimPrefix(out.Partial+"; network names: "+err.Error(), "; ")
		return out, nil
	}
	for i := range out.Rows {
		out.Rows[i].NetworkName = nets[out.Rows[i].NetworkID]
	}
	return out, nil
}

func newNovelFleetDownCmd(flags *rootFlags) *cobra.Command {
	var orgFilter []string
	var includeNotConnected bool
	var kind string
	cmd := &cobra.Command{
		Use:   "down",
		Short: "List every offline or alerting device and failed MX uplink across all orgs, with how long each has been down (--kind device|uplink)",
		Long: strings.TrimSpace(`
Lists every device that is offline or alerting and every MX/Z WAN uplink that
has failed, across every organization the API key can see, joined to org and
network names. down_for is computed from the device's lastReportedAt. Every
page of every org is read, so the list is complete or the org is reported as
failed on stderr.

Use this command to list individual devices that are offline or alerting, and MX uplinks
that are failed or on failover, across every org right now.
Do NOT use this command for per-org counts or for what went down earlier; use
'fleet health' (counts) or 'fleet flaps' (history) instead.`),
		Example: strings.Trim(`
  cisco-meraki-cli fleet down --agent
  cisco-meraki-cli fleet down --kind uplink --json
  cisco-meraki-cli fleet down --agent --select organization_name,network_name,serial,kind,status,down_for`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet down")
			}
			kind = strings.ToLower(strings.TrimSpace(kind))
			if kind != "" && kind != "device" && kind != "uplink" {
				return usageErr(fmt.Errorf("--kind must be device or uplink, got %q", kind))
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, orgs, err := fleetClientOrgs(ctx, cmd, flags, orgFilter)
			if err != nil {
				return err
			}
			outcomes := fanOrgs(ctx, orgs, func(ctx context.Context, o fleetOrg) (partialRows[fleetDownRow], error) {
				return fetchOrgDown(ctx, c, o, kind, includeNotConnected)
			})
			rows := make([]fleetDownRow, 0)
			for _, oc := range outcomes {
				warnPartial(cmd.ErrOrStderr(), oc.Org, oc.Value.Partial)
				rows = append(rows, oc.Value.Rows...)
			}
			reportFleetFailures(cmd.ErrOrStderr(), outcomes)
			sortDownRows(rows)
			return printFleetRows(cmd, flags, rows, []string{"organization_name", "network_name", "kind", "serial", "name", "status", "interface", "down_for"}, "Nothing is down: no offline or alerting devices and no failed uplinks.")
		},
	}
	addOrgFlag(cmd, &orgFilter)
	cmd.Flags().BoolVar(&includeNotConnected, "include-not-connected", false, "Also list WAN uplinks that are not connected (often an unused wan2)")
	cmd.Flags().StringVar(&kind, "kind", "", "Only devices or only uplinks: device | uplink")
	return cmd
}
