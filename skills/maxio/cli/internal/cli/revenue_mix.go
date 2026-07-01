package cli

import (
	"database/sql"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"maxio-pp-cli/internal/revenue"
)

type revMixKind struct {
	Kind        string  `json:"kind"`
	AmountCents int64   `json:"amount_cents"`
	Pct         float64 `json:"pct"`
}

type revMixView struct {
	Month          string       `json:"month"`
	TotalCents     int64        `json:"total_cents"`
	RecurringPct   float64      `json:"recurring_pct"`
	UsagePct       float64      `json:"usage_pct"`
	ServicesPct    float64      `json:"services_pct"`
	RecurringCents int64        `json:"recurring_cents"`
	UsageCents     int64        `json:"usage_cents"`
	ServicesCents  int64        `json:"services_cents"`
	ByKind         []revMixKind `json:"by_kind"`
	Note           string       `json:"note,omitempty"`
}

// kindToType maps a Chargify invoice line-item kind to a revenue type. In
// Chargify, quantity_based_component is RECURRING per-unit pricing (e.g.
// per-protected-device), not consumption — only metered_component is usage.
// baseline/on_off/delay_capture/quantity_based are recurring subscription
// revenue; metered is usage; unknown kinds fall to services/other.
func kindToType(kind string) string {
	switch kind {
	case "baseline", "on_off_component", "delay_capture", "quantity_based_component":
		return "recurring"
	case "metered_component", "metered_usage", "prepaid_usage":
		return "usage"
	default:
		return "services"
	}
}

// pp:data-source local
//
// newNovelRevenueMixCmd splits the most recent month's billed line items into
// recurring vs usage vs services, from invoice line-item kinds. This is a
// billing-line-based mix (Maxio), distinct from the GL recognized-revenue
// figures the deal-room skill pulls from QuickBooks.
func newNovelRevenueMixCmd(flags *rootFlags) *cobra.Command {
	var dbPath, flagMonth string
	cmd := &cobra.Command{
		Use:         "revenue-mix",
		Short:       "Recurring vs usage vs services split of the latest month's billed line items.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example:     "  maxio-cli revenue-mix\n  maxio-cli revenue-mix --month 2026-05 --json",
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

			month := flagMonth
			if month == "" {
				var m sql.NullString
				_ = db.QueryRowContext(cmd.Context(), `
					SELECT MAX(substr(json_extract(data,'$.issue_date'),1,7))
					FROM resources WHERE resource_type='invoices-json'`).Scan(&m)
				month = m.String
			}
			if month == "" {
				hintIfNoInvoices(cmd.ErrOrStderr())
				return emitRevenue(cmd, flags, revMixView{ByKind: []revMixKind{}, Note: "no invoices synced — run `maxio-cli sync --resources invoices-json --param line_items=true`"}, func(w io.Writer) {
					fmt.Fprintln(w, "no invoices synced")
				})
			}

			// json_each iterates the line_items array; total_amount is a dollar
			// string, converted to cents. Voided/canceled/draft invoices excluded.
			rows, qerr := db.QueryContext(cmd.Context(), `
				SELECT COALESCE(json_extract(li.value,'$.kind'),'unknown') AS kind,
				       COALESCE(SUM(CAST(json_extract(li.value,'$.total_amount') AS REAL)), 0) AS amt
				FROM resources inv, json_each(json_extract(inv.data,'$.line_items')) li
				WHERE inv.resource_type='invoices-json'
				  AND json_extract(inv.data,'$.status') NOT IN ('voided','canceled','draft')
				  AND substr(json_extract(inv.data,'$.issue_date'),1,7) = ?
				GROUP BY kind
				ORDER BY amt DESC`, month)
			if qerr != nil {
				return fmt.Errorf("querying revenue mix: %w", qerr)
			}
			defer rows.Close()

			view := revMixView{Month: month, ByKind: []revMixKind{}}
			for rows.Next() {
				var kind sql.NullString
				var amt sql.NullFloat64
				if err := rows.Scan(&kind, &amt); err != nil {
					continue
				}
				cents := int64(amt.Float64*100 + 0.5)
				view.ByKind = append(view.ByKind, revMixKind{Kind: kind.String, AmountCents: cents})
				view.TotalCents += cents
				switch kindToType(kind.String) {
				case "recurring":
					view.RecurringCents += cents
				case "usage":
					view.UsageCents += cents
				default:
					view.ServicesCents += cents
				}
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("scanning revenue mix: %w", err)
			}
			if view.TotalCents > 0 {
				for i := range view.ByKind {
					view.ByKind[i].Pct = round2(float64(view.ByKind[i].AmountCents) / float64(view.TotalCents) * 100)
				}
				view.RecurringPct = round2(float64(view.RecurringCents) / float64(view.TotalCents) * 100)
				view.UsagePct = round2(float64(view.UsageCents) / float64(view.TotalCents) * 100)
				view.ServicesPct = round2(float64(view.ServicesCents) / float64(view.TotalCents) * 100)
			}
			view.Note = "billing-line mix from invoice line-item kinds (baseline/on_off/delay_capture=recurring, quantity_based=usage). For recognized-revenue GL figures use the QuickBooks side."

			return emitRevenue(cmd, flags, view, func(w io.Writer) {
				fmt.Fprintf(w, "month %s   total billed: %s\n", view.Month, revenue.Cents(view.TotalCents))
				fmt.Fprintf(w, "recurring %5.1f%%   usage %5.1f%%   services %5.1f%%\n\n", view.RecurringPct, view.UsagePct, view.ServicesPct)
				fmt.Fprintf(w, "%-28s %14s %8s\n", "kind", "amount", "pct")
				for _, k := range view.ByKind {
					fmt.Fprintf(w, "%-28s %14s %7.1f%%\n", k.Kind, revenue.Cents(k.AmountCents), k.Pct)
				}
			})
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	cmd.Flags().StringVar(&flagMonth, "month", "", "Month as YYYY-MM (default: latest invoiced month)")
	return cmd
}

func hintIfNoInvoices(w io.Writer) {
	fmt.Fprintln(w, "hint: no invoices in the local store. Run: maxio-cli sync --resources invoices-json --param line_items=true")
}
