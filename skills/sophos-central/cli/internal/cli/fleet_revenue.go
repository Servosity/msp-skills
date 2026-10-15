// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"sophos-central-pp-cli/internal/cliutil"
)

// billingLine is the subset of a partner usage report line we aggregate.
type billingLine struct {
	AccountID         string `json:"accountId"`
	AccountIdentifier string `json:"assignedAccountIdentifier"`
	ContactName       string `json:"contactName"`
	ProductCode       string `json:"productCode"`
	ProductDesc       string `json:"productDescription"`
	SKU               string `json:"sku"`
	BillableQuantity  int    `json:"billableQuantity"`
	OrderedQuantity   int    `json:"orderedQuantity"`
	ActualQuantity    int    `json:"actualQuantity"`
	UnitPrice         string `json:"unitPrice"`
	UnitPriceCurrency string `json:"unitPriceCurrency"`
}

type billingProduct struct {
	Product  string  `json:"product"`
	Billable int     `json:"billable"`
	Revenue  float64 `json:"revenue"`
	Currency string  `json:"currency"`
	// One machine can consume several products (protection plus encryption),
	// so billed seats are compared with deployed endpoints per product.
	BillableMinusDeployed *int `json:"billable_minus_deployed,omitempty"`
}

// billingCustomer is one customer's monthly total.
type billingCustomer struct {
	AccountID         string             `json:"account_id"`
	TenantName        string             `json:"tenant_name"`
	Matched           bool               `json:"matched_tenant"`
	Revenue           map[string]float64 `json:"revenue"` // per currency; never summed across currencies
	Billable          int                `json:"billable_quantity"`
	Ordered           int                `json:"ordered_quantity"`
	Actual            int                `json:"actual_quantity"`
	DeployedEndpoints *int               `json:"deployed_endpoints"`
	Variance          *int               `json:"billable_minus_deployed"`
	Products          []billingProduct   `json:"products"`
	sortKey           float64
}

type billingView struct {
	Month          string             `json:"month"`
	Items          []billingCustomer  `json:"items"`
	Customers      int                `json:"customers"`
	TotalsCurrency map[string]float64 `json:"totals_by_currency"`
	Unmatched      []string           `json:"unmatched_account_ids"`
	Lines          int                `json:"lines"`
	JoinKey        string             `json:"join_key"`
	RankedBy       string             `json:"ranked_by_currency,omitempty"`
}

func newNovelFleetRevenueCmd(flags *rootFlags) *cobra.Command {
	var flagMonth, dbPath string
	var flagLimit int

	cmd := &cobra.Command{
		Use:   "revenue",
		Short: "Rank customers by monthly Sophos revenue for a billing month and compare billed quantities with deployed endpoints.",
		Long: `Use this command to rank customers by monthly Sophos revenue for a billing month and compare billed quantities with deployed endpoints. Do NOT use this command to fetch the raw usage report line items; use 'billing' instead.

Needs a partner credential. Reads every page of GET /partner/v1/billing/usage/{year}/{month}, totals billableQuantity x unitPrice per customer in each line's own currency (mixed currencies are reported separately, never summed), and joins each line's accountId to the tenants synced by 'fleet refresh' to compare billed seats with deployed endpoints. Lines whose accountId matches no synced tenant are listed under unmatched_account_ids rather than guessed by name. One machine can consume several products (protection plus encryption), so billable_minus_deployed is reported per product, and the customer-level value compares the largest single product line (usually the endpoint licence) with deployed endpoints.`,
		Example: strings.Trim(`
  sophos-central-cli fleet revenue --month 2026-09 --limit 20
  sophos-central-cli fleet revenue --agent --select items.tenant_name,items.revenue,items.billable_minus_deployed`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet revenue")
			}
			year, month, err := parseBillingMonth(flagMonth, time.Now().UTC())
			if err != nil {
				return usageErr(err)
			}
			if flagLimit <= 0 {
				return usageErr(fmt.Errorf("--limit must be positive"))
			}
			// No boundCtx: these runs span many tenants under the rate limit;
			// --timeout bounds each request inside the generated client.
			ctx := cmd.Context()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			who, err := callerIdentity(ctx, c)
			if err != nil {
				return classifyAPIErrorOnly(err)
			}
			if who.IDType != "partner" {
				return usageErr(fmt.Errorf("fleet revenue reads the partner usage report and needs a partner credential; this credential is a %s", who.IDType))
			}
			path := fmt.Sprintf("/partner/v1/billing/usage/%04d/%02d", year, month)
			max := 0
			if cliutil.IsDogfoodEnv() {
				max = 100
			}
			raws, err := fetchKeyPaged(ctx, c, path, map[string]string{"pageSize": "100"}, "", max)
			if err != nil {
				return classifyAPIErrorOnly(err)
			}
			lines := make([]billingLine, 0, len(raws))
			for _, r := range raws {
				var l billingLine
				if err := json.Unmarshal(r, &l); err != nil {
					return apiErr(fmt.Errorf("decoding a billing usage line for %04d-%02d: %w", year, month, err))
				}
				lines = append(lines, l)
			}
			names, deployed := billingJoinData(cmd, dbPath)
			view := buildBilling(fmt.Sprintf("%04d-%02d", year, month), lines, names, deployed)
			if len(view.Items) > flagLimit {
				view.Items = view.Items[:flagLimit]
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if len(view.Items) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No billable usage reported for %s.\n", view.Month)
				return nil
			}
			rows := make([]map[string]any, 0, len(view.Items))
			for _, it := range view.Items {
				rows = append(rows, map[string]any{"customer": it.TenantName, "revenue": formatRevenue(it.Revenue), "billable": it.Billable, "deployed": intOrDash(it.DeployedEndpoints), "variance": intOrDash(it.Variance)})
			}
			return printAutoTable(cmd.OutOrStdout(), rows)
		},
	}
	cmd.Flags().StringVar(&flagMonth, "month", "", "Billing month as YYYY-MM (default: the previous calendar month)")
	cmd.Flags().IntVar(&flagLimit, "limit", 20, "Maximum customers to return")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file used for the tenant join (default: resolved data directory data.db)")
	// The usage report is a partner-global call; the tenant selector does not apply.
	return cmd
}

// parseBillingMonth reads YYYY-MM; empty means the previous calendar month.
func parseBillingMonth(raw string, now time.Time) (int, int, error) {
	if raw == "" {
		prev := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -1, 0)
		return prev.Year(), int(prev.Month()), nil
	}
	t, err := time.Parse("2006-01", raw)
	if err != nil {
		return 0, 0, fmt.Errorf("--month %q: want YYYY-MM, e.g. 2026-09", raw)
	}
	return t.Year(), int(t.Month()), nil
}

// billingJoinData loads tenant names and endpoint counts from the local store
// when it exists; the join is optional, so a missing store is not an error.
func billingJoinData(cmd *cobra.Command, dbPath string) (map[string]string, map[string]int) {
	names, deployed := map[string]string{}, map[string]int{}
	db, exists, err := openFleetStore(cmd.Context(), fleetDBPath(dbPath))
	if err != nil || !exists {
		fmt.Fprintln(cmd.ErrOrStderr(), "hint: run 'sophos-central-cli fleet refresh' first to join billing to tenant names and deployed endpoint counts")
		return names, deployed
	}
	defer db.Close()
	ledger, _ := loadFleetLedger(db)
	names = tenantNames(db, ledger)
	_ = db.ListScan("endpoints", func(_ string, raw json.RawMessage) bool {
		var e struct {
			Tenant struct {
				ID string `json:"id"`
			} `json:"tenant"`
		}
		if json.Unmarshal(raw, &e) == nil && e.Tenant.ID != "" {
			deployed[e.Tenant.ID]++
		}
		return true
	})
	// Deployment is only known for tenants a fleet refresh walked in full;
	// rows a single-page live read or a capped sync cached are not a count.
	// A walked tenant with no endpoints is deployed 0, so its billed seats
	// still show as variance instead of null.
	for tid := range deployed {
		if ledger[tid].Resources["endpoints"].FullAt.IsZero() {
			delete(deployed, tid)
		}
	}
	for tid, row := range ledger {
		if _, seen := deployed[tid]; !seen && !row.Resources["endpoints"].FullAt.IsZero() {
			deployed[tid] = 0
		}
	}
	return names, deployed
}

func buildBilling(month string, lines []billingLine, names map[string]string, deployed map[string]int) billingView {
	byAccount := map[string]*billingCustomer{}
	products := map[string]map[string]*billingProduct{}
	view := billingView{Month: month, TotalsCurrency: map[string]float64{}, Unmatched: make([]string, 0), Lines: len(lines),
		JoinKey: "usage line accountId == synced tenant id (lines that do not match are listed, not guessed)"}
	for _, l := range lines {
		key := l.AccountID
		if key == "" {
			key = l.AccountIdentifier
		}
		cust := byAccount[key]
		if cust == nil {
			cust = &billingCustomer{AccountID: key, Revenue: map[string]float64{}}
			byAccount[key] = cust
			products[key] = map[string]*billingProduct{}
		}
		price, _ := strconv.ParseFloat(strings.TrimSpace(l.UnitPrice), 64)
		currency := l.UnitPriceCurrency
		revenue := float64(l.BillableQuantity) * price
		cust.Revenue[currency] += revenue
		cust.Billable += l.BillableQuantity
		cust.Ordered += l.OrderedQuantity
		cust.Actual += l.ActualQuantity
		view.TotalsCurrency[currency] += revenue
		pname := l.ProductDesc
		if pname == "" {
			pname = l.ProductCode
		}
		pk := pname + "|" + currency
		p := products[key][pk]
		if p == nil {
			p = &billingProduct{Product: pname, Currency: currency}
			products[key][pk] = p
		}
		p.Billable += l.BillableQuantity
		p.Revenue += revenue
		if cust.TenantName == "" {
			cust.TenantName = l.ContactName
		}
	}
	for key, cust := range byAccount {
		if name, ok := names[key]; ok {
			cust.Matched = true
			cust.TenantName = name
			if n, ok := deployed[key]; ok {
				cust.DeployedEndpoints = intPtr(n)
				// Compare the largest single product line (usually the endpoint
				// licence) with deployed endpoints; summing seats across
				// products would count one machine once per product.
				largest := 0
				for _, p := range products[key] {
					p.BillableMinusDeployed = intPtr(p.Billable - n)
					if p.Billable > largest {
						largest = p.Billable
					}
				}
				cust.Variance = intPtr(largest - n)
			}
		} else {
			view.Unmatched = append(view.Unmatched, key)
		}
		for _, p := range products[key] {
			p.Revenue = round2(p.Revenue)
			cust.Products = append(cust.Products, *p)
		}
		sort.Slice(cust.Products, func(i, j int) bool { return cust.Products[i].Revenue > cust.Products[j].Revenue })
		for cur, v := range cust.Revenue {
			cust.Revenue[cur] = round2(v)
		}
		view.Items = append(view.Items, *cust)
	}
	// Rank within one currency, never by summing different currencies: the
	// currency with the largest total leads, customers billed in it sort by
	// that amount, and customers without it follow.
	for cur, v := range view.TotalsCurrency {
		if view.RankedBy == "" || v > view.TotalsCurrency[view.RankedBy] || (v == view.TotalsCurrency[view.RankedBy] && cur < view.RankedBy) {
			view.RankedBy = cur
		}
	}
	for i := range view.Items {
		if v, ok := view.Items[i].Revenue[view.RankedBy]; ok {
			view.Items[i].sortKey = v
		} else {
			view.Items[i].sortKey = -1
		}
	}
	for cur, v := range view.TotalsCurrency {
		view.TotalsCurrency[cur] = round2(v)
	}
	sort.Slice(view.Items, func(i, j int) bool {
		if view.Items[i].sortKey != view.Items[j].sortKey {
			return view.Items[i].sortKey > view.Items[j].sortKey
		}
		return view.Items[i].AccountID < view.Items[j].AccountID
	})
	sort.Strings(view.Unmatched)
	if view.Items == nil {
		view.Items = make([]billingCustomer, 0)
	}
	view.Customers = len(view.Items)
	return view
}

func round2(v float64) float64 { return float64(int64(v*100+0.5)) / 100 }

func formatRevenue(m map[string]float64) string {
	parts := make([]string, 0, len(m))
	for cur, v := range m {
		parts = append(parts, fmt.Sprintf("%.2f %s", v, cur))
	}
	sort.Strings(parts)
	return strings.Join(parts, " + ")
}
