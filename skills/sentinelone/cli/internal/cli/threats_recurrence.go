// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

type recurrenceGroup struct {
	Key              string   `json:"key"`
	ThreatName       string   `json:"threat_name,omitempty"`
	Occurrences      int      `json:"occurrences"`
	DistinctAgents   int      `json:"distinct_endpoints"`
	Endpoints        []string `json:"endpoints,omitempty"`
	MitigatedAndBack bool     `json:"recurred_after_mitigation"`
}

type recurrenceAgent struct {
	Endpoint    string `json:"endpoint"`
	Site        string `json:"site,omitempty"`
	Recurring   int    `json:"recurring_threats"`
	Occurrences int    `json:"total_occurrences"`
}

// newNovelThreatsRecurrenceCmd surfaces threats whose same hash/name re-appears
// across endpoints or returns on an endpoint after a prior mitigation — the
// unkilled-root-cause signal. No Get_Threats call says "4th hit of this hash,
// twice mitigated".
// pp:data-source local
func newNovelThreatsRecurrenceCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var byAgent bool
	var limit int

	cmd := &cobra.Command{
		Use:   "recurrence",
		Short: "Find threats whose hash/name recurs across endpoints or returns after mitigation",
		Long: `Group threats by file hash (falling back to name) and surface the ones that
recur — appearing on more than one endpoint, or more than once with a prior
mitigation in between (the "we cleaned it but it came back" signal of an
unkilled root cause).

Use --by-agent to instead rank endpoints by how many recurring threats they
carry (the repeat-offender / misconfigured-host view).`,
		Example: `  # Recurring threats across the fleet
  sentinelone-cli threats recurrence

  # Repeat-offender endpoints
  sentinelone-cli threats recurrence --by-agent --agent`,
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

			if !hintIfUnsynced(cmd, db, "threats") {
				hintIfStale(cmd, db, "threats", flags.maxAge)
			}

			threats, err := loadThreats(cmd.Context(), db)
			if err != nil {
				return fmt.Errorf("loading threats: %w", err)
			}
			if len(threats) == 0 {
				return honestEmptyJSON(cmd, flags,
					"No threats in the local store. Run 'sentinelone-cli sync' first.", nil)
			}

			// Group by hash (fallback name).
			type grp struct {
				name      string
				endpoints map[string]bool
				count     int
				mitigated int
				active    int
			}
			groups := map[string]*grp{}
			for _, t := range threats {
				key := threatSHA1(t)
				if key == "" {
					key = threatName(t)
				}
				if key == "" {
					continue
				}
				g := groups[key]
				if g == nil {
					g = &grp{name: threatName(t), endpoints: map[string]bool{}}
					groups[key] = g
				}
				g.count++
				if ep := threatEndpoint(t); ep != "" {
					g.endpoints[ep] = true
				}
				if threatActive(t) {
					g.active++
				} else {
					g.mitigated++
				}
			}

			if byAgent {
				// Rank endpoints by how many recurring threat-keys they carry.
				recurKeys := map[string]bool{}
				for k, g := range groups {
					if g.count > 1 || len(g.endpoints) > 1 {
						recurKeys[k] = true
					}
				}
				type agg struct {
					site        string
					recurring   map[string]bool
					occurrences int
				}
				byEp := map[string]*agg{}
				for _, t := range threats {
					key := threatSHA1(t)
					if key == "" {
						key = threatName(t)
					}
					if !recurKeys[key] {
						continue
					}
					ep := threatEndpoint(t)
					if ep == "" {
						continue
					}
					a := byEp[ep]
					if a == nil {
						a = &agg{site: orUnknown(threatSite(t)), recurring: map[string]bool{}}
						byEp[ep] = a
					}
					a.recurring[key] = true
					a.occurrences++
				}
				var rows []recurrenceAgent
				for ep, a := range byEp {
					rows = append(rows, recurrenceAgent{Endpoint: ep, Site: a.site, Recurring: len(a.recurring), Occurrences: a.occurrences})
				}
				sort.SliceStable(rows, func(i, j int) bool {
					if rows[i].Recurring != rows[j].Recurring {
						return rows[i].Recurring > rows[j].Recurring
					}
					return rows[i].Occurrences > rows[j].Occurrences
				})
				if len(rows) == 0 {
					return honestEmptyJSON(cmd, flags, "No recurring threats found — every threat hash appears once on a single endpoint.", nil)
				}
				total := len(rows)
				if limit > 0 && len(rows) > limit {
					rows = rows[:limit]
				}
				if flags.asJSON {
					return flags.printJSON(cmd, map[string]any{"total_endpoints": total, "endpoints": rows})
				}
				w := cmd.OutOrStdout()
				fmt.Fprintf(w, "Repeat-offender endpoints (%d):\n\n", total)
				fmt.Fprintf(w, "%-30s %-20s %10s %12s\n", "ENDPOINT", "SITE", "RECURRING", "OCCURRENCES")
				for _, r := range rows {
					fmt.Fprintf(w, "%-30s %-20s %10d %12d\n", clip(r.Endpoint, 30), clip(r.Site, 20), r.Recurring, r.Occurrences)
				}
				return nil
			}

			var out []recurrenceGroup
			for key, g := range groups {
				if g.count <= 1 && len(g.endpoints) <= 1 {
					continue
				}
				eps := make([]string, 0, len(g.endpoints))
				for e := range g.endpoints {
					eps = append(eps, e)
				}
				sort.Strings(eps)
				out = append(out, recurrenceGroup{
					Key:              key,
					ThreatName:       g.name,
					Occurrences:      g.count,
					DistinctAgents:   len(g.endpoints),
					Endpoints:        eps,
					MitigatedAndBack: g.mitigated > 0 && g.active > 0,
				})
			}
			if len(out) == 0 {
				return honestEmptyJSON(cmd, flags, "No recurring threats found — every threat hash appears once on a single endpoint.", nil)
			}
			sort.SliceStable(out, func(i, j int) bool {
				if out[i].Occurrences != out[j].Occurrences {
					return out[i].Occurrences > out[j].Occurrences
				}
				return out[i].DistinctAgents > out[j].DistinctAgents
			})
			total := len(out)
			if limit > 0 && len(out) > limit {
				out = out[:limit]
			}

			if flags.asJSON {
				return flags.printJSON(cmd, map[string]any{"total_recurring": total, "threats": out})
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Recurring threats (%d):\n\n", total)
			fmt.Fprintf(w, "%-34s %5s %8s %10s  %s\n", "THREAT", "HITS", "HOSTS", "RECURRED", "KEY")
			for _, g := range out {
				recurred := ""
				if g.MitigatedAndBack {
					recurred = "after-mitig"
				}
				label := g.ThreatName
				if label == "" {
					label = g.Key
				}
				fmt.Fprintf(w, "%-34s %5d %8d %10s  %s\n", clip(label, 34), g.Occurrences, g.DistinctAgents, recurred, clip(g.Key, 16))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/sentinelone-cli/data.db)")
	cmd.Flags().BoolVar(&byAgent, "by-agent", false, "Rank endpoints by recurring-threat load instead of grouping by hash")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum rows to show (0 = all)")
	return cmd
}
