// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature: per-deal time-in-stage and per-stage dwell stats from
// dealstage change history.

package cli

import (
	"database/sql"
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"hubspot-pp-cli/internal/store"
)

// pp:data-source local
func newNovelDealsVelocityCmd(flags *rootFlags) *cobra.Command {
	var pipeline string
	var stage string
	var limit int
	var dbPath string

	cmd := &cobra.Command{
		Use:         "velocity",
		Short:       "Per-deal time-in-stage and per-stage dwell stats from dealstage change history",
		Long:        "Compute how long each open deal has sat in its current stage, plus per-stage median and p90 dwell days, from dealstage transitions captured in the local hubspot_property_history table. Requires a prior 'hubspot-cli sync --resources hubspot-deals-crm --with-history dealstage'.\n\nUse this command for \"where do deals rot\" time-in-stage analytics.\nDo NOT use this command for a single record's full engagement timeline; use 'engagements of' instead.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example: `  hubspot-cli deals velocity --pipeline default
  hubspot-cli deals velocity --stage qualifiedtobuy --limit 25`,
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

			view, err := queryDealsVelocity(cmd, db, pipeline, stage, limit)
			if err != nil {
				return err
			}

			if flags.asJSON {
				return flags.printJSON(cmd, view)
			}
			// Two human tables: per-deal rows, then per-stage stats.
			if view.Note != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), view.Note)
			}
			headers := []string{"id", "name", "amount", "stage", "days_in_stage", "entered_stage_at"}
			rows := make([][]string, 0, len(view.Deals))
			for _, d := range view.Deals {
				rows = append(rows, []string{
					d.ID, d.Name, formatAmount(d.Amount), d.Stage,
					fmt.Sprintf("%d", d.DaysInStage), d.EnteredStageAt,
				})
			}
			if err := flags.printTabular(cmd, headers, rows); err != nil {
				return err
			}
			statHeaders := []string{"stage", "deal_count", "median_days", "p90_days"}
			statRows := make([][]string, 0, len(view.StageStats))
			for _, s := range view.StageStats {
				statRows = append(statRows, []string{
					s.Stage, fmt.Sprintf("%d", s.DealCount),
					fmt.Sprintf("%d", s.MedianDays), fmt.Sprintf("%d", s.P90Days),
				})
			}
			return flags.printTabular(cmd, statHeaders, statRows)
		},
	}
	cmd.Flags().StringVar(&pipeline, "pipeline", "", "Filter by pipeline id")
	cmd.Flags().StringVar(&stage, "stage", "", "Filter by dealstage id")
	cmd.Flags().IntVar(&limit, "limit", 50, "Max deal rows to return")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

type velocityDeal struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Amount         float64 `json:"amount"`
	Stage          string  `json:"stage"`
	DaysInStage    int64   `json:"days_in_stage"`
	EnteredStageAt string  `json:"entered_stage_at"`
}

type velocityStageStat struct {
	Stage      string `json:"stage"`
	DealCount  int64  `json:"deal_count"`
	MedianDays int64  `json:"median_days"`
	P90Days    int64  `json:"p90_days"`
}

type velocityView struct {
	Deals      []velocityDeal      `json:"deals"`
	StageStats []velocityStageStat `json:"stage_stats"`
	Note       string              `json:"note,omitempty"`
}

func queryDealsVelocity(cmd *cobra.Command, db *store.Store, pipeline, stage string, limit int) (velocityView, error) {
	view := velocityView{Deals: []velocityDeal{}, StageStats: []velocityStageStat{}}

	// Absence check: zero dealstage history rows for deals -> empty + note.
	var historyRows int
	if err := db.DB().QueryRowContext(cmd.Context(),
		`SELECT COUNT(*) FROM hubspot_property_history WHERE object_type = 'deals' AND property = 'dealstage'`,
	).Scan(&historyRows); err != nil {
		// Missing table behaves like zero history (idempotent migrations should
		// create it, but degrade gracefully rather than error).
		historyRows = 0
	}
	if historyRows == 0 {
		view.Note = "no dealstage history found — run 'hubspot-cli sync --resources hubspot-deals-crm --with-history dealstage' first"
		return view, nil
	}

	// Each deal's latest dealstage transition (max timestamp) joined to the
	// current open deal row. days_in_current_stage = now - that timestamp.
	q := `
SELECT d.id,
  COALESCE(json_extract(d.data, '$.properties.dealname'), '') AS name,
  CAST(json_extract(d.data, '$.properties.amount') AS REAL) AS amount,
  COALESCE(json_extract(d.data, '$.properties.dealstage'), '') AS stage,
  h.entered_at AS entered_stage_at,
  -- The history timestamp may be stored either as RFC3339 ("...T..Z") or as
  -- Go's time.Time text ("2026-04-27 12:04:57.5 +0000 UTC"); both forms break
  -- julianday(). Normalize to "YYYY-MM-DD HH:MM:SS" (first 19 chars, T->space)
  -- before the date arithmetic so days_in_stage is never NULL.
  CAST((julianday('now') - julianday(replace(substr(h.entered_at, 1, 19), 'T', ' '))) AS INTEGER) AS days_in_stage
FROM hubspot_deals_crm d
JOIN (
  SELECT object_id, MAX(timestamp) AS entered_at
  FROM hubspot_property_history
  WHERE object_type = 'deals' AND property = 'dealstage'
  GROUP BY object_id
) h ON h.object_id = d.id
WHERE COALESCE(d.archived, 0) = 0
  AND COALESCE(json_extract(d.data, '$.properties.dealstage'), '') NOT IN ('closedwon', 'closedlost')
  AND (? = '' OR json_extract(d.data, '$.properties.pipeline') = ?)
  AND (? = '' OR COALESCE(json_extract(d.data, '$.properties.dealstage'), '') = ?)
ORDER BY days_in_stage DESC`
	rows, err := db.DB().QueryContext(cmd.Context(), q, pipeline, pipeline, stage, stage)
	if err != nil {
		return view, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	// Accumulate dwell days per stage across the full (pre-limit) candidate set
	// so the stats describe the whole population, not just the rendered top-N.
	dwellByStage := map[string][]int64{}
	all := []velocityDeal{}
	for rows.Next() {
		var d velocityDeal
		var amt sql.NullFloat64
		var entered sql.NullString
		if err := rows.Scan(&d.ID, &d.Name, &amt, &d.Stage, &entered, &d.DaysInStage); err != nil {
			return view, err
		}
		d.Amount = nullF(amt)
		d.EnteredStageAt = nullStr(entered)
		all = append(all, d)
		dwellByStage[d.Stage] = append(dwellByStage[d.Stage], d.DaysInStage)
	}
	if err := rows.Err(); err != nil {
		return view, fmt.Errorf("iterating deal velocity rows: %w", err)
	}

	// Per-stage median + p90 over the full population.
	stats := make([]velocityStageStat, 0, len(dwellByStage))
	for st, vals := range dwellByStage {
		stats = append(stats, velocityStageStat{
			Stage:      st,
			DealCount:  int64(len(vals)),
			MedianDays: percentileInt64(vals, 0.5),
			P90Days:    percentileInt64(vals, 0.9),
		})
	}
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].DealCount != stats[j].DealCount {
			return stats[i].DealCount > stats[j].DealCount
		}
		return stats[i].Stage < stats[j].Stage
	})
	view.StageStats = stats

	// Apply the row limit to the per-deal output only.
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	if len(all) > 0 {
		view.Deals = all
	}
	return view, nil
}

// percentileInt64 returns the value at the given percentile (0..1) using the
// nearest-rank method. Returns 0 for an empty slice. Mutates a copy, not the
// caller's slice.
func percentileInt64(vals []int64, p float64) int64 {
	if len(vals) == 0 {
		return 0
	}
	cp := make([]int64, len(vals))
	copy(cp, vals)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	if len(cp) == 1 {
		return cp[0]
	}
	// Nearest-rank: index = ceil(p * N) - 1, clamped to [0, N-1].
	rank := int(p*float64(len(cp)) + 0.999999)
	if rank < 1 {
		rank = 1
	}
	idx := rank - 1
	if idx >= len(cp) {
		idx = len(cp) - 1
	}
	return cp[idx]
}
