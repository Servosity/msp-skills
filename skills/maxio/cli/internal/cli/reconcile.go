package cli

import (
	"database/sql"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"maxio-pp-cli/internal/revenue"
)

type reconcileRow struct {
	Customer      string `json:"customer"`
	CustomerID    string `json:"customer_id"`
	MRRCents      int64  `json:"mrr_cents"`
	InvoicedCents int64  `json:"invoiced_cents"`
	ExpectedCents int64  `json:"expected_cents"`
	GapCents      int64  `json:"gap_cents"`
	Flag          bool   `json:"flag"`
	FlagReason    string `json:"flag_reason,omitempty"`
}

type reconcileTotals struct {
	MRRCents      int64 `json:"mrr_cents"`
	InvoicedCents int64 `json:"invoiced_cents"`
	ExpectedCents int64 `json:"expected_cents"`
	GapCents      int64 `json:"gap_cents"`
}

type reconcileView struct {
	Since     string          `json:"since,omitempty"`
	Months    int             `json:"months"`
	Customers []reconcileRow  `json:"customers"`
	Totals    reconcileTotals `json:"totals"`
	Note      string          `json:"note"`
}

// pp:data-source local
//
// newNovelReconcileCmd compares normalized monthly MRR against the amount
// actually invoiced over the period, per customer, and flags large mismatches.
// This reconciles normalized MRR vs Advanced Billing invoices only — it is NOT
// GAAP recognized revenue (deferred-revenue recognition is out of scope).
func newNovelReconcileCmd(flags *rootFlags) *cobra.Command {
	var flagSince, flagCustomer, dbPath string
	var limit int
	cmd := &cobra.Command{
		Use:         "reconcile",
		Short:       "Per-customer gaps between normalized MRR and amounts actually invoiced, with mismatches flagged.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example:     "  maxio-cli reconcile --since 3m\n  maxio-cli reconcile --customer \"Acme\" --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			since := strings.TrimSpace(flagSince)
			if since == "" {
				since = "1m"
			}
			now := time.Now()
			cutoff, err := revenue.ParseSince(since, now)
			if err != nil {
				return usageErr(err)
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
			}

			// months_in_period: how many months the invoiced window spans, used to
			// scale monthly MRR up to an expected billed amount.
			months := 1
			if cutoff != "" {
				if cut, perr := time.Parse("2006-01-02", cutoff); perr == nil {
					days := now.Sub(cut).Hours() / 24
					months = int(math.Round(days / 30))
					if months < 1 {
						months = 1
					}
				}
			}

			// Per active subscription: latest MRR + invoiced-in-period, joined to a
			// customer display name. Invoiced amount comes from invoices-json whose
			// total_amount is dollars-as-string (×100 → cents).
			rows, qerr := db.QueryContext(cmd.Context(), `
				SELECT CAST(json_extract(sub.data,'$.customer.id') AS TEXT) AS cust_id,
				       COALESCE(NULLIF(json_extract(cust.data,'$.organization'),''),
				                TRIM(COALESCE(json_extract(cust.data,'$.first_name'),'')||' '||COALESCE(json_extract(cust.data,'$.last_name'),'')),
				                CAST(json_extract(sub.data,'$.customer.id') AS TEXT)) AS cust_name,
				       COALESCE(json_extract(cust.data,'$.reference'),'') AS cust_ref,
				       COALESCE(s.mrr_cents,0) AS mrr_cents,
				       (SELECT COALESCE(SUM(CAST(json_extract(inv.data,'$.total_amount') AS REAL)*100),0)
				          FROM resources inv
				         WHERE inv.resource_type='invoices-json'
				           AND CAST(json_extract(inv.data,'$.subscription_id') AS TEXT) = sub.id
				           AND (? = '' OR json_extract(inv.data,'$.issue_date') >= ?)) AS invoiced_cents
				FROM resources sub
				LEFT JOIN rev_sub_mrr_snapshots s ON s.subscription_id = sub.id AND s.snapshot_at = ?
				LEFT JOIN resources cust ON cust.resource_type='customers-json'
				     AND cust.id = CAST(json_extract(sub.data,'$.customer.id') AS TEXT)
				WHERE sub.resource_type='subscriptions-json' AND json_extract(sub.data,'$.state')='active'`,
				cutoff, cutoff, latest.String)
			if qerr != nil {
				return fmt.Errorf("querying reconciliation: %w", qerr)
			}
			defer rows.Close()

			type custAgg struct {
				name, ref     string
				mrr, invoiced int64
			}
			byCust := map[string]*custAgg{}
			var order []string
			for rows.Next() {
				var custID, name, ref sql.NullString
				var mrr sql.NullInt64
				var invoiced sql.NullFloat64
				if err := rows.Scan(&custID, &name, &ref, &mrr, &invoiced); err != nil {
					continue
				}
				id := custID.String
				a := byCust[id]
				if a == nil {
					a = &custAgg{name: name.String, ref: ref.String}
					byCust[id] = a
					order = append(order, id)
				}
				a.mrr += mrr.Int64
				a.invoiced += int64(math.Round(invoiced.Float64))
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("scanning reconciliation: %w", err)
			}

			filter := strings.TrimSpace(strings.ToLower(flagCustomer))
			out := make([]reconcileRow, 0, len(order))
			var totals reconcileTotals
			for _, id := range order {
				a := byCust[id]
				if filter != "" {
					if !strings.Contains(strings.ToLower(a.name), filter) &&
						!strings.EqualFold(id, flagCustomer) &&
						!strings.EqualFold(a.ref, flagCustomer) {
						continue
					}
				}
				expected := a.mrr * int64(months)
				gap := a.invoiced - expected
				flag, reason := reconcileFlag(a.mrr, a.invoiced, expected, gap)
				row := reconcileRow{
					Customer:      a.name,
					CustomerID:    id,
					MRRCents:      a.mrr,
					InvoicedCents: a.invoiced,
					ExpectedCents: expected,
					GapCents:      gap,
					Flag:          flag,
					FlagReason:    reason,
				}
				out = append(out, row)
				totals.MRRCents += a.mrr
				totals.InvoicedCents += a.invoiced
				totals.ExpectedCents += expected
				totals.GapCents += gap
			}

			sort.SliceStable(out, func(i, j int) bool {
				return absInt64(out[i].GapCents) > absInt64(out[j].GapCents)
			})
			if limit > 0 && len(out) > limit {
				out = out[:limit]
			}

			view := reconcileView{
				Since:     cutoff,
				Months:    months,
				Customers: out,
				Totals:    totals,
				Note:      "reconciles normalized MRR vs Advanced Billing invoices only — not GAAP recognized revenue; a customer is flagged only when |gap| exceeds 50% of expected billed amount (or has MRR with no invoices, or invoices with no MRR), so a sizable book-wide gap can show with few per-customer flags",
			}
			if view.Customers == nil {
				view.Customers = []reconcileRow{}
			}

			return emitRevenue(cmd, flags, view, func(w io.Writer) {
				fmt.Fprintf(w, "MRR vs invoiced reconciliation (%d month window)\n", view.Months)
				fmt.Fprintf(w, "%-34s %12s %12s %12s %12s\n", "customer", "mrr/mo", "invoiced", "expected", "gap")
				for _, r := range view.Customers {
					mark := " "
					if r.Flag {
						mark = "!"
					}
					fmt.Fprintf(w, "%s %-32s %12s %12s %12s %12s\n", mark, truncate(r.Customer, 32),
						revenue.Cents(r.MRRCents), revenue.Cents(r.InvoicedCents),
						revenue.Cents(r.ExpectedCents), revenue.Cents(r.GapCents))
				}
				t := view.Totals
				fmt.Fprintf(w, "  %-32s %12s %12s %12s %12s\n", "TOTAL",
					revenue.Cents(t.MRRCents), revenue.Cents(t.InvoicedCents),
					revenue.Cents(t.ExpectedCents), revenue.Cents(t.GapCents))
				fmt.Fprintf(w, "note: %s\n", view.Note)
			})
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "", "Reconciliation window (YYYY-MM-DD or 1m/3m/...; default 1m)")
	cmd.Flags().StringVar(&flagCustomer, "customer", "", "Filter to one customer by id, reference, or name substring")
	cmd.Flags().IntVar(&limit, "limit", 50, "Max customers to return (ranked by |gap|)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

// reconcileFlag decides whether a customer's MRR-vs-invoiced gap warrants a flag
// and returns a human reason. A gap larger than half the expected amount, or a
// one-sided presence (MRR with no invoices, or invoices with no MRR), flags.
func reconcileFlag(mrr, invoiced, expected, gap int64) (bool, string) {
	if mrr > 0 && invoiced == 0 {
		return true, "normalized MRR but nothing invoiced in window"
	}
	if invoiced > 0 && mrr == 0 {
		return true, "invoiced but no normalized MRR (one-time / non-recurring?)"
	}
	if expected > 0 && absInt64(gap) > expected/2 {
		return true, "gap exceeds 50% of expected billed amount"
	}
	return false, ""
}

func absInt64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
