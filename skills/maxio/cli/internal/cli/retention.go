package cli

import (
	"database/sql"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"maxio-pp-cli/internal/revenue"
)

type retentionPeriod struct {
	Period          string   `json:"period"`
	StartingMRR     int64    `json:"starting_mrr_cents"`
	NewCents        int64    `json:"new_cents"`
	ExpansionCents  int64    `json:"expansion_cents"`
	ContractionCent int64    `json:"contraction_cents"`
	ChurnCents      int64    `json:"churn_cents"`
	ReactivCents    int64    `json:"reactivation_cents"`
	EndingMRR       int64    `json:"ending_mrr_cents"`
	NRRPct          *float64 `json:"nrr_pct,omitempty"`
	GRRPct          *float64 `json:"grr_pct,omitempty"`
	RevenueChurnPct *float64 `json:"revenue_churn_pct,omitempty"`
	QuickRatio      *float64 `json:"quick_ratio,omitempty"`
}

type retentionView struct {
	Since   string            `json:"since,omitempty"`
	GroupBy string            `json:"group_by"`
	Periods []retentionPeriod `json:"periods"`
}

// pp:data-source local
//
// newNovelRetentionCmd computes revenue-retention metrics (NRR / GRR / revenue
// churn / quick ratio) per period from the locally-stored MRR movement log.
// Starting MRR for a period is the cumulative signed sum of every movement
// (including the initial "imported" book of business) strictly before the
// period boundary, so retention ratios are anchored to a true point-in-time
// MRR rather than only the movements inside the window.
func newNovelRetentionCmd(flags *rootFlags) *cobra.Command {
	var flagSince, flagGroupBy, dbPath string
	cmd := &cobra.Command{
		Use:         "retention",
		Short:       "Net and gross revenue retention, logo churn, revenue churn, and quick ratio over any window.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example:     "  maxio-cli retention --since 12m --group-by month\n  maxio-cli retention --since 2026-01-01 --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			groupBy := flagGroupBy
			if groupBy == "" {
				groupBy = "month"
			}
			switch groupBy {
			case "month", "week", "day":
			default:
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--group-by must be month, week, or day"))
			}
			cutoff, err := revenue.ParseSince(flagSince, time.Now())
			if err != nil {
				return usageErr(err)
			}
			st, db, err := openRevenueStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer st.Close()

			if has, _ := revenue.HasMovements(cmd.Context(), db); !has {
				hintRunMrrSync(cmd.ErrOrStderr())
				return emitRevenue(cmd, flags, retentionView{Since: cutoff, GroupBy: groupBy, Periods: []retentionPeriod{}}, func(w io.Writer) {
					fmt.Fprintln(w, "No MRR movements stored yet.")
				})
			}

			// Pull bucketed movements within the window, keyed by period. Track
			// the earliest ts in each period so the starting-MRR boundary query
			// has a concrete date to compare against (handles week/day cleanly).
			rows, qerr := movementsSince(cmd, db, cutoff)
			if qerr != nil {
				return qerr
			}
			defer rows.Close()

			type agg struct {
				new, expansion, contraction, churn, reactiv int64
				firstTS                                     string
			}
			byPeriod := map[string]*agg{}
			var order []string
			for rows.Next() {
				var ts, bucket string
				var amt int64
				if err := rows.Scan(&ts, &bucket, &amt); err != nil {
					continue
				}
				key := periodKey(ts, groupBy)
				a := byPeriod[key]
				if a == nil {
					a = &agg{firstTS: ts}
					byPeriod[key] = a
					order = append(order, key)
				}
				if ts < a.firstTS {
					a.firstTS = ts
				}
				switch bucket {
				case revenue.BucketNew:
					a.new += amt
				case revenue.BucketExpansion:
					a.expansion += amt
				case revenue.BucketContraction:
					a.contraction += amt
				case revenue.BucketChurn:
					a.churn += amt
				case revenue.BucketReactivation:
					a.reactiv += amt
				}
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("scanning movements: %w", err)
			}

			sort.Strings(order)
			view := retentionView{Since: cutoff, GroupBy: groupBy, Periods: make([]retentionPeriod, 0, len(order))}
			for _, key := range order {
				a := byPeriod[key]
				boundary := periodStartDate(key, groupBy, a.firstTS)
				var starting sql.NullInt64
				_ = db.QueryRowContext(cmd.Context(),
					`SELECT COALESCE(SUM(amount_cents),0) FROM rev_mrr_movements WHERE ts < ?`, boundary).Scan(&starting)
				start := starting.Int64

				p := retentionPeriod{
					Period:          key,
					StartingMRR:     start,
					NewCents:        a.new,
					ExpansionCents:  a.expansion,
					ContractionCent: a.contraction,
					ChurnCents:      a.churn,
					ReactivCents:    a.reactiv,
				}
				p.EndingMRR = start + a.new + a.expansion + a.contraction + a.churn + a.reactiv

				if start != 0 {
					fStart := float64(start)
					// NRR excludes new business; includes expansion/contraction/churn/reactivation.
					nrr := round2((fStart + float64(a.expansion+a.contraction+a.churn+a.reactiv)) / fStart * 100)
					grr := round2((fStart + float64(a.contraction+a.churn)) / fStart * 100)
					revChurn := round2(-float64(a.contraction+a.churn) / fStart * 100)
					p.NRRPct = &nrr
					p.GRRPct = &grr
					p.RevenueChurnPct = &revChurn
				}
				if denom := -(a.contraction + a.churn); denom > 0 {
					qr := round2(float64(a.new+a.expansion) / float64(denom))
					p.QuickRatio = &qr
				}
				view.Periods = append(view.Periods, p)
			}

			return emitRevenue(cmd, flags, view, func(w io.Writer) {
				fmt.Fprintf(w, "%-10s %12s %12s %10s %10s %10s\n", "period", "start_mrr", "end_mrr", "nrr%", "grr%", "quick")
				for _, p := range view.Periods {
					fmt.Fprintf(w, "%-10s %12s %12s %10s %10s %10s\n", p.Period,
						revenue.Cents(p.StartingMRR), revenue.Cents(p.EndingMRR),
						pctStr(p.NRRPct), pctStr(p.GRRPct), ratioStr(p.QuickRatio))
				}
			})
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "", "Only movements on/after this date (YYYY-MM-DD or window like 12m, 90d, 4w)")
	cmd.Flags().StringVar(&flagGroupBy, "group-by", "month", "Period granularity: month, week, or day")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

// movementsSince returns ts/bucket/amount_cents rows ordered by ts, optionally
// filtered to a cutoff date string.
func movementsSince(cmd *cobra.Command, db *sql.DB, cutoff string) (*sql.Rows, error) {
	if cutoff != "" {
		r, err := db.QueryContext(cmd.Context(),
			`SELECT ts, bucket, amount_cents FROM rev_mrr_movements WHERE ts >= ? ORDER BY ts`, cutoff)
		if err != nil {
			return nil, fmt.Errorf("querying movements: %w", err)
		}
		return r, nil
	}
	r, err := db.QueryContext(cmd.Context(),
		`SELECT ts, bucket, amount_cents FROM rev_mrr_movements ORDER BY ts`)
	if err != nil {
		return nil, fmt.Errorf("querying movements: %w", err)
	}
	return r, nil
}

// periodStartDate derives the first-day boundary date for a period label so the
// starting-MRR query has a concrete `ts < <date>` cutoff. For week/day the
// period's earliest observed ts is the simplest correct boundary.
func periodStartDate(periodKey, groupBy, firstTS string) string {
	switch groupBy {
	case "month":
		if len(periodKey) == 7 { // YYYY-MM
			return periodKey + "-01"
		}
		return firstTSDate(firstTS)
	case "day":
		if len(periodKey) == 10 { // YYYY-MM-DD
			return periodKey
		}
		return firstTSDate(firstTS)
	default: // week
		return firstTSDate(firstTS)
	}
}

func firstTSDate(ts string) string {
	if len(ts) >= 10 {
		return ts[:10]
	}
	return ts
}

func pctStr(p *float64) string {
	if p == nil {
		return "—"
	}
	return fmt.Sprintf("%.1f%%", *p)
}

func ratioStr(p *float64) string {
	if p == nil {
		return "—"
	}
	return fmt.Sprintf("%.2f", *p)
}
