package cli

import (
	"database/sql"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"maxio-pp-cli/internal/revenue"
)

type cohortRow struct {
	Cohort               string `json:"cohort"`
	SubscriptionsStarted int    `json:"subscriptions_started"`
	// StillActive counts the cohort's subscriptions still current (every live
	// or problem state, revenue.CurrentStates).
	StillActive int `json:"still_active"`
	// CustomersStarted are the customers whose FIRST subscription was created
	// in the cohort month; CustomersRetained those of them with a current
	// subscription now. Logo retention is customers, never subscriptions
	// (Codex r1, maxio 4.34.2 reprint, class subscription-counted-as-logo).
	CustomersStarted  int     `json:"customers_started"`
	CustomersRetained int     `json:"customers_retained"`
	LogoRetentionPct  float64 `json:"logo_retention_pct"`
	CurrentMRRCents   int64   `json:"current_mrr_cents"`
	// ActiveNoMRR is the count of still-active subscriptions in the cohort that
	// the per-subscription MRR surface did not report (trial / dunning / free /
	// otherwise non-MRR-contributing). It explains a cohort showing active subs
	// but $0 current MRR — those subscriptions are active but contribute no MRR.
	ActiveNoMRR int `json:"active_no_mrr"`
}

type cohortView struct {
	Currency string      `json:"currency,omitempty"`
	Cohorts  []cohortRow `json:"cohorts"`
	Note     string      `json:"note,omitempty"`
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
			currency, money := revenueMoney(cmd.Context(), db)
			states := revenue.CurrentStatesArg()

			// 0 (or negative) means "all", matching the other revenue commands;
			// SQLite treats LIMIT -1 as unlimited.
			effectiveLimit := limit
			if effectiveLimit <= 0 {
				effectiveLimit = -1
			}
			rows, qerr := db.QueryContext(cmd.Context(), `
				SELECT substr(json_extract(sub.data,'$.created_at'),1,7) AS cohort,
				       COUNT(*) AS started,
				       SUM(CASE WHEN LOWER(COALESCE(json_extract(sub.data,'$.state'),'')) IN (SELECT value FROM json_each(?)) THEN 1 ELSE 0 END) AS still_active,
				       SUM(CASE WHEN LOWER(COALESCE(json_extract(sub.data,'$.state'),'')) IN (SELECT value FROM json_each(?)) THEN COALESCE(s.mrr_cents,0) ELSE 0 END) AS mrr_cents,
				       SUM(CASE WHEN LOWER(COALESCE(json_extract(sub.data,'$.state'),'')) IN (SELECT value FROM json_each(?)) AND s.subscription_id IS NULL THEN 1 ELSE 0 END) AS active_no_mrr
				FROM resources sub
				LEFT JOIN rev_sub_mrr_snapshots s
				  ON s.subscription_id = sub.id AND s.snapshot_at = ?
				WHERE sub.resource_type='subscriptions-json'
				  AND substr(json_extract(sub.data,'$.created_at'),1,7) <> ''
				  AND json_extract(sub.data,'$.created_at') IS NOT NULL
				GROUP BY cohort
				ORDER BY cohort DESC
				LIMIT ?`, states, states, states, latest.String, effectiveLimit)
			if qerr != nil {
				return fmt.Errorf("querying cohorts: %w", qerr)
			}
			defer rows.Close()

			view := cohortView{Currency: currency, Cohorts: []cohortRow{}}
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
				view.Cohorts = append(view.Cohorts, r)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("scanning cohorts: %w", err)
			}

			// Logos: each customer joins the cohort of its first subscription's
			// creation month and is retained while any subscription is current.
			logoRows, lerr := db.QueryContext(cmd.Context(), `
				SELECT substr(MIN(json_extract(sub.data,'$.created_at')),1,7) AS cohort,
				       MAX(CASE WHEN LOWER(COALESCE(json_extract(sub.data,'$.state'),'')) IN (SELECT value FROM json_each(?)) THEN 1 ELSE 0 END) AS retained
				FROM resources sub
				WHERE sub.resource_type='subscriptions-json'
				  AND json_extract(sub.data,'$.created_at') IS NOT NULL
				  AND substr(json_extract(sub.data,'$.created_at'),1,7) <> ''
				  AND json_extract(sub.data,'$.customer.id') IS NOT NULL
				GROUP BY CAST(json_extract(sub.data,'$.customer.id') AS TEXT)`, states)
			if lerr != nil {
				return fmt.Errorf("querying cohort customers: %w", lerr)
			}
			defer logoRows.Close()
			started, retained := map[string]int{}, map[string]int{}
			for logoRows.Next() {
				var cohort sql.NullString
				var kept sql.NullInt64
				if err := logoRows.Scan(&cohort, &kept); err != nil {
					return fmt.Errorf("scanning cohort customers: %w", err)
				}
				started[cohort.String]++
				if kept.Int64 > 0 {
					retained[cohort.String]++
				}
			}
			if err := logoRows.Err(); err != nil {
				return fmt.Errorf("scanning cohort customers: %w", err)
			}
			for i := range view.Cohorts {
				r := &view.Cohorts[i]
				r.CustomersStarted = started[r.Cohort]
				r.CustomersRetained = retained[r.Cohort]
				if r.CustomersStarted > 0 {
					r.LogoRetentionPct = round2(float64(r.CustomersRetained) / float64(r.CustomersStarted) * 100)
				}
			}

			if !latest.Valid || latest.String == "" {
				hintRunMrrSync(cmd.ErrOrStderr())
				view.Note = "no MRR snapshots yet — logo retention shown; current MRR is 0 until `maxio-cli mrr sync` runs. Logo retention counts customers by their first subscription's month (retained = any subscription still current)"
			} else {
				view.Note = "logo_retention_pct counts customers by their first subscription's month (retained = any subscription still current, live or problem state); subscriptions_started/still_active count the month's subscriptions. current_mrr_cents reflects the latest snapshot; MRR-retention-over-time depth grows as more snapshots accrue. active_no_mrr counts still-active subs the per-subscription MRR surface did not report (trial / dunning / free / non-recurring) — a cohort with active subs but $0 current MRR is explained by active_no_mrr, not a missing join"
			}

			return emitRevenue(cmd, flags, view, func(w io.Writer) {
				fmt.Fprintf(w, "%-9s %8s %8s %8s %8s %8s %9s %14s\n", "cohort", "subs", "current", "no_mrr", "logos", "kept", "logo%", "current_mrr")
				for _, r := range view.Cohorts {
					fmt.Fprintf(w, "%-9s %8d %8d %8d %8d %8d %8.1f%% %14s\n", r.Cohort,
						r.SubscriptionsStarted, r.StillActive, r.ActiveNoMRR, r.CustomersStarted, r.CustomersRetained, r.LogoRetentionPct, money(r.CurrentMRRCents))
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
