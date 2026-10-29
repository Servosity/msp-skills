// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel feature (Phase 3): billing reconciliation. Not generator-managed.

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"pax8-pp-cli/internal/store"
)

type reconcileFinding struct {
	Kind           string  `json:"kind"` // "billed_without_active_subscription" | "active_without_invoice"
	CompanyID      string  `json:"companyId,omitempty"`
	CompanyName    string  `json:"companyName,omitempty"`
	SubscriptionID string  `json:"subscriptionId,omitempty"`
	InvoiceID      string  `json:"invoiceId,omitempty"`
	InvoiceLineID  string  `json:"invoiceLineId,omitempty"`
	Status         string  `json:"status,omitempty"`
	ProductID      string  `json:"productId,omitempty"`
	Amount         float64 `json:"amount,omitempty"`
	Note           string  `json:"note"`
}

type reconcileResult struct {
	BilledWithoutActiveSubscription []reconcileFinding `json:"billedWithoutActiveSubscription"`
	ActiveWithoutInvoice            []reconcileFinding `json:"activeWithoutInvoice"`
	Summary                         struct {
		Companies             int `json:"companies"`
		BilledWithoutSub      int `json:"billedWithoutActiveSubscription"`
		ActiveWithoutInvoiceN int `json:"activeWithoutInvoice"`
		InvoiceLines          int `json:"invoiceLines"`
		UnlinkedLines         int `json:"unlinkedLines"` // lines with no subscriptionId (credits, fees): not joined
	} `json:"summary"`
}

// pp:data-source local
func newNovelReconcileCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var draft bool

	cmd := &cobra.Command{
		Use:         "reconcile",
		Short:       "Flag invoice lines that no longer match an active subscription, and active subscriptions that never got billed.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Reconcile billing against subscriptions from the local store.

Surfaces two leakage classes the Pax8 portal can't show in one place, joining
each invoice line to its subscription by the line's subscriptionId:
  • Invoice lines for a subscription that is not active (billed-but-cancelled,
    or a subscription the store does not hold)
  • Billing subscriptions (Active, Activated, PendingCancel; not trials) with
    no invoice line (active-but-unbilled); in --draft mode only those billed
    monthly, since an Annual or One-Time subscription is not on every invoice
Lines without a subscriptionId (credits, fees) are counted, not joined.

Posted mode reads the synced invoice lines (invoice items, synced with
invoices). With --draft, the same join runs against the NEXT (draft, unposted)
invoice lines instead, so leakage is caught before the invoice finalizes.
Sync 'invoices-draft-items' first for draft mode.

Use this command to find billing mismatches between invoices and subscriptions.
Do NOT use this command to roll up a single customer's spend across invoices; use 'spend' instead.
Do NOT use this command for revenue/margin totals; use 'mrr' instead.

Run 'pax8-cli sync' first.`,
		Example: `  # Reconcile the synced store
  pax8-cli reconcile

  # Pre-check the next (unposted) invoice before it finalizes
  pax8-cli reconcile --draft

  # JSON for an agent
  pax8-cli reconcile --agent`,
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
			// pax8 handfix: novel-documented-shapes. Invoice lines
			// (spec.json#/components/schemas/InvoiceItem) carry the
			// subscriptionId they bill, so the join is per subscription: one
			// company's invoice can no longer hide another subscription's
			// mismatch.
			lineNames, invoiceLabel := pax8InvoiceLines, "invoice line"
			if draft {
				lineNames, invoiceLabel = pax8DraftLines, "draft invoice line"
			}
			maybeEmitSyncHints(cmd, db, lineNames[0], flags.maxAge)

			companies, err := pax8Collection(db, pax8Companies)
			if err != nil {
				return fmt.Errorf("reading companies: %w", err)
			}
			nameByID := pax8NameByID(companies, []string{"id", "companyId"}, []string{"name", "companyName"})

			subs, err := pax8Collection(db, pax8Subscriptions)
			if err != nil {
				return fmt.Errorf("reading subscriptions: %w", err)
			}
			subByID := map[string]map[string]any{}
			for _, s := range subs {
				if id := pax8FieldStr(s, "id", "subscriptionId"); id != "" {
					subByID[id] = s
				}
			}

			lines, err := pax8Collection(db, lineNames)
			if err != nil {
				return fmt.Errorf("reading %ss: %w", invoiceLabel, err)
			}
			type billed struct {
				amount             float64
				company, product   string
				invoice, firstLine string
			}
			billedBySub := map[string]*billed{}
			var billedOrder []string
			res := reconcileResult{BilledWithoutActiveSubscription: []reconcileFinding{}, ActiveWithoutInvoice: []reconcileFinding{}} // pax8 handfix: novel-json-empty-is-array
			res.Summary.InvoiceLines = len(lines)
			billedAny := map[string]bool{}
			for _, line := range lines {
				sid := pax8FieldStr(line, "subscriptionId", "subscription_id")
				if sid == "" {
					res.Summary.UnlinkedLines++
					continue
				}
				// A credit (refund, rebate, negative proration) is not billing:
				// it neither bills a subscription nor shows one was billed
				// (pax8 handfix: novel-documented-shapes, InvoiceItem.type).
				if !pax8LineIsCharge(line) {
					continue
				}
				billedAny[sid] = true
				// Posted lines are the whole billing history: a line for a
				// period that began before a non-billing subscription ended
				// was billed while it ran. Only a line that began after the
				// end, or whose period or end cannot be read, is a finding
				// (fail closed).
				if sub, known := subByID[sid]; known && !mrrIsActive(pax8FieldStr(sub, "status")) && pax8LineBeforeEnd(line, sub) {
					continue
				}
				b := billedBySub[sid]
				if b == nil {
					b = &billed{
						company:   pax8FieldStr(line, "companyId", "company_id"),
						product:   pax8FieldStr(line, "productId", "product_id"),
						invoice:   pax8FieldStr(line, "invoiceId", "invoices_id", "parent_id"),
						firstLine: pax8FieldStr(line, "id"),
					}
					billedBySub[sid] = b
					billedOrder = append(billedOrder, sid)
				}
				if amount, ok := pax8FieldNum(line, "total", "subTotal", "amountDue"); ok {
					b.amount += amount
				}
			}

			// Billed, but the subscription is not active (or not in the store).
			for _, sid := range billedOrder {
				b := billedBySub[sid]
				sub, known := subByID[sid]
				status := pax8FieldStr(sub, "status")
				if known && mrrIsActive(status) {
					continue
				}
				note := fmt.Sprintf("%s for a subscription with status %q", invoiceLabel, status)
				if !known {
					note = fmt.Sprintf("%s for a subscription not in the store", invoiceLabel)
				}
				cid := b.company
				if cid == "" {
					cid = pax8FieldStr(sub, "companyId", "company_id")
				}
				res.BilledWithoutActiveSubscription = append(res.BilledWithoutActiveSubscription, reconcileFinding{
					Kind:           "billed_without_active_subscription",
					CompanyID:      cid,
					CompanyName:    nameByID[cid],
					SubscriptionID: sid,
					InvoiceID:      b.invoice,
					InvoiceLineID:  b.firstLine,
					ProductID:      b.product,
					Status:         status,
					Amount:         b.amount,
					Note:           note,
				})
			}

			// Active subscription with no invoice line (only the ones an
			// invoice should carry: pax8ExpectsInvoiceLine).
			for _, s := range subs {
				status := pax8FieldStr(s, "status")
				if !pax8ExpectsInvoiceLine(s, draft) {
					continue
				}
				sid := pax8FieldStr(s, "id", "subscriptionId")
				if billedAny[sid] {
					continue
				}
				cid := pax8FieldStr(s, "companyId", "company_id")
				res.ActiveWithoutInvoice = append(res.ActiveWithoutInvoice, reconcileFinding{
					Kind:           "active_without_invoice",
					CompanyID:      cid,
					CompanyName:    nameByID[cid],
					SubscriptionID: sid,
					ProductID:      pax8FieldStr(s, "productId", "product_id"),
					Status:         status,
					Note:           fmt.Sprintf("active subscription with no %s in the store", invoiceLabel),
				})
			}

			sort.Slice(res.BilledWithoutActiveSubscription, func(i, j int) bool {
				return res.BilledWithoutActiveSubscription[i].Amount > res.BilledWithoutActiveSubscription[j].Amount
			})
			sort.Slice(res.ActiveWithoutInvoice, func(i, j int) bool {
				if c := strings.Compare(res.ActiveWithoutInvoice[i].CompanyID, res.ActiveWithoutInvoice[j].CompanyID); c != 0 {
					return c < 0
				}
				return res.ActiveWithoutInvoice[i].SubscriptionID < res.ActiveWithoutInvoice[j].SubscriptionID
			})
			res.Summary.Companies = len(companies)
			res.Summary.BilledWithoutSub = len(res.BilledWithoutActiveSubscription)
			res.Summary.ActiveWithoutInvoiceN = len(res.ActiveWithoutInvoice)

			if flags.asJSON {
				return printJSONFiltered(out, res, flags)
			}
			fmt.Fprintf(out, "Reconciliation summary: %d companies, %d %ss (%d without a subscription), %d billed-without-active-sub, %d active-without-invoice\n\n",
				res.Summary.Companies, res.Summary.InvoiceLines, invoiceLabel, res.Summary.UnlinkedLines, res.Summary.BilledWithoutSub, res.Summary.ActiveWithoutInvoiceN)
			if res.Summary.InvoiceLines == 0 && len(subs) > 0 {
				fmt.Fprintf(out, "No %ss in the local store, so every active subscription reads as unbilled. Run 'pax8-cli sync' first.\n\n", invoiceLabel)
			}
			if len(res.BilledWithoutActiveSubscription) == 0 && len(res.ActiveWithoutInvoice) == 0 {
				if len(companies) == 0 {
					fmt.Fprintln(out, "Local store is empty. Run 'pax8-cli sync' first.")
				} else {
					fmt.Fprintln(out, "No billing mismatches found.")
				}
				return nil
			}
			if len(res.BilledWithoutActiveSubscription) > 0 {
				fmt.Fprintln(out, "Billed without active subscription:")
				fmt.Fprintln(out, "Company\tSubscription\tStatus\tAmount")
				for _, f := range res.BilledWithoutActiveSubscription {
					name := f.CompanyName
					if name == "" {
						name = f.CompanyID
					}
					status := f.Status
					if status == "" {
						status = "(not in store)"
					}
					fmt.Fprintf(out, "%s\t%s\t%s\t%.2f\n", name, f.SubscriptionID, status, f.Amount)
				}
				fmt.Fprintln(out)
			}
			if len(res.ActiveWithoutInvoice) > 0 {
				fmt.Fprintln(out, "Active without invoice:")
				fmt.Fprintln(out, "Company\tSubscription\tProduct")
				for _, f := range res.ActiveWithoutInvoice {
					name := f.CompanyName
					if name == "" {
						name = f.CompanyID
					}
					fmt.Fprintf(out, "%s\t%s\t%s\n", name, f.SubscriptionID, f.ProductID)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	cmd.Flags().BoolVar(&draft, "draft", false, "Reconcile against the next (draft, unposted) invoice lines instead of posted invoices")
	return cmd
}
