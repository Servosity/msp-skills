// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: normalized licence-position export.
// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// positionSchema versions the shared licence-position row format. Pax8,
// Sherweb or PSA exports are mapped into these column names for position trueup.
const positionSchema = "licence-position/v1"

// positionColumns is the canonical column order for CSV and docs.
var positionColumns = []string{"schema", "distributor", "customer_id", "customer_name", "tenant_id", "customer_domain",
	"vendor", "product", "sku_key", "sku_name", "seats", "term", "billing_cycle", "monthly_cost", "monthly_price",
	"currency", "status", "auto_renew", "renews_on", "source_id", "cost_known"}

type positionRow struct {
	Schema         string  `json:"schema"`
	Distributor    string  `json:"distributor"`
	CustomerID     string  `json:"customer_id"`
	CustomerName   string  `json:"customer_name"`
	TenantID       string  `json:"tenant_id"`
	CustomerDomain string  `json:"customer_domain"`
	Vendor         string  `json:"vendor"`
	Product        string  `json:"product"`
	SKUKey         string  `json:"sku_key"`
	SKUName        string  `json:"sku_name"`
	Seats          float64 `json:"seats"`
	SeatsKnown     bool    `json:"seats_known"` // false when ION reported no licence count; seats is then 0, and position trueup adds no seats and flags seats_incomplete
	Term           string  `json:"term"`
	BillingCycle   string  `json:"billing_cycle"`
	MonthlyCost    float64 `json:"monthly_cost"`
	MonthlyPrice   float64 `json:"monthly_price"`
	PriceKnown     bool    `json:"price_known"` // false when ION reported no price or only part of it, or the billing cycle is not recognized: monthly_price is then 0
	Currency       string  `json:"currency"`
	Status         string  `json:"status"`
	AutoRenew      string  `json:"auto_renew"`
	RenewsOn       string  `json:"renews_on"`
	SourceID       string  `json:"source_id"`
	Trial          bool    `json:"trial"` // a TD SYNNEX trial subscription: its seats are not paid for
	seatsProblem   string  // why a file row's seat count is unknown
	CostKnown      bool    `json:"cost_known"` // false when ION (or the file) reported no cost or only part of it: monthly_cost is then 0 or a partial sum, not a real price
}

func ionPositions(b *ionBook, includeInactive bool) []positionRow {
	rows := make([]positionRow, 0, len(b.Subs))
	for _, s := range b.Subs {
		if s.Inactive() && !includeInactive {
			continue
		}
		m := s.MonthlyMoney()
		vendor := strings.ToLower(s.ResourceType)
		if i := strings.Index(vendor, "::"); i >= 0 {
			vendor = vendor[:i]
		}
		product := s.ProductInfo.ProductDisplayName
		if product == "" {
			product = s.CCPProductID
		}
		r := positionRow{
			Schema: positionSchema, Distributor: "td-synnex", CustomerID: s.CustomerID,
			CustomerName: b.CustomerLabel(s.CustomerID, s.CustomerName), Vendor: vendor, Product: product,
			SKUKey: s.SKUKey(), SKUName: s.Label(), Seats: s.Seats.V, SeatsKnown: s.Seats.Set, Term: s.BillingTerm,
			BillingCycle: s.BillingCycle, MonthlyCost: round2(m.Cost), MonthlyPrice: round2(m.Price), PriceKnown: m.PriceKnown,
			Currency: m.Currency, Status: s.Status, AutoRenew: s.AutoRenewState(), SourceID: s.ID, CostKnown: m.CostKnown, Trial: s.IsTrial,
		}
		if t := s.RenewOn(); !t.IsZero() {
			r.RenewsOn = t.Format("2006-01-02")
		}
		rows = append(rows, r)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].CustomerName != rows[j].CustomerName {
			return rows[i].CustomerName < rows[j].CustomerName
		}
		return rows[i].SKUKey < rows[j].SKUKey
	})
	return rows
}

func newNovelPositionExportCmd(flags *rootFlags) *cobra.Command {
	var includeInactive bool
	var dbPath string

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export your TD SYNNEX licences as normalized, schema-versioned rows (customer, SKU key, seats, monthly cost and price, renewal)",
		Long: "Export every active synced subscription as one normalized licence-position row (schema " + positionSchema + "): " +
			"distributor, customer, vendor, product, sku_key, seats, term, monthly cost and price, currency, status, " +
			"auto-renew and renewal date. sku_key is the Microsoft product:SKU pair (for example CFQ7TTC0LDPB:0001) when " +
			"ION has it, which is the key Pax8 and Sherweb exports can be mapped onto. tenant_id and customer_domain are " +
			"left empty because ION subscriptions do not carry them; fill them in mapped files if you have them. Reads the " +
			"local store; run sync --resources subscriptions,customers first.\n\n" +
			"Use this command to export TD SYNNEX licences in the shared position format for a PSA or another tool. Do NOT " +
			"use this command to compare distributors; use 'position trueup' instead. Do NOT use it for margin analysis; " +
			"use 'margin' instead.",
		Example: strings.Trim(`
  td-synnex-cli position export --agent --select customer_name,sku_key,seats,seats_known,monthly_price,price_known
  td-synnex-cli position export --csv`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "position export")
			}
			book, closeDB, err := openIONBook(cmd, flags, dbPath, "subscriptions", "customers")
			if err != nil {
				return err
			}
			defer closeDB()
			rows := make([]positionRow, 0)
			if book != nil {
				rows = ionPositions(book, includeInactive)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printIONJSON(cmd.OutOrStdout(), rows, flags)
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No subscriptions in the local store.")
				return nil
			}
			table := make([][]string, 0, len(rows))
			for _, r := range rows {
				table = append(table, []string{r.CustomerName, r.SKUKey, r.SKUName, seatsText(r.Seats, r.SeatsKnown),
					moneyText(r.MonthlyCost, r.CostKnown), moneyText(r.MonthlyPrice, r.PriceKnown), r.Currency, r.RenewsOn})
			}
			return printTable(cmd.OutOrStdout(), []string{"CUSTOMER", "SKU KEY", "SKU", "SEATS", "COST/MO", "PRICE/MO", "CUR", "RENEWS"}, table)
		},
	}
	cmd.Flags().BoolVar(&includeInactive, "include-inactive", false, "Include cancelled, deleted and expired subscriptions")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: the CLI's synced database)")
	return cmd
}
