// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type triageRow struct {
	Rank     int     `json:"rank"`
	Score    float64 `json:"score"`
	Threat   string  `json:"threat"`
	SHA1     string  `json:"sha1,omitempty"`
	Endpoint string  `json:"endpoint,omitempty"`
	Site     string  `json:"site,omitempty"`
	Verdict  string  `json:"verdict,omitempty"`
	Incident string  `json:"incident_status,omitempty"`
	AgeDays  int     `json:"age_days"`
}

// newNovelThreatsTriageCmd ranks every open threat across all sites into one
// worklist by fusing confidence, incident status, and age into a single score
// the console cannot return — older, higher-confidence, unresolved threats rise
// to the top so the analyst works the riskiest first.
// pp:data-source local
func newNovelThreatsTriageCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit int
	var site string

	cmd := &cobra.Command{
		Use:   "triage",
		Short: "Ranked cross-site triage worklist of open threats (confidence × severity × age)",
		Long: `Use this command for the daily ranked worklist of open threats across all
sites. Do NOT use this command to scope one threat's spread (use
'threats blast-radius') or to find recurring threats (use 'threats
recurrence').

Each open threat is scored:

  confidence weight   malicious = 3, suspicious = 2, other/unknown = 1
  incident weight     unresolved = 2, in_progress = 1.5, other = 1
  age factor          (days-open + 1, capped at 30) / 10

  score = confidence × incident × (1 + age factor)

So an older, high-confidence, still-unresolved threat outranks a fresh,
low-confidence one. Resolved and mitigated threats are excluded.`,
		Example: `  # Top of the daily worklist across every site
  sentinelone-cli threats triage

  # Worklist for one site, as JSON
  sentinelone-cli threats triage --site Acme --agent`,
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
					"No threats in the local store. Run 'sentinelone-cli sync --resources threats' first.", nil)
			}

			now := time.Now()
			type scored struct {
				row   triageRow
				score float64
			}
			var open []scored
			for _, t := range threats {
				if !threatActive(t) {
					continue
				}
				if site != "" && !strings.EqualFold(threatSite(t), site) {
					continue
				}

				confidenceW := 1.0
				switch strings.ToLower(threatConfidence(t)) {
				case "malicious":
					confidenceW = 3
				case "suspicious":
					confidenceW = 2
				}

				incidentW := 1.0
				switch strings.ToLower(threatIncident(t)) {
				case "unresolved":
					incidentW = 2
				case "in_progress":
					incidentW = 1.5
				}

				ageDays := 0
				if d, ok := daysSince(now, threatCreatedAt(t)); ok && d > 0 {
					ageDays = d
				}
				capped := ageDays + 1
				if capped > 30 {
					capped = 30
				}
				ageFactor := float64(capped) / 10
				sc := confidenceW * incidentW * (1 + ageFactor)

				open = append(open, scored{
					score: sc,
					row: triageRow{
						Score:    round1(sc),
						Threat:   threatName(t),
						SHA1:     clip(threatSHA1(t), 12),
						Endpoint: threatEndpoint(t),
						Site:     orUnknown(threatSite(t)),
						Verdict:  threatVerdict(t),
						Incident: threatIncident(t),
						AgeDays:  ageDays,
					},
				})
			}

			if len(open) == 0 {
				reason := "No open threats in the local store. The fleet looks clear."
				if site != "" {
					reason = fmt.Sprintf("No open threats for site %q in the local store.", site)
				}
				return honestEmptyJSON(cmd, flags, reason, map[string]any{"open_threats": 0})
			}

			sortByScoreDesc(open, func(s scored) float64 { return s.score })
			total := len(open)
			if limit > 0 && len(open) > limit {
				open = open[:limit]
			}

			rows := make([]triageRow, 0, len(open))
			for i := range open {
				open[i].row.Rank = i + 1
				rows = append(rows, open[i].row)
			}

			if flags.asJSON {
				return flags.printJSON(cmd, map[string]any{
					"open_threats": total,
					"showing":      len(rows),
					"items":        rows,
				})
			}

			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Open-threat triage worklist (%d shown of %d open):\n\n", len(rows), total)
			fmt.Fprintf(w, "%4s %7s  %-28s %-13s %-22s %-16s %-10s %-12s %5s\n",
				"RANK", "SCORE", "THREAT", "SHA1", "ENDPOINT", "SITE", "VERDICT", "INCIDENT", "AGE-D")
			for _, r := range rows {
				fmt.Fprintf(w, "%4d %7.1f  %-28s %-13s %-22s %-16s %-10s %-12s %5d\n",
					r.Rank, r.Score, clip(r.Threat, 28), r.SHA1, clip(r.Endpoint, 22),
					clip(r.Site, 16), clip(orUnknown(r.Verdict), 10), clip(orUnknown(r.Incident), 12), r.AgeDays)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/sentinelone-cli/data.db)")
	cmd.Flags().IntVar(&limit, "limit", 20, "Maximum threats to show (0 = all)")
	cmd.Flags().StringVar(&site, "site", "", "Only triage threats for this site (case-insensitive)")
	return cmd
}
