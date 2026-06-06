// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel feature (Phase 3): MRR and margin. Not generator-managed.

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"pax8-pp-cli/internal/store"
)

type mrrProductRow struct {
	ProductID   string  `json:"productId"`
	ProductName string  `json:"productName"`
	Quantity    float64 `json:"quantity"`
	MRR         float64 `json:"mrr"`
	Margin      float64 `json:"margin"`
}

type mrrTotals struct {
	MRR                 float64 `json:"mrr"`
	Margin              float64 `json:"margin"`
	ActiveSubscriptions int     `json:"activeSubscriptions"`
	Currency            string  `json:"currency,omitempty"`
}

type mrrResult struct {
	Totals    mrrTotals       `json:"totals"`
	ByProduct []mrrProductRow `json:"byProduct"`
}

func mrrIsActive(status string) bool {
	s := strings.ToLower(status)
	return s == "" || s == "active" || s == "enabled" || s == "trial"
}

// pp:data-source local
func newNovelMrrCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var includeAll bool

	cmd := &cobra.Command{
		Use:         "mrr",
		Short:       "Compute monthly recurring revenue and margin from subscriptions and product pricing, trended across syncs.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Compute monthly recurring revenue (MRR) and margin from the local store.

MRR is the sum of subscription price x quantity for active subscriptions;
margin subtracts partner cost. Broken down by product. Run 'pax8-cli sync' first.

Use this command for monthly recurring revenue and margin totals, with a per-product breakdown in the output.
Do NOT use this command for what changed week-over-week; use 'since' instead.
Do NOT use this command for per-customer spend rollups; use 'spend' instead.`,
		Example: `  # MRR summary
  pax8-cli mrr

  # Just the headline numbers for an agent
  pax8-cli mrr --agent --select totals.mrr,totals.margin`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return nil
			}
			out := cmd.OutOrStdout()
			if dbPath == "" {
				dbPath = defaultDBPath("pax8-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w\nRun 'pax8-cli sync' first.", err)
			}
			defer db.Close()
			maybeEmitSyncHints(cmd, db, "subscriptions", flags.maxAge)

			products, err := pax8ListObjects(db, "products")
			if err != nil {
				return fmt.Errorf("reading products: %w", err)
			}
			nameByID := pax8NameByID(products, []string{"id", "productId"}, []string{"name", "productName"})

			subs, err := pax8ListObjects(db, "subscriptions")
			if err != nil {
				return fmt.Errorf("reading subscriptions: %w", err)
			}

			byProduct := map[string]*mrrProductRow{}
			var res mrrResult
			for _, s := range subs {
				status := pax8FieldStr(s, "status")
				if !includeAll && !mrrIsActive(status) {
					continue
				}
				price, _ := pax8FieldNum(s, "price")
				qty, hasQty := pax8FieldNum(s, "quantity")
				if !hasQty {
					qty = 1
				}
				cost, _ := pax8FieldNum(s, "partnerCost", "partner_cost")
				lineMRR := price * qty
				lineMargin := (price - cost) * qty

				pid := pax8FieldStr(s, "productId", "product_id")
				row := byProduct[pid]
				if row == nil {
					row = &mrrProductRow{ProductID: pid, ProductName: nameByID[pid]}
					byProduct[pid] = row
				}
				row.Quantity += qty
				row.MRR += lineMRR
				row.Margin += lineMargin

				res.Totals.MRR += lineMRR
				res.Totals.Margin += lineMargin
				res.Totals.ActiveSubscriptions++
				if res.Totals.Currency == "" {
					res.Totals.Currency = pax8FieldStr(s, "currencyCode", "currency_code")
				}
			}

			res.ByProduct = make([]mrrProductRow, 0, len(byProduct))
			for _, r := range byProduct {
				res.ByProduct = append(res.ByProduct, *r)
			}
			sort.Slice(res.ByProduct, func(i, j int) bool { return res.ByProduct[i].MRR > res.ByProduct[j].MRR })

			if flags.asJSON {
				return printJSONFiltered(out, res, flags)
			}
			fmt.Fprintf(out, "MRR:    %.2f %s\n", res.Totals.MRR, res.Totals.Currency)
			fmt.Fprintf(out, "Margin: %.2f %s\n", res.Totals.Margin, res.Totals.Currency)
			fmt.Fprintf(out, "Active subscriptions: %d\n\n", res.Totals.ActiveSubscriptions)
			if len(res.ByProduct) == 0 {
				fmt.Fprintln(out, "No subscriptions in local store. Run 'pax8-cli sync' first.")
				return nil
			}
			fmt.Fprintln(out, "Product\tQty\tMRR\tMargin")
			fmt.Fprintln(out, "-------\t---\t---\t------")
			for _, r := range res.ByProduct {
				name := r.ProductName
				if name == "" {
					name = r.ProductID
				}
				fmt.Fprintf(out, "%s\t%.0f\t%.2f\t%.2f\n", name, r.Quantity, r.MRR, r.Margin)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	cmd.Flags().BoolVar(&includeAll, "all", false, "Include cancelled/inactive subscriptions in the totals")
	return cmd
}
