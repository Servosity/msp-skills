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

Surfaces two leakage classes the Pax8 portal can't show in one place:
  • Companies that were invoiced but have no active subscription (billed-but-cancelled)
  • Companies with active subscriptions but no invoice (active-but-unbilled)

With --draft, the same join runs against the NEXT (draft, unposted) invoice
lines instead of posted invoices, so leakage is caught before the invoice
finalizes. Sync 'invoices-draft-items' first for draft mode.

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
			invoiceResource := "invoices"
			invoiceLabel := "invoice"
			if draft {
				invoiceResource = "invoices-draft-items"
				invoiceLabel = "draft invoice line"
			}
			maybeEmitSyncHints(cmd, db, invoiceResource, flags.maxAge)

			companies, err := pax8ListObjects(db, "companies")
			if err != nil {
				return fmt.Errorf("reading companies: %w", err)
			}
			nameByID := pax8NameByID(companies, []string{"id", "companyId"}, []string{"name", "companyName"})

			subs, err := pax8ListObjects(db, "subscriptions")
			if err != nil {
				return fmt.Errorf("reading subscriptions: %w", err)
			}
			activeByCompany := map[string]int{}
			for _, s := range subs {
				cid := pax8FieldStr(s, "companyId", "company_id")
				if mrrIsActive(pax8FieldStr(s, "status")) {
					activeByCompany[cid]++
				}
			}

			invoices, err := pax8ListObjects(db, invoiceResource)
			if err != nil {
				return fmt.Errorf("reading %s: %w", invoiceResource, err)
			}
			invoicedByCompany := map[string]float64{}
			invoiceIDByCompany := map[string]string{}
			for _, inv := range invoices {
				cid := pax8FieldStr(inv, "companyId", "company_id")
				if total, ok := pax8FieldNum(inv, "total"); ok {
					invoicedByCompany[cid] += total
				} else if amount, ok := pax8FieldNum(inv, "subTotal", "amountDue"); ok {
					// Pax8 InvoiceItem rows (draft mode) carry per-line total/subTotal/amountDue;
					// recover from subTotal/amountDue when a row lacks total.
					invoicedByCompany[cid] += amount
				} else if _, seen := invoicedByCompany[cid]; !seen {
					invoicedByCompany[cid] = 0
				}
				if _, seen := invoiceIDByCompany[cid]; !seen {
					invoiceIDByCompany[cid] = pax8FieldStr(inv, "id", "invoiceId")
				}
			}

			var res reconcileResult

			// Billed but no active subscription.
			for cid, total := range invoicedByCompany {
				if activeByCompany[cid] == 0 {
					res.BilledWithoutActiveSubscription = append(res.BilledWithoutActiveSubscription, reconcileFinding{
						Kind:        "billed_without_active_subscription",
						CompanyID:   cid,
						CompanyName: nameByID[cid],
						InvoiceID:   invoiceIDByCompany[cid],
						Amount:      total,
						Note:        fmt.Sprintf("%s present but no active subscription found", invoiceLabel),
					})
				}
			}

			// Active subscription but no invoice.
			for _, s := range subs {
				if !mrrIsActive(pax8FieldStr(s, "status")) {
					continue
				}
				cid := pax8FieldStr(s, "companyId", "company_id")
				if _, billed := invoicedByCompany[cid]; !billed {
					res.ActiveWithoutInvoice = append(res.ActiveWithoutInvoice, reconcileFinding{
						Kind:           "active_without_invoice",
						CompanyID:      cid,
						CompanyName:    nameByID[cid],
						SubscriptionID: pax8FieldStr(s, "id", "subscriptionId"),
						ProductID:      pax8FieldStr(s, "productId", "product_id"),
						Note:           fmt.Sprintf("active subscription with no %s in the store", invoiceLabel),
					})
				}
			}

			sort.Slice(res.BilledWithoutActiveSubscription, func(i, j int) bool {
				return res.BilledWithoutActiveSubscription[i].Amount > res.BilledWithoutActiveSubscription[j].Amount
			})
			sort.Slice(res.ActiveWithoutInvoice, func(i, j int) bool {
				return strings.Compare(res.ActiveWithoutInvoice[i].CompanyID, res.ActiveWithoutInvoice[j].CompanyID) < 0
			})
			res.Summary.Companies = len(companies)
			res.Summary.BilledWithoutSub = len(res.BilledWithoutActiveSubscription)
			res.Summary.ActiveWithoutInvoiceN = len(res.ActiveWithoutInvoice)

			if flags.asJSON {
				return printJSONFiltered(out, res, flags)
			}
			fmt.Fprintf(out, "Reconciliation summary: %d companies, %d billed-without-active-sub, %d active-without-invoice\n\n",
				res.Summary.Companies, res.Summary.BilledWithoutSub, res.Summary.ActiveWithoutInvoiceN)
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
				fmt.Fprintln(out, "Company\tInvoice\tAmount")
				for _, f := range res.BilledWithoutActiveSubscription {
					name := f.CompanyName
					if name == "" {
						name = f.CompanyID
					}
					fmt.Fprintf(out, "%s\t%s\t%.2f\n", name, f.InvoiceID, f.Amount)
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
