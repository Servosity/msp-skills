// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

type fleetCounts struct {
	Total          int `json:"total"`
	Online         int `json:"online"`
	Offline        int `json:"offline"`
	Unknown        int `json:"unknown_status"`
	Decommissioned int `json:"decommissioned"`
	Infected       int `json:"infected"`
	OutOfDate      int `json:"out_of_date"`
	NotInProtect   int `json:"under_protected"`
}

type fleetSiteRow struct {
	Site   string `json:"site"`
	SiteID string `json:"site_id,omitempty"`
	fleetCounts
}

// newNovelFleetHealthSummaryCmd rolls the whole fleet into one at-a-glance
// counts block — online/offline, infected, out-of-date, under-protected —
// across every site at once, optionally broken out per site. No single console
// view counts all of these together.
// pp:data-source local
func newNovelFleetHealthSummaryCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var bySite bool

	cmd := &cobra.Command{
		Use:   "summary",
		Short: "At-a-glance fleet counts: online/offline, infected, out-of-date, under-protected",
		Long: `Use this command for at-a-glance fleet status counts across all sites. Do
NOT use this command to rank individual decaying endpoints; use 'fleet-health
stale' instead.

Counts (decommissioned agents are excluded from the live-fleet health counts
since they are expected offline):

  online          active (isActive true): checking in to the console
  offline         inactive (isActive false) and not decommissioned
  unknown_status  no readable isActive and not decommissioned; never
                  counted as online
  decommissioned  retired endpoints
  infected        currently flagged infected
  out_of_date     agent not on the current build
  under_protected agent in detect-only (not Protect) mode

The network status is not used for online/offline: it is the "Disconnect
from network" isolation state, and an isolated agent that still checks in
counts as online.

Add --by-site to break the totals out per tenant.`,
		Example: `  # Fleet totals across every site
  sentinelone-cli fleet-health summary

  # Per-site breakdown, as JSON
  sentinelone-cli fleet-health summary --by-site --agent`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openS1Store(cmd, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, "agents") {
				hintIfStale(cmd, db, "agents", flags.maxAge)
			}

			agents, err := loadAgents(cmd.Context(), db)
			if err != nil {
				return fmt.Errorf("loading agents: %w", err)
			}
			if len(agents) == 0 {
				return honestEmptyJSON(cmd, flags,
					"No agents in the local store. Run 'sentinelone-cli sync --resources agents' first.", nil)
			}

			var fleet fleetCounts
			perSite := map[string]*fleetCounts{}
			siteRefs := map[string]siteRef{}
			getSite := func(r siteRef) *fleetCounts {
				c := perSite[r.Key]
				if c == nil {
					c = &fleetCounts{}
					perSite[r.Key] = c
					siteRefs[r.Key] = r
				}
				return c
			}

			tally := func(c *fleetCounts, a map[string]any) {
				c.Total++
				decomm := agentDecommissioned(a)
				if decomm {
					c.Decommissioned++
				}
				switch agentLiveness(a) {
				case livenessOnline:
					c.Online++
				case livenessOffline:
					if !decomm {
						c.Offline++
					}
				default:
					if !decomm {
						c.Unknown++
					}
				}
				// Live-fleet quality counts exclude decommissioned endpoints.
				if decomm {
					return
				}
				if gbool(a, "infected") {
					c.Infected++
				}
				if !gbool(a, "isUpToDate") {
					c.OutOfDate++
				}
				if !agentInProtect(a) {
					c.NotInProtect++
				}
			}

			for _, a := range agents {
				tally(&fleet, a)
				if bySite {
					tally(getSite(agentSiteRef(a)), a)
				}
			}

			var siteRows []fleetSiteRow
			if bySite {
				for key, c := range perSite {
					r := siteRefs[key]
					siteRows = append(siteRows, fleetSiteRow{Site: r.Label(), SiteID: r.ID, fleetCounts: *c})
				}
				sort.SliceStable(siteRows, func(i, j int) bool { return siteRows[i].Site < siteRows[j].Site })
			}

			if flags.asJSON {
				payload := map[string]any{
					"fleet":        fleet,
					"agents_total": len(agents),
				}
				if bySite {
					payload["sites"] = siteRows
				}
				return flags.printJSON(cmd, payload)
			}

			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Fleet summary (%d agents):\n\n", fleet.Total)
			fmt.Fprintf(w, "  online           %d\n", fleet.Online)
			fmt.Fprintf(w, "  offline          %d\n", fleet.Offline)
			fmt.Fprintf(w, "  unknown status   %d\n", fleet.Unknown)
			fmt.Fprintf(w, "  decommissioned   %d\n", fleet.Decommissioned)
			fmt.Fprintf(w, "  infected         %d\n", fleet.Infected)
			fmt.Fprintf(w, "  out-of-date      %d\n", fleet.OutOfDate)
			fmt.Fprintf(w, "  under-protected  %d\n", fleet.NotInProtect)
			if bySite {
				fmt.Fprintf(w, "\n%-26s %6s %7s %7s %7s %7s %8s %9s %9s\n",
					"SITE", "TOTAL", "ONLINE", "OFFLN", "UNKNWN", "DECOMM", "INFECTED", "OUT-DATE", "UNDERPRO")
				for _, r := range siteRows {
					fmt.Fprintf(w, "%-26s %6d %7d %7d %7d %7d %8d %9d %9d\n",
						clip(r.Site, 26), r.Total, r.Online, r.Offline, r.Unknown, r.Decommissioned, r.Infected, r.OutOfDate, r.NotInProtect)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/sentinelone-cli/data.db)")
	cmd.Flags().BoolVar(&bySite, "by-site", false, "Break the totals out per site as well as the fleet rollup")
	return cmd
}
