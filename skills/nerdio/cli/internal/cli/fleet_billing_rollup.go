// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature: fleet billing-rollup (invoices + usages joined per account).
// pp:data-source auto

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// billingRollupRow is one account's joined billing picture for the period.
type billingRollupRow struct {
	AccountID    int64   `json:"account_id"`
	Account      string  `json:"account"`
	InvoiceCount int     `json:"invoice_count"`
	InvoiceTotal float64 `json:"invoice_total"`
	UnpaidCount  int     `json:"unpaid_count"`
	UnpaidTotal  float64 `json:"unpaid_total"`
	UsageTotal   float64 `json:"usage_total"`
	UsageRows    int     `json:"usage_rows"`
}

// billingRollupView is the JSON envelope for fleet billing-rollup.
type billingRollupView struct {
	PeriodStart   string              `json:"period_start"`
	PeriodEnd     string              `json:"period_end"`
	Rows          []billingRollupRow  `json:"rows"`
	GrandTotal    float64             `json:"grand_total"`
	UnpaidTotal   float64             `json:"unpaid_total"`
	FetchFailures []fleetFetchFailure `json:"fetch_failures,omitempty"`
	Note          string              `json:"note,omitempty"`
}

func newNovelFleetBillingRollupCmd(flags *rootFlags) *cobra.Command {
	var flagPeriod string
	var flagUnpaidOnly bool

	cmd := &cobra.Command{
		Use:   "billing-rollup",
		Short: "Per-account billed/paid/unpaid/usage rollup for a billing period",
		Long: strings.Trim(`
Use this command for a current-period per-account billing/usage rollup.
Do NOT use it for month-over-month usage trends; use 'usages drift' instead.

Pulls /invoices and /usages for the period, joins them to account names from
the synced local store (falling back to the live accounts list), and emits a
per-account rollup of invoice totals, unpaid balance, and usage - the
PSA-reconciliation view the API never returns assembled. Rows whose amounts
cannot be extracted from the payload still count toward invoice_count and a
note explains the gap; nothing is silently averaged in as zero.
`, "\n"),
		Example: strings.Trim(`
  nerdio-cli fleet billing-rollup --period 2026-05-01:2026-05-31
  nerdio-cli fleet billing-rollup --period 2026-05-01:2026-05-31 --unpaid-only --agent
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "--period=2026-05-01:2026-05-31",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would roll up invoices and usage per account for the period")
				return nil
			}
			if flagPeriod == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--period is required (e.g. --period 2026-05-01:2026-05-31)"))
			}
			start, end, err := parsePeriod(flagPeriod)
			if err != nil {
				return usageErr(err)
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx := cmd.Context()

			view := billingRollupView{PeriodStart: start, PeriodEnd: end, Rows: make([]billingRollupRow, 0)}
			byAccount := map[int64]*billingRollupRow{}
			amountMisses := 0
			paidStatusMisses := 0

			// Account names: synced store first (with sync hints), live fallback.
			names := map[int64]string{}
			for _, a := range fleetAccountsFromStore(ctx, cmd, flags) {
				names[a.ID] = a.Name
			}
			rowFor := func(id int64) *billingRollupRow {
				if row, ok := byAccount[id]; ok {
					return row
				}
				name := names[id]
				if name == "" {
					name = fmt.Sprintf("account-%d", id)
				}
				row := &billingRollupRow{AccountID: id, Account: name}
				byAccount[id] = row
				return row
			}

			// Invoices for the period.
			invoiceParams := map[string]string{"periodStart": start, "periodEnd": end}
			if flagUnpaidOnly {
				invoiceParams["hidePaid"] = "true"
			}
			invData, err := c.Get(ctx, "/rest-api/v1/invoices", invoiceParams)
			if err != nil {
				view.FetchFailures = append(view.FetchFailures, fleetFetchFailure{Account: "invoices", Error: err.Error()})
			} else {
				for _, inv := range decodeObjects(invData) {
					accountID, ok := extractIntAny(inv, "accountId", "nmmId", "account", "customerId")
					if !ok {
						continue
					}
					row := rowFor(accountID)
					row.InvoiceCount++
					amount, haveAmount := extractNumberAny(inv, "total", "amount", "totalAmount", "cost", "price")
					if haveAmount {
						row.InvoiceTotal += amount
					} else {
						amountMisses++
					}
					// With --unpaid-only, hidePaid=true upstream guarantees every
					// returned row is unpaid, so a missing isPaid/paid key must
					// still count as unpaid (never silently under-count).
					paid, havePaid := extractBoolAny(inv, "isPaid", "paid")
					unpaid := (havePaid && !paid) || (!havePaid && flagUnpaidOnly)
					if !havePaid && !flagUnpaidOnly {
						paidStatusMisses++
					}
					if unpaid {
						row.UnpaidCount++
						if haveAmount {
							row.UnpaidTotal += amount
						}
					}
				}
			}

			// Usage for the same window.
			usageData, err := c.Get(ctx, "/rest-api/v1/usages", map[string]string{
				"startDate": start, "endDate": end, "withDetails": "true",
			})
			if err != nil {
				view.FetchFailures = append(view.FetchFailures, fleetFetchFailure{Account: "usages", Error: err.Error()})
			} else {
				for _, usage := range decodeObjects(usageData) {
					accountID, ok := extractIntAny(usage, "accountId", "nmmId", "account", "customerId")
					if !ok {
						continue
					}
					row := rowFor(accountID)
					row.UsageRows++
					if amount, ok := extractNumberAny(usage, "total", "amount", "totalAmount", "cost", "quantity", "usage"); ok {
						row.UsageTotal += amount
					}
				}
			}

			ids := make([]int64, 0, len(byAccount))
			for id := range byAccount {
				ids = append(ids, id)
			}
			sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
			for _, id := range ids {
				row := byAccount[id]
				if flagUnpaidOnly && row.UnpaidCount == 0 && row.InvoiceCount == 0 {
					continue
				}
				view.Rows = append(view.Rows, *row)
				view.GrandTotal += row.InvoiceTotal
				view.UnpaidTotal += row.UnpaidTotal
			}
			var notes []string
			if amountMisses > 0 {
				notes = append(notes, fmt.Sprintf("%d invoice rows had no extractable amount field and are counted but not totalled", amountMisses))
			}
			if paidStatusMisses > 0 {
				notes = append(notes, fmt.Sprintf("%d invoice rows had no extractable paid-status field; unpaid totals may under-count", paidStatusMisses))
			}
			if len(notes) > 0 {
				view.Note = strings.Join(notes, "; ")
			}
			if len(view.FetchFailures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of 2 source fetches failed; rollup is partial\n", len(view.FetchFailures))
			}
			if len(view.Rows) == 0 && len(view.FetchFailures) == 0 && view.Note == "" {
				view.Note = "no invoices or usage rows found for the period"
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().StringVar(&flagPeriod, "period", "", "Billing period as <start>:<end> dates (e.g. 2026-05-01:2026-05-31)")
	cmd.Flags().BoolVar(&flagUnpaidOnly, "unpaid-only", false, "Only include accounts with unpaid invoices (passes hidePaid upstream)")
	return cmd
}
