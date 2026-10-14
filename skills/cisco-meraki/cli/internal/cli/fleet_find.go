// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source auto

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"cisco-meraki-pp-cli/internal/client"
	"cisco-meraki-pp-cli/internal/store"
)

type findRow struct {
	MatchType        string `json:"match_type"` // organization | network | device | inventory | uplink | client
	MatchedOn        string `json:"matched_on"` // serial | mac | ip | network_id | name
	OrganizationID   string `json:"organization_id"`
	OrganizationName string `json:"organization_name"`
	NetworkID        string `json:"network_id,omitempty"`
	NetworkName      string `json:"network_name,omitempty"`
	Serial           string `json:"serial,omitempty"`
	Name             string `json:"name,omitempty"`
	MAC              string `json:"mac,omitempty"`
	IP               string `json:"ip,omitempty"`
	Model            string `json:"model,omitempty"`
	Detail           string `json:"detail,omitempty"`
	Source           string `json:"source"`
	ObservedAt       string `json:"observed_at"`
}

var (
	macColonRe  = regexp.MustCompile(`^([0-9a-f]{2}[:-]){5}[0-9a-f]{2}$`)
	macDotRe    = regexp.MustCompile(`^[0-9a-f]{4}\.[0-9a-f]{4}\.[0-9a-f]{4}$`)
	macBareRe   = regexp.MustCompile(`^[0-9a-f]{12}$`)
	serialRe    = regexp.MustCompile(`^Q[0-9A-Z]{3}-[0-9A-Z]{4}-[0-9A-Z]{4}$`)
	networkIDRe = regexp.MustCompile(`^[LN]_[0-9]+$`)
)

// classifyIdentifier returns the identifier kind and its canonical form:
// MACs become lowercase colon-separated, serials uppercase.
func classifyIdentifier(raw string) (kind, value string) {
	s := strings.TrimSpace(raw)
	lower := strings.ToLower(s)
	switch {
	case macColonRe.MatchString(lower):
		return "mac", strings.ReplaceAll(lower, "-", ":")
	case macDotRe.MatchString(lower), macBareRe.MatchString(lower):
		hex := strings.ReplaceAll(lower, ".", "")
		parts := make([]string, 0, 6)
		for i := 0; i < 12; i += 2 {
			parts = append(parts, hex[i:i+2])
		}
		return "mac", strings.Join(parts, ":")
	case serialRe.MatchString(strings.ToUpper(s)):
		return "serial", strings.ToUpper(s)
	case networkIDRe.MatchString(s):
		return "network_id", s
	case net.ParseIP(s) != nil:
		return "ip", s
	default:
		return "name", s
	}
}

type findRecord struct {
	Serial    string `json:"serial"`
	Name      string `json:"name"`
	MAC       string `json:"mac"`
	LanIP     string `json:"lanIp"`
	PublicIP  string `json:"publicIp"`
	Model     string `json:"model"`
	NetworkID string `json:"networkId"`
	Uplinks   []struct {
		IP       string `json:"ip"`
		PublicIP string `json:"publicIp"`
	} `json:"uplinks"`
}

// matchRecord reports whether a device-shaped record matches the identifier
// on the field its kind names. Name matches are case-insensitive substrings.
func matchRecord(r findRecord, kind, value string) bool {
	switch kind {
	case "serial":
		return strings.EqualFold(r.Serial, value)
	case "mac":
		return strings.EqualFold(r.MAC, value)
	case "ip":
		if r.LanIP == value || r.PublicIP == value {
			return true
		}
		for _, u := range r.Uplinks {
			if u.IP == value || u.PublicIP == value {
				return true
			}
		}
		return false
	case "name":
		return r.Name != "" && strings.Contains(strings.ToLower(r.Name), strings.ToLower(value))
	}
	return false
}

func rowFromRecord(matchType, matchedOn, value string, org fleetOrg, nets map[string]string, r findRecord, source string, now time.Time) findRow {
	ip := r.LanIP
	if matchedOn == "ip" {
		ip = value
	}
	return findRow{MatchType: matchType, MatchedOn: matchedOn, OrganizationID: org.ID, OrganizationName: org.Name,
		NetworkID: r.NetworkID, NetworkName: nets[r.NetworkID], Serial: r.Serial, Name: r.Name, MAC: r.MAC, IP: ip,
		Model: r.Model, Source: source, ObservedAt: now.Format(time.RFC3339)}
}

// errNoMirror means sync has not written a local mirror yet: an empty local
// state, not a failure.
var errNoMirror = errors.New("no local mirror")

// storedOrg extracts the parent organization from a dependent row's storage
// id ("<id>\x00<organizationId>"); flat rows have no parent.
func storedOrg(id string) string {
	if i := strings.IndexByte(id, 0); i >= 0 {
		return id[i+1:]
	}
	return ""
}

// findLocal searches the mirror `sync` writes. Every synced resource lives in
// the generic resources table; dependent rows carry their org in the id.
func findLocal(ctx context.Context, cmd *cobra.Command, flags *rootFlags, dbPath, kind, value string) ([]findRow, error) {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, errNoMirror
	} else if err != nil {
		return nil, fmt.Errorf("local mirror at %s: %w", dbPath, err)
	}
	db, err := store.OpenReadOnlyContext(ctx, dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening local mirror: %w", err)
	}
	defer db.Close()
	if !hintIfUnsynced(cmd, db, "organizations") {
		hintIfStale(cmd, db, "organizations", flags.maxAge)
	}
	type stored struct{ id, data string }
	read := func(resourceType string) ([]stored, error) {
		rows, err := db.DB().QueryContext(ctx, `SELECT id, data FROM resources WHERE resource_type = ?`, resourceType)
		if err != nil {
			return nil, fmt.Errorf("reading %s from the local mirror: %w", resourceType, err)
		}
		out := make([]stored, 0)
		for rows.Next() {
			var s stored
			if err := rows.Scan(&s.id, &s.data); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("scanning %s: %w", resourceType, err)
			}
			out = append(out, s)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		return out, rows.Close()
	}
	orgRows, err := read("organizations")
	if err != nil {
		return nil, err
	}
	orgNames := map[string]string{}
	for _, s := range orgRows {
		var o struct{ ID, Name string }
		if json.Unmarshal([]byte(s.data), &o) == nil && o.ID != "" {
			orgNames[o.ID] = o.Name
		}
	}
	netRows, err := read("organizations_networks")
	if err != nil {
		return nil, err
	}
	nets := map[string]string{}
	netOrg := map[string]string{}
	for _, s := range netRows {
		var n struct{ ID, Name string }
		if json.Unmarshal([]byte(s.data), &n) == nil {
			nets[n.ID], netOrg[n.ID] = n.Name, storedOrg(s.id)
		}
	}
	now := time.Now().UTC()
	rows := make([]findRow, 0)
	org := func(id string) fleetOrg { return fleetOrg{ID: id, Name: orgNames[id]} }
	switch kind {
	case "network_id":
		if name, ok := nets[value]; ok {
			rows = append(rows, findRow{MatchType: "network", MatchedOn: kind, OrganizationID: netOrg[value], OrganizationName: orgNames[netOrg[value]],
				NetworkID: value, NetworkName: name, Source: "local", ObservedAt: now.Format(time.RFC3339)})
		}
		return rows, nil
	case "name":
		for id, name := range orgNames {
			if strings.Contains(strings.ToLower(name), strings.ToLower(value)) {
				rows = append(rows, findRow{MatchType: "organization", MatchedOn: kind, OrganizationID: id, OrganizationName: name, Source: "local", ObservedAt: now.Format(time.RFC3339)})
			}
		}
		for id, name := range nets {
			if strings.Contains(strings.ToLower(name), strings.ToLower(value)) {
				rows = append(rows, findRow{MatchType: "network", MatchedOn: kind, OrganizationID: netOrg[id], OrganizationName: orgNames[netOrg[id]], NetworkID: id, NetworkName: name, Source: "local", ObservedAt: now.Format(time.RFC3339)})
			}
		}
	}
	seen := map[string]bool{}
	for _, t := range []struct{ resource, matchType string }{
		{"organizations_devices", "device"},
		{"organizations_devices_statuses", "device"},
		{"organizations_appliance_uplink_statuses", "uplink"},
		{"inventory", "inventory"},
	} {
		items, err := read(t.resource)
		if err != nil {
			return nil, err
		}
		for _, s := range items {
			var r findRecord
			if json.Unmarshal([]byte(s.data), &r) != nil {
				continue
			}
			ok := matchRecord(r, kind, value)
			orgID := storedOrg(s.id)
			key := t.matchType + "|" + orgID + "|" + r.Serial
			if t.matchType != "uplink" {
				key = "device|" + orgID + "|" + r.Serial // one row per device even when several resources match
			}
			if !ok || seen[key] {
				continue
			}
			seen[key] = true
			rows = append(rows, rowFromRecord(t.matchType, kind, value, org(orgID), nets, r, "local", now))
		}
	}
	return rows, nil
}

// findLive asks the Dashboard. Serials and network IDs resolve directly
// (and are kept only when their org is in scope); MACs, IPs and names fan out
// across every org in scope.
func findLive(ctx context.Context, c *client.Client, orgs []fleetOrg, kind, value string) ([]findRow, []orgOutcome[partialRows[findRow]], error) {
	now := time.Now().UTC()
	byID := map[string]fleetOrg{}
	for _, o := range orgs {
		byID[o.ID] = o
	}
	rows := make([]findRow, 0)
	// networkOf resolves a network's org; found=false on 404, err otherwise.
	networkOf := func(netID string) (org fleetOrg, name string, found bool, err error) {
		raw, err := c.GetNoCache(ctx, "/networks/"+url.PathEscape(netID), nil)
		if isHTTPStatus(err, http.StatusNotFound) {
			return fleetOrg{}, "", false, nil
		}
		if err != nil {
			return fleetOrg{}, "", false, fmt.Errorf("resolving network %s: %w", netID, err)
		}
		var n struct {
			OrganizationID string `json:"organizationId"`
			Name           string `json:"name"`
		}
		if err := json.Unmarshal(raw, &n); err != nil {
			return fleetOrg{}, "", false, fmt.Errorf("decoding network %s: %w", netID, err)
		}
		o, inScope := byID[n.OrganizationID]
		return o, n.Name, inScope, nil
	}
	switch kind {
	case "network_id":
		o, name, found, err := networkOf(value)
		if err != nil {
			return nil, nil, err
		}
		if found {
			rows = append(rows, findRow{MatchType: "network", MatchedOn: kind, OrganizationID: o.ID, OrganizationName: o.Name, NetworkID: value, NetworkName: name, Source: "live", ObservedAt: now.Format(time.RFC3339)})
		}
		return rows, nil, nil
	case "serial":
		raw, err := c.GetNoCache(ctx, "/devices/"+url.PathEscape(value), nil)
		if err != nil && !isHTTPStatus(err, http.StatusNotFound) {
			return nil, nil, fmt.Errorf("looking up device %s: %w", value, err)
		}
		if err == nil {
			var r findRecord
			if json.Unmarshal(raw, &r) == nil && r.Serial != "" && r.NetworkID != "" {
				o, netName, inScope, err := networkOf(r.NetworkID)
				if err != nil {
					return nil, nil, err
				}
				if inScope {
					rows = append(rows, rowFromRecord("device", kind, value, o, map[string]string{r.NetworkID: netName}, r, "live", now))
				}
				return rows, nil, nil
			}
		}
		// Not in a network: it may still be claimed into an org's inventory.
	}
	outcomes := fanOrgs(ctx, orgs, func(ctx context.Context, o fleetOrg) (partialRows[findRow], error) {
		var res partialRows[findRow]
		if !o.APIEnabled {
			return res, errOrgAPIDisabled
		}
		nets, err := fleetNetworks(ctx, c, o.ID)
		if err != nil {
			return res, fmt.Errorf("networks: %w", err)
		}
		switch kind {
		case "serial":
			inv, err := merakiGetAll(ctx, c, orgPath("/organizations/{organizationId}/inventory/devices", o.ID), map[string]string{"serials[]": value}, 0)
			if err != nil {
				return res, err
			}
			for _, raw := range inv.Items {
				var r findRecord
				if json.Unmarshal(raw, &r) == nil && strings.EqualFold(r.Serial, value) {
					res.Rows = append(res.Rows, rowFromRecord("inventory", kind, value, o, nets, r, "live", now))
				}
			}
		case "mac":
			devs, err := merakiGetAll(ctx, c, orgPath("/organizations/{organizationId}/devices", o.ID), map[string]string{"mac": value}, 0)
			if err != nil {
				res.Partial = "devices: " + err.Error()
			}
			found := map[string]bool{}
			for _, raw := range devs.Items {
				var r findRecord
				if json.Unmarshal(raw, &r) == nil && strings.EqualFold(r.MAC, value) {
					res.Rows = append(res.Rows, rowFromRecord("device", kind, value, o, nets, r, "live", now))
					found[r.Serial] = true
				}
			}
			// A claimed device not yet in a network only shows in inventory.
			inv, err := merakiGetAll(ctx, c, orgPath("/organizations/{organizationId}/inventory/devices", o.ID), map[string]string{"macs[]": value}, 0)
			if err != nil {
				res.Partial = strings.TrimPrefix(res.Partial+"; inventory: "+err.Error(), "; ")
			}
			for _, raw := range inv.Items {
				var r findRecord
				if json.Unmarshal(raw, &r) == nil && strings.EqualFold(r.MAC, value) && !found[r.Serial] {
					res.Rows = append(res.Rows, rowFromRecord("inventory", kind, value, o, nets, r, "live", now))
				}
			}
			cres, err := merakiGetAll(ctx, c, orgPath("/organizations/{organizationId}/clients/search", o.ID), map[string]string{"mac": value}, 0)
			if err != nil && !isHTTPStatus(err, http.StatusNotFound) {
				res.Partial = strings.TrimPrefix(res.Partial+"; client search: "+err.Error(), "; ")
				return res, nil
			}
			if err == nil && len(cres.Items) == 1 {
				raw := cres.Items[0]
				var cs struct {
					ClientID string `json:"clientId"`
					MAC      string `json:"mac"`
					Records  []struct {
						Network struct {
							ID   string `json:"id"`
							Name string `json:"name"`
						} `json:"network"`
						IP          string `json:"ip"`
						Description string `json:"description"`
						SSID        string `json:"ssid"`
					} `json:"records"`
				}
				if json.Unmarshal(raw, &cs) == nil {
					for _, rec := range cs.Records {
						detail := fmt.Sprintf("client %s", cs.ClientID)
						if rec.SSID != "" {
							detail += " on SSID " + rec.SSID
						}
						res.Rows = append(res.Rows, findRow{MatchType: "client", MatchedOn: kind, OrganizationID: o.ID, OrganizationName: o.Name,
							NetworkID: rec.Network.ID, NetworkName: rec.Network.Name, Name: rec.Description, MAC: cs.MAC, IP: rec.IP,
							Detail: detail, Source: "live", ObservedAt: now.Format(time.RFC3339)})
					}
				}
			}
		case "ip", "name":
			params := map[string]string{}
			if kind == "name" {
				params["name"] = value
				if strings.Contains(strings.ToLower(o.Name), strings.ToLower(value)) {
					res.Rows = append(res.Rows, findRow{MatchType: "organization", MatchedOn: kind, OrganizationID: o.ID, OrganizationName: o.Name, Source: "live", ObservedAt: now.Format(time.RFC3339)})
				}
				for id, name := range nets {
					if strings.Contains(strings.ToLower(name), strings.ToLower(value)) {
						res.Rows = append(res.Rows, findRow{MatchType: "network", MatchedOn: kind, OrganizationID: o.ID, OrganizationName: o.Name, NetworkID: id, NetworkName: name, Source: "live", ObservedAt: now.Format(time.RFC3339)})
					}
				}
			}
			devs, err := merakiGetAll(ctx, c, orgPath("/organizations/{organizationId}/devices", o.ID), params, 0)
			if err != nil {
				// Keep the org/network name matches already found.
				res.Partial = "devices: " + err.Error()
				return res, nil
			}
			for _, raw := range devs.Items {
				var r findRecord
				if json.Unmarshal(raw, &r) != nil {
					continue
				}
				if matchRecord(r, kind, value) {
					res.Rows = append(res.Rows, rowFromRecord("device", kind, value, o, nets, r, "live", now))
				}
			}
			if kind == "ip" {
				// WAN public IPs from SOC alerts usually belong to an MX uplink.
				up, err := merakiGetAll(ctx, c, orgPath("/organizations/{organizationId}/appliance/uplink/statuses", o.ID), nil, 0)
				if err != nil {
					res.Partial = "uplink statuses: " + err.Error()
				} else {
					for _, raw := range up.Items {
						var r findRecord
						if json.Unmarshal(raw, &r) == nil {
							if matchRecord(findRecord{Uplinks: r.Uplinks}, "ip", value) {
								res.Rows = append(res.Rows, rowFromRecord("uplink", "ip", value, o, nets, r, "live", now))
							}
						}
					}
				}
			}
		}
		return res, nil
	})
	for _, oc := range outcomes {
		rows = append(rows, oc.Value.Rows...)
	}
	return rows, outcomes, nil
}

func newNovelFleetFindCmd(flags *rootFlags) *cobra.Command {
	var orgFilter []string
	var dbPath string
	cmd := &cobra.Command{
		Use:   "find [mac|ip|serial|network-id|name]",
		Short: "Resolve a MAC, IP, serial, network ID or name to the org and network that owns it",
		Long: strings.TrimSpace(`
Works out what kind of identifier you pasted (MAC in any common format, IPv4 or
IPv6, Meraki serial, network ID, or free text) and returns every organization >
network > device, inventory entry, MX uplink or client that matches. With
--data-source auto (default) the local mirror written by 'sync' is searched
first and the live Dashboard is asked only when the mirror has no match; a live
MAC search also checks client records.

Use this command to resolve a MAC, IP, serial or name to the org, network and device or client it belongs to.
Do NOT use this command for free-text search within one synced resource; use 'search' instead.`),
		Example: strings.Trim(`
  cisco-meraki-cli fleet find Q2QN-9J8L-SLPD --agent
  cisco-meraki-cli fleet find 00:18:0a:12:34:56 --data-source live --json
  cisco-meraki-cli fleet find 203.0.113.10 --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:happy-args": "identifier=Q2QN-9J8L-SLPD"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet find")
			}
			if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("exactly one identifier is required (MAC, IP, serial, network ID or name)"))
			}
			kind, value := classifyIdentifier(args[0])
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			if dbPath == "" {
				dbPath = defaultDBPath("cisco-meraki-cli")
			}
			mode := strings.ToLower(flags.dataSource)
			rows := make([]findRow, 0)
			if mode != "live" {
				local, err := findLocal(ctx, cmd, flags, dbPath, kind, value)
				switch {
				case errors.Is(err, errNoMirror):
					if mode == "local" {
						fmt.Fprintln(cmd.ErrOrStderr(), noMirrorHint(dbPath))
					}
				case err != nil && mode == "local":
					return err
				case err != nil:
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: local mirror unreadable (%v); asking the Dashboard\n", err)
				}
				keep := fleetOrgFilter(orgFilter)
				for _, r := range local {
					if keep(fleetOrg{ID: r.OrganizationID, Name: r.OrganizationName}) {
						rows = append(rows, r)
					}
				}
			}
			if mode == "live" || (mode != "local" && len(rows) == 0) {
				c, orgs, err := fleetClientOrgs(ctx, cmd, flags, orgFilter)
				if err != nil {
					return err
				}
				live, outcomes, err := findLive(ctx, c, orgs, kind, value)
				if err != nil {
					return classifyAPIError(cmd.OutOrStdout(), err, flags)
				}
				for _, oc := range outcomes {
					warnPartial(cmd.ErrOrStderr(), oc.Org, oc.Value.Partial)
				}
				reportFleetFailures(cmd.ErrOrStderr(), outcomes)
				rows = live
			}
			sort.SliceStable(rows, func(i, j int) bool {
				if rows[i].OrganizationName != rows[j].OrganizationName {
					return strings.ToLower(rows[i].OrganizationName) < strings.ToLower(rows[j].OrganizationName)
				}
				return rows[i].MatchType < rows[j].MatchType
			})
			if len(rows) == 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "no %s match for %q in any organization the key can see\n", kind, value)
			}
			return printFleetRows(cmd, flags, rows, []string{"match_type", "organization_name", "network_name", "serial", "name", "mac", "ip", "source"}, fmt.Sprintf("No match for %s %q.", kind, value))
		},
	}
	addOrgFlag(cmd, &orgFilter)
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror path (default: the sync database)")
	return cmd
}
