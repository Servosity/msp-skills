// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

type siteRiskRow struct {
	Site           string  `json:"site"`
	RiskScore      float64 `json:"risk_score"`
	Agents         int     `json:"agents"`
	OpenThreats    int     `json:"open_threats"`
	CoverageGapPct float64 `json:"coverage_gap_pct"`
	StalePct       float64 `json:"stale_pct"`
	OldestOpenDays int     `json:"oldest_open_days"`
}

// newNovelSitesRiskCmd ranks every site/client against the others by a single
// composite risk score — open-threat density, protection-coverage gaps, stale
// agents, and the age of the oldest open threat — a cross-tenant comparison no
// single console object returns.
// pp:data-source local
func newNovelSitesRiskCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "risk",
		Short: "Rank sites by composite risk: open threats, coverage gaps, stale agents, age",
		Long: `Use this command to rank clients/sites against each other by composite risk.
Do NOT use this command for one tenant's detailed scorecard; use 'posture'
instead.

Each site is scored:

  threat density   open threats / agents in the site          × 40
  coverage gap %   non-decommissioned agents not in Protect    × 0.3
  stale %          non-decommissioned agents unseen > 7 days    × 0.2
  oldest open      age in days of the oldest open threat (≤30)  × 0.5

The highest-risk tenants sort to the top so you know where to look first.`,
		Example: `  # Rank every client by risk
  sentinelone-cli sites risk

  # JSON for a multi-tenant dashboard
  sentinelone-cli sites risk --agent`,
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

			// Multi-entity command: hint on agents, the primary entity.
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
			threats, err := loadThreats(cmd.Context(), db)
			if err != nil {
				return fmt.Errorf("loading threats: %w", err)
			}

			now := time.Now()
			type acc struct {
				agents       int
				live         int // non-decommissioned
				notInProtect int
				stale        int
				openThreats  int
				oldestDays   int
			}
			sites := map[string]*acc{}
			get := func(s string) *acc {
				s = orUnknown(s)
				a := sites[s]
				if a == nil {
					a = &acc{}
					sites[s] = a
				}
				return a
			}

			for _, a := range agents {
				s := get(agentSite(a))
				s.agents++
				if agentDecommissioned(a) {
					continue
				}
				s.live++
				if !agentInProtect(a) {
					s.notInProtect++
				}
				if d, ok := daysSince(now, gstrFirst(a, "lastActiveDate", "data.lastActiveDate")); ok && d > 7 {
					s.stale++
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

			var rows []siteRiskRow
			for site, a := range sites {
				denom := a.agents
				if denom < 1 {
					denom = 1
				}
				threatDensity := float64(a.openThreats) / float64(denom)
				// Cap density so a site visible only via threats (zero synced
				// agents, denom forced to 1) ranks high but cannot drown out
				// every real site. posture handles the zero-agent case too.
				if threatDensity > 3 {
					threatDensity = 3
				}
				coverageGapPct := 0.0
				stalePct := 0.0
				if a.live > 0 {
					coverageGapPct = round1(float64(a.notInProtect) / float64(a.live) * 100)
					stalePct = round1(float64(a.stale) / float64(a.live) * 100)
				}
				openAge := a.oldestDays
				if openAge > 30 {
					openAge = 30
				}
				risk := round1(threatDensity*40 + coverageGapPct*0.3 + stalePct*0.2 + float64(openAge)*0.5)
				rows = append(rows, siteRiskRow{
					Site:           site,
					RiskScore:      risk,
					Agents:         a.agents,
					OpenThreats:    a.openThreats,
					CoverageGapPct: coverageGapPct,
					StalePct:       stalePct,
					OldestOpenDays: a.oldestDays,
				})
			}

			sortByScoreDesc(rows, func(r siteRiskRow) float64 { return r.RiskScore })

			if flags.asJSON {
				return flags.printJSON(cmd, map[string]any{"sites": rows})
			}

			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Site risk ranking (%d sites):\n\n", len(rows))
			fmt.Fprintf(w, "%-26s %6s %7s %7s %10s %8s %9s\n",
				"SITE", "RISK", "AGENTS", "OPEN", "COV-GAP%", "STALE%", "OLDEST-D")
			for _, r := range rows {
				fmt.Fprintf(w, "%-26s %6.1f %7d %7d %9.1f%% %7.1f%% %9d\n",
					clip(r.Site, 26), r.RiskScore, r.Agents, r.OpenThreats, r.CoverageGapPct, r.StalePct, r.OldestOpenDays)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/sentinelone-cli/data.db)")
	return cmd
}
