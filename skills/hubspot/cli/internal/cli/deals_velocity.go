// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
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
		Long:        "Compute how long each open deal has sat in its current stage, plus per-stage median and p90 dwell days, from dealstage transitions captured in the local hubspot_property_history table. Requires a prior 'hubspot-cli sync --resources hubspot-deals-crm --with-history dealstage'.\n\nA deal's time in stage is known only when its newest stored dealstage history entry IS its current stage. When the stage changed after the last history sync (or the deal has no history), its time in stage is unknown: it is left out of deals and stage_stats and listed under skipped (skipped_count, skipped_by_reason), with a stderr warning naming the sync that refreshes it.\n\nUse this command for \"where do deals rot\" time-in-stage analytics.\nDo NOT use this command for a single record's full engagement timeline; use 'engagements of' instead.",
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
			if err := hsRequireCollected(cmd.Context(), db, "deals velocity", "hubspot-deals-crm"); err != nil { // hand-wired (handfixes.json: hubspot-reports-read-only-collected-properties)
				return err
			}

			view, err := queryDealsVelocity(cmd, db, pipeline, stage, limit)
			if err != nil {
				return err
			}
			if view.SkippedCount > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d open deal(s) left out of deals velocity (%d changed stage after the last dealstage history sync, %d have no dealstage history): their time in stage is unknown. Refresh with '%s'.\n",
					view.SkippedCount, view.SkippedByReason[velocitySkipStaleHistory], view.SkippedByReason[velocitySkipNoHistory], velocityHistorySync)
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

// velocitySkipped is an open deal whose time in its current stage is
// unknown, so it is in neither deals nor stage_stats.
type velocitySkipped struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Stage        string `json:"stage"`
	Reason       string `json:"reason"`
	HistoryStage string `json:"history_stage,omitempty"`
	HistoryAt    string `json:"history_at,omitempty"`
}

const (
	// velocitySkipStaleHistory: the stored dealstage history cannot date the
	// entry into the current stage: its newest entry is another stage, or it
	// was captured before the deal's last change (hs_lastmodifieddate), or the
	// deal has no readable last-change time. A deal that left its stage and
	// came back after the capture carries the same stage as an old entry.
	velocitySkipStaleHistory = "stale_history"
	// velocitySkipNoHistory: no parseable dealstage history for the deal.
	velocitySkipNoHistory = "no_history"
	velocityHistorySync   = "hubspot-cli sync --resources hubspot-deals-crm --with-history dealstage"
)

type velocityView struct {
	Deals           []velocityDeal      `json:"deals"`
	StageStats      []velocityStageStat `json:"stage_stats"`
	SkippedCount    int                 `json:"skipped_count"`
	SkippedByReason map[string]int      `json:"skipped_by_reason"`
	Skipped         []velocitySkipped   `json:"skipped"`
	Note            string              `json:"note,omitempty"`
}

func queryDealsVelocity(cmd *cobra.Command, db *store.Store, pipeline, stage string, limit int) (velocityView, error) {
	view := velocityView{
		Deals: []velocityDeal{}, StageStats: []velocityStageStat{},
		SkippedByReason: map[string]int{velocitySkipStaleHistory: 0, velocitySkipNoHistory: 0},
		Skipped:         []velocitySkipped{},
	}
	if err := requireKnownDealStates(cmd.Context(), db, "deals velocity", pipeline); err != nil {
		return view, err
	}

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
		view.Note = "no dealstage history found; run '" + velocityHistorySync + "' first"
		return view, nil
	}

	// A deal's time in stage comes only from history that describes its
	// CURRENT stage: the newest dealstage entries (every row at the deal's
	// latest parseable instant) must all carry the current stage. When any of
	// them names another stage, the stage changed after the last
	// --with-history sync and the stored history cannot date the entry into
	// the current stage; that deal is skipped and reported, never dated from
	// an older entry. The newest entries are trusted only when they were captured
	// no earlier than the deal's last change (MAX snapshot_at, the
	// hs_lastmodifieddate HubSpot reported with the history, against the
	// deal's current hs_lastmodifieddate; never the local write time): a deal
	// can leave its stage and come back after the capture, and the same stage
	// name then proves nothing. The latest instant is MAX(julianday()), never MAX() of
	// the stored text: RFC3339 with or without a fraction and the driver's
	// time.Time text do not sort chronologically as strings.
	q := `
WITH hist AS (
  SELECT object_id, COALESCE(value, '') AS value, CAST(timestamp AS TEXT) AS ts,
    ` + hsJulianSQL("timestamp") + ` AS jd
  FROM hubspot_property_history
  WHERE object_type = 'deals' AND property = 'dealstage'
),
h AS (
  -- Each deal's latest parseable dealstage instant. A deal with no
  -- parseable transition has no known entry time (reported no_history).
  SELECT object_id, MAX(` + hsJulianSQL("timestamp") + `) AS entered_jd,
    MAX(` + hsJulianSQL("snapshot_at") + `) AS captured_jd
  FROM hubspot_property_history
  WHERE object_type = 'deals' AND property = 'dealstage'
  GROUP BY object_id
  HAVING entered_jd IS NOT NULL
),
open_deals AS (
  SELECT id AS deal_id,
    COALESCE(json_extract(data, '$.properties.dealname'), '') AS deal_name,
    CAST(json_extract(data, '$.properties.amount') AS REAL) AS deal_amount,
    COALESCE(json_extract(data, '$.properties.dealstage'), '') AS deal_stage,
    ` + hsJulianSQL("json_extract(data, '$.properties.hs_lastmodifieddate')") + ` AS modified_jd
  FROM hubspot_deals_crm
  WHERE COALESCE(archived, 0) = 0
    AND NOT ` + dealClosedSQL("") + `
    AND (?1 = '' OR json_extract(data, '$.properties.pipeline') = ?1)
    AND (?2 = '' OR COALESCE(json_extract(data, '$.properties.dealstage'), '') = ?2)
)
SELECT deal_id, deal_name, deal_amount, deal_stage,
  COUNT(n.object_id) AS newest_rows,
  COALESCE(SUM(CASE WHEN n.value <> deal_stage THEN 1 ELSE 0 END), 0) AS other_stage_rows,
  MIN(CASE WHEN n.value = deal_stage THEN n.ts END) AS entered_at,
  MIN(CASE WHEN n.value <> deal_stage THEN n.value END) AS history_stage,
  MIN(CASE WHEN n.value <> deal_stage THEN n.ts END) AS history_at,
  h.entered_jd,
  CAST((julianday('now') - h.entered_jd) AS INTEGER) AS days_in_stage,
  (h.captured_jd IS NOT NULL AND modified_jd IS NOT NULL AND h.captured_jd >= modified_jd) AS captured_after_change
FROM open_deals
LEFT JOIN h ON h.object_id = deal_id
LEFT JOIN hist n ON n.object_id = deal_id AND n.jd = h.entered_jd
GROUP BY deal_id
ORDER BY h.entered_jd IS NULL, h.entered_jd ASC, deal_id`
	rows, err := db.DB().QueryContext(cmd.Context(), q, pipeline, stage)
	if err != nil {
		return view, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	// Accumulate dwell days per stage across the full (pre-limit) candidate set
	// so the stats describe the whole population, not just the rendered top-N.
	// Only deals whose history describes their current stage contribute.
	dwellByStage := map[string][]int64{}
	all := []velocityDeal{}
	skipped := []velocitySkipped{}
	for rows.Next() {
		var d velocityDeal
		var amt, enteredJD sql.NullFloat64
		var entered, historyStage, historyAt sql.NullString
		var days sql.NullInt64
		var newestRows, otherStageRows int64
		var capturedAfterChange bool
		if err := rows.Scan(&d.ID, &d.Name, &amt, &d.Stage, &newestRows, &otherStageRows, &entered, &historyStage, &historyAt, &enteredJD, &days, &capturedAfterChange); err != nil {
			return view, err
		}
		d.Amount = nullF(amt)
		switch {
		case newestRows == 0 || !enteredJD.Valid || !days.Valid:
			skipped = append(skipped, velocitySkipped{ID: d.ID, Name: d.Name, Stage: d.Stage, Reason: velocitySkipNoHistory})
			continue
		case otherStageRows > 0 || !entered.Valid || !capturedAfterChange:
			skipped = append(skipped, velocitySkipped{
				ID: d.ID, Name: d.Name, Stage: d.Stage, Reason: velocitySkipStaleHistory,
				HistoryStage: nullStr(historyStage), HistoryAt: nullStr(historyAt),
			})
			continue
		}
		d.EnteredStageAt = entered.String
		d.DaysInStage = days.Int64
		all = append(all, d)
		dwellByStage[d.Stage] = append(dwellByStage[d.Stage], d.DaysInStage)
	}
	if err := rows.Err(); err != nil {
		return view, fmt.Errorf("iterating deal velocity rows: %w", err)
	}
	view.SkippedCount = len(skipped)
	for _, sk := range skipped {
		view.SkippedByReason[sk.Reason]++
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

	// Apply the row limit to the per-deal and skipped lists only; the stats
	// and skipped_count describe the whole population.
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	if limit > 0 && len(skipped) > limit {
		skipped = skipped[:limit]
	}
	if len(all) > 0 {
		view.Deals = all
	}
	view.Skipped = skipped
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
