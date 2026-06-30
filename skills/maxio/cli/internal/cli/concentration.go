package cli

import (
	"context"
	"database/sql"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"maxio-pp-cli/internal/revenue"
)

// custMRR is one customer's recurring revenue at the latest snapshot.
type custMRR struct {
	CustomerID string `json:"customer_id"`
	Customer   string `json:"customer"`
	MRRCents   int64  `json:"mrr_cents"`
}

// latestPerCustomerMRR returns per-customer MRR at the most recent snapshot,
// descending by MRR, plus the total. Mirrors cohort.go's snapshot→subscription
// join (s.subscription_id = sub.id). Shared by concentration and arpa.
func latestPerCustomerMRR(ctx context.Context, db *sql.DB) ([]custMRR, int64, error) {
	var latest sql.NullString
	_ = db.QueryRowContext(ctx, `SELECT MAX(snapshot_at) FROM rev_sub_mrr_snapshots`).Scan(&latest)
	if !latest.Valid || latest.String == "" {
		return nil, 0, nil
	}
	rows, err := db.QueryContext(ctx, `
		SELECT COALESCE(json_extract(sub.data,'$.customer.id'), 'unknown') AS cid,
		       COALESCE(NULLIF(json_extract(cust.data,'$.organization'),''),
		                TRIM(COALESCE(json_extract(cust.data,'$.first_name'),'') || ' ' ||
		                     COALESCE(json_extract(cust.data,'$.last_name'),'')),
		                'customer ' || COALESCE(json_extract(sub.data,'$.customer.id'),'?')) AS name,
		       SUM(COALESCE(s.mrr_cents,0)) AS mrr
		FROM rev_sub_mrr_snapshots s
		JOIN resources sub ON sub.id = s.subscription_id AND sub.resource_type='subscriptions-json'
		LEFT JOIN resources cust ON cust.resource_type='customers-json'
		                        AND cust.id = json_extract(sub.data,'$.customer.id')
		WHERE s.snapshot_at = ?
		  AND json_extract(sub.data,'$.state')='active'
		GROUP BY cid
		HAVING mrr > 0
		ORDER BY mrr DESC`, latest.String)
	if err != nil {
		return nil, 0, fmt.Errorf("querying per-customer MRR: %w", err)
	}
	defer rows.Close()
	var out []custMRR
	var total int64
	for rows.Next() {
		var cid, name sql.NullString
		var mrr sql.NullInt64
		if err := rows.Scan(&cid, &name, &mrr); err != nil {
			continue
		}
		out = append(out, custMRR{CustomerID: cid.String, Customer: name.String, MRRCents: mrr.Int64})
		total += mrr.Int64
	}
	return out, total, rows.Err()
}

type concentrationView struct {
	TotalCustomers int       `json:"total_customers"`
	TotalMRRCents  int64     `json:"total_mrr_cents"`
	Top1Pct        float64   `json:"top_1_customer_pct"`
	Top5Pct        float64   `json:"top_5_customers_pct"`
	Top10Pct       float64   `json:"top_10_customers_pct"`
	Top            []custMRR `json:"top"`
	Note           string    `json:"note,omitempty"`
}

// pp:data-source local
//
// newNovelConcentrationCmd reports customer concentration — the share of ARR
// held by the largest 1, 5, and 10 customers — the deal-room risk metric a
// buyer asks for first. Computed from per-customer MRR at the latest snapshot.
func newNovelConcentrationCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit int
	cmd := &cobra.Command{
		Use:         "concentration",
		Short:       "Customer concentration: share of ARR held by the top 1 / 5 / 10 customers.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example:     "  maxio-cli concentration\n  maxio-cli concentration --limit 20 --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			st, db, err := openRevenueStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer st.Close()

			custs, total, err := latestPerCustomerMRR(cmd.Context(), db)
			if err != nil {
				return err
			}
			view := concentrationView{TotalCustomers: len(custs), TotalMRRCents: total, Top: []custMRR{}}
			if len(custs) == 0 || total == 0 {
				hintRunMrrSync(cmd.ErrOrStderr())
				view.Note = "no MRR snapshots yet — run `maxio-cli mrr sync` first"
				return emitRevenue(cmd, flags, view, func(w io.Writer) { fmt.Fprintln(w, view.Note) })
			}
			view.Top1Pct = topNShare(custs, total, 1)
			view.Top5Pct = topNShare(custs, total, 5)
			view.Top10Pct = topNShare(custs, total, 10)
			if limit <= 0 {
				limit = 10
			}
			if limit > len(custs) {
				limit = len(custs)
			}
			view.Top = custs[:limit]
			view.Note = "shares computed over the per-subscription MRR snapshot, which excludes annual-prepay subscriptions; the full-book recognized-revenue total is in QuickBooks (report pnl). Concentration shares are still representative of the recurring book."

			return emitRevenue(cmd, flags, view, func(w io.Writer) {
				fmt.Fprintf(w, "customers: %d   total MRR: %s\n", view.TotalCustomers, revenue.Cents(view.TotalMRRCents))
				fmt.Fprintf(w, "top 1:  %5.1f%%\ntop 5:  %5.1f%%\ntop 10: %5.1f%%\n", view.Top1Pct, view.Top5Pct, view.Top10Pct)
				fmt.Fprintf(w, "\n%-40s %14s %8s\n", "customer", "mrr", "share%")
				for _, c := range view.Top {
					fmt.Fprintf(w, "%-40s %14s %7.1f%%\n", truncReason(c.Customer), revenue.Cents(c.MRRCents), float64(c.MRRCents)/float64(total)*100)
				}
			})
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	cmd.Flags().IntVar(&limit, "limit", 10, "Number of top customers to list")
	return cmd
}

// topNShare returns the percentage of total MRR held by the n largest
// customers. custs must be sorted descending by MRR.
func topNShare(custs []custMRR, total int64, n int) float64 {
	if total == 0 {
		return 0
	}
	if n > len(custs) {
		n = len(custs)
	}
	var top int64
	for i := 0; i < n; i++ {
		top += custs[i].MRRCents
	}
	return round2(float64(top) / float64(total) * 100)
}
