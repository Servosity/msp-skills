// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

// newNovelVersionsCmd is the versions parent: bare invocation prints the
// current agent-version distribution across the fleet (which is "current" and
// how many are behind); `versions rollout` shows the per-site progression over
// time built from history snapshots.
// pp:data-source local
func newNovelVersionsCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:   "versions",
		Short: "Agent-version distribution across the fleet; 'versions rollout' shows progress over time",
		Long: `Show how agent versions are distributed across the fleet right now: the
de-facto current version (the most common), how many endpoints are behind it,
and the full histogram. Use 'versions rollout' to see how a version is
spreading per site over time (requires repeated syncs).`,
		Example: `  # Current version spread
  sentinelone-cli versions

  # Rollout progress over time
  sentinelone-cli versions rollout`,
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

			modal := modalVersion(agents)
			counts := map[string]int{}
			total := 0
			behind := 0
			for _, a := range agents {
				if agentDecommissioned(a) {
					continue
				}
				v := gstr(a, "agentVersion")
				if v == "" {
					v = "(unknown)"
				}
				counts[v]++
				total++
				if v != "(unknown)" && modal != "" && compareS1Version(v, modal) < 0 {
					behind++
				}
			}

			type vrow struct {
				Version string  `json:"version"`
				Count   int     `json:"count"`
				Pct     float64 `json:"pct"`
				Status  string  `json:"status"`
			}
			var dist []vrow
			for v, c := range counts {
				status := "behind"
				switch {
				case v == "(unknown)":
					status = "unknown"
				case v == modal:
					status = "current"
				case compareS1Version(v, modal) > 0:
					status = "ahead"
				}
				pct := 0.0
				if total > 0 {
					pct = round1(float64(c) / float64(total) * 100)
				}
				dist = append(dist, vrow{Version: v, Count: c, Pct: pct, Status: status})
			}
			sort.SliceStable(dist, func(i, j int) bool { return compareS1Version(dist[i].Version, dist[j].Version) > 0 })

			if flags.asJSON {
				return flags.printJSON(cmd, map[string]any{
					"current_version": modal,
					"total_agents":    total,
					"behind_current":  behind,
					"distribution":    dist,
				})
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Agent versions across %d agents — current is %s, %d behind:\n\n", total, modal, behind)
			fmt.Fprintf(w, "%-18s %7s %8s  %s\n", "VERSION", "COUNT", "PCT", "STATUS")
			for _, r := range dist {
				fmt.Fprintf(w, "%-18s %7d %7.1f%%  %s\n", r.Version, r.Count, r.Pct, r.Status)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/sentinelone-cli/data.db)")
	cmd.AddCommand(newNovelVersionsRolloutCmd(flags))
	return cmd
}
