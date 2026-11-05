// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: seat and billing reconciliation.
// pp:data-source live

package cli

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"dropsuite-pp-cli/internal/cliutil"
)

// billableFields are the vendor counters a user may bill on.
var billableFields = map[string]bool{"seats_used": true, "active_seats": true, "required_seats": true}

type billingRow struct {
	OrgID               string   `json:"org_id"`
	OrgEmail            string   `json:"org_email"`
	OrgName             string   `json:"org_name"`
	ExternalID          string   `json:"external_id"`
	PlanID              string   `json:"plan_id"`
	SeatsUsed           int64    `json:"seats_used"`
	SeatsAvailable      int64    `json:"seats_available"`
	ActiveSeats         *int64   `json:"active_seats"`
	RequiredSeats       *int64   `json:"required_seats"`
	DeactivatedSeats    *int64   `json:"deactivated_seats"`
	PaidSharedMailboxes *int64   `json:"paid_shared_mailboxes"`
	FreeSharedMailboxes *int64   `json:"free_shared_mailboxes"`
	AutoLicense         *bool    `json:"auto_license"`
	CustomerDeactivated bool     `json:"customer_deactivated"`
	Suspended           bool     `json:"suspended"`
	Billable            *int64   `json:"billable"`
	Invoiced            *float64 `json:"invoiced"`
	Delta               *float64 `json:"delta"`
	Flags               []string `json:"flags"`
	DetailError         string   `json:"detail_error,omitempty"`
}

type invoiceLine struct {
	Key      string
	Quantity float64
	Line     int
}

// invoiceAgg sums every invoice line that shares a key.
type invoiceAgg struct {
	Key       string
	Quantity  float64
	Lines     []int
	matched   bool
	ambiguous int
}

func aggregateInvoice(lines []invoiceLine, keyCol string) (map[string]*invoiceAgg, []string) {
	byKey := map[string]*invoiceAgg{}
	order := make([]string, 0)
	for _, l := range lines {
		k := l.Key
		if keyCol == "email" {
			k = strings.ToLower(k)
		}
		agg, ok := byKey[k]
		if !ok {
			agg = &invoiceAgg{Key: l.Key}
			byKey[k] = agg
			order = append(order, k)
		}
		agg.Quantity += l.Quantity
		agg.Lines = append(agg.Lines, l.Line)
	}
	return byKey, order
}

type billingReport struct {
	GeneratedAt          string             `json:"generated_at"`
	BillableField        string             `json:"billable_field"`
	AddPaidShared        bool               `json:"add_paid_shared"`
	InvoiceFile          string             `json:"invoice_file,omitempty"`
	Summary              billingSummary     `json:"summary"`
	Rows                 []billingRow       `json:"rows"`
	UnmatchedInvoiceRows []unmatchedInvoice `json:"unmatched_invoice_rows"`
	FetchFailures        []fetchFailure     `json:"fetch_failures"`
}

type billingSummary struct {
	Orgs        int     `json:"orgs"`
	Billable    int64   `json:"billable"`
	Invoiced    float64 `json:"invoiced"`
	UnderBilled int     `json:"under_billed"`
	OverBilled  int     `json:"over_billed"`
	Flagged     int     `json:"flagged"`
}

type unmatchedInvoice struct {
	Key      string  `json:"key"`
	Quantity float64 `json:"quantity"`
	Lines    []int   `json:"lines"`
	Reason   string  `json:"reason"`
}

// parseInvoiceCSV reads a PSA export. It needs a header with a key column
// (external_id, org_id, id or email) and a quantity column (quantity, qty,
// seats, billed or units).
func parseInvoiceCSV(r io.Reader) (keyCol string, lines []invoiceLine, err error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true
	header, err := cr.Read()
	if err != nil {
		return "", nil, usageErr(fmt.Errorf("reading invoice header: %w", err))
	}
	keyIdx, qtyIdx := -1, -1
	norm := func(s string) string {
		return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(s, "\ufeff")))
	}
	for _, want := range []string{"external_id", "org_id", "id", "email"} {
		for i, h := range header {
			if norm(h) == want && keyIdx < 0 {
				keyIdx, keyCol = i, want
			}
		}
	}
	for _, want := range []string{"quantity", "qty", "seats", "billed", "units"} {
		for i, h := range header {
			if norm(h) == want && qtyIdx < 0 {
				qtyIdx = i
			}
		}
	}
	if keyIdx < 0 || qtyIdx < 0 {
		// The header row is not echoed: --invoice is reachable from the MCP
		// server, and echoing the first line of an arbitrary file would hand
		// back its contents (a .env or credentials file) without a prompt.
		return "", nil, usageErr(fmt.Errorf("invoice CSV header must name a key column (external_id, org_id, id or email) and a quantity column (quantity, qty, seats, billed or units); the first row has %d column(s) and does not name both", len(header)))
	}
	lines = make([]invoiceLine, 0)
	lineNo := 1
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		lineNo++
		if err != nil {
			return "", nil, usageErr(fmt.Errorf("invoice CSV line %d: %w", lineNo, err))
		}
		if keyIdx >= len(rec) || qtyIdx >= len(rec) {
			return "", nil, usageErr(fmt.Errorf("invoice CSV line %d: has %d column(s), needs the %s and quantity columns", lineNo, len(rec), keyCol))
		}
		key := strings.TrimSpace(rec[keyIdx])
		if key == "" {
			return "", nil, usageErr(fmt.Errorf("invoice CSV line %d: empty %s", lineNo, keyCol))
		}
		q, err := strconv.ParseFloat(strings.TrimSpace(rec[qtyIdx]), 64)
		if err != nil || math.IsNaN(q) || math.IsInf(q, 0) {
			return "", nil, usageErr(fmt.Errorf("invoice CSV line %d: quantity %q is not a number", lineNo, rec[qtyIdx]))
		}
		lines = append(lines, invoiceLine{Key: key, Quantity: q, Line: lineNo})
	}
	return keyCol, lines, nil
}

func invoiceKeyFor(o dsOrg, keyCol string) string {
	switch keyCol {
	case "external_id":
		return o.ExternalID
	case "org_id", "id":
		return o.ID
	case "email":
		return strings.ToLower(o.Email)
	}
	return ""
}

func int64Ptr(m map[string]any, key string) *int64 {
	if _, ok := m[key]; !ok || m[key] == nil {
		return nil
	}
	v := num(m, key)
	return &v
}

// reconcileRow computes billable, delta and flags for one organization. When
// the chosen counter is missing (detail read failed or the field is absent)
// billable stays unknown instead of becoming zero.
func reconcileRow(row *billingRow, billableField string, addPaidShared bool) {
	var billable *int64
	switch billableField {
	case "active_seats":
		billable = row.ActiveSeats
	case "required_seats":
		billable = row.RequiredSeats
	default:
		v := row.SeatsUsed
		billable = &v
	}
	if billable != nil && addPaidShared {
		if row.PaidSharedMailboxes == nil {
			billable = nil
		} else {
			v := *billable + *row.PaidSharedMailboxes
			billable = &v
		}
	}
	if billable != nil {
		v := *billable
		billable = &v
	}
	row.Billable = billable
	flags := make([]string, 0)
	if row.SeatsUsed > row.SeatsAvailable && row.SeatsAvailable > 0 {
		flags = append(flags, "seats_used_exceeds_available")
	}
	if row.RequiredSeats != nil && *row.RequiredSeats > row.SeatsAvailable && row.SeatsAvailable > 0 {
		flags = append(flags, "required_exceeds_available")
	}
	if (row.CustomerDeactivated || row.Suspended) && row.SeatsUsed > 0 {
		flags = append(flags, "inactive_customer_holding_seats")
	}
	if row.DeactivatedSeats != nil && *row.DeactivatedSeats > 0 {
		flags = append(flags, "has_deactivated_seats")
	}
	switch {
	case billable == nil:
		flags = append(flags, "billable_unknown")
	case row.Invoiced != nil:
		d := *row.Invoiced - float64(*billable)
		row.Delta = &d
		switch {
		case d < 0:
			flags = append(flags, "under_billed")
		case d > 0:
			flags = append(flags, "over_billed")
		}
	case *billable > 0:
		flags = append(flags, "not_on_invoice")
	}
	row.Flags = flags
}

func newNovelBillingCmd(flags *rootFlags) *cobra.Command {
	var invoicePath, billableField string
	var addPaidShared, includeInactive, flaggedOnly bool
	var concurrency int
	var sweepTimeout time.Duration

	cmd := &cobra.Command{
		Use:   "billing",
		Short: "Compare each client's seat counters and shared mailboxes with what your PSA invoices",
		Long: `Use this command for per-org seat counts, shared-mailbox counts and their match against what the PSA invoices. Do NOT use this command for individual deactivated or deleted mailboxes still holding storage; use 'hygiene' instead.

It reads every organization's seat counters (GET /users) and subscription
detail (GET /users/{id}: active, required and deactivated seats, paid and free
shared mailboxes). Dropsuite does not document which counter it bills, so you
pick it with --billable-field (seats_used by default) and decide whether paid
shared mailboxes count (--add-paid-shared).

With --invoice, a CSV export from your PSA is joined on its key column
(external_id, org_id, id or email) and quantity column (quantity, qty, seats,
billed or units). delta = invoiced - billable: negative means under-billed.
Organizations with billable seats but no invoice line are flagged
not_on_invoice; invoice lines that match no organization are listed.`,
		Example: strings.Trim(`
  dropsuite-cli billing --invoice psa-seats.csv --agent
  dropsuite-cli billing --billable-field active_seats --add-paid-shared --flagged-only
  dropsuite-cli billing --org 26561-14 --json`, "\n"),
		// --invoice opens a local CSV: never an MCP argument (mcp:read-flags blocks it in the mirror).
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "mcp:read-flags": "invoice"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if err := rejectArgs(args); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "billing")
			}
			if !billableFields[billableField] {
				return usageErr(fmt.Errorf("--billable-field must be seats_used, active_seats or required_seats; got %q", billableField))
			}
			var keyCol string
			var invoice []invoiceLine
			if invoicePath != "" {
				// --invoice is the PSA export the operator names at the command line (the MCP
				// mirror refuses it: mcp:read-flags); a non-CSV file's first row is never echoed back.
				f, err := os.Open(filepath.Clean(invoicePath))
				if err != nil {
					return usageErr(fmt.Errorf("--invoice: %w", err))
				}
				keyCol, invoice, err = parseInvoiceCSV(f)
				_ = f.Close()
				if err != nil {
					return err
				}
			}
			ctx, cancel := sweepCtx(cmd, sweepTimeout)
			defer cancel()
			admin, err := adminClient(flags)
			if err != nil {
				return err
			}
			orgs, err := fetchOrgs(ctx, admin)
			if err != nil {
				return err
			}
			picked, _, err := selectOrgs(orgs, orgSelector, true)
			if err != nil {
				return err
			}
			if !includeInactive && orgSelector == "" {
				// Inactive customers still matter for billing; keep those that hold seats.
				kept := make([]dsOrg, 0, len(picked))
				for _, o := range picked {
					if !o.inactive() || o.SeatsUsed > 0 {
						kept = append(kept, o)
					}
				}
				picked = kept
			}
			type detailResult struct {
				detail map[string]any
				err    error
			}
			details, _ := cliutil.FanoutRun(ctx, picked,
				func(o dsOrg) string { return o.ID },
				func(ctx context.Context, o dsOrg) (detailResult, error) {
					data, err := admin.Get(ctx, "/users/"+url.PathEscape(o.ID), nil)
					if err != nil {
						return detailResult{err: err}, nil
					}
					m, err := singleObject(data)
					if err != nil {
						return detailResult{err: fmt.Errorf("organization detail: %w", err)}, nil
					}
					return detailResult{detail: m}, nil
				},
				cliutil.WithConcurrency(concurrency),
			)
			detailByID := map[string]detailResult{}
			for _, d := range details {
				detailByID[d.Source] = d.Value
			}
			byKey, invoiceOrder := aggregateInvoice(invoice, keyCol)
			// An invoice key shared by several organizations cannot be split
			// between them; flag those organizations instead of crediting each
			// with the whole quantity.
			// Ownership counts every organization, not only those --org kept.
			keyOwners := map[string]int{}
			for _, o := range orgs {
				if k := invoiceKeyFor(o, keyCol); k != "" {
					keyOwners[k]++
				}
			}
			rep := billingReport{
				GeneratedAt:          nowRFC3339(),
				BillableField:        billableField,
				AddPaidShared:        addPaidShared,
				InvoiceFile:          invoicePath,
				Rows:                 make([]billingRow, 0, len(picked)),
				UnmatchedInvoiceRows: make([]unmatchedInvoice, 0),
				FetchFailures:        make([]fetchFailure, 0),
			}
			for _, o := range picked {
				row := billingRow{
					OrgID: o.ID, OrgEmail: o.Email, OrgName: o.Name, ExternalID: o.ExternalID, PlanID: o.PlanID,
					SeatsUsed: o.SeatsUsed, SeatsAvailable: o.SeatsAvailable,
					CustomerDeactivated: o.CustomerDeactivated, Suspended: o.Suspended,
				}
				if d, ok := detailByID[o.ID]; ok && d.err == nil {
					m := d.detail
					if n := str(m, "organization_name"); n != "" {
						row.OrgName = n
					}
					row.ActiveSeats = int64Ptr(m, "active_seats")
					row.RequiredSeats = int64Ptr(m, "required_seats")
					row.DeactivatedSeats = int64Ptr(m, "deactivated_seats")
					row.PaidSharedMailboxes = int64Ptr(m, "paid_shared_mailboxes")
					row.FreeSharedMailboxes = int64Ptr(m, "free_shared_mailboxes")
					if _, ok := m["auto_license"]; ok {
						v := boolv(m, "auto_license")
						row.AutoLicense = &v
					}
				} else {
					msg := "organization detail missing"
					if ok && d.err != nil {
						msg = errText(d.err)
					}
					row.DetailError = msg
					rep.FetchFailures = append(rep.FetchFailures, fetchFailure{OrgID: o.ID, OrgEmail: o.Email, Scope: "organization detail", Error: msg})
				}
				if invoicePath != "" {
					if key := invoiceKeyFor(o, keyCol); key != "" {
						if agg, ok := byKey[key]; ok {
							if keyOwners[key] > 1 {
								agg.ambiguous = keyOwners[key]
							} else {
								q := agg.Quantity
								row.Invoiced = &q
								agg.matched = true
							}
						}
					}
				}
				reconcileRow(&row, billableField, addPaidShared)
				if key := invoiceKeyFor(o, keyCol); invoicePath != "" && keyOwners[key] > 1 && byKey[key] != nil {
					// The invoice bills this shared key; it cannot be split.
					row.Flags = append(removeFlag(row.Flags, "not_on_invoice"), "ambiguous_invoice_key")
				}
				if invoicePath == "" {
					// Without an invoice there is nothing to be missing from.
					row.Flags = removeFlag(row.Flags, "not_on_invoice")
				}
				rep.Summary.Orgs++
				if row.Billable != nil {
					rep.Summary.Billable += *row.Billable
				}
				if row.Invoiced != nil {
					rep.Summary.Invoiced += *row.Invoiced
				}
				for _, f := range row.Flags {
					switch f {
					case "under_billed":
						rep.Summary.UnderBilled++
					case "over_billed":
						rep.Summary.OverBilled++
					}
				}
				if len(row.Flags) > 0 {
					rep.Summary.Flagged++
				}
				if flaggedOnly && len(row.Flags) == 0 {
					continue
				}
				rep.Rows = append(rep.Rows, row)
			}
			for _, k := range invoiceOrder {
				if agg := byKey[k]; !agg.matched {
					reason := "no organization has this " + keyCol
					switch {
					case agg.ambiguous > 1:
						reason = fmt.Sprintf("%s is shared by %d organizations", keyCol, agg.ambiguous)
					case keyOwners[k] > 0 && orgSelector != "":
						continue // another organization's line; outside this --org report
					case keyOwners[k] > 0:
						reason = "belongs to an inactive organization with no seats in use (add --include-inactive to reconcile it)"
					}
					rep.UnmatchedInvoiceRows = append(rep.UnmatchedInvoiceRows, unmatchedInvoice{Key: agg.Key, Quantity: agg.Quantity, Lines: agg.Lines, Reason: reason})
				}
			}
			sort.SliceStable(rep.Rows, func(i, j int) bool {
				if len(rep.Rows[i].Flags) != len(rep.Rows[j].Flags) {
					return len(rep.Rows[i].Flags) > len(rep.Rows[j].Flags)
				}
				return rep.Rows[i].OrgEmail < rep.Rows[j].OrgEmail
			})
			if len(rep.FetchFailures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d organization detail read(s) failed; those rows are flagged billable_unknown unless --billable-field is seats_used\n", len(rep.FetchFailures))
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), rep, flags)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%d organizations, %d billable (%s", rep.Summary.Orgs, rep.Summary.Billable, billableField)
			if addPaidShared {
				fmt.Fprint(w, " + paid shared mailboxes")
			}
			fmt.Fprintf(w, "); %d flagged, %d under-billed, %d over-billed\n", rep.Summary.Flagged, rep.Summary.UnderBilled, rep.Summary.OverBilled)
			unmatchedNote := func() {
				if len(rep.UnmatchedInvoiceRows) > 0 {
					fmt.Fprintf(w, "\n%d invoice line(s) not matched to a reported organization (see --json for keys and reasons)\n", len(rep.UnmatchedInvoiceRows))
				}
			}
			if len(rep.Rows) == 0 {
				fmt.Fprintln(w, "No organizations to show.")
				unmatchedNote()
				return nil
			}
			rows := make([][]string, 0, len(rep.Rows))
			for _, r := range rep.Rows {
				inv, delta := "-", "-"
				if r.Invoiced != nil {
					inv = strconv.FormatFloat(*r.Invoiced, 'f', -1, 64)
				}
				if r.Delta != nil {
					delta = strconv.FormatFloat(*r.Delta, 'f', -1, 64)
				}
				billable := "?"
				if r.Billable != nil {
					billable = strconv.FormatInt(*r.Billable, 10)
				}
				rows = append(rows, []string{r.OrgEmail, strconv.FormatInt(r.SeatsUsed, 10) + "/" + strconv.FormatInt(r.SeatsAvailable, 10), billable, inv, delta, strings.Join(r.Flags, ",")})
			}
			if err := flags.printTable(cmd, []string{"ORG", "USED/AVAIL", "BILLABLE", "INVOICED", "DELTA", "FLAGS"}, rows); err != nil {
				return err
			}
			unmatchedNote()
			return nil
		},
	}
	cmd.Flags().StringVar(&invoicePath, "invoice", "", "PSA invoice CSV with a key column (external_id, org_id, id or email) and a quantity column")
	cmd.Flags().StringVar(&billableField, "billable-field", "seats_used", "Vendor counter you bill on: seats_used, active_seats or required_seats")
	cmd.Flags().BoolVar(&addPaidShared, "add-paid-shared", false, "Add paid_shared_mailboxes to the billable count")
	cmd.Flags().BoolVar(&includeInactive, "include-inactive", false, "Include deactivated or suspended customers even when they hold no seats")
	cmd.Flags().BoolVar(&flaggedOnly, "flagged-only", false, "Only return organizations with at least one flag")
	cmd.Flags().IntVar(&concurrency, "concurrency", 4, "Organization details read in parallel")
	cmd.Flags().DurationVar(&sweepTimeout, "sweep-timeout", defaultSweepTimeout, "Budget for the whole run across all organizations (each request still uses --timeout)")
	return cmd
}

func removeFlag(flags []string, drop string) []string {
	out := make([]string, 0, len(flags))
	for _, f := range flags {
		if f != drop {
			out = append(out, f)
		}
	}
	return out
}
