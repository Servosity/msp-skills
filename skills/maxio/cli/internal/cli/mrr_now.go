package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"maxio-pp-cli/internal/revenue"
)

type growthView struct {
	PriorAt    string  `json:"prior_snapshot_at"`
	PriorCents int64   `json:"prior_mrr_cents"`
	DeltaCents int64   `json:"delta_cents"`
	Pct        float64 `json:"pct"`
}

type mrrNowView struct {
	MRRCents   int64       `json:"mrr_cents"`
	MRR        string      `json:"mrr"`
	ARRCents   int64       `json:"arr_cents"`
	ARR        string      `json:"arr"`
	ActiveSubs int64       `json:"active_subscriptions"`
	Currency   string      `json:"currency"`
	AsOf       string      `json:"as_of"`
	Source     string      `json:"source"`
	Growth     *growthView `json:"growth,omitempty"`
	Empty      bool        `json:"empty,omitempty"`
}

// pp:data-source auto
//
// newNovelMrrNowCmd reports current site MRR + ARR + growth vs the prior
// snapshot. Live by default (auto), falling back to the latest local snapshot.
func newNovelMrrNowCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:         "now",
		Short:       "Current MRR + ARR + growth vs the prior snapshot",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example:     "  maxio-cli mrr now\n  maxio-cli mrr now --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "auto"); err != nil {
				return err
			}
			st, db, err := openRevenueStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer st.Close()
			view, err := buildMrrNow(cmd.Context(), flags, db, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			return emitRevenue(cmd, flags, view, func(w io.Writer) {
				if view.Empty {
					return
				}
				fmt.Fprintf(w, "MRR  %s\nARR  %s\nActive subscriptions: %d\n",
					revenue.Cents(view.MRRCents), revenue.Cents(view.ARRCents), view.ActiveSubs)
				if view.Growth != nil {
					fmt.Fprintf(w, "Growth vs %s: %s (%+.1f%%)\n",
						shortDate(view.Growth.PriorAt), revenue.Cents(view.Growth.DeltaCents), view.Growth.Pct)
				}
				fmt.Fprintf(w, "Source: %s · as of %s\n", view.Source, view.AsOf)
			})
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

func buildMrrNow(ctx context.Context, flags *rootFlags, db *sql.DB, stderr io.Writer) (*mrrNowView, error) {
	view := &mrrNowView{Currency: "USD"}
	usedLive := false
	if flags.dataSource != "local" {
		if c, err := flags.newClient(); err == nil {
			if data, gerr := c.Get(ctx, "/mrr.json", nil); gerr == nil {
				var s struct {
					MRR struct {
						AmountInCents int64  `json:"amount_in_cents"`
						AtTime        string `json:"at_time"`
						Currency      string `json:"currency"`
					} `json:"mrr"`
				}
				if json.Unmarshal(data, &s) == nil {
					view.MRRCents = s.MRR.AmountInCents
					view.AsOf = s.MRR.AtTime
					if s.MRR.Currency != "" {
						view.Currency = s.MRR.Currency
					}
					usedLive = true
				}
			}
			if usedLive {
				if data, gerr := c.Get(ctx, "/stats.json", nil); gerr == nil {
					var st struct {
						Stats struct {
							TotalActiveSubscriptions int64 `json:"total_active_subscriptions"`
						} `json:"stats"`
					}
					if json.Unmarshal(data, &st) == nil {
						view.ActiveSubs = st.Stats.TotalActiveSubscriptions
					}
				}
			}
		}
	}

	var priorAt sql.NullString
	var priorCents, priorActive sql.NullInt64
	_ = db.QueryRowContext(ctx,
		`SELECT snapshot_at, mrr_cents, active_subs FROM rev_site_snapshots ORDER BY snapshot_at DESC LIMIT 1`).
		Scan(&priorAt, &priorCents, &priorActive)

	if !usedLive {
		if !priorAt.Valid {
			if flags.dataSource == "live" {
				return nil, fmt.Errorf("live MRR fetch failed and no data was returned")
			}
			hintRunMrrSync(stderr)
			view.Source = "local"
			view.Empty = true
			return view, nil
		}
		view.MRRCents = priorCents.Int64
		view.ActiveSubs = priorActive.Int64
		view.AsOf = priorAt.String
		view.Source = "local-snapshot"
		var p2At sql.NullString
		var p2Cents sql.NullInt64
		_ = db.QueryRowContext(ctx,
			`SELECT snapshot_at, mrr_cents FROM rev_site_snapshots WHERE snapshot_at < ? ORDER BY snapshot_at DESC LIMIT 1`,
			priorAt.String).Scan(&p2At, &p2Cents)
		if p2At.Valid {
			setGrowth(view, p2At.String, p2Cents.Int64)
		}
	} else {
		view.Source = "live"
		if priorAt.Valid {
			setGrowth(view, priorAt.String, priorCents.Int64)
		}
	}
	view.MRR = revenue.Cents(view.MRRCents)
	view.ARRCents = view.MRRCents * 12
	view.ARR = revenue.Cents(view.ARRCents)
	return view, nil
}

func setGrowth(view *mrrNowView, priorAt string, priorCents int64) {
	delta := view.MRRCents - priorCents
	pct := 0.0
	if priorCents != 0 {
		denom := priorCents
		if denom < 0 {
			denom = -denom
		}
		pct = round2(float64(delta) / float64(denom) * 100)
	}
	view.Growth = &growthView{PriorAt: priorAt, PriorCents: priorCents, DeltaCents: delta, Pct: pct}
}

func shortDate(ts string) string {
	if len(ts) >= 10 {
		return ts[:10]
	}
	return ts
}
