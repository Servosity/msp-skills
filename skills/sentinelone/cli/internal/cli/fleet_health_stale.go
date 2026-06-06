// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type staleAgent struct {
	ID           string   `json:"id"`
	ComputerName string   `json:"computer_name"`
	Site         string   `json:"site"`
	Version      string   `json:"agent_version"`
	LastActive   string   `json:"last_active_date,omitempty"`
	DaysOffline  int      `json:"days_offline"`
	Score        float64  `json:"decay_score"`
	Reasons      []string `json:"reasons"`
}

// newNovelFleetHealthStaleCmd ranks endpoints by a composite decay score the
// API cannot return: it fuses last-seen age, last-scan age, version-vs-modal,
// protection mode, and infection into one weighted rank so the riskiest,
// most-neglected agents triage first.
// pp:data-source local
func newNovelFleetHealthStaleCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit int
	var minScore float64
	var includeDecommissioned bool

	cmd := &cobra.Command{
		Use:   "stale",
		Short: "Rank endpoints by a composite decay score (last-seen, last-scan, version, protection, infection)",
		Long: `Rank agents by a composite "decay" score that fuses signals the console
exposes only as separate filters:

  + 1.0  per day since last check-in
  + 0.3  per day since last completed scan (or +20 if never scanned)
  + 25   out-of-date agent (or +15 if merely behind the fleet's modal version)
  + 20   running in detect-only (not Protect) mode
  + 50   currently infected

Decommissioned agents are excluded by default (they are expected offline).`,
		Example: `  # Top 25 most-decayed endpoints
  sentinelone-cli fleet-health stale

  # Only the worst offenders, as JSON
  sentinelone-cli fleet-health stale --min-score 50 --agent`,
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

			now := time.Now()
			modal := modalVersion(agents)

			var ranked []staleAgent
			for _, a := range agents {
				if agentDecommissioned(a) && !includeDecommissioned {
					continue
				}
				score := 0.0
				var reasons []string

				lastActive := gstr(a, "lastActiveDate")
				daysOff := 0
				if d, ok := daysSince(now, lastActive); ok {
					if d > 0 {
						daysOff = d
						score += float64(d)
						if d >= 7 {
							reasons = append(reasons, fmt.Sprintf("last seen %dd ago", d))
						}
					}
				} else {
					score += 10
					reasons = append(reasons, "no last-active date")
				}

				if d, ok := daysSince(now, gstr(a, "scanFinishedAt")); ok {
					if d > 0 {
						score += float64(d) * 0.3
						if d >= 30 {
							reasons = append(reasons, fmt.Sprintf("last scan %dd ago", d))
						}
					}
				} else {
					score += 20
					reasons = append(reasons, "never completed a scan")
				}

				version := gstr(a, "agentVersion")
				if !gbool(a, "isUpToDate") {
					score += 25
					reasons = append(reasons, "out-of-date agent")
				} else if modal != "" && version != "" && compareS1Version(version, modal) < 0 {
					score += 15
					reasons = append(reasons, "behind fleet version")
				}

				if !agentInProtect(a) {
					score += 20
					reasons = append(reasons, "detect-only mode")
				}
				if gbool(a, "infected") {
					score += 50
					reasons = append(reasons, "infected")
				}

				if score < minScore {
					continue
				}
				ranked = append(ranked, staleAgent{
					ID:           gstrFirst(a, "id", "uuid"),
					ComputerName: gstrFirst(a, "computerName", "id"),
					Site:         orUnknown(agentSite(a)),
					Version:      version,
					LastActive:   lastActive,
					DaysOffline:  daysOff,
					Score:        score,
					Reasons:      reasons,
				})
			}

			sortByScoreDesc(ranked, func(s staleAgent) float64 { return s.Score })
			total := len(ranked)
			if limit > 0 && len(ranked) > limit {
				ranked = ranked[:limit]
			}

			if len(ranked) == 0 {
				return honestEmptyJSON(cmd, flags,
					fmt.Sprintf("No agents scored at or above the decay threshold (%.0f). The fleet looks healthy.", minScore), nil)
			}

			if flags.asJSON {
				return flags.printJSON(cmd, map[string]any{
					"total_matching": total,
					"showing":        len(ranked),
					"modal_version":  modal,
					"agents":         ranked,
				})
			}

			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Most-decayed endpoints (%d shown of %d; fleet modal version %s):\n\n", len(ranked), total, modal)
			fmt.Fprintf(w, "%6s  %-26s %-18s %5s  %s\n", "SCORE", "ENDPOINT", "SITE", "DAYS", "WHY")
			for _, s := range ranked {
				fmt.Fprintf(w, "%6.0f  %-26s %-18s %5d  %s\n",
					s.Score, clip(s.ComputerName, 26), clip(s.Site, 18), s.DaysOffline, strings.Join(s.Reasons, ", "))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/sentinelone-cli/data.db)")
	cmd.Flags().IntVar(&limit, "limit", 25, "Maximum endpoints to show (0 = all)")
	cmd.Flags().Float64Var(&minScore, "min-score", 0, "Only show agents scoring at or above this decay score")
	cmd.Flags().BoolVar(&includeDecommissioned, "include-decommissioned", false, "Include decommissioned agents (excluded by default)")
	return cmd
}
