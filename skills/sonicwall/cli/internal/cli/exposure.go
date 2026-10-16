// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"sonicwall-pp-cli/internal/cliutil"
	"sonicwall-pp-cli/internal/posture"
)

type advisoryRank struct {
	AdvisoryID     string   `json:"advisory_id"`
	Title          string   `json:"title"`
	Impact         string   `json:"impact,omitempty"`
	HasWorkaround  *bool    `json:"has_workaround"` // null when NSM did not report it
	NoticeLink     string   `json:"notice_link,omitempty"`
	FirewallsHit   int      `json:"firewalls_hit"`
	FirewallSerial []string `json:"serials"`
}

type exposureRow struct {
	AdvisoryID    string   `json:"advisory_id"`
	Serial        string   `json:"serial"`
	Name          string   `json:"name"`
	TenantID      string   `json:"tenant_id,omitempty"`
	Firmware      string   `json:"firmware,omitempty"`
	Online        *bool    `json:"online"`         // null when the device inventory did not report it
	HasWorkaround *bool    `json:"has_workaround"` // null when NSM did not report it
	SSLVPNZones   []string `json:"sslvpn_zones"`
	WANReachable  string   `json:"sslvpn_wan_reachable"` // yes, no or unknown
	Error         string   `json:"error,omitempty"`
}

// rankAdvisories groups NSM's vulnerable-device list by advisory, most firewalls first.
func rankAdvisories(vulns []vulnerableDevice) []advisoryRank {
	byID := map[string]*advisoryRank{}
	for _, v := range vulns {
		if v.SerialNumber == "" {
			continue
		}
		for _, a := range v.advisories() {
			if a.AdvisoryID == "" {
				continue
			}
			r := byID[a.AdvisoryID]
			if r == nil {
				r = &advisoryRank{AdvisoryID: a.AdvisoryID, Title: a.Title, Impact: a.Impact, HasWorkaround: a.HaveWorkaround, NoticeLink: a.NoticeLink, FirewallSerial: make([]string, 0)}
				byID[a.AdvisoryID] = r
			}
			r.FirewallSerial = append(r.FirewallSerial, v.SerialNumber)
			r.FirewallsHit = len(r.FirewallSerial)
		}
	}
	out := make([]advisoryRank, 0, len(byID))
	for _, r := range byID {
		sort.Strings(r.FirewallSerial)
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].FirewallsHit != out[j].FirewallsHit {
			return out[i].FirewallsHit > out[j].FirewallsHit
		}
		return out[i].AdvisoryID < out[j].AdvisoryID
	})
	return out
}

func newNovelExposureCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exposure [advisory-or-cve]",
		Short: "Rank SonicWall advisories by how many firewalls they hit, or pass one SNWLID ID (exact) or CVE ID to list the affected firewalls and whether SSL-VPN is on a WAN zone; a CVE matches only when NSM's advisory text names it, so an empty list is not zero exposure",
		Long: "Use this command for advisory triage: which firewalls a SonicWall PSIRT advisory (SNWLID ID) or CVE hits, and\n" +
			"whether SSL-VPN is reachable on a WAN zone for each. Do NOT use this command for every firewall's\n" +
			"firmware-behind status regardless of advisories; use 'firmware drift' instead.\n\n" +
			"With no argument, ranks the advisories NSM matched to your firewalls by how many firewalls each hits.\n" +
			"With an argument, matches an advisory ID exactly (e.g. SNWLID-2024-0015) or text in the advisory title\n" +
			"or notice link (a CVE ID only matches when NSM's advisory text names it), then reads each hit firewall's\n" +
			"SSL-VPN zones. A query that matches nothing returns an empty list, not 'zero exposed'.",
		Example: strings.Trim(`
  sonicwall-cli exposure --agent
  sonicwall-cli exposure SNWLID-2024-0015 --json
  sonicwall-cli exposure "SSLVPN" --agent`, "\n"),
		Annotations: map[string]string{"pp:data-source": "live", "mcp:read-only": "true", "pp:happy-args": "advisory=SNWLID-2024-0015"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "exposure")
			}
			ctx := cmd.Context()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			vulns, err := fetchVulnerable(ctx, c)
			if err != nil {
				return classifyReadError(cmd.OutOrStdout(), err, flags)
			}
			// NSM's vulnerable list covers only firewalls it could judge: an empty
			// fleet is refused, and a firewall with no reported SonicOS version (or
			// an unreadable inventory) makes the answer partial, never a clean list.
			devices, derr := fetchFleet(ctx, c, "", "")
			if derr == nil && len(devices) == 0 {
				return noFirewallsErr(cmd.OutOrStdout(), flags)
			}
			var coverage error
			if derr != nil {
				coverage = fmt.Errorf("device inventory unavailable (%s); firewalls NSM could not judge are not covered", errText(derr))
			} else {
				unjudged := 0
				for _, d := range devices {
					if sonicOSVersionPattern.FindString(strings.TrimSpace(d.FirmwareVersion)) == "" {
						unjudged++
					}
				}
				if unjudged > 0 {
					coverage = fmt.Errorf("%d of %d firewalls have no reported SonicOS version, so NSM could not match advisories to them", unjudged, len(devices))
				}
			}
			if coverage != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: "+coverage.Error())
			}
			if len(args) == 0 {
				ranked := rankAdvisories(vulns)
				stopped := interrupted(ctx)
				if stopped == nil && coverage != nil {
					stopped = partialFailureErr(coverage)
				}
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					if err := printJSONFiltered(cmd.OutOrStdout(), ranked, flags); err != nil {
						return err
					}
					return stopped
				}
				if len(ranked) == 0 {
					if stopped == nil {
						fmt.Fprintln(cmd.OutOrStdout(), "NSM matched no published SonicWall advisories to your firewalls.")
					}
					return stopped
				}
				rows := make([]map[string]any, 0, len(ranked))
				for _, r := range ranked {
					rows = append(rows, map[string]any{"advisory": r.AdvisoryID, "firewalls": r.FirewallsHit, "impact": r.Impact, "workaround": triState(r.HasWorkaround), "title": r.Title})
				}
				if err := printOrderedTable(cmd.OutOrStdout(), []string{"advisory", "firewalls", "impact", "workaround", "title"}, rows); err != nil {
					return err
				}
				return stopped
			}

			query := args[0]
			var partial error
			hits := make([]exposureRow, 0)
			for _, v := range vulns {
				if v.SerialNumber == "" {
					continue
				}
				for _, a := range v.advisories() {
					if posture.MatchesAdvisory(query, a.AdvisoryID, a.Title, a.NoticeLink) {
						hits = append(hits, exposureRow{AdvisoryID: a.AdvisoryID, Serial: v.SerialNumber, Name: v.FriendlyName,
							HasWorkaround: a.HaveWorkaround, SSLVPNZones: make([]string, 0), WANReachable: "unknown"})
					}
				}
			}
			if len(hits) == 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: no advisory in NSM's vulnerable-device list matched %q; an empty list is not zero exposure (try the SNWLID advisory ID)\n", query)
				partial = interrupted(ctx)
			} else {
				if derr != nil {
					fmt.Fprintln(cmd.ErrOrStderr(), "warning: online, firmware and tenant are unknown (null) without the device inventory")
				} else {
					byserial := map[string]fleetDevice{}
					for _, d := range devices {
						byserial[strings.ToUpper(d.SerialNumber)] = d
					}
					for i := range hits {
						if d, ok := byserial[strings.ToUpper(hits[i].Serial)]; ok {
							hits[i].TenantID, hits[i].Firmware, hits[i].Online = d.TenantID, d.FirmwareVersion, d.LiveStatus
						}
					}
				}
				results, fails := cliutil.FanoutRun(ctx, hits,
					func(h exposureRow) string { return h.AdvisoryID + "|" + h.Serial },
					func(ctx context.Context, h exposureRow) (exposureRow, error) {
						raw, err := sonicOSGet(ctx, c, h.Serial, "/firewall/ssl-vpn/server/accesses")
						if err != nil {
							h.Error = errText(err)
							return h, nil
						}
						if zones, wan, ok := posture.SSLVPNZones(raw); ok {
							h.SSLVPNZones = zones
							h.WANReachable = "no"
							if wan {
								h.WANReachable = "yes"
							}
						} else {
							h.Error = "ssl-vpn zones: unexpected response shape"
						}
						return h, nil
					})
				done := map[string]exposureRow{}
				for _, res := range results {
					done[res.Source] = res.Value
				}
				failErr := map[string]error{}
				for _, f := range fails {
					failErr[f.Source] = f.Err
				}
				for i, h := range hits {
					key := h.AdvisoryID + "|" + h.Serial
					if r, ok := done[key]; ok {
						hits[i] = r
					} else {
						hits[i].Error = "not checked: " + errText(failErr[key])
					}
				}
				partial = interrupted(ctx)
				if partial == nil && len(fails) > 0 {
					fmt.Fprintln(cmd.ErrOrStderr(), "warning: "+fanoutFailureNote(fails, len(hits)))
					partial = partialFailureErr(fmt.Errorf("%s", fanoutFailureNote(fails, len(hits))))
				}
				sort.SliceStable(hits, func(i, j int) bool { return hits[i].WANReachable == "yes" && hits[j].WANReachable != "yes" })
			}
			if partial == nil && coverage != nil {
				partial = partialFailureErr(coverage)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFiltered(cmd.OutOrStdout(), hits, flags); err != nil {
					return err
				}
				return partial
			}
			if len(hits) == 0 {
				return partial
			}
			rows := make([]map[string]any, 0, len(hits))
			for _, h := range hits {
				rows = append(rows, map[string]any{"advisory": h.AdvisoryID, "serial": h.Serial, "name": h.Name, "wan_reachable": h.WANReachable, "zones": strings.Join(h.SSLVPNZones, ","), "workaround": triState(h.HasWorkaround)})
			}
			if err := printOrderedTable(cmd.OutOrStdout(), []string{"advisory", "serial", "name", "wan_reachable", "zones", "workaround"}, rows); err != nil {
				return err
			}
			return partial
		},
	}
	return cmd
}
