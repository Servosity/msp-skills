// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature: probability-weighted pipeline forecast bucketed by close-date
// month.

package cli

import (
	"database/sql"
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"hubspot-pp-cli/internal/store"
)

// pp:data-source local
func newNovelDealsForecastCmd(flags *rootFlags) *cobra.Command {
	var pipeline string
	var month string
	var dbPath string

	cmd := &cobra.Command{
		Use:         "forecast",
		Short:       "Probability-weighted pipeline forecast bucketed by close-date month",
		Long:        "Sum amount × stage-probability across open deals grouped by close-date month (Closed Lost = 0, Closed Won = 1.0, intermediate stages use their HubSpot-provided probability).\n\nUse this command for close-month-bucketed weighted forecast totals.\nDo NOT use this command for per-stage dollar-at-risk; use 'pipeline-health' instead.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example: `  hubspot-cli deals forecast --pipeline default
  hubspot-cli deals forecast --month 2026-07`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return nil
			}
			if dbPath == "" {
				dbPath = defaultDBPath("hubspot-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "hubspot-deals-crm") {
				hintIfStale(cmd, db, "hubspot-deals-crm", flags.maxAge)
			}

			view, err := queryDealsForecast(cmd, db, pipeline, month)
			if err != nil {
				return err
			}

			if flags.asJSON {
				return flags.printJSON(cmd, view)
			}
			headers := []string{"month", "deal_count", "weighted_amount", "unweighted_amount"}
			rows := make([][]string, 0, len(view.Months))
			for _, m := range view.Months {
				rows = append(rows, []string{
					m.Month, fmt.Sprintf("%d", m.DealCount),
					formatAmount(m.WeightedAmount), formatAmount(m.UnweightedAmount),
				})
			}
			if err := flags.printTabular(cmd, headers, rows); err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(),
				"total_weighted: %s | undated: %d deals, %s weighted\n",
				formatAmount(view.TotalWeighted), view.Undated.DealCount,
				formatAmount(view.Undated.WeightedAmount))
			return nil
		},
	}
	cmd.Flags().StringVar(&pipeline, "pipeline", "", "Restrict to a single pipeline id")
	cmd.Flags().StringVar(&month, "month", "", "Filter to a single close month (YYYY-MM)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

type forecastMonth struct {
	Month            string  `json:"month"`
	DealCount        int64   `json:"deal_count"`
	WeightedAmount   float64 `json:"weighted_amount"`
	UnweightedAmount float64 `json:"unweighted_amount"`
}

type forecastUndated struct {
	DealCount      int64   `json:"deal_count"`
	WeightedAmount float64 `json:"weighted_amount"`
}

type forecastView struct {
	Months        []forecastMonth `json:"months"`
	Undated       forecastUndated `json:"undated"`
	TotalWeighted float64         `json:"total_weighted"`
}

func queryDealsForecast(cmd *cobra.Command, db *store.Store, pipeline, month string) (forecastView, error) {
	view := forecastView{Months: []forecastMonth{}}

	// Reuse deals_top.go's stageId -> probability map verbatim (same table,
	// columns, and metadata.probability fallback rules).
	stageProb, err := loadAllStageProbabilities(db)
	if err != nil {
		return view, err
	}

	// Open deals only; close month is the YYYY-MM prefix of the closedate
	// property. Deals with no closedate bucket into `undated`.
	q := `
SELECT
  COALESCE(substr(json_extract(data, '$.properties.closedate'), 1, 7), '') AS close_month,
  COALESCE(json_extract(data, '$.properties.dealstage'), '') AS stage,
  CAST(json_extract(data, '$.properties.amount') AS REAL) AS amount
FROM hubspot_deals_crm
WHERE COALESCE(archived, 0) = 0
  AND COALESCE(json_extract(data, '$.properties.dealstage'), '') NOT IN ('closedwon', 'closedlost')
  AND (? = '' OR json_extract(data, '$.properties.pipeline') = ?)`
	rows, err := db.DB().QueryContext(cmd.Context(), q, pipeline, pipeline)
	if err != nil {
		return view, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	type agg struct {
		count      int64
		weighted   float64
		unweighted float64
	}
	byMonth := map[string]*agg{}
	var undated agg
	var totalWeighted float64

	for rows.Next() {
		var closeMonth, stage string
		var amt sql.NullFloat64
		if err := rows.Scan(&closeMonth, &stage, &amt); err != nil {
			return view, err
		}
		amount := nullF(amt)
		// Closed Lost = 0, Closed Won = 1.0 are honored by the stage metadata
		// (loadAllStageProbabilities); open intermediate stages use their
		// HubSpot-provided probability, defaulting to 0 when absent.
		prob := stageProb[stage]
		weighted := amount * prob

		if closeMonth == "" {
			// When a single --month is requested, undated deals don't belong to
			// that month; skip them entirely so the totals describe the month.
			if month != "" {
				continue
			}
			undated.count++
			undated.weighted += weighted
			undated.unweighted += amount
			totalWeighted += weighted
			continue
		}
		// --month filter (single close month).
		if month != "" && closeMonth != month {
			continue
		}
		a := byMonth[closeMonth]
		if a == nil {
			a = &agg{}
			byMonth[closeMonth] = a
		}
		a.count++
		a.weighted += weighted
		a.unweighted += amount
		totalWeighted += weighted
	}
	if err := rows.Err(); err != nil {
		return view, fmt.Errorf("iterating forecast rows: %w", err)
	}

	months := make([]string, 0, len(byMonth))
	for m := range byMonth {
		months = append(months, m)
	}
	sort.Strings(months)
	out := make([]forecastMonth, 0, len(months))
	for _, m := range months {
		a := byMonth[m]
		out = append(out, forecastMonth{
			Month:            m,
			DealCount:        a.count,
			WeightedAmount:   a.weighted,
			UnweightedAmount: a.unweighted,
		})
	}
	if len(out) > 0 {
		view.Months = out
	}
	view.Undated = forecastUndated{DealCount: undated.count, WeightedAmount: undated.weighted}
	view.TotalWeighted = totalWeighted
	return view, nil
}
