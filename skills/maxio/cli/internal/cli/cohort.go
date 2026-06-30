package cli

import (
	"database/sql"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"maxio-pp-cli/internal/revenue"
)

type cohortRow struct {
	Cohort               string  `json:"cohort"`
	SubscriptionsStarted int     `json:"subscriptions_started"`
	StillActive          int     `json:"still_active"`
	LogoRetentionPct     float64 `json:"logo_retention_pct"`
	CurrentMRRCents      int64   `json:"current_mrr_cents"`
	// ActiveNoMRR is the count of still-active subscriptions in the cohort that
	// the per-subscription MRR surface did not report (trial / dunning / free /
	// otherwise non-MRR-contributing). It explains a cohort showing active subs
	// but $0 current MRR — those subscriptions are active but contribute no MRR.
	ActiveNoMRR int `json:"active_no_mrr"`
}

type cohortView struct {
	Cohorts []cohortRow `json:"cohorts"`
	Note    string      `json:"note,omitempty"`
}

// pp:data-source local
//
// newNovelCohortCmd builds a signup-month cohort table from the locally-mirrored
// subscriptions, joined to the latest per-subscription MRR snapshot. Logo
// retention (still-active / started) is available immediately; MRR-retention
// depth (how each cohort's revenue moves over time) grows as snapshots accrue.
func newNovelCohortCmd(flags *rootFlags) *cobra.Command {
	var flagBy, dbPath string
	var flagPeriods, limit int
	cmd := &cobra.Command{
		Use:         "cohort",
		Short:       "Revenue and logo retention by signup-month cohort across N periods.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example:     "  maxio-cli cohort\n  maxio-cli cohort --limit 12 --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			_ = flagBy      // currently only signup-month is supported
			_ = flagPeriods // reserved: depth of MRR-over-time once snapshots accrue
			st, db, err := openRevenueStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer st.Close()

			// Latest per-sub snapshot drives current MRR. May be NULL (no sync yet)
			// — cohorts still render with logo retention and MRR 0 in that case.
			var latest sql.NullString
			_ = db.QueryRowContext(cmd.Context(), `SELECT MAX(snapshot_at) FROM rev_sub_mrr_snapshots`).Scan(&latest)

			// 0 (or negative) means "all", matching the other revenue commands;
			// SQLite treats LIMIT -1 as unlimited.
			effectiveLimit := limit
			if effectiveLimit <= 0 {
				effectiveLimit = -1
			}
			rows, qerr := db.QueryContext(cmd.Context(), `
				SELECT substr(json_extract(sub.data,'$.created_at'),1,7) AS cohort,
				       COUNT(*) AS started,
				       SUM(CASE WHEN json_extract(sub.data,'$.state')='active' THEN 1 ELSE 0 END) AS still_active,
				       SUM(CASE WHEN json_extract(sub.data,'$.state')='active' THEN COALESCE(s.mrr_cents,0) ELSE 0 END) AS mrr_cents,
				       SUM(CASE WHEN json_extract(sub.data,'$.state')='active' AND s.subscription_id IS NULL THEN 1 ELSE 0 END) AS active_no_mrr
				FROM resources sub
				LEFT JOIN rev_sub_mrr_snapshots s
				  ON s.subscription_id = sub.id AND s.snapshot_at = ?
				WHERE sub.resource_type='subscriptions-json'
				  AND substr(json_extract(sub.data,'$.created_at'),1,7) <> ''
				  AND json_extract(sub.data,'$.created_at') IS NOT NULL
				GROUP BY cohort
				ORDER BY cohort DESC
				LIMIT ?`, latest.String, effectiveLimit)
			if qerr != nil {
				return fmt.Errorf("querying cohorts: %w", qerr)
			}
			defer rows.Close()

			view := cohortView{Cohorts: []cohortRow{}}
			for rows.Next() {
				var cohort sql.NullString
				var started, active sql.NullInt64
				var mrr sql.NullInt64
				var activeNoMRR sql.NullInt64
				if err := rows.Scan(&cohort, &started, &active, &mrr, &activeNoMRR); err != nil {
					continue
				}
				r := cohortRow{
					Cohort:               cohort.String,
					SubscriptionsStarted: int(started.Int64),
					StillActive:          int(active.Int64),
					CurrentMRRCents:      mrr.Int64,
					ActiveNoMRR:          int(activeNoMRR.Int64),
				}
				if r.SubscriptionsStarted > 0 {
					r.LogoRetentionPct = round2(float64(r.StillActive) / float64(r.SubscriptionsStarted) * 100)
				}
				view.Cohorts = append(view.Cohorts, r)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("scanning cohorts: %w", err)
			}

			if !latest.Valid || latest.String == "" {
				hintRunMrrSync(cmd.ErrOrStderr())
				view.Note = "no MRR snapshots yet — logo retention shown; current MRR is 0 until `maxio-cli mrr sync` runs"
			} else {
				view.Note = "current_mrr_cents reflects the latest snapshot; MRR-retention-over-time depth grows as more snapshots accrue. active_no_mrr counts still-active subs the per-subscription MRR surface did not report (trial / dunning / free / non-recurring) — a cohort with active subs but $0 current MRR is explained by active_no_mrr, not a missing join"
			}

			return emitRevenue(cmd, flags, view, func(w io.Writer) {
				fmt.Fprintf(w, "%-9s %8s %8s %8s %9s %14s\n", "cohort", "started", "active", "no_mrr", "logo%", "current_mrr")
				for _, r := range view.Cohorts {
					fmt.Fprintf(w, "%-9s %8d %8d %8d %8.1f%% %14s\n", r.Cohort,
						r.SubscriptionsStarted, r.StillActive, r.ActiveNoMRR, r.LogoRetentionPct, revenue.Cents(r.CurrentMRRCents))
				}
				if view.Note != "" {
					fmt.Fprintf(w, "note: %s\n", view.Note)
				}
			})
		},
	}
	cmd.Flags().StringVar(&flagBy, "by", "signup-month", "Cohort dimension (currently only signup-month)")
	cmd.Flags().IntVar(&flagPeriods, "periods", 12, "Number of trailing periods of MRR retention to track (reserved; grows as snapshots accrue)")
	cmd.Flags().IntVar(&limit, "limit", 24, "Max cohorts to return (newest first)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
