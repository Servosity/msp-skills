// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

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
		Long: `Show how agent versions are distributed across the fleet right now, per OS:
the de-facto current version of each OS (the most common among live agents),
how many endpoints are behind it, and the full histogram. Windows, macOS and
Linux agents ship separate packages, so a version is only compared with
versions of the same OS. Use 'versions rollout' to see how a version is
spreading over time (requires repeated syncs).`,
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

			current := modalVersionByOS(agents)
			type vkey struct{ os, version string }
			counts := map[vkey]int{}
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
				counts[vkey{agentOS(a), v}]++
				total++
				if agentBehindCurrent(a, current) {
					behind++
				}
			}

			type vrow struct {
				OS      string  `json:"os"`
				Version string  `json:"version"`
				Count   int     `json:"count"`
				Pct     float64 `json:"pct"`
				Status  string  `json:"status"`
			}
			dist := []vrow{}
			for k, c := range counts {
				cur := current[k.os]
				status := "behind"
				switch {
				case k.version == "(unknown)" || cur == "":
					status = "unknown"
				case k.version == cur:
					status = "current"
				case compareS1Version(k.version, cur) > 0:
					status = "ahead"
				}
				pct := 0.0
				if total > 0 {
					pct = round1(float64(c) / float64(total) * 100)
				}
				dist = append(dist, vrow{OS: k.os, Version: k.version, Count: c, Pct: pct, Status: status})
			}
			sort.SliceStable(dist, func(i, j int) bool {
				if dist[i].OS != dist[j].OS {
					return dist[i].OS < dist[j].OS
				}
				return compareS1Version(dist[i].Version, dist[j].Version) > 0
			})

			if flags.asJSON {
				return flags.printJSON(cmd, map[string]any{
					"current_by_os":  current,
					"total_agents":   total,
					"behind_current": behind,
					"distribution":   dist,
				})
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Agent versions across %d agents — current is %s, %d behind:\n\n", total, formatTargets(current), behind)
			fmt.Fprintf(w, "%-15s %-18s %7s %8s  %s\n", "OS", "VERSION", "COUNT", "PCT", "STATUS")
			for _, r := range dist {
				fmt.Fprintf(w, "%-15s %-18s %7d %7.1f%%  %s\n", r.OS, r.Version, r.Count, r.Pct, r.Status)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/sentinelone-cli/data.db)")
	cmd.AddCommand(newNovelVersionsRolloutCmd(flags))
	cmd.AddCommand(newNovelVersionsPlanCmd(flags))
	return cmd
}
