// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

// newNovelCoverageCmd is the coverage parent: bare invocation prints a per-site
// protection-coverage rollup (% of agents actually in Protect mode); `coverage
// gaps` lists the specific endpoints with protection holes.
// pp:data-source local
func newNovelCoverageCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:   "coverage",
		Short: "Per-site protection coverage (% of agents in Protect mode); 'coverage gaps' lists the holes",
		Long: `Compute, per site, what fraction of non-decommissioned agents are actually
running in Protect (prevention) mode versus detect-only. This is the
"are we really protecting everyone?" rollup the console cannot produce because
it would require joining agent policy state to site membership.

Use 'coverage gaps' to list the specific endpoints with protection holes.`,
		Example: `  # Per-site coverage rollup
  sentinelone-cli coverage

  # The specific endpoints with gaps
  sentinelone-cli coverage gaps --agent`,
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

			type cov struct {
				Site      string  `json:"site"`
				Total     int     `json:"total"`
				Protected int     `json:"protected"`
				Coverage  float64 `json:"coverage_pct"`
			}
			sites := map[string]*cov{}
			var grandTotal, grandProtected int
			for _, a := range agents {
				if agentDecommissioned(a) {
					continue
				}
				site := orUnknown(agentSite(a))
				c := sites[site]
				if c == nil {
					c = &cov{Site: site}
					sites[site] = c
				}
				c.Total++
				grandTotal++
				if agentInProtect(a) {
					c.Protected++
					grandProtected++
				}
			}
			list := make([]*cov, 0, len(sites))
			for _, c := range sites {
				if c.Total > 0 {
					c.Coverage = round1(float64(c.Protected) / float64(c.Total) * 100)
				}
				list = append(list, c)
			}
			// Worst coverage first — that is where attention is needed.
			sort.SliceStable(list, func(i, j int) bool { return list[i].Coverage < list[j].Coverage })

			overall := 0.0
			if grandTotal > 0 {
				overall = round1(float64(grandProtected) / float64(grandTotal) * 100)
			}

			if flags.asJSON {
				return flags.printJSON(cmd, map[string]any{
					"overall_coverage_pct": overall,
					"agents_in_protect":    grandProtected,
					"agents_total":         grandTotal,
					"sites":                list,
				})
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Protection coverage: %.1f%% (%d/%d agents in Protect mode)\n\n", overall, grandProtected, grandTotal)
			fmt.Fprintf(w, "%-30s %6s %10s %9s\n", "SITE", "TOTAL", "PROTECTED", "COVERAGE")
			for _, c := range list {
				fmt.Fprintf(w, "%-30s %6d %10d %8.1f%%\n", clip(c.Site, 30), c.Total, c.Protected, c.Coverage)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/sentinelone-cli/data.db)")
	cmd.AddCommand(newNovelCoverageGapsCmd(flags))
	return cmd
}

// round1 rounds to one decimal place.
func round1(f float64) float64 {
	return float64(int(f*10+0.5)) / 10
}
