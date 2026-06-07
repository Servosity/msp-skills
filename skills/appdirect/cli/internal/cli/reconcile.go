// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-implemented novel feature: billing reconciliation across the local
// subscriptions / invoices / payments tables. Originally scaffolded by the
// CLI Printing Press; body is hand-authored.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"appdirect-pp-cli/internal/cliutil"
	"appdirect-pp-cli/internal/store"
)

// pp:data-source local

type reconcileFinding struct {
	Kind           string  `json:"kind"`
	SubscriptionID string  `json:"subscriptionId,omitempty"`
	InvoiceID      string  `json:"invoiceId,omitempty"`
	PaymentID      string  `json:"paymentId,omitempty"`
	CompanyID      string  `json:"companyId,omitempty"`
	CompanyName    string  `json:"companyName,omitempty"`
	Amount         float64 `json:"amount,omitempty"`
	Currency       string  `json:"currency,omitempty"`
	Date           string  `json:"date,omitempty"`
	Detail         string  `json:"detail"`
}

type reconcileView struct {
	Since                string             `json:"since"`
	Cutoff               string             `json:"cutoff"`
	Findings             []reconcileFinding `json:"findings"`
	Counts               map[string]int     `json:"counts"`
	ScannedSubscriptions int                `json:"scanned_subscriptions"`
	ScannedInvoices      int                `json:"scanned_invoices"`
	ScannedPayments      int                `json:"scanned_payments"`
	Note                 string             `json:"note,omitempty"`
}

func newNovelReconcileCmd(flags *rootFlags) *cobra.Command {
	var flagSince string
	var flagCompany string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "reconcile",
		Short: "Flag subscriptions without invoices, overdue invoices, and failed payments",
		Long: strings.TrimSpace(`
Use this command to match billing across subscriptions, invoices, and payments
and find mismatches before month-close. It joins the locally synced
billing-v1-subscriptions, billing-v1-invoices, and billing-v1-payments tables:

  - subscription-without-invoice: ACTIVE subscriptions whose order never
    appears in any synced invoice's orderIds inside the window
  - overdue-invoice: UNPAID invoices whose dueDate has passed
  - failed-payment: payments with result FAILED or GATEWAY_NOT_AVAILABLE
    inside the window

Do NOT use this command to list only failed payments; use 'payments unpaid'
instead. Do NOT use it for a single customer's full record; use 'company show'
instead. Run 'sync --resources billing' first to populate the local store.`),
		Example: strings.Trim(`
  # Flag billing mismatches from the last 30 days across every company
  appdirect-cli reconcile --since 30d --agent

  # Reconcile a single company
  appdirect-cli reconcile --company 12345678-aaaa-bbbb-cccc-1234567890ab --json

  # Narrow the report for agent triage
  appdirect-cli reconcile --since 30d --agent --select findings.kind,findings.subscriptionId,findings.companyName
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would reconcile local subscriptions, invoices, and payments")
				return nil
			}
			if flagSince == "" {
				flagSince = "30d"
			}
			sinceDur, err := cliutil.ParseDurationLoose(flagSince)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--since: %w", err))
			}
			now := time.Now().UTC()
			cutoff := now.Add(-sinceDur)

			if dbPath == "" {
				dbPath = defaultDBPath("appdirect-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()

			for _, rt := range []string{rtSubscriptions, rtInvoices, rtPayments} {
				if !hintIfUnsynced(cmd, db, rt) {
					hintIfStale(cmd, db, rt, flags.maxAge)
				}
			}

			subs, err := loadResourceObjects(cmd.Context(), db, rtSubscriptions)
			if err != nil {
				return err
			}
			invoices, err := loadResourceObjects(cmd.Context(), db, rtInvoices)
			if err != nil {
				return err
			}
			payments, err := loadResourceObjects(cmd.Context(), db, rtPayments)
			if err != nil {
				return err
			}
			companies, err := loadResourceObjects(cmd.Context(), db, rtCompanies)
			if err != nil {
				return err
			}
			names := companyNameIndex(companies)
			lookupName := func(sub map[string]any, companyID string) string {
				if n := names[companyID]; n != "" {
					return n
				}
				if sub != nil {
					return novelRefName(sub, "company")
				}
				return ""
			}

			// Index: order id -> whether any invoice references it, plus the
			// latest invoice creationDate when one is present. Presence is
			// tracked separately from recency so an invoice with a missing
			// creationDate still counts as covering its orders.
			type orderInvoiceInfo struct {
				seen   bool
				latest time.Time
			}
			invoiceByOrder := map[string]orderInvoiceInfo{}
			for _, inv := range invoices {
				created, _ := novelEpochMS(inv, "creationDate")
				rawOrders, _ := inv["orderIds"].([]any)
				for _, ro := range rawOrders {
					oid := fmt.Sprintf("%v", ro)
					info := invoiceByOrder[oid]
					info.seen = true
					if created.After(info.latest) {
						info.latest = created
					}
					invoiceByOrder[oid] = info
				}
			}

			findings := make([]reconcileFinding, 0)
			counts := map[string]int{}
			add := func(f reconcileFinding) {
				findings = append(findings, f)
				counts[f.Kind]++
			}

			for _, sub := range subs {
				companyID := novelRefID(sub, "company")
				if flagCompany != "" && companyID != flagCompany {
					continue
				}
				if !strings.EqualFold(novelStr(sub, "status"), "ACTIVE") {
					continue
				}
				orderID := novelRefID(sub, "order")
				lastInvoice, invoiced := time.Time{}, false
				if orderID != "" {
					info := invoiceByOrder[orderID]
					invoiced, lastInvoice = info.seen, info.latest
				}
				if invoiced && (lastInvoice.IsZero() || !lastInvoice.Before(cutoff)) {
					// Covered: a recent invoice exists, or an invoice exists
					// whose creationDate the API omitted (recency unknowable).
					continue
				}
				detail := "no synced invoice references this subscription's order"
				if invoiced {
					detail = fmt.Sprintf("last invoice for this subscription's order was %s, before the %s window", lastInvoice.Format("2006-01-02"), flagSince)
				} else if orderID == "" {
					detail = "subscription has no order reference and no matching invoice"
				}
				add(reconcileFinding{
					Kind:           "subscription-without-invoice",
					SubscriptionID: novelStr(sub, "id"),
					CompanyID:      companyID,
					CompanyName:    lookupName(sub, companyID),
					Detail:         detail,
				})
			}

			for _, inv := range invoices {
				companyID := novelRefID(inv, "company")
				if flagCompany != "" && companyID != flagCompany {
					continue
				}
				if !strings.EqualFold(novelStr(inv, "status"), "UNPAID") {
					continue
				}
				due, hasDue := novelEpochMS(inv, "dueDate")
				if !hasDue || due.After(now) {
					continue
				}
				amount, _ := novelNum(inv, "total")
				add(reconcileFinding{
					Kind:        "overdue-invoice",
					InvoiceID:   novelStr(inv, "invoiceId"),
					CompanyID:   companyID,
					CompanyName: lookupName(nil, companyID),
					Amount:      amount,
					Currency:    novelStr(inv, "currency"),
					Date:        isoOrEmpty(due),
					Detail:      fmt.Sprintf("UNPAID invoice %d days past due", ageDays(due, now)),
				})
			}

			for _, pay := range payments {
				companyID := novelRefID(pay, "company")
				if flagCompany != "" && companyID != flagCompany {
					continue
				}
				result := strings.ToUpper(novelStr(pay, "result"))
				if result != "FAILED" && result != "GATEWAY_NOT_AVAILABLE" {
					continue
				}
				date, _ := novelEpochMS(pay, "date")
				if !date.IsZero() && date.Before(cutoff) {
					continue
				}
				amount, _ := novelNum(pay, "amount")
				paymentID := novelStr(pay, "id")
				if paymentID == "" {
					paymentID = novelStr(pay, "paymentNumber")
				}
				add(reconcileFinding{
					Kind:        "failed-payment",
					PaymentID:   paymentID,
					CompanyID:   companyID,
					CompanyName: lookupName(nil, companyID),
					Amount:      amount,
					Currency:    novelStr(pay, "currency"),
					Date:        isoOrEmpty(date),
					Detail:      fmt.Sprintf("payment result %s", result),
				})
			}

			sort.SliceStable(findings, func(i, j int) bool {
				if findings[i].Kind != findings[j].Kind {
					return findings[i].Kind < findings[j].Kind
				}
				return findings[i].Amount > findings[j].Amount
			})

			view := reconcileView{
				Since:                flagSince,
				Cutoff:               cutoff.Format(time.RFC3339),
				Findings:             findings,
				Counts:               counts,
				ScannedSubscriptions: len(subs),
				ScannedInvoices:      len(invoices),
				ScannedPayments:      len(payments),
			}
			if len(findings) == 0 {
				view.Note = fmt.Sprintf("scanned %d subscriptions, %d invoices, %d payments with no mismatches in the %s window; widen with --since or run 'sync --resources billing' to refresh", len(subs), len(invoices), len(payments), flagSince)
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "30d", "Window for invoice/payment recency checks (e.g. 7d, 30d, 12h)")
	cmd.Flags().StringVar(&flagCompany, "company", "", "Restrict reconciliation to one company UUID")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
