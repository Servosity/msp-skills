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

type newCustomersPeriod struct {
	Period         string `json:"period"`
	NewCustomers   int    `json:"new_customers"`
	NewMRRCents    int64  `json:"new_mrr_cents"`
	AvgNewMRRCents int64  `json:"avg_new_mrr_cents"`
}

type newCustomersView struct {
	Since   string               `json:"since,omitempty"`
	Periods []newCustomersPeriod `json:"periods"`
	Note    string               `json:"note,omitempty"`
}

// pp:data-source local
//
// newNovelNewCustomersCmd reports newly acquired logos per period (a customer's
// first activated subscription month) plus the new MRR they brought. This is
// the CAC denominator — the deal-room skill divides sales-and-marketing spend
// (from QuickBooks) by new_customers to compute CAC.
func newNovelNewCustomersCmd(flags *rootFlags) *cobra.Command {
	var flagSince, dbPath string
	cmd := &cobra.Command{
		Use:         "new-customers",
		Short:       "New logos acquired per period (first-activation month) plus the new MRR they brought.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example:     "  maxio-cli new-customers --since 12m\n  maxio-cli new-customers --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
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

			// New logos = customers whose earliest subscription activation falls
			// in the period. MIN(activated_at) per customer avoids counting a
			// multi-subscription customer once per subscription.
			rows, qerr := db.QueryContext(cmd.Context(), `
				SELECT substr(first_activ,1,7) AS period, COUNT(*) AS new_customers
				FROM (
				  SELECT json_extract(data,'$.customer.id') AS cid,
				         MIN(json_extract(data,'$.activated_at')) AS first_activ
				  FROM resources
				  WHERE resource_type='subscriptions-json'
				    AND json_extract(data,'$.activated_at') IS NOT NULL
				  GROUP BY cid
				)
				WHERE substr(first_activ,1,7) >= ?
				GROUP BY period
				ORDER BY period`, monthOf(cutoff))
			if qerr != nil {
				return fmt.Errorf("querying new customers: %w", qerr)
			}
			defer rows.Close()

			byPeriod := map[string]*newCustomersPeriod{}
			var order []string
			for rows.Next() {
				var period sql.NullString
				var n sql.NullInt64
				if err := rows.Scan(&period, &n); err != nil {
					continue
				}
				key := period.String
				if key == "" {
					continue
				}
				byPeriod[key] = &newCustomersPeriod{Period: key, NewCustomers: int(n.Int64)}
				order = append(order, key)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("scanning new customers: %w", err)
			}

			// New MRR from the movement log (new bucket).
			if has, _ := revenue.HasMovements(cmd.Context(), db); has {
				mvRows, mverr := db.QueryContext(cmd.Context(), `
					SELECT substr(ts,1,7) AS period, COALESCE(SUM(amount_cents),0)
					FROM rev_mrr_movements WHERE bucket = ? AND ts >= ?
					GROUP BY period`, revenue.BucketNew, cutoff)
				if mverr == nil {
					defer mvRows.Close()
					for mvRows.Next() {
						var period sql.NullString
						var cents sql.NullInt64
						if err := mvRows.Scan(&period, &cents); err != nil {
							continue
						}
						if p := byPeriod[period.String]; p != nil {
							p.NewMRRCents = cents.Int64
						}
					}
				}
			}

			sort.Strings(order)
			view := newCustomersView{Since: cutoff, Periods: make([]newCustomersPeriod, 0, len(order))}
			for _, key := range order {
				p := byPeriod[key]
				if p.NewCustomers > 0 {
					p.AvgNewMRRCents = p.NewMRRCents / int64(p.NewCustomers)
				}
				view.Periods = append(view.Periods, *p)
			}
			view.Note = "new_customers counts distinct logos by first subscription activation month; new_mrr_cents is the new-bucket total from the movement log."

			return emitRevenue(cmd, flags, view, func(w io.Writer) {
				fmt.Fprintf(w, "%-9s %14s %16s %16s\n", "period", "new_customers", "new_mrr", "avg_new_mrr")
				for _, p := range view.Periods {
					fmt.Fprintf(w, "%-9s %14d %16s %16s\n", p.Period, p.NewCustomers, revenue.Cents(p.NewMRRCents), revenue.Cents(p.AvgNewMRRCents))
				}
			})
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "", "Only activations on/after this date (YYYY-MM-DD or window like 12m)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
