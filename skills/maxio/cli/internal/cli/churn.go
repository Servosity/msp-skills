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
	Since    string        `json:"since,omitempty"`
	Currency string        `json:"currency,omitempty"`
	Periods  []churnPeriod `json:"periods"`
	Reasons  []churnReason `json:"reasons"`
	Note     string        `json:"note,omitempty"`
}

// pp:data-source local
//
// newNovelChurnCmd reports logo churn (count) split voluntary vs involuntary,
// dollar churn from the MRR movement log, and a reason-code breakdown — the
// deal-room churn view that retention's revenue-only ratios don't carry.
// Involuntary churn is failed-card / dunning cancellation; everything else is
// voluntary. Reason comes from cancellation_message, falling back to
// cancellation_method.
//
// A churned logo is a customer, not a subscription (Codex r1, maxio 4.34.2
// reprint, class subscription-counted-as-logo): a customer churns when none of
// its subscriptions is current (revenue.CurrentStates) and at least one was
// canceled; its churn month, method and reason are those of its last
// cancellation.
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
			currency, money := revenueMoney(cmd.Context(), db)

			// Logo churn: customers with no current subscription, bucketed by
			// their last cancellation's month. Involuntary = dunning/payment-
			// failure cancellation method of that last cancellation.
			logoRows, qerr := db.QueryContext(cmd.Context(), `
				WITH subs AS (
				  SELECT CAST(json_extract(data,'$.customer.id') AS TEXT) AS cid,
				         json_extract(data,'$.state') AS state,
				         json_extract(data,'$.canceled_at') AS canceled_at,
				         LOWER(COALESCE(json_extract(data,'$.cancellation_method'),'')) AS method,
				         TRIM(COALESCE(NULLIF(json_extract(data,'$.cancellation_message'),''),
				                       NULLIF(json_extract(data,'$.cancellation_method'),''),
				                       'unspecified')) AS reason
				  FROM resources
				  WHERE resource_type='subscriptions-json' AND json_extract(data,'$.customer.id') IS NOT NULL
				),
				churned AS (
				  SELECT cid, MAX(canceled_at) AS churned_at
				  FROM subs
				  GROUP BY cid
				  HAVING SUM(CASE WHEN LOWER(COALESCE(state,'')) IN (SELECT value FROM json_each(?)) THEN 1 ELSE 0 END) = 0
				     AND MAX(canceled_at) IS NOT NULL
				)
				SELECT c.cid, substr(c.churned_at,1,7) AS period, s.method, s.reason
				FROM churned c JOIN subs s ON s.cid = c.cid AND s.canceled_at = c.churned_at
				WHERE substr(c.churned_at,1,7) >= ?
				ORDER BY period, c.cid`, revenue.CurrentStatesArg(), monthOf(cutoff))
			if qerr != nil {
				return fmt.Errorf("querying churned customers: %w", qerr)
			}
			defer logoRows.Close()

			byPeriod := map[string]*churnPeriod{}
			var order []string
			counted := map[string]bool{}
			reasonCount := map[string]int{}
			for logoRows.Next() {
				var cid, period, method, reason sql.NullString
				if err := logoRows.Scan(&cid, &period, &method, &reason); err != nil {
					return fmt.Errorf("scanning churned customers: %w", err)
				}
				key := period.String
				if key == "" || counted[cid.String] {
					continue // one logo per customer, whichever last cancellation ties
				}
				counted[cid.String] = true
				p := byPeriod[key]
				if p == nil {
					p = &churnPeriod{Period: key}
					byPeriod[key] = p
					order = append(order, key)
				}
				p.ChurnedLogos++
				if isInvoluntaryCancellation(method.String) {
					p.InvoluntaryLogos++
				} else {
					p.VoluntaryLogos++
				}
				reasonCount[reason.String]++
			}
			if err := logoRows.Err(); err != nil {
				return fmt.Errorf("scanning churned customers: %w", err)
			}

			// Dollar churn from the movement log (churn bucket, negative cents).
			warnIfMovementsIncomplete(cmd.Context(), db, cmd.ErrOrStderr())
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

			// Reason breakdown across the window: one per churned customer, the
			// reason of its last cancellation.
			reasons := []churnReason{}
			for reason, n := range reasonCount {
				reasons = append(reasons, churnReason{Reason: reason, Count: n})
			}
			sort.Slice(reasons, func(i, j int) bool {
				if reasons[i].Count != reasons[j].Count {
					return reasons[i].Count > reasons[j].Count
				}
				return reasons[i].Reason < reasons[j].Reason
			})

			sort.Strings(order)
			view := churnView{Since: cutoff, Currency: currency, Periods: make([]churnPeriod, 0, len(order)), Reasons: reasons}
			for _, key := range order {
				view.Periods = append(view.Periods, *byPeriod[key])
			}
			view.Note = "involuntary = failed-card/dunning cancellation; voluntary = everything else. churn_mrr_cents is the signed (negative) churned MRR from the movement log; a churned logo is a customer with no current subscription left, counted in the month of its last cancellation (reasons likewise, one per churned customer)."

			return emitRevenue(cmd, flags, view, func(w io.Writer) {
				fmt.Fprintf(w, "%-9s %8s %8s %8s %14s\n", "period", "logos", "vol", "invol", "churn_mrr")
				for _, p := range view.Periods {
					fmt.Fprintf(w, "%-9s %8d %8d %8d %14s\n", p.Period, p.ChurnedLogos, p.VoluntaryLogos, p.InvoluntaryLogos, money(p.ChurnMRRCents))
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
