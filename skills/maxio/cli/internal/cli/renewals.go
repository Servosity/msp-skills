package cli

import (
	"database/sql"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"maxio-pp-cli/internal/revenue"
)

type renewalRow struct {
	Customer       string `json:"customer"`
	SubscriptionID string `json:"subscription_id"`
	EndsAt         string `json:"ends_at"`
	MRRCents       int64  `json:"mrr_cents"`
}

type renewalsView struct {
	WithinDays     int          `json:"within_days"`
	Count          int          `json:"count"`
	TotalMRRAtRisk int64        `json:"total_mrr_at_risk_cents"`
	Renewals       []renewalRow `json:"renewals"`
	Note           string       `json:"note,omitempty"`
}

// pp:data-source local
//
// newNovelRenewalsCmd lists active subscriptions whose current period ends
// within the next N days (default 90) — the deal-room renewal-visibility view
// alongside concentration. MRR-at-risk joins the latest snapshot.
func newNovelRenewalsCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var within int
	cmd := &cobra.Command{
		Use:         "renewals",
		Short:       "Active subscriptions renewing within the next N days (default 90), with MRR at risk.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example:     "  maxio-cli renewals\n  maxio-cli renewals --within 30 --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if within <= 0 {
				within = 90
			}
			st, db, err := openRevenueStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer st.Close()

			now := time.Now().UTC()
			lo := now.Format("2006-01-02")
			hi := now.AddDate(0, 0, within).Format("2006-01-02")

			var latest sql.NullString
			_ = db.QueryRowContext(cmd.Context(), `SELECT MAX(snapshot_at) FROM rev_sub_mrr_snapshots`).Scan(&latest)

			// Compare on the YYYY-MM-DD prefix so timezone offsets in the stored
			// ISO timestamp don't perturb the window boundary.
			rows, qerr := db.QueryContext(cmd.Context(), `
				SELECT sub.id,
				       COALESCE(NULLIF(json_extract(cust.data,'$.organization'),''),
				                'customer ' || COALESCE(json_extract(sub.data,'$.customer.id'),'?')) AS name,
				       json_extract(sub.data,'$.current_period_ends_at') AS ends_at,
				       COALESCE(s.mrr_cents,0) AS mrr
				FROM resources sub
				LEFT JOIN rev_sub_mrr_snapshots s ON s.subscription_id = sub.id AND s.snapshot_at = ?
				LEFT JOIN resources cust ON cust.resource_type='customers-json'
				                        AND cust.id = json_extract(sub.data,'$.customer.id')
				WHERE sub.resource_type='subscriptions-json'
				  AND json_extract(sub.data,'$.state')='active'
				  AND substr(json_extract(sub.data,'$.current_period_ends_at'),1,10) >= ?
				  AND substr(json_extract(sub.data,'$.current_period_ends_at'),1,10) <= ?
				ORDER BY ends_at`, latest.String, lo, hi)
			if qerr != nil {
				return fmt.Errorf("querying renewals: %w", qerr)
			}
			defer rows.Close()

			view := renewalsView{WithinDays: within, Renewals: []renewalRow{}}
			for rows.Next() {
				var id, name, ends sql.NullString
				var mrr sql.NullInt64
				if err := rows.Scan(&id, &name, &ends, &mrr); err != nil {
					continue
				}
				view.Renewals = append(view.Renewals, renewalRow{
					Customer: name.String, SubscriptionID: id.String, EndsAt: ends.String, MRRCents: mrr.Int64,
				})
				view.TotalMRRAtRisk += mrr.Int64
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("scanning renewals: %w", err)
			}
			view.Count = len(view.Renewals)
			view.Note = fmt.Sprintf("active subscriptions with current_period_ends_at in %s..%s; mrr_cents is from the latest snapshot (0 if unsynced).", lo, hi)

			return emitRevenue(cmd, flags, view, func(w io.Writer) {
				fmt.Fprintf(w, "renewals in next %d days: %d   MRR at risk: %s\n\n", view.WithinDays, view.Count, revenue.Cents(view.TotalMRRAtRisk))
				fmt.Fprintf(w, "%-40s %-12s %14s\n", "customer", "ends", "mrr")
				for _, r := range view.Renewals {
					ends := r.EndsAt
					if len(ends) >= 10 {
						ends = ends[:10]
					}
					fmt.Fprintf(w, "%-40s %-12s %14s\n", truncReason(r.Customer), ends, revenue.Cents(r.MRRCents))
				}
			})
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	cmd.Flags().IntVar(&within, "within", 90, "Renewal window in days")
	return cmd
}
