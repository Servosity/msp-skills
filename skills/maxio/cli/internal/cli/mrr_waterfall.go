package cli

import (
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"maxio-pp-cli/internal/revenue"
)

type waterfallPeriod struct {
	Period       string `json:"period"`
	NewCents     int64  `json:"new_cents"`
	Expansion    int64  `json:"expansion_cents"`
	Contraction  int64  `json:"contraction_cents"`
	Churn        int64  `json:"churn_cents"`
	Reactivation int64  `json:"reactivation_cents"`
	Imported     int64  `json:"imported_cents"`
	NetCents     int64  `json:"net_cents"`
}

type waterfallView struct {
	Since   string            `json:"since,omitempty"`
	GroupBy string            `json:"group_by"`
	Periods []waterfallPeriod `json:"periods"`
	Totals  waterfallPeriod   `json:"totals"`
}

// pp:data-source local
//
// newNovelMrrWaterfallCmd computes the MRR movement waterfall (New / Expansion /
// Contraction / Churn / Reactivation) per period from the locally-stored
// movement log, so it survives the upstream deprecation of the movement API.
func newNovelMrrWaterfallCmd(flags *rootFlags) *cobra.Command {
	var flagSince, flagGroupBy, dbPath string
	cmd := &cobra.Command{
		Use:         "waterfall",
		Short:       "MRR movement waterfall: New / Expansion / Contraction / Churn / Reactivation per period",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example:     "  maxio-cli mrr waterfall --since 2026-01-01 --group-by month\n  maxio-cli mrr waterfall --since 12m --group-by month --json",
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
			}

			q := `SELECT ts, bucket, amount_cents FROM rev_mrr_movements`
			var rows interface {
				Next() bool
				Scan(...any) error
				Close() error
				Err() error
			}
			if cutoff != "" {
				r, qerr := db.QueryContext(cmd.Context(), q+` WHERE ts >= ? ORDER BY ts`, cutoff)
				if qerr != nil {
					return fmt.Errorf("querying movements: %w", qerr)
				}
				rows = r
			} else {
				r, qerr := db.QueryContext(cmd.Context(), q+` ORDER BY ts`)
				if qerr != nil {
					return fmt.Errorf("querying movements: %w", qerr)
				}
				rows = r
			}
			defer rows.Close()

			periods := map[string]*waterfallPeriod{}
			for rows.Next() {
				var ts, bucket string
				var amt int64
				if err := rows.Scan(&ts, &bucket, &amt); err != nil {
					continue
				}
				key := periodKey(ts, groupBy)
				p := periods[key]
				if p == nil {
					p = &waterfallPeriod{Period: key}
					periods[key] = p
				}
				addToBucket(p, bucket, amt)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("scanning movements: %w", err)
			}

			view := waterfallView{Since: cutoff, GroupBy: groupBy, Periods: make([]waterfallPeriod, 0, len(periods))}
			for _, p := range periods {
				p.NetCents = p.NewCents + p.Expansion + p.Contraction + p.Churn + p.Reactivation
				view.Periods = append(view.Periods, *p)
				addToBucket(&view.Totals, "new", p.NewCents)
				addToBucket(&view.Totals, "expansion", p.Expansion)
				addToBucket(&view.Totals, "contraction", p.Contraction)
				addToBucket(&view.Totals, "churn", p.Churn)
				addToBucket(&view.Totals, "reactivation", p.Reactivation)
				addToBucket(&view.Totals, "imported", p.Imported)
			}
			view.Totals.Period = "TOTAL"
			view.Totals.NetCents = view.Totals.NewCents + view.Totals.Expansion + view.Totals.Contraction + view.Totals.Churn + view.Totals.Reactivation
			sort.Slice(view.Periods, func(i, j int) bool { return view.Periods[i].Period < view.Periods[j].Period })

			return emitRevenue(cmd, flags, view, func(w io.Writer) {
				fmt.Fprintf(w, "%-9s %12s %12s %12s %12s %12s %12s\n", "period", "new", "expansion", "contract", "churn", "reactiv", "net")
				for _, p := range view.Periods {
					fmt.Fprintf(w, "%-9s %12s %12s %12s %12s %12s %12s\n", p.Period,
						revenue.Cents(p.NewCents), revenue.Cents(p.Expansion), revenue.Cents(p.Contraction),
						revenue.Cents(p.Churn), revenue.Cents(p.Reactivation), revenue.Cents(p.NetCents))
				}
				t := view.Totals
				fmt.Fprintf(w, "%-9s %12s %12s %12s %12s %12s %12s\n", "TOTAL",
					revenue.Cents(t.NewCents), revenue.Cents(t.Expansion), revenue.Cents(t.Contraction),
					revenue.Cents(t.Churn), revenue.Cents(t.Reactivation), revenue.Cents(t.NetCents))
			})
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "", "Only movements on/after this date (YYYY-MM-DD or window like 12m, 90d, 4w)")
	cmd.Flags().StringVar(&flagGroupBy, "group-by", "month", "Period granularity: month, week, or day")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

func addToBucket(p *waterfallPeriod, bucket string, amt int64) {
	switch bucket {
	case revenue.BucketNew:
		p.NewCents += amt
	case revenue.BucketExpansion:
		p.Expansion += amt
	case revenue.BucketContraction:
		p.Contraction += amt
	case revenue.BucketChurn:
		p.Churn += amt
	case revenue.BucketReactivation:
		p.Reactivation += amt
	case revenue.BucketImported:
		p.Imported += amt
	}
}

// periodKey buckets an RFC3339-ish timestamp into a period label.
func periodKey(ts, groupBy string) string {
	switch groupBy {
	case "day":
		if len(ts) >= 10 {
			return ts[:10]
		}
		return ts
	case "week":
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			y, wk := t.ISOWeek()
			return fmt.Sprintf("%d-W%02d", y, wk)
		}
		if len(ts) >= 10 {
			return ts[:10]
		}
		return ts
	default: // month
		return revenue.MonthKey(ts)
	}
}
