package cli

import (
	"database/sql"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"maxio-pp-cli/internal/revenue"
)

type clientMRRRow struct {
	Customer      string `json:"customer"`
	CustomerID    string `json:"customer_id"`
	MRRCents      int64  `json:"mrr_cents"`
	MRR           string `json:"mrr"`
	PlanCents     int64  `json:"plan_cents"`
	UsageCents    int64  `json:"usage_cents"`
	Subscriptions int    `json:"subscriptions"`
	ref           string
}

type clientHistoryPoint struct {
	SnapshotAt string `json:"snapshot_at"`
	MRRCents   int64  `json:"mrr_cents"`
}

type clientMRRView struct {
	SnapshotAt   string               `json:"snapshot_at"`
	Customers    []clientMRRRow       `json:"customers"`
	TotalCents   int64                `json:"total_mrr_cents"`
	SiteMRRCents int64                `json:"site_mrr_cents,omitempty"`
	History      []clientHistoryPoint `json:"history,omitempty"`
	Note         string               `json:"note,omitempty"`
}

// pp:data-source local
//
// newNovelMrrClientCmd reports normalized MRR per customer (current, from the
// latest snapshot) for the whole book or one customer, with per-snapshot
// history the live API cannot reconstruct.
func newNovelMrrClientCmd(flags *rootFlags) *cobra.Command {
	var flagCustomer, flagSince, dbPath string
	var limit int
	cmd := &cobra.Command{
		Use:         "client",
		Short:       "Per-customer recurring revenue (current + historic) from the local snapshot store",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example:     "  maxio-cli mrr client\n  maxio-cli mrr client --customer \"Safe Haven\" --json\n  maxio-cli mrr client --limit 50",
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

			var latest sql.NullString
			_ = db.QueryRowContext(cmd.Context(), `SELECT MAX(snapshot_at) FROM rev_sub_mrr_snapshots`).Scan(&latest)
			if !latest.Valid || latest.String == "" {
				hintRunMrrSync(cmd.ErrOrStderr())
				return emitRevenue(cmd, flags, clientMRRView{Customers: []clientMRRRow{}, Note: "no MRR snapshots yet"}, func(w io.Writer) {})
			}

			// Drive from active subscriptions (the complete customer set), LEFT
			// JOIN per-sub MRR. Maxio's /subscriptions_mrr.json omits some
			// subscription types (e.g. annual prepay), so an INNER JOIN on the
			// snapshot would drop whole customers; the LEFT JOIN keeps every
			// active customer visible (with MRR 0 where Maxio reports none).
			rows, err := db.QueryContext(cmd.Context(), `
				SELECT CAST(json_extract(sub.data,'$.customer.id') AS TEXT) AS cust_id,
				       COALESCE(NULLIF(json_extract(cust.data,'$.organization'),''),
				                TRIM(COALESCE(json_extract(cust.data,'$.first_name'),'')||' '||COALESCE(json_extract(cust.data,'$.last_name'),'')),
				                CAST(json_extract(sub.data,'$.customer.id') AS TEXT)) AS cust_name,
				       COALESCE(json_extract(cust.data,'$.reference'),'') AS cust_ref,
				       SUM(COALESCE(s.mrr_cents,0)), SUM(COALESCE(s.plan_cents,0)), SUM(COALESCE(s.usage_cents,0)),
				       COUNT(DISTINCT sub.id)
				FROM resources sub
				LEFT JOIN rev_sub_mrr_snapshots s ON s.subscription_id = sub.id AND s.snapshot_at = ?
				LEFT JOIN resources cust ON cust.resource_type='customers-json'
				     AND cust.id = CAST(json_extract(sub.data,'$.customer.id') AS TEXT)
				WHERE sub.resource_type='subscriptions-json' AND json_extract(sub.data,'$.state')='active'
				GROUP BY cust_id
				ORDER BY SUM(COALESCE(s.mrr_cents,0)) DESC`, latest.String)
			if err != nil {
				return fmt.Errorf("querying per-customer MRR: %w", err)
			}
			defer rows.Close()

			var all []clientMRRRow
			for rows.Next() {
				var r clientMRRRow
				var custID, name, ref sql.NullString
				var mrr, plan, usage sql.NullInt64
				var subs sql.NullInt64
				if err := rows.Scan(&custID, &name, &ref, &mrr, &plan, &usage, &subs); err != nil {
					continue
				}
				r.CustomerID = custID.String
				r.Customer = name.String
				r.ref = ref.String
				r.MRRCents = mrr.Int64
				r.PlanCents = plan.Int64
				r.UsageCents = usage.Int64
				r.Subscriptions = int(subs.Int64)
				r.MRR = revenue.Cents(r.MRRCents)
				all = append(all, r)
			}

			filter := strings.TrimSpace(strings.ToLower(flagCustomer))
			if filter != "" {
				kept := all[:0:0]
				for _, r := range all {
					if strings.Contains(strings.ToLower(r.Customer), filter) ||
						strings.EqualFold(r.CustomerID, flagCustomer) ||
						strings.EqualFold(r.ref, flagCustomer) {
						kept = append(kept, r)
					}
				}
				all = kept
			}

			view := clientMRRView{SnapshotAt: latest.String}
			for _, r := range all {
				view.TotalCents += r.MRRCents
			}
			// Surface the site-MRR reconciliation gap honestly: Maxio computes
			// site MRR (/mrr.json) and per-subscription MRR separately, so the
			// per-customer total may not equal `mrr now`.
			if filter == "" {
				var siteMRR sql.NullInt64
				_ = db.QueryRowContext(cmd.Context(),
					`SELECT mrr_cents FROM rev_site_snapshots ORDER BY snapshot_at DESC LIMIT 1`).Scan(&siteMRR)
				if siteMRR.Valid {
					view.SiteMRRCents = siteMRR.Int64
					if d := siteMRR.Int64 - view.TotalCents; d > view.TotalCents/100 || d < -view.TotalCents/100 {
						view.Note = fmt.Sprintf("per-subscription MRR total (%s) differs from site MRR (%s); Maxio computes them separately",
							revenue.Cents(view.TotalCents), revenue.Cents(siteMRR.Int64))
					}
				}
			}

			// History when the filter narrows to exactly one customer.
			if filter != "" && len(all) == 1 {
				hrows, herr := db.QueryContext(cmd.Context(), `
					SELECT s.snapshot_at, SUM(s.mrr_cents)
					FROM rev_sub_mrr_snapshots s
					JOIN resources sub ON sub.resource_type='subscriptions-json' AND sub.id = s.subscription_id
					WHERE CAST(json_extract(sub.data,'$.customer.id') AS TEXT) = ?
					GROUP BY s.snapshot_at ORDER BY s.snapshot_at`, all[0].CustomerID)
				if herr == nil {
					defer hrows.Close()
					for hrows.Next() {
						var at sql.NullString
						var c sql.NullInt64
						if err := hrows.Scan(&at, &c); err == nil {
							view.History = append(view.History, clientHistoryPoint{SnapshotAt: at.String, MRRCents: c.Int64})
						}
					}
				}
				if len(view.History) <= 1 {
					view.Note = "history grows as snapshots accrue; run `maxio-cli mrr sync` on a schedule to build it"
				}
			}

			if limit > 0 && len(all) > limit && filter == "" {
				all = all[:limit]
			}
			view.Customers = all
			if view.Customers == nil {
				view.Customers = []clientMRRRow{}
			}

			return emitRevenue(cmd, flags, view, func(w io.Writer) {
				fmt.Fprintf(w, "Per-customer MRR (snapshot %s)\n", shortDate(view.SnapshotAt))
				for _, r := range view.Customers {
					fmt.Fprintf(w, "  %-40s %12s  (%d sub)\n", truncate(r.Customer, 40), revenue.Cents(r.MRRCents), r.Subscriptions)
				}
				fmt.Fprintf(w, "  %-40s %12s\n", "TOTAL", revenue.Cents(view.TotalCents))
				for _, h := range view.History {
					fmt.Fprintf(w, "  history %s  %s\n", shortDate(h.SnapshotAt), revenue.Cents(h.MRRCents))
				}
			})
		},
	}
	cmd.Flags().StringVar(&flagCustomer, "customer", "", "Filter to one customer by id, reference, or name substring")
	cmd.Flags().StringVar(&flagSince, "since", "", "Reserved for history range filtering")
	cmd.Flags().IntVar(&limit, "limit", 25, "Max customers to return when not filtering (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
