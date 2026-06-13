// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Novel feature: stage-aging / bottleneck detection. Hand-authored against the local store.

package cli

import (
	"database/sql"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"pipedrive-pp-cli/internal/pipeintel"
)

type agingStage struct {
	StageID    int64   `json:"stage_id"`
	StageName  string  `json:"stage_name"`
	PipelineID int64   `json:"pipeline_id"`
	OpenDeals  int     `json:"open_deals"`
	MedianDays float64 `json:"median_days"`
	MaxDays    int     `json:"max_days"`
}

type agingDeal struct {
	ID              string  `json:"id"`
	Title           string  `json:"title"`
	Value           float64 `json:"value"`
	StageID         int64   `json:"stage_id"`
	StageName       string  `json:"stage_name"`
	PipelineID      int64   `json:"pipeline_id"`
	DaysInStage     int     `json:"days_in_stage"`
	StageMedianDays float64 `json:"stage_median_days"`
}

type agingResult struct {
	Stages     []agingStage `json:"stages"`
	StuckDeals []agingDeal  `json:"stuck_deals"`
}

// pp:data-source local
func newNovelAgingCmd(flags *rootFlags) *cobra.Command {
	var pipeline int
	var limit int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "aging",
		Short: "Show which deals are stuck in a stage longer than that stage's typical dwell time.",
		Long:  "Use this command to find deals dwelling past their stage's median time-in-stage (a stage-velocity / bottleneck problem).\nDo NOT use this command for open deals nobody has contacted recently regardless of stage; use 'stale' instead.",
		Example: strings.Trim(`
  pipedrive-cli aging
  pipedrive-cli aging --pipeline 1 --json
  pipedrive-cli aging --limit 20 --agent
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := pdOpenStore(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local store: %w", err)
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, "deals") {
				hintIfStale(cmd, db, "deals", flags.maxAge)
			}

			where := "d.status='open' AND (d.is_archived IS NULL OR d.is_archived=0) AND d.stage_change_time IS NOT NULL"
			var qargs []any
			if pipeline > 0 {
				where += " AND d.pipeline_id = ?"
				qargs = append(qargs, pipeline)
			}
			query := fmt.Sprintf(`
				SELECT d.id, COALESCE(d.title,''), COALESCE(d.value,0),
				       d.stage_id, COALESCE(s.name,'(unknown stage)'), d.pipeline_id,
				       CAST(julianday('now') - julianday(d.stage_change_time) AS INTEGER) AS days_in_stage
				FROM deals d
				LEFT JOIN stages s ON CAST(s.id AS INTEGER) = d.stage_id
				WHERE %s`, where)
			rows, err := db.DB().QueryContext(cmd.Context(), query, qargs...)
			if err != nil {
				return fmt.Errorf("querying aging: %w", err)
			}
			defer rows.Close()

			type rec struct {
				deal agingDeal
				days int
			}
			byStage := map[int64][]rec{}
			stageName := map[int64]string{}
			stagePipe := map[int64]int64{}
			for rows.Next() {
				var d agingDeal
				var stage, pipe sql.NullInt64
				var days int
				if err := rows.Scan(&d.ID, &d.Title, &d.Value, &stage, &d.StageName, &pipe, &days); err != nil {
					return fmt.Errorf("scanning deal: %w", err)
				}
				d.StageID = nullI64(stage)
				d.PipelineID = nullI64(pipe)
				d.DaysInStage = days
				byStage[d.StageID] = append(byStage[d.StageID], rec{d, days})
				stageName[d.StageID] = d.StageName
				stagePipe[d.StageID] = d.PipelineID
			}
			if err := rows.Err(); err != nil {
				return err
			}

			res := agingResult{Stages: []agingStage{}, StuckDeals: []agingDeal{}}
			for sid, recs := range byStage {
				daysF := make([]float64, len(recs))
				maxDays := 0
				for i, r := range recs {
					daysF[i] = float64(r.days)
					if r.days > maxDays {
						maxDays = r.days
					}
				}
				median := pipeintel.Median(daysF)
				res.Stages = append(res.Stages, agingStage{
					StageID: sid, StageName: stageName[sid], PipelineID: stagePipe[sid],
					OpenDeals: len(recs), MedianDays: median, MaxDays: maxDays,
				})
				for _, r := range recs {
					if float64(r.days) > median && r.days > 0 {
						d := r.deal
						d.StageMedianDays = median
						res.StuckDeals = append(res.StuckDeals, d)
					}
				}
			}
			sort.Slice(res.Stages, func(i, j int) bool {
				if res.Stages[i].PipelineID != res.Stages[j].PipelineID {
					return res.Stages[i].PipelineID < res.Stages[j].PipelineID
				}
				return res.Stages[i].StageID < res.Stages[j].StageID
			})
			sort.Slice(res.StuckDeals, func(i, j int) bool {
				return res.StuckDeals[i].DaysInStage > res.StuckDeals[j].DaysInStage
			})
			if limit > 0 && len(res.StuckDeals) > limit {
				res.StuckDeals = res.StuckDeals[:limit]
			}

			return emitNovel(cmd, flags, res, func(w io.Writer) {
				if len(res.Stages) == 0 {
					fmt.Fprintln(w, "No open deals with a recorded stage-change time. (Run 'sync' if the local store is empty.)")
					return
				}
				fmt.Fprintln(w, "Stage dwell times (open deals):")
				tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
				fmt.Fprintln(tw, "STAGE\tOPEN\tMEDIAN_DAYS\tMAX_DAYS")
				for _, s := range res.Stages {
					fmt.Fprintf(tw, "%s\t%d\t%.0f\t%d\n", s.StageName, s.OpenDeals, s.MedianDays, s.MaxDays)
				}
				_ = tw.Flush()
				if len(res.StuckDeals) == 0 {
					fmt.Fprintln(w, "\nNo deals dwelling past their stage median.")
					return
				}
				fmt.Fprintf(w, "\n%d deal(s) stuck past their stage median:\n", len(res.StuckDeals))
				tw2 := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
				fmt.Fprintln(tw2, "DAYS\tMEDIAN\tVALUE\tSTAGE\tTITLE")
				for _, d := range res.StuckDeals {
					fmt.Fprintf(tw2, "%d\t%.0f\t%.0f\t%s\t%s\n", d.DaysInStage, d.StageMedianDays, d.Value, d.StageName, truncateRunes(d.Title, 40))
				}
				_ = tw2.Flush()
			})
		},
	}
	cmd.Flags().IntVar(&pipeline, "pipeline", 0, "Restrict to a single pipeline ID")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum stuck deals to return (0 = no limit)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/pipedrive-cli/data.db)")
	return cmd
}
