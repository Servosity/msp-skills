package cli

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"math"

	"github.com/spf13/cobra"

	"maxio-pp-cli/internal/revenue"
)

type arpaDecomp struct {
	PriorSnapshot   string `json:"prior_snapshot"`
	PriorMRRCents   int64  `json:"prior_mrr_cents"`
	PriorLogos      int    `json:"prior_logos"`
	CurrentSnapshot string `json:"current_snapshot"`
	CurrentMRRCents int64  `json:"current_mrr_cents"`
	CurrentLogos    int    `json:"current_logos"`
	DeltaMRRCents   int64  `json:"delta_mrr_cents"`
	VolumeCents     int64  `json:"volume_cents"`
	PriceCents      int64  `json:"price_cents"`
}

type arpaView struct {
	SiteMRRCents       int64       `json:"site_mrr_cents"`
	ActiveCustomers    int         `json:"active_customers"`
	ActiveSubscription int         `json:"active_subscriptions"`
	ARPACents          int64       `json:"arpa_cents"`
	ARPUCents          int64       `json:"arpu_cents"`
	P50Cents           int64       `json:"p50_cents"`
	P90Cents           int64       `json:"p90_cents"`
	P99Cents           int64       `json:"p99_cents"`
	Decomposition      *arpaDecomp `json:"price_vs_volume,omitempty"`
	Note               string      `json:"note,omitempty"`
}

// pp:data-source local
//
// newNovelArpaCmd reports ARPA (per account) and ARPU (per subscription) at the
// latest snapshot, the MRR distribution percentiles, and a price-vs-volume
// decomposition of the MRR change between the two most recent snapshots:
// ΔMRR = Δlogos×ARPA_prior (volume) + logos_now×ΔARPA (price).
func newNovelArpaCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:         "arpa",
		Short:       "ARPA/ARPU, MRR percentiles, and price-vs-volume decomposition of MRR change.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example:     "  maxio-cli arpa\n  maxio-cli arpa --json",
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
			view := arpaView{SiteMRRCents: total, ActiveCustomers: len(custs)}
			if len(custs) == 0 || total == 0 {
				hintRunMrrSync(cmd.ErrOrStderr())
				view.Note = "no MRR snapshots yet — run `maxio-cli mrr sync` first"
				return emitRevenue(cmd, flags, view, func(w io.Writer) { fmt.Fprintln(w, view.Note) })
			}

			// active subscriptions in the latest snapshot
			var latest sql.NullString
			_ = db.QueryRowContext(cmd.Context(), `SELECT MAX(snapshot_at) FROM rev_sub_mrr_snapshots`).Scan(&latest)
			var subCount sql.NullInt64
			_ = db.QueryRowContext(cmd.Context(),
				`SELECT COUNT(*) FROM rev_sub_mrr_snapshots WHERE snapshot_at = ? AND mrr_cents > 0`, latest.String).Scan(&subCount)
			view.ActiveSubscription = int(subCount.Int64)

			view.ARPACents = total / int64(len(custs))
			if view.ActiveSubscription > 0 {
				view.ARPUCents = total / int64(view.ActiveSubscription)
			}
			// custs is sorted descending by MRR; percentiles index from the
			// bottom so p90 is the 90th-percentile (large) customer.
			view.P50Cents = percentileDesc(custs, 50)
			view.P90Cents = percentileDesc(custs, 90)
			view.P99Cents = percentileDesc(custs, 99)

			view.Decomposition = priceVolumeDecomp(cmd.Context(), db)
			view.Note = "ARPA/ARPU and percentiles are over the per-subscription MRR snapshot, which excludes annual-prepay subscriptions; the full-book recognized-revenue total is in QuickBooks (report pnl)."

			return emitRevenue(cmd, flags, view, func(w io.Writer) {
				fmt.Fprintf(w, "site MRR: %s   customers: %d   subscriptions: %d\n", revenue.Cents(view.SiteMRRCents), view.ActiveCustomers, view.ActiveSubscription)
				fmt.Fprintf(w, "ARPA: %s/mo   ARPU: %s/mo\n", revenue.Cents(view.ARPACents), revenue.Cents(view.ARPUCents))
				fmt.Fprintf(w, "MRR percentiles  p50 %s   p90 %s   p99 %s\n", revenue.Cents(view.P50Cents), revenue.Cents(view.P90Cents), revenue.Cents(view.P99Cents))
				if d := view.Decomposition; d != nil {
					fmt.Fprintf(w, "\nprice-vs-volume %s → %s  (ΔMRR %s = volume %s + price %s)\n",
						d.PriorSnapshot[:min(10, len(d.PriorSnapshot))], d.CurrentSnapshot[:min(10, len(d.CurrentSnapshot))],
						revenue.Cents(d.DeltaMRRCents), revenue.Cents(d.VolumeCents), revenue.Cents(d.PriceCents))
				} else {
					fmt.Fprintln(w, "\nprice-vs-volume: needs ≥2 snapshots (run `mrr sync` over time)")
				}
			})
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

// percentileDesc returns the MRR at the given percentile from a descending-
// sorted slice. p90 → a large customer, p50 → the median.
func percentileDesc(custs []custMRR, p int) int64 {
	n := len(custs)
	if n == 0 {
		return 0
	}
	// rank from the bottom: p-th percentile is the value below which p% fall.
	idxFromBottom := (p * n) / 100
	if idxFromBottom >= n {
		idxFromBottom = n - 1
	}
	// custs[0] is the largest, custs[n-1] the smallest; convert bottom-rank to
	// the descending index.
	i := n - 1 - idxFromBottom
	if i < 0 {
		i = 0
	}
	return custs[i].MRRCents
}

// priceVolumeDecomp computes the volume/price split of the MRR change between
// the two most recent snapshots. price is set to delta-volume so the two terms
// sum to ΔMRR exactly (no ±1¢ truncation drift). Returns nil when fewer than
// two snapshots exist or either has zero logos.
func priceVolumeDecomp(ctx context.Context, db *sql.DB) *arpaDecomp {
	rows, err := db.QueryContext(ctx, `
		SELECT s.snapshot_at,
		       SUM(s.mrr_cents) AS mrr,
		       COUNT(DISTINCT json_extract(sub.data,'$.customer.id')) AS logos
		FROM rev_sub_mrr_snapshots s
		JOIN resources sub ON sub.id = s.subscription_id AND sub.resource_type='subscriptions-json'
		WHERE s.mrr_cents > 0
		GROUP BY s.snapshot_at
		ORDER BY s.snapshot_at DESC
		LIMIT 2`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	type snap struct {
		at    string
		mrr   int64
		logos int
	}
	var snaps []snap
	for rows.Next() {
		var at sql.NullString
		var mrr, logos sql.NullInt64
		if err := rows.Scan(&at, &mrr, &logos); err != nil {
			continue
		}
		snaps = append(snaps, snap{at.String, mrr.Int64, int(logos.Int64)})
	}
	if len(snaps) < 2 || snaps[0].logos == 0 || snaps[1].logos == 0 {
		return nil
	}
	cur, prior := snaps[0], snaps[1]
	arpaPrior := float64(prior.mrr) / float64(prior.logos)
	delta := cur.mrr - prior.mrr
	volume := int64(math.Round(float64(cur.logos-prior.logos) * arpaPrior))
	return &arpaDecomp{
		PriorSnapshot:   prior.at,
		PriorMRRCents:   prior.mrr,
		PriorLogos:      prior.logos,
		CurrentSnapshot: cur.at,
		CurrentMRRCents: cur.mrr,
		CurrentLogos:    cur.logos,
		DeltaMRRCents:   delta,
		VolumeCents:     volume,
		PriceCents:      delta - volume,
	}
}
