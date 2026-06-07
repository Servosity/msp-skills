// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"
)

// velocityRow aggregates sales-cycle metrics for one pipeline stage.
type velocityRow struct {
	Stage           string  `json:"stage"`
	Count           int     `json:"count"`
	AvgCycleDays    float64 `json:"avgCycleDays"`
	MedianCycleDays float64 `json:"medianCycleDays"`
	AvgDaysInStage  float64 `json:"avgDaysInStage"` // open opportunities only, from stageUpdatedAt
}

type velocityReport struct {
	Total           int           `json:"total"`
	Rows            []velocityRow `json:"rows"`
	AvgCycleDays    float64       `json:"avgCycleDays"`
	MedianCycleDays float64       `json:"medianCycleDays"`
	Note            string        `json:"note,omitempty"`
}

func medianOf(sorted []float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

// newNovelOpportunityVelocityCmd implements the "opportunity velocity"
// transcendence command: cycle-duration and time-in-stage aggregates.
// pp:data-source local
func newNovelOpportunityVelocityCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "velocity",
		Short: "Aggregate sales-cycle duration and time-in-stage across the opportunity pipeline",
		Long: "Use this command for sales-cycle duration and time-in-stage aggregates across the pipeline.\n" +
			"Do NOT use this command for won/lost conversion ratios; use 'opportunity winrate' instead.\n\n" +
			"Groups synced opportunities by pipeline stage and reports count, average and median sales-cycle\n" +
			"days (salesCycleDurationDays), and average days the open deals have sat in their current stage\n" +
			"(from stageUpdatedAt). The API exposes per-opportunity fields, never the aggregate. Run `sync` first.",
		Example: "  salesbuildr-cli opportunity velocity\n" +
			"  salesbuildr-cli opportunity velocity --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			opps, err := loadOpportunitiesHinted(cmd, flags, dbPath)
			if err != nil {
				return err
			}

			now := time.Now().UTC()
			type agg struct {
				cycles    []float64
				stageDays []float64
				count     int
			}
			byStage := map[string]*agg{}
			var allCycles []float64
			for _, o := range opps {
				stage := firstNonEmpty(o.Stage, "(no stage)")
				a := byStage[stage]
				if a == nil {
					a = &agg{}
					byStage[stage] = a
				}
				a.count++
				if o.SalesCycleDays > 0 {
					a.cycles = append(a.cycles, o.SalesCycleDays)
					allCycles = append(allCycles, o.SalesCycleDays)
				}
				if !o.isClosed() && !o.StageUpdatedAt.IsZero() {
					days := now.Sub(o.StageUpdatedAt).Hours() / 24
					if days >= 0 {
						a.stageDays = append(a.stageDays, days)
					}
				}
			}

			report := velocityReport{Total: len(opps), Rows: make([]velocityRow, 0, len(byStage))}
			for stage, a := range byStage {
				row := velocityRow{Stage: stage, Count: a.count}
				if len(a.cycles) > 0 {
					var sum float64
					for _, c := range a.cycles {
						sum += c
					}
					row.AvgCycleDays = sum / float64(len(a.cycles))
					sort.Float64s(a.cycles)
					row.MedianCycleDays = medianOf(a.cycles)
				}
				if len(a.stageDays) > 0 {
					var sum float64
					for _, d := range a.stageDays {
						sum += d
					}
					row.AvgDaysInStage = sum / float64(len(a.stageDays))
				}
				report.Rows = append(report.Rows, row)
			}
			sort.Slice(report.Rows, func(i, j int) bool { return report.Rows[i].Count > report.Rows[j].Count })
			if len(allCycles) > 0 {
				var sum float64
				for _, c := range allCycles {
					sum += c
				}
				report.AvgCycleDays = sum / float64(len(allCycles))
				sort.Float64s(allCycles)
				report.MedianCycleDays = medianOf(allCycles)
			}
			if len(opps) == 0 {
				report.Note = "no opportunities in local store — run `salesbuildr-cli sync` first"
			}

			w := cmd.OutOrStdout()
			if flags.asJSON || flags.agent || !isTerminal(w) {
				return printJSONFiltered(w, report, flags)
			}
			if len(opps) == 0 {
				fmt.Fprintln(w, report.Note)
				return nil
			}
			tw := newTabWriter(w)
			fmt.Fprintln(tw, "STAGE\tCOUNT\tAVG-CYCLE(d)\tMEDIAN(d)\tAVG-IN-STAGE(d)")
			for _, r := range report.Rows {
				fmt.Fprintf(tw, "%s\t%d\t%.1f\t%.1f\t%.1f\n", truncate(r.Stage, 28), r.Count, r.AvgCycleDays, r.MedianCycleDays, r.AvgDaysInStage)
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			fmt.Fprintf(w, "\noverall: avg %.1fd median %.1fd across %d opportunities\n", report.AvgCycleDays, report.MedianCycleDays, report.Total)
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to standard location)")
	return cmd
}
