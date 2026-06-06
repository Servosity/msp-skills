// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

type siteHealth struct {
	Site         string `json:"site"`
	Total        int    `json:"total"`
	Online       int    `json:"online"`
	Offline      int    `json:"offline"`
	Infected     int    `json:"infected"`
	OutOfDate    int    `json:"out_of_date"`
	NotInProtect int    `json:"not_in_protect"`
}

// newNovelFleetHealthCmd is the fleet-health parent: bare invocation prints a
// cross-site health rollup (a view the console only shows one site at a time);
// `fleet-health stale` ranks the most-decayed endpoints.
// pp:data-source local
func newNovelFleetHealthCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:   "fleet-health",
		Short: "Summarize agent fleet health (online/offline/infected/out-of-date) across all sites",
		Long: `Roll up the health of every synced agent across all sites in one view:
online vs offline, infected, out-of-date, decommissioned, and how many are not
running in Protect mode. The console shows one site at a time; this joins them.

Run 'sentinelone-cli sync' first to populate the local store.`,
		Example: `  # Cross-site fleet health rollup
  sentinelone-cli fleet-health

  # JSON, including per-site breakdown
  sentinelone-cli fleet-health --agent

  # Rank the most-decayed endpoints
  sentinelone-cli fleet-health stale`,
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
					"No agents in the local store. Run 'sentinelone-cli sync' first.", nil)
			}

			total := len(agents)
			var online, infected, decommissioned, outOfDate, notInProtect int
			sites := map[string]*siteHealth{}
			for _, a := range agents {
				site := orUnknown(agentSite(a))
				sh := sites[site]
				if sh == nil {
					sh = &siteHealth{Site: site}
					sites[site] = sh
				}
				sh.Total++
				if agentDecommissioned(a) {
					decommissioned++
				}
				if agentOnline(a) {
					online++
					sh.Online++
				} else {
					sh.Offline++
				}
				if gbool(a, "infected") {
					infected++
					sh.Infected++
				}
				if !gbool(a, "isUpToDate") {
					outOfDate++
					sh.OutOfDate++
				}
				if !agentInProtect(a) {
					notInProtect++
					sh.NotInProtect++
				}
			}
			offline := total - online

			siteList := make([]*siteHealth, 0, len(sites))
			for _, sh := range sites {
				siteList = append(siteList, sh)
			}
			sort.SliceStable(siteList, func(i, j int) bool { return siteList[i].Total > siteList[j].Total })

			if flags.asJSON {
				return flags.printJSON(cmd, map[string]any{
					"total_agents":   total,
					"online":         online,
					"offline":        offline,
					"infected":       infected,
					"out_of_date":    outOfDate,
					"not_in_protect": notInProtect,
					"decommissioned": decommissioned,
					"sites":          siteList,
				})
			}

			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Fleet health: %d agents — %d online, %d offline, %d infected, %d out-of-date, %d not-in-protect, %d decommissioned\n\n",
				total, online, offline, infected, outOfDate, notInProtect, decommissioned)
			fmt.Fprintf(w, "%-28s %6s %7s %9s %9s %11s\n", "SITE", "TOTAL", "ONLINE", "INFECTED", "OUTDATED", "NOTPROTECT")
			for _, sh := range siteList {
				fmt.Fprintf(w, "%-28s %6d %7d %9d %9d %11d\n",
					clip(sh.Site, 28), sh.Total, sh.Online, sh.Infected, sh.OutOfDate, sh.NotInProtect)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/sentinelone-cli/data.db)")
	cmd.AddCommand(newNovelFleetHealthStaleCmd(flags))
	cmd.AddCommand(newNovelFleetHealthSummaryCmd(flags))
	return cmd
}
