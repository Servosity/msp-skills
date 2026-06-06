// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"
)

type mttrRow struct {
	Site             string  `json:"site"`
	Mitigated        int     `json:"mitigated"`
	MeanHours        float64 `json:"mean_mttr_hours"`
	Active           int     `json:"active"`
	OldestActiveDays int     `json:"oldest_active_days"`
	SLABreaches      int     `json:"sla_breaches"`
}

// newNovelThreatsMttrCmd derives mean time-to-mitigate per site from detection
// and mitigation timestamps across the threat history, flags SLA breaches, and
// surfaces the longest-unresolved threats — a duration metric no single
// endpoint computes.
// pp:data-source local
func newNovelThreatsMttrCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var slaHours float64

	cmd := &cobra.Command{
		Use:   "mttr",
		Short: "Mean time-to-mitigate per site, SLA breaches, and the longest-unresolved threats",
		Long: `Compute, per site, the mean time from threat detection (createdAt) to
mitigation (the threat's last update once mitigated), plus how many active
threats remain and the age of the oldest. Threats whose time-to-mitigate
exceeded the --sla window, and active threats older than it, count as breaches.`,
		Example: `  # MTTR with a 24h SLA (default)
  sentinelone-cli threats mttr

  # Tighten the SLA to 4 hours, as JSON
  sentinelone-cli threats mttr --sla 4 --agent`,
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

			now := time.Now()
			type acc struct {
				sumHours    float64
				mitigated   int
				measured    int // mitigated threats with usable timestamps; the mean's denominator
				active      int
				oldestDays  int
				slaBreaches int
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
			var totMitigated, totMeasured int
			var totSumHours float64
			for _, t := range threats {
				a := get(threatSite(t))
				created, hasCreated := parseS1Time(threatCreatedAt(t))
				if threatActive(t) {
					a.active++
					if hasCreated {
						d := int(now.Sub(created).Hours() / 24)
						if d > a.oldestDays {
							a.oldestDays = d
						}
						if now.Sub(created).Hours() > slaHours {
							a.slaBreaches++
						}
					}
					continue
				}
				// Mitigated: duration = last update - detection.
				mitigatedAt, hasMit := parseS1Time(threatUpdatedAt(t))
				if hasCreated && hasMit && mitigatedAt.After(created) {
					h := mitigatedAt.Sub(created).Hours()
					a.sumHours += h
					a.mitigated++
					a.measured++
					totSumHours += h
					totMitigated++
					totMeasured++
					if h > slaHours {
						a.slaBreaches++
					}
				} else {
					// Mitigated but no usable timestamps; count it without a duration.
					a.mitigated++
					totMitigated++
				}
			}

			var rows []mttrRow
			for site, a := range sites {
				mean := 0.0
				if a.measured > 0 && a.sumHours > 0 {
					// Mean over threats with measured durations only; mitigated
					// threats lacking timestamps must not dilute the MTTR.
					mean = round1(a.sumHours / float64(a.measured))
				}
				rows = append(rows, mttrRow{
					Site:             site,
					Mitigated:        a.mitigated,
					MeanHours:        mean,
					Active:           a.active,
					OldestActiveDays: a.oldestDays,
					SLABreaches:      a.slaBreaches,
				})
			}
			// Worst (most active / breaches) first.
			sort.SliceStable(rows, func(i, j int) bool {
				if rows[i].SLABreaches != rows[j].SLABreaches {
					return rows[i].SLABreaches > rows[j].SLABreaches
				}
				return rows[i].Active > rows[j].Active
			})

			overallMean := 0.0
			if totMeasured > 0 && totSumHours > 0 {
				overallMean = round1(totSumHours / float64(totMeasured))
			}

			if flags.asJSON {
				return flags.printJSON(cmd, map[string]any{
					"sla_hours":          slaHours,
					"overall_mttr_hours": overallMean,
					"total_mitigated":    totMitigated,
					"sites":              rows,
				})
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Threat MTTR (SLA %.0fh) — overall mean %.1fh across %d mitigated:\n\n", slaHours, overallMean, totMitigated)
			fmt.Fprintf(w, "%-26s %10s %10s %8s %10s %8s\n", "SITE", "MITIGATED", "MEAN-HRS", "ACTIVE", "OLDEST-D", "BREACH")
			for _, r := range rows {
				fmt.Fprintf(w, "%-26s %10d %10.1f %8d %10d %8d\n",
					clip(r.Site, 26), r.Mitigated, r.MeanHours, r.Active, r.OldestActiveDays, r.SLABreaches)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/sentinelone-cli/data.db)")
	cmd.Flags().Float64Var(&slaHours, "sla", 24, "SLA window in hours; threats exceeding it count as breaches")
	return cmd
}
