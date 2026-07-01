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

type triageAccount struct {
	Customer    string   `json:"customer"`
	CustomerID  string   `json:"customer_id"`
	MRRCents    int64    `json:"mrr_cents"`
	Score       int      `json:"score"`
	State       string   `json:"state"`
	NextRenewal string   `json:"next_renewal,omitempty"`
	Reasons     []string `json:"reasons"`
}

type triageView struct {
	GeneratedAtWindow string          `json:"generated_at_window"`
	Accounts          []triageAccount `json:"accounts"`
}

// pp:data-source local
//
// newNovelTriageCmd ranks active-subscription customers by an attention score
// built from past-due/unpaid state, large renewals approaching inside the next
// 30 days, and high MRR concentration (top-decile accounts). It reads only the
// local store; nothing here mutates external state.
func newNovelTriageCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit int
	cmd := &cobra.Command{
		Use:         "triage",
		Short:       "A ranked list of accounts that need attention: past-due, large upcoming renewals, high-value concentration.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example:     "  maxio-cli triage\n  maxio-cli triage --limit 30 --json",
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

			// One row per active subscription, joined to the latest per-sub MRR
			// snapshot and the customer record for a display name.
			rows, qerr := db.QueryContext(cmd.Context(), `
				SELECT CAST(json_extract(sub.data,'$.customer.id') AS TEXT) AS cust_id,
				       COALESCE(NULLIF(json_extract(cust.data,'$.organization'),''),
				                TRIM(COALESCE(json_extract(cust.data,'$.first_name'),'')||' '||COALESCE(json_extract(cust.data,'$.last_name'),'')),
				                CAST(json_extract(sub.data,'$.customer.id') AS TEXT)) AS cust_name,
				       json_extract(sub.data,'$.state') AS state,
				       json_extract(sub.data,'$.current_period_ends_at') AS period_ends,
				       COALESCE(s.mrr_cents,0) AS mrr_cents
				FROM resources sub
				LEFT JOIN rev_sub_mrr_snapshots s ON s.subscription_id = sub.id AND s.snapshot_at = ?
				LEFT JOIN resources cust ON cust.resource_type='customers-json'
				     AND cust.id = CAST(json_extract(sub.data,'$.customer.id') AS TEXT)
				WHERE sub.resource_type='subscriptions-json' AND json_extract(sub.data,'$.state')='active'`,
				latest.String)
			if qerr != nil {
				return fmt.Errorf("querying active subscriptions: %w", qerr)
			}
			defer rows.Close()

			type custAgg struct {
				name        string
				mrr         int64
				state       string
				nextRenewal string // earliest upcoming period-end among the customer's subs
				reasons     []string
				score       int
			}
			byCust := map[string]*custAgg{}
			var order []string
			now := time.Now()
			window := now.AddDate(0, 0, 30)

			for rows.Next() {
				var custID, name, state, periodEnds sql.NullString
				var mrr sql.NullInt64
				if err := rows.Scan(&custID, &name, &state, &periodEnds, &mrr); err != nil {
					continue
				}
				id := custID.String
				a := byCust[id]
				if a == nil {
					a = &custAgg{name: name.String, state: state.String}
					byCust[id] = a
					order = append(order, id)
				}
				a.mrr += mrr.Int64

				// Past-due / unpaid / dunning subscription state on any sub.
				switch state.String {
				case "past_due", "unpaid", "dunning":
					if a.state != "past_due" && a.state != "unpaid" && a.state != "dunning" {
						a.state = state.String
					}
					if !containsStr(a.reasons, "past-due/unpaid subscription") {
						a.reasons = append(a.reasons, "past-due/unpaid subscription")
						a.score += 50
					}
				}

				// Large renewal approaching inside the next 30 days.
				if periodEnds.Valid && periodEnds.String != "" {
					if t, perr := time.Parse(time.RFC3339, periodEnds.String); perr == nil {
						if t.After(now) && t.Before(window) {
							days := int(t.Sub(now).Hours() / 24)
							if a.nextRenewal == "" || periodEnds.String < a.nextRenewal {
								a.nextRenewal = periodEnds.String
							}
							subMRR := mrr.Int64
							if subMRR > 0 {
								// Weight by MRR: +30 base scaled up to +60 for a
								// $1k+/mo sub so a large renewal outranks a small one.
								bump := 30
								if extra := int(subMRR / 100000 * 30); extra > 0 {
									if extra > 30 {
										extra = 30
									}
									bump += extra
								}
								a.score += bump
								a.reasons = append(a.reasons,
									fmt.Sprintf("large renewal in %dd (%s)", days, revenue.Cents(subMRR)))
							}
						}
					}
				}
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("scanning active subscriptions: %w", err)
			}

			if len(order) == 0 {
				hintRunMrrSync(cmd.ErrOrStderr())
				return emitRevenue(cmd, flags, triageView{GeneratedAtWindow: "30d", Accounts: []triageAccount{}}, func(w io.Writer) {
					fmt.Fprintln(w, "No active subscriptions found.")
				})
			}

			// High MRR concentration: flag accounts in the top decile by MRR.
			mrrs := make([]int64, 0, len(byCust))
			for _, a := range byCust {
				mrrs = append(mrrs, a.mrr)
			}
			decile := topDecileThreshold(mrrs)
			for _, a := range byCust {
				if decile > 0 && a.mrr >= decile {
					a.score += 10
					a.reasons = append(a.reasons, "high-value account")
				}
			}

			accounts := make([]triageAccount, 0, len(order))
			for _, id := range order {
				a := byCust[id]
				accounts = append(accounts, triageAccount{
					Customer:    a.name,
					CustomerID:  id,
					MRRCents:    a.mrr,
					Score:       a.score,
					State:       a.state,
					NextRenewal: a.nextRenewal,
					Reasons:     a.reasons,
				})
			}
			sort.SliceStable(accounts, func(i, j int) bool {
				if accounts[i].Score != accounts[j].Score {
					return accounts[i].Score > accounts[j].Score
				}
				return accounts[i].MRRCents > accounts[j].MRRCents
			})
			if limit > 0 && len(accounts) > limit {
				accounts = accounts[:limit]
			}

			view := triageView{GeneratedAtWindow: "30d", Accounts: accounts}
			return emitRevenue(cmd, flags, view, func(w io.Writer) {
				fmt.Fprintf(w, "Accounts needing attention (renewal window 30d)\n")
				for _, a := range view.Accounts {
					fmt.Fprintf(w, "  [%3d] %-34s %12s  %s\n", a.Score, truncate(a.Customer, 34), revenue.Cents(a.MRRCents), a.State)
					for _, r := range a.Reasons {
						fmt.Fprintf(w, "         - %s\n", r)
					}
				}
			})
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "Max accounts to return")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

func containsStr(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// topDecileThreshold returns the MRR value at the 90th percentile across the
// supplied values, or 0 when there are too few customers to bother
// distinguishing a "top" segment.
func topDecileThreshold(mrrs []int64) int64 {
	if len(mrrs) < 10 {
		return 0
	}
	sorted := make([]int64, len(mrrs))
	copy(sorted, mrrs)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := int(float64(len(sorted)) * 0.9)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
