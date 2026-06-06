// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel feature (Phase 3): usage overage detection. Not generator-managed.

package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"pax8-pp-cli/internal/store"
)

type overageRow struct {
	UsageSummaryID string  `json:"usageSummaryId"`
	ProductID      string  `json:"productId"`
	VendorName     string  `json:"vendorName,omitempty"`
	CompanyID      string  `json:"companyId,omitempty"`
	CurrentCharges float64 `json:"currentCharges"`
	ProductAverage float64 `json:"productAverage"`
	OverageFactor  float64 `json:"overageFactor"`
}

// pp:data-source local
func newNovelOverageCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var threshold float64

	cmd := &cobra.Command{
		Use:         "overage",
		Short:       "Aggregate usage-lines per subscription and surface overages before they land on the customer invoice.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Surface metered-usage overages from the local store before they invoice.

For each product, the command computes the average current usage charge across
all synced usage summaries, then flags summaries whose charges exceed that average
by the threshold factor (default 1.5x). Run 'pax8-cli sync' first.`,
		Example: `  # Flag usage summaries 1.5x over their product average
  pax8-cli overage

  # Stricter: flag anything 2x over average, JSON
  pax8-cli overage --threshold 2 --agent`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return nil
			}
			out := cmd.OutOrStdout()
			if threshold <= 0 {
				threshold = 1.5
			}
			if dbPath == "" {
				dbPath = defaultDBPath("pax8-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w\nRun 'pax8-cli sync' first.", err)
			}
			defer db.Close()
			maybeEmitSyncHints(cmd, db, "usage_summaries", flags.maxAge)

			summaries, err := pax8ListObjects(db, "usage_summaries")
			if err != nil {
				return fmt.Errorf("reading usage summaries: %w", err)
			}

			// Pass 1: per-product running average of current charges.
			type acc struct {
				sum float64
				n   int
			}
			perProduct := map[string]*acc{}
			for _, s := range summaries {
				pid := pax8FieldStr(s, "productId", "product_id")
				charge, ok := pax8FieldNum(s, "currentCharges", "current_charges")
				if !ok {
					continue
				}
				a := perProduct[pid]
				if a == nil {
					a = &acc{}
					perProduct[pid] = a
				}
				a.sum += charge
				a.n++
			}
			avg := map[string]float64{}
			for pid, a := range perProduct {
				if a.n > 0 {
					avg[pid] = a.sum / float64(a.n)
				}
			}

			// Pass 2: flag summaries above threshold x product average.
			var flagged []overageRow
			for _, s := range summaries {
				pid := pax8FieldStr(s, "productId", "product_id")
				charge, ok := pax8FieldNum(s, "currentCharges", "current_charges")
				if !ok {
					continue
				}
				a := avg[pid]
				if a <= 0 {
					continue
				}
				factor := charge / a
				if factor < threshold {
					continue
				}
				flagged = append(flagged, overageRow{
					UsageSummaryID: pax8FieldStr(s, "id", "usageSummaryId"),
					ProductID:      pid,
					VendorName:     pax8FieldStr(s, "vendorName", "vendor_name"),
					CompanyID:      pax8FieldStr(s, "companyId", "company_id"),
					CurrentCharges: charge,
					ProductAverage: a,
					OverageFactor:  factor,
				})
			}
			sort.Slice(flagged, func(i, j int) bool { return flagged[i].OverageFactor > flagged[j].OverageFactor })

			if flags.asJSON {
				return printJSONFiltered(out, flagged, flags)
			}
			if len(flagged) == 0 {
				if len(summaries) == 0 {
					fmt.Fprintln(out, "No usage summaries in local store. Run 'pax8-cli sync' first.")
				} else {
					fmt.Fprintf(out, "No overages: no usage summary exceeds %.1fx its product average.\n", threshold)
				}
				return nil
			}
			fmt.Fprintln(out, "UsageSummary\tProduct\tCharges\tProdAvg\tFactor")
			fmt.Fprintln(out, "------------\t-------\t-------\t-------\t------")
			for _, r := range flagged {
				fmt.Fprintf(out, "%s\t%s\t%.2f\t%.2f\t%.2fx\n", r.UsageSummaryID, r.ProductID, r.CurrentCharges, r.ProductAverage, r.OverageFactor)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	cmd.Flags().Float64Var(&threshold, "threshold", 1.5, "Flag usage charges exceeding this multiple of the product average")
	return cmd
}
