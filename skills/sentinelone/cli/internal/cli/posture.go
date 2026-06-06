// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"
)

type postureRow struct {
	Site            string  `json:"site"`
	Agents          int     `json:"agents"`
	HealthPct       float64 `json:"health_pct"`
	CoveragePct     float64 `json:"coverage_pct"`
	VersionComplPct float64 `json:"version_compliance_pct"`
	OpenThreats     int     `json:"open_threats"`
	OldestOpenDays  int     `json:"oldest_open_threat_days"`
}

// newNovelPostureCmd composes a per-tenant (per-site) posture scorecard —
// health %, protection coverage %, version compliance %, open threats, and the
// oldest unresolved threat — into one rollup the API has no single object for.
// Ideal for a morning MSSP review or a client QBR.
// pp:data-source local
func newNovelPostureCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:   "posture",
		Short: "Per-tenant security posture scorecard (health %, coverage %, open threats, version compliance)",
		Long: `Compose a one-row-per-site scorecard from the local store:

  health %             online AND up-to-date AND in-protect AND not-infected
  coverage %           agents in Protect mode
  version compliance % agents on the fleet's current (modal) version
  open threats         threats not resolved/mitigated for that site
  oldest open          age in days of the oldest unresolved threat

No single API call returns a tenant-level composite like this.`,
		Example: `  # Posture scorecard for every site
  sentinelone-cli posture

  # JSON for a QBR deck or dashboard
  sentinelone-cli posture --agent`,
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

			if !hintIfUnsynced(cmd, db, "") {
				hintIfStale(cmd, db, "", flags.maxAge)
			}

			agents, err := loadAgents(cmd.Context(), db)
			if err != nil {
				return fmt.Errorf("loading agents: %w", err)
			}
			threats, err := loadThreats(cmd.Context(), db)
			if err != nil {
				return fmt.Errorf("loading threats: %w", err)
			}
			if len(agents) == 0 {
				return honestEmptyJSON(cmd, flags,
					"No agents in the local store. Run 'sentinelone-cli sync' first.", nil)
			}

			modal := modalVersion(agents)
			now := time.Now()

			type acc struct {
				total, healthy, protected, onVersion int
				openThreats, oldestDays              int
			}
			sites := map[string]*acc{}
			get := func(site string) *acc {
				site = orUnknown(site)
				a := sites[site]
				if a == nil {
					a = &acc{}
					sites[site] = a
				}
				return a
			}
			for _, a := range agents {
				if agentDecommissioned(a) {
					continue
				}
				s := get(agentSite(a))
				s.total++
				if agentInProtect(a) {
					s.protected++
				}
				if v := gstr(a, "agentVersion"); v != "" && v == modal {
					s.onVersion++
				}
				if agentOnline(a) && gbool(a, "isUpToDate") && agentInProtect(a) && !gbool(a, "infected") {
					s.healthy++
				}
			}
			for _, t := range threats {
				if !threatActive(t) {
					continue
				}
				s := get(threatSite(t))
				s.openThreats++
				if d, ok := daysSince(now, threatCreatedAt(t)); ok && d > s.oldestDays {
					s.oldestDays = d
				}
			}

			var rows []postureRow
			for site, a := range sites {
				if a.total == 0 {
					// Site only seen via threats (no managed agents synced).
					rows = append(rows, postureRow{Site: site, OpenThreats: a.openThreats, OldestOpenDays: a.oldestDays})
					continue
				}
				rows = append(rows, postureRow{
					Site:            site,
					Agents:          a.total,
					HealthPct:       round1(float64(a.healthy) / float64(a.total) * 100),
					CoveragePct:     round1(float64(a.protected) / float64(a.total) * 100),
					VersionComplPct: round1(float64(a.onVersion) / float64(a.total) * 100),
					OpenThreats:     a.openThreats,
					OldestOpenDays:  a.oldestDays,
				})
			}
			// Lowest health first — that is where attention goes.
			sort.SliceStable(rows, func(i, j int) bool { return rows[i].HealthPct < rows[j].HealthPct })

			if flags.asJSON {
				return flags.printJSON(cmd, map[string]any{
					"current_version": modal,
					"sites":           rows,
				})
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Tenant posture (current fleet version %s):\n\n", modal)
			fmt.Fprintf(w, "%-26s %6s %8s %9s %9s %8s %9s\n", "SITE", "AGENTS", "HEALTH", "COVERAGE", "VER-COMPL", "OPEN", "OLDEST-D")
			for _, r := range rows {
				fmt.Fprintf(w, "%-26s %6d %7.1f%% %8.1f%% %8.1f%% %8d %9d\n",
					clip(r.Site, 26), r.Agents, r.HealthPct, r.CoveragePct, r.VersionComplPct, r.OpenThreats, r.OldestOpenDays)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/sentinelone-cli/data.db)")
	return cmd
}
