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

type churnPeriod struct {
	Period           string `json:"period"`
	ChurnedLogos     int    `json:"churned_logos"`
	VoluntaryLogos   int    `json:"voluntary_logos"`
	InvoluntaryLogos int    `json:"involuntary_logos"`
	ChurnMRRCents    int64  `json:"churn_mrr_cents"`
}

type churnReason struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

type churnView struct {
	Since   string        `json:"since,omitempty"`
	Periods []churnPeriod `json:"periods"`
	Reasons []churnReason `json:"reasons"`
	Note    string        `json:"note,omitempty"`
}

// pp:data-source local
//
// newNovelChurnCmd reports logo churn (count) split voluntary vs involuntary,
// dollar churn from the MRR movement log, and a reason-code breakdown — the
// deal-room churn view that retention's revenue-only ratios don't carry.
// Involuntary churn is failed-card / dunning cancellation; everything else is
// voluntary. Reason comes from cancellation_message, falling back to
// cancellation_method.
func newNovelChurnCmd(flags *rootFlags) *cobra.Command {
	var flagSince, dbPath string
	cmd := &cobra.Command{
		Use:         "churn",
		Short:       "Logo and revenue churn per period, voluntary vs involuntary, by reason code.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example:     "  maxio-cli churn --since 12m\n  maxio-cli churn --since 2026-01-01 --json",
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

			// Logo churn from canceled subscriptions, bucketed by cancellation
			// month. Involuntary = dunning/payment-failure cancellation method.
			logoRows, qerr := db.QueryContext(cmd.Context(), `
				SELECT substr(json_extract(data,'$.canceled_at'),1,7) AS period,
				       LOWER(COALESCE(json_extract(data,'$.cancellation_method'),'')) AS method,
				       COUNT(*) AS n
				FROM resources
				WHERE resource_type='subscriptions-json'
				  AND json_extract(data,'$.canceled_at') IS NOT NULL
				  AND substr(json_extract(data,'$.canceled_at'),1,7) >= ?
				GROUP BY period, method
				ORDER BY period`, monthOf(cutoff))
			if qerr != nil {
				return fmt.Errorf("querying churned subscriptions: %w", qerr)
			}
			defer logoRows.Close()

			byPeriod := map[string]*churnPeriod{}
			var order []string
			for logoRows.Next() {
				var period, method sql.NullString
				var n sql.NullInt64
				if err := logoRows.Scan(&period, &method, &n); err != nil {
					continue
				}
				key := period.String
				if key == "" {
					continue
				}
				p := byPeriod[key]
				if p == nil {
					p = &churnPeriod{Period: key}
					byPeriod[key] = p
					order = append(order, key)
				}
				p.ChurnedLogos += int(n.Int64)
				if isInvoluntaryCancellation(method.String) {
					p.InvoluntaryLogos += int(n.Int64)
				} else {
					p.VoluntaryLogos += int(n.Int64)
				}
			}
			if err := logoRows.Err(); err != nil {
				return fmt.Errorf("scanning churned subscriptions: %w", err)
			}

			// Dollar churn from the movement log (churn bucket, negative cents).
			if has, _ := revenue.HasMovements(cmd.Context(), db); has {
				mvRows, mverr := db.QueryContext(cmd.Context(), `
					SELECT substr(ts,1,7) AS period, COALESCE(SUM(amount_cents),0)
					FROM rev_mrr_movements
					WHERE bucket = ? AND ts >= ?
					GROUP BY period`, revenue.BucketChurn, cutoff)
				if mverr == nil {
					defer mvRows.Close()
					for mvRows.Next() {
						var period sql.NullString
						var cents sql.NullInt64
						if err := mvRows.Scan(&period, &cents); err != nil {
							continue
						}
						key := period.String
						p := byPeriod[key]
						if p == nil {
							p = &churnPeriod{Period: key}
							byPeriod[key] = p
							order = append(order, key)
						}
						p.ChurnMRRCents = cents.Int64
					}
				}
			}

			// Reason breakdown across the window.
			reasonRows, rerr := db.QueryContext(cmd.Context(), `
				SELECT TRIM(COALESCE(NULLIF(json_extract(data,'$.cancellation_message'),''),
				                     NULLIF(json_extract(data,'$.cancellation_method'),''),
				                     'unspecified')) AS reason,
				       COUNT(*) AS n
				FROM resources
				WHERE resource_type='subscriptions-json'
				  AND json_extract(data,'$.canceled_at') IS NOT NULL
				  AND substr(json_extract(data,'$.canceled_at'),1,7) >= ?
				GROUP BY reason
				ORDER BY n DESC`, monthOf(cutoff))
			reasons := []churnReason{}
			if rerr == nil {
				defer reasonRows.Close()
				for reasonRows.Next() {
					var reason sql.NullString
					var n sql.NullInt64
					if err := reasonRows.Scan(&reason, &n); err != nil {
						continue
					}
					reasons = append(reasons, churnReason{Reason: reason.String, Count: int(n.Int64)})
				}
			}

			sort.Strings(order)
			view := churnView{Since: cutoff, Periods: make([]churnPeriod, 0, len(order)), Reasons: reasons}
			for _, key := range order {
				view.Periods = append(view.Periods, *byPeriod[key])
			}
			view.Note = "involuntary = failed-card/dunning cancellation; voluntary = everything else. churn_mrr_cents is the signed (negative) dollar churn from the movement log; logo counts come from canceled subscription records."

			return emitRevenue(cmd, flags, view, func(w io.Writer) {
				fmt.Fprintf(w, "%-9s %8s %8s %8s %14s\n", "period", "logos", "vol", "invol", "churn_mrr")
				for _, p := range view.Periods {
					fmt.Fprintf(w, "%-9s %8d %8d %8d %14s\n", p.Period, p.ChurnedLogos, p.VoluntaryLogos, p.InvoluntaryLogos, revenue.Cents(p.ChurnMRRCents))
				}
				if len(view.Reasons) > 0 {
					fmt.Fprintln(w, "\nreasons:")
					for _, r := range view.Reasons {
						fmt.Fprintf(w, "  %-40s %d\n", truncReason(r.Reason), r.Count)
					}
				}
			})
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "", "Only cancellations on/after this date (YYYY-MM-DD or window like 12m, 90d)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

// isInvoluntaryCancellation reports whether a Chargify cancellation_method
// indicates failed-payment / dunning churn rather than a deliberate cancel.
func isInvoluntaryCancellation(method string) bool {
	switch method {
	case "dunning", "automatic", "remittance_failure", "payment_failure":
		return true
	}
	return false
}

// monthOf returns the YYYY-MM prefix of a YYYY-MM-DD cutoff so a month-grain
// canceled_at comparison stays inclusive of the cutoff's month. Empty cutoff
// (no --since) returns "" which compares >= everything.
func monthOf(cutoff string) string {
	if len(cutoff) >= 7 {
		return cutoff[:7]
	}
	return cutoff
}

func truncReason(s string) string {
	if len(s) <= 40 {
		return s
	}
	return s[:37] + "..."
}
