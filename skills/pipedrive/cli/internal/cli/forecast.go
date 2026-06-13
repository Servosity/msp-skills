// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Novel feature: weighted pipeline forecast. Hand-authored against the local store.

package cli

import (
	"database/sql"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"pipedrive-pp-cli/internal/pipeintel"
)

type forecastPipeline struct {
	PipelineID    int64   `json:"pipeline_id"`
	PipelineName  string  `json:"pipeline_name"`
	OpenDeals     int     `json:"open_deals"`
	TotalValue    float64 `json:"total_value"`
	WeightedValue float64 `json:"weighted_value"`
}

type forecastExpected struct {
	Period        string  `json:"period"`
	PeriodStart   string  `json:"period_start"`
	PeriodEnd     string  `json:"period_end"`
	Deals         int     `json:"deals"`
	Value         float64 `json:"value"`
	WeightedValue float64 `json:"weighted_value"`
}

type forecastResult struct {
	Pipelines          []forecastPipeline `json:"pipelines"`
	TotalOpenDeals     int                `json:"total_open_deals"`
	TotalValue         float64            `json:"total_value"`
	TotalWeightedValue float64            `json:"total_weighted_value"`
	ExpectedToClose    *forecastExpected  `json:"expected_to_close,omitempty"`
}

// weightedExpr weights each open deal by its own probability, falling back to
// its stage's deal_probability, then 100%.
const weightedExpr = "d.value * COALESCE(NULLIF(d.probability,0), s.deal_probability, 100) / 100.0"

// pp:data-source local
func newNovelForecastCmd(flags *rootFlags) *cobra.Command {
	var period string
	var pipeline int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "forecast",
		Short: "Weighted pipeline value (deal value times stage probability) by pipeline, plus what is expected to close this period.",
		Long: `Computes weighted pipeline value — each open deal's value times its win
probability (the deal's own probability, else its stage's deal_probability) —
grouped by pipeline, and the value expected to close within --period based on
each deal's expected close date.

Reads the local store, so run 'pipedrive-cli sync' first.`,
		Example: strings.Trim(`
  pipedrive-cli forecast
  pipedrive-cli forecast --period this-quarter
  pipedrive-cli forecast --period this-month --pipeline 1 --agent
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

			where := "d.status='open' AND (d.is_archived IS NULL OR d.is_archived=0)"
			var qargs []any
			if pipeline > 0 {
				where += " AND d.pipeline_id = ?"
				qargs = append(qargs, pipeline)
			}

			query := fmt.Sprintf(`
				SELECT d.pipeline_id, COALESCE(p.name,'(no pipeline)'),
				       COUNT(*) AS open_deals,
				       COALESCE(SUM(d.value),0) AS total_value,
				       COALESCE(SUM(%s),0) AS weighted
				FROM deals d
				LEFT JOIN stages s ON CAST(s.id AS INTEGER) = d.stage_id
				LEFT JOIN pipelines p ON CAST(p.id AS INTEGER) = d.pipeline_id
				WHERE %s
				GROUP BY d.pipeline_id
				ORDER BY weighted DESC`, weightedExpr, where)
			rows, err := db.DB().QueryContext(cmd.Context(), query, qargs...)
			if err != nil {
				return fmt.Errorf("querying forecast: %w", err)
			}
			defer rows.Close()

			res := forecastResult{Pipelines: []forecastPipeline{}}
			for rows.Next() {
				var fp forecastPipeline
				var pid sql.NullInt64
				if err := rows.Scan(&pid, &fp.PipelineName, &fp.OpenDeals, &fp.TotalValue, &fp.WeightedValue); err != nil {
					return fmt.Errorf("scanning pipeline: %w", err)
				}
				fp.PipelineID = nullI64(pid)
				res.Pipelines = append(res.Pipelines, fp)
				res.TotalOpenDeals += fp.OpenDeals
				res.TotalValue += fp.TotalValue
				res.TotalWeightedValue += fp.WeightedValue
			}
			if err := rows.Err(); err != nil {
				return err
			}

			// Expected-to-close within the period (default this-quarter).
			if period == "" {
				period = "this-quarter"
			}
			if start, end, ok := pipeintel.PeriodRange(period, time.Now().UTC()); ok {
				ewhere := where + " AND d.expected_close_date >= ? AND d.expected_close_date < ?"
				eargs := append(append([]any{}, qargs...), start.Format("2006-01-02"), end.Format("2006-01-02"))
				eq := fmt.Sprintf(`
					SELECT COUNT(*), COALESCE(SUM(d.value),0), COALESCE(SUM(%s),0)
					FROM deals d
					LEFT JOIN stages s ON CAST(s.id AS INTEGER) = d.stage_id
					WHERE %s`, weightedExpr, ewhere)
				var exp forecastExpected
				exp.Period = period
				exp.PeriodStart = start.Format("2006-01-02")
				exp.PeriodEnd = end.Format("2006-01-02")
				if err := db.DB().QueryRowContext(cmd.Context(), eq, eargs...).Scan(&exp.Deals, &exp.Value, &exp.WeightedValue); err != nil {
					return fmt.Errorf("querying expected-to-close: %w", err)
				}
				res.ExpectedToClose = &exp
			} else {
				return fmt.Errorf("unknown --period %q: try today, this-week, this-month, this-quarter, next-quarter, this-year", period)
			}

			return emitNovel(cmd, flags, res, func(w io.Writer) {
				if len(res.Pipelines) == 0 {
					fmt.Fprintln(w, "No open deals. (Run 'sync' if the local store is empty.)")
					return
				}
				fmt.Fprintf(w, "Weighted pipeline (%d open deals):\n\n", res.TotalOpenDeals)
				tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
				fmt.Fprintln(tw, "PIPELINE\tOPEN\tTOTAL_VALUE\tWEIGHTED")
				for _, p := range res.Pipelines {
					fmt.Fprintf(tw, "%s\t%d\t%.0f\t%.0f\n", p.PipelineName, p.OpenDeals, p.TotalValue, p.WeightedValue)
				}
				fmt.Fprintf(tw, "TOTAL\t%d\t%.0f\t%.0f\n", res.TotalOpenDeals, res.TotalValue, res.TotalWeightedValue)
				_ = tw.Flush()
				if e := res.ExpectedToClose; e != nil {
					fmt.Fprintf(w, "\nExpected to close %s (%s..%s): %d deals, %.0f value, %.0f weighted\n",
						e.Period, e.PeriodStart, e.PeriodEnd, e.Deals, e.Value, e.WeightedValue)
				}
			})
		},
	}
	cmd.Flags().StringVar(&period, "period", "this-quarter", "Expected-to-close window: today, this-week, this-month, this-quarter, next-quarter, this-year")
	cmd.Flags().IntVar(&pipeline, "pipeline", 0, "Restrict to a single pipeline ID")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/pipedrive-cli/data.db)")
	return cmd
}
