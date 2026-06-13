// Copyright 2026 servosity. Licensed under Apache-2.0. See LICENSE.

// Package cli — bill / bill --reconcile.
//
// `bill` is the smart user-facing front-end on top of the generated
// /resellers/{id}/bill/ endpoint:
//   - resolves the reseller ID automatically from /current-user/ so MSP
//     owners do not have to look it up.
//   - plain mode just renders the bill (table / json / csv).
//   - --reconcile mode joins the bill against a CSV the MSP exports from
//     their own invoicing system and surfaces every drift line — clients
//     being undercharged, overcharged, missing on either side.
//
// Money is held as integer cents end-to-end. Floats are only ever used at
// the parse boundary (CSV ingest, API JSON decode) and converted to cents
// at first opportunity. The display formatter divides by 100 once, at the
// edge.
package cli

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"servosity-msp-pp-cli/internal/store"
)

// reconcileThresholdCents is the absolute-value cents floor below which a
// drift is treated as rounding noise and not flagged in the NOTE column.
// $5.00 = 500 cents matches the brief.
const reconcileThresholdCents = 500

// billLineItem is a tolerant decode of one row from the bill's "line_items"
// array. Each row is one billed backup: a company NAME (suffixed with a tier
// tag like " [Simplified]"), the product, and a count. The endpoint exposes
// no per-line dollar amount and no company_id to a partner token — the old
// decode looked for "company_id"/"amount" (neither exists), so every line
// collapsed to an empty id and the Servosity side of the reconcile was all
// zero. We extract the company name + count instead and resolve the name to
// an id against the local companies table.
type billLineItem struct {
	Company string          `json:"company"`
	Product string          `json:"product"`
	Count   int             `json:"count"`
	Amount  json.RawMessage `json:"amount"` // tolerated if a priced bill ever appears
}

// reconcileRow is one joined company across the Servosity bill vs. the MSP's
// own invoice. The Servosity side is measured in BILLED BACKUPS (the bill
// exposes no dollar amount to partner tokens), so ServosityBackups is the
// real signal; ServosityCent stays 0 unless a priced bill is ever returned.
// Either side can be zero (missing on that side).
type reconcileRow struct {
	CompanyID        string `json:"company_id,omitempty"`
	CompanyName      string `json:"company_name"`
	ServosityBackups int    `json:"servosity_backups"`
	Products         string `json:"products,omitempty"`
	ServosityCent    int64  `json:"servosity_cents"`
	InvoicedCent     int64  `json:"invoiced_cents"`
	DeltaCent        int64  `json:"delta_cents"`
	Note             string `json:"note,omitempty"`
}

// reconcileReport is the JSON envelope when --format json is requested
// (or --json is set). Totals are surfaced separately so spreadsheet
// pipelines don't have to re-sum.
type reconcileReport struct {
	Month  string                 `json:"month"`
	Totals map[string]int64       `json:"totals"`
	Rows   []reconcileRow         `json:"rows"`
	Meta   map[string]interface{} `json:"meta,omitempty"`
}

// pp:data-source live
func newNovelBillCmd(flags *rootFlags) *cobra.Command {
	var reconcilePath string
	var month string
	var format string

	cmd := &cobra.Command{
		Use:   "bill",
		Short: "Pull your monthly Servosity bill — with optional reconcile against your client invoicing",
		Long: `Pull the MSP's monthly Servosity bill, with two modes:

  bill                       — show the bill (table | json | csv).
  bill --reconcile <csv>     — compare line-by-line against a CSV of what
                                you've invoiced YOUR clients and surface
                                the drift (over/under-charges, missing rows).

The CSV must have header columns: company_id,company_name,invoiced_amount.
Amounts may be formatted with or without a leading "$" (e.g. "128.50" or
"$128.50"). All money is computed in integer cents to avoid float drift.`,
		Example: `  # Show this month's Servosity bill
  servosity-cli bill

  # Last month, JSON for piping into jq
  servosity-cli bill --month 2026-04 --format json

  # Reconcile against your invoicing system
  servosity-cli bill --reconcile ./invoices-2026-05.csv`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate --format early so usage errors don't waste an API call.
			format = strings.ToLower(strings.TrimSpace(format))
			switch format {
			case "", "table":
				format = "table"
			case "json", "csv":
				// ok
			default:
				return usageErr(fmt.Errorf("--format must be one of: table, json, csv (got %q)", format))
			}

			// Default month = current month in YYYY-MM. Validate any user value.
			if strings.TrimSpace(month) == "" {
				month = time.Now().Format("2006-01")
			} else {
				if _, err := time.Parse("2006-01", month); err != nil {
					return usageErr(fmt.Errorf("--month must be YYYY-MM (got %q): %w", month, err))
				}
			}

			// Verify-friendly: short-circuit BEFORE any IO. The printing-press
			// verify pipeline runs every hand-written command with --dry-run;
			// this guard keeps it green. The CSV existence check below is
			// IO and must come AFTER the dry-run short-circuit.
			if dryRunOK(flags) {
				return nil
			}

			// --reconcile <path>: fail fast on a missing file BEFORE we
			// burn an API call. Exit 2 (usage / input validation).
			if reconcilePath != "" {
				if _, err := os.Stat(reconcilePath); err != nil {
					if errors.Is(err, os.ErrNotExist) {
						return usageErr(fmt.Errorf("--reconcile file does not exist: %s", reconcilePath))
					}
					return usageErr(fmt.Errorf("--reconcile file %s: %w", reconcilePath, err))
				}
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}

			// Step 1: resolve reseller ID. The legacy resolveResellerID
			// here probed /current-user/, which doesn't expose the
			// reseller field on partner-scoped tokens. resolveResellerID
			// derives it from the first company's reseller URL field,
			// with SERVOSITY_MSP_RESELLER_ID as an override.
			resellerInt64, err := resolveResellerID(cmd.Context(), c)
			if err != nil {
				return fmt.Errorf("resolving reseller ID: %w", err)
			}
			resellerID := strconv.FormatInt(resellerInt64, 10)

			// Step 2: pull the bill. The OpenAPI path doesn't expose a
			// month query param explicitly; pass it through anyway so the
			// API gets the hint when it does support it, and we surface
			// the requested month in our output envelope regardless.
			//
			// Fetched directly (not via resolveRead) on purpose: the bill is a
			// computed report object, not a CRUD resource. Routing it through
			// the write-through cache made the store's batch upsert try to
			// extract a primary key from the report envelope, fail, and print
			// "1/1 bill items skipped (no extractable ID field found)" to
			// stderr on every run. A direct GET keeps that noise out.
			billPath := replacePathParam("/resellers/{id}/bill/", "id", resellerID)
			params := map[string]string{}
			if month != "" {
				params["month"] = month
			}
			data, err := c.GetWithHeaders(cmd.Context(), billPath, params, nil)
			if err != nil {
				return classifyAPIError(err, flags)
			}

			// Plain bill mode: pass the API response through the standard
			// format pipeline. Honor --format as a sugar for the global
			// --json / --csv flags so users don't have to mix flags.
			if reconcilePath == "" {
				return renderPlainBill(cmd.OutOrStdout(), data, format, flags)
			}

			// Reconcile mode.
			lines, err := extractBillLines(data)
			if err != nil {
				return apiErr(fmt.Errorf("decoding bill response: %w", err))
			}
			invoices, err := readInvoicesCSV(reconcilePath)
			if err != nil {
				return usageErr(fmt.Errorf("reading %s: %w", reconcilePath, err))
			}
			// Resolve bill line-item company NAMES to ids against the local
			// companies table so the bill side joins to the invoice side.
			nameToID := loadCompanyNameIndex(cmd.Context())
			rows := joinAndScore(lines, invoices, nameToID)
			report := buildReport(month, rows)

			return renderReconcile(cmd.OutOrStdout(), report, format, flags)
		},
	}

	cmd.Flags().StringVar(&reconcilePath, "reconcile", "", "Path to invoicing CSV (columns: company_id,company_name,invoiced_amount). Triggers reconcile mode.")
	cmd.Flags().StringVar(&month, "month", "", "Billing period in YYYY-MM (default: current month)")
	cmd.Flags().StringVar(&format, "format", "table", "Output format: table | json | csv")

	return cmd
}

// extractBillLines decodes /resellers/{id}/bill/ into a flat slice of
// line items. The endpoint can return either a top-level array or an
// object wrapping the array under one of several keys — tolerate both.
func extractBillLines(data json.RawMessage) ([]billLineItem, error) {
	if len(data) == 0 {
		return nil, nil
	}
	// Try flat array first.
	var arr []billLineItem
	if err := json.Unmarshal(data, &arr); err == nil {
		return arr, nil
	}
	// Try wrapped object.
	var wrap map[string]json.RawMessage
	if err := json.Unmarshal(data, &wrap); err != nil {
		return nil, err
	}
	for _, key := range []string{"results", "items", "line_items", "lines", "bill"} {
		if v, ok := wrap[key]; ok {
			if err := json.Unmarshal(v, &arr); err == nil {
				return arr, nil
			}
		}
	}
	return nil, fmt.Errorf("bill response shape not recognized (expected array or {results|items|line_items|lines|bill: [...]})")
}

// readInvoicesCSV parses the MSP's invoicing CSV. Header row is REQUIRED
// and must include company_id, company_name, invoiced_amount (case-insens,
// order-agnostic so MSPs can hand us whatever their accounting tool
// exports without re-arranging columns).
func readInvoicesCSV(path string) (map[string]reconcileRow, error) {
	// path is the operator's --reconcile CSV: a file the user explicitly
	// asks us to read, not attacker-controlled inclusion.
	f, err := os.Open(path) // #nosec G304 -- operator-controlled --reconcile path
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	r := csv.NewReader(f)
	r.TrimLeadingSpace = true
	// Allow some sloppy exports (variable field counts inside reason cols).
	r.FieldsPerRecord = -1

	header, err := r.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("CSV is empty")
		}
		return nil, err
	}
	idx := map[string]int{}
	for i, h := range header {
		idx[strings.ToLower(strings.TrimSpace(h))] = i
	}
	for _, required := range []string{"company_id", "company_name", "invoiced_amount"} {
		if _, ok := idx[required]; !ok {
			return nil, fmt.Errorf("CSV missing required header column %q (got %v)", required, header)
		}
	}

	out := map[string]reconcileRow{}
	lineNum := 1
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		lineNum++
		if err != nil {
			return nil, fmt.Errorf("CSV line %d: %w", lineNum, err)
		}
		if len(rec) == 0 {
			continue
		}
		cid := strings.TrimSpace(rec[idx["company_id"]])
		if cid == "" {
			// Skip blank rows silently — common when MSPs hand-export.
			continue
		}
		amtStr := rec[idx["invoiced_amount"]]
		cents, err := parseMoneyToCents(amtStr)
		if err != nil {
			return nil, fmt.Errorf("CSV line %d: invoiced_amount %q: %w", lineNum, amtStr, err)
		}
		row := reconcileRow{
			CompanyID:    cid,
			CompanyName:  strings.TrimSpace(rec[idx["company_name"]]),
			InvoicedCent: cents,
		}
		out[cid] = row
	}
	return out, nil
}

// parseMoneyToCents accepts "$128.50" / "128.50" / "128" / "1,234.56" /
// "(45.00)" (accounting negative) and returns integer cents. Negative
// amounts are allowed (credits). Rounding is to nearest cent.
func parseMoneyToCents(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	negative := false
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		negative = true
		s = s[1 : len(s)-1]
	}
	s = strings.TrimPrefix(s, "$")
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "-") {
		negative = !negative
		s = s[1:]
	}
	if s == "" {
		return 0, fmt.Errorf("empty amount after strip")
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	cents := int64(math.Round(f * 100))
	if negative {
		cents = -cents
	}
	return cents, nil
}

// billTagRE strips a trailing tier tag the bill appends to company names,
// e.g. "Rhino Corp [Simplified]" or "Hippo Inc [Silver]" → "Rhino Corp".
var billTagRE = regexp.MustCompile(`\s*\[[^\]]*\]\s*$`)

// stripBillTag removes the bill's tier-tag suffix and trims whitespace.
func stripBillTag(name string) string {
	return strings.TrimSpace(billTagRE.ReplaceAllString(name, ""))
}

// billAgg accumulates one company's bill-side facts.
type billAgg struct {
	companyID string
	name      string
	backups   int
	products  map[string]struct{}
}

// joinAndScore folds bill line items and the invoice CSV into a single
// row-per-company set. The bill side is keyed by company id, resolved from
// each line item's company NAME via nameToID; the Servosity signal is the
// count of billed backups (the bill carries no dollar amount). NOTE is
// assigned by classifyDrift from billing-presence vs. invoiced.
func joinAndScore(billLines []billLineItem, invoices map[string]reconcileRow, nameToID map[string]int64) []reconcileRow {
	billByID := map[string]*billAgg{}
	for _, line := range billLines {
		cleanName := stripBillTag(line.Company)
		if cleanName == "" {
			continue
		}
		cid := ""
		if id, ok := nameToID[strings.ToLower(cleanName)]; ok {
			cid = strconv.FormatInt(id, 10)
		} else {
			// Unresolved name: key by the cleaned name so the row still shows.
			cid = "name:" + cleanName
		}
		a := billByID[cid]
		if a == nil {
			a = &billAgg{companyID: cid, name: cleanName, products: map[string]struct{}{}}
			billByID[cid] = a
		}
		count := line.Count
		if count <= 0 {
			count = 1
		}
		a.backups += count
		if p := strings.TrimSpace(line.Product); p != "" {
			a.products[p] = struct{}{}
		}
	}

	// Union of all company ids across both sides.
	idSet := map[string]struct{}{}
	for cid := range billByID {
		idSet[cid] = struct{}{}
	}
	for cid := range invoices {
		idSet[cid] = struct{}{}
	}

	rows := make([]reconcileRow, 0, len(idSet))
	for cid := range idSet {
		b := billByID[cid]
		inv := invoices[cid]
		row := reconcileRow{
			CompanyID:    cid,
			InvoicedCent: inv.InvoicedCent,
		}
		if b != nil {
			row.ServosityBackups = b.backups
			row.Products = joinProducts(b.products)
			row.CompanyName = firstNonEmpty(inv.CompanyName, b.name)
		} else {
			row.CompanyName = inv.CompanyName
		}
		// ServosityCent stays 0 (no priced bill); delta echoes invoiced so the
		// existing dollar columns/JSON keys remain populated for consumers.
		row.DeltaCent = row.InvoicedCent - row.ServosityCent
		row.Note = classifyDrift(row)
		rows = append(rows, row)
	}

	// Sort: most billed backups first (biggest exposure), then by id.
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].ServosityBackups != rows[j].ServosityBackups {
			return rows[i].ServosityBackups > rows[j].ServosityBackups
		}
		if rows[i].InvoicedCent != rows[j].InvoicedCent {
			return rows[i].InvoicedCent > rows[j].InvoicedCent
		}
		return rows[i].CompanyID < rows[j].CompanyID
	})
	return rows
}

// joinProducts renders a product set as a stable comma-separated string.
func joinProducts(set map[string]struct{}) string {
	if len(set) == 0 {
		return ""
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

// loadCompanyNameIndex builds a lower-cased name → id map from the local
// companies table. Best-effort: a missing/empty store yields an empty map,
// and unresolved bill names fall back to name-keyed rows in joinAndScore.
func loadCompanyNameIndex(ctx context.Context) map[string]int64 {
	out := map[string]int64{}
	db, err := store.OpenWithContext(ctx, defaultDBPath("servosity-cli"))
	if err != nil {
		return out
	}
	defer db.Close()
	rows, err := db.DB().QueryContext(ctx,
		`SELECT CAST(id AS INTEGER), COALESCE(name,'') FROM companies WHERE name IS NOT NULL AND name != ''`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var name string
		if rows.Scan(&id, &name) == nil && name != "" {
			out[strings.ToLower(strings.TrimSpace(name))] = id
		}
	}
	return out
}

// classifyDrift flags presence mismatches between the Servosity bill (billed
// backups) and the MSP's invoice (dollars). The bill exposes no per-client
// price to a partner token, so this is a presence reconcile, not a dollar
// reconcile: the high-value catch is a client Servosity bills you for that you
// forgot to invoice (lost revenue).
func classifyDrift(row reconcileRow) string {
	billed := row.ServosityBackups > 0
	invoiced := row.InvoicedCent != 0
	switch {
	case billed && !invoiced:
		return "Servosity-billed, not invoiced (lost revenue)"
	case !billed && invoiced:
		return "invoiced but no Servosity backups"
	default:
		return "" // matched on both sides (or absent on both)
	}
}

func buildReport(month string, rows []reconcileRow) reconcileReport {
	totals := map[string]int64{"servosity": 0, "invoiced": 0, "delta": 0, "servosity_backups": 0}
	for _, r := range rows {
		totals["servosity"] += r.ServosityCent
		totals["invoiced"] += r.InvoicedCent
		totals["servosity_backups"] += int64(r.ServosityBackups)
	}
	totals["delta"] = totals["invoiced"] - totals["servosity"]
	return reconcileReport{Month: month, Totals: totals, Rows: rows}
}

// renderPlainBill is the no-reconcile path: render the raw bill response
// in the requested format. Honors --format as a sugar over the global
// --json / --csv flags so the user gets the format they asked for even if
// they didn't combine flags.
func renderPlainBill(w io.Writer, data json.RawMessage, format string, flags *rootFlags) error {
	switch format {
	case "json":
		// Pretty JSON, no envelope — this is the "smart" command, not the
		// generated raw one, so the user expects clean output they can
		// pipe to jq without unwrapping a provenance layer.
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		var v any
		if err := json.Unmarshal(data, &v); err != nil {
			// Fall back to raw bytes if it isn't valid JSON.
			_, err := w.Write(data)
			return err
		}
		return enc.Encode(v)
	case "csv":
		// Reuse the standard pipeline by forcing the flag and routing
		// through printOutputWithFlags — it knows how to flatten arrays.
		orig := flags.csv
		flags.csv = true
		defer func() { flags.csv = orig }()
		return printOutputWithFlags(w, data, flags)
	default: // "table"
		// Standard auto-table over an array of objects.
		var items []map[string]any
		if err := json.Unmarshal(data, &items); err == nil && len(items) > 0 {
			return printAutoTable(w, items)
		}
		// Fallback for object-shaped responses.
		return printOutputWithFlags(w, data, flags)
	}
}

// renderReconcile writes the report in the requested format.
func renderReconcile(w io.Writer, report reconcileReport, format string, flags *rootFlags) error {
	// --json wins over --format=table (global flag is more specific).
	if flags.asJSON {
		format = "json"
	} else if flags.csv {
		format = "csv"
	}
	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	case "csv":
		cw := csv.NewWriter(w)
		defer cw.Flush()
		if err := cw.Write([]string{"company_id", "company_name", "servosity_backups", "invoiced", "note", "products"}); err != nil {
			return err
		}
		for _, r := range report.Rows {
			if err := cw.Write([]string{
				r.CompanyID,
				r.CompanyName,
				strconv.Itoa(r.ServosityBackups),
				formatCents(r.InvoicedCent),
				r.Note,
				r.Products,
			}); err != nil {
				return err
			}
		}
		// Totals row.
		return cw.Write([]string{
			"", "TOTAL",
			strconv.FormatInt(report.Totals["servosity_backups"], 10),
			formatCents(report.Totals["invoiced"]),
			"", "",
		})
	default: // table
		tw := newTabWriter(w)
		fmt.Fprintln(tw, "COMPANY\tSVT_BACKUPS\tINVOICED\tNOTE")
		for _, r := range report.Rows {
			label := r.CompanyName
			if r.CompanyID != "" && !strings.HasPrefix(r.CompanyID, "name:") {
				label = fmt.Sprintf("%s (%s)", r.CompanyName, r.CompanyID)
			}
			if r.CompanyName == "" && r.CompanyID != "" {
				label = fmt.Sprintf("(%s)", r.CompanyID)
			}
			fmt.Fprintf(tw, "%s\t%d\t$%s\t%s\n",
				label,
				r.ServosityBackups,
				formatCents(r.InvoicedCent),
				r.Note,
			)
		}
		if err := tw.Flush(); err != nil {
			return err
		}
		fmt.Fprintf(w, "\nTOTAL Servosity-billed backups: %d  ·  invoiced: $%s\n",
			report.Totals["servosity_backups"],
			formatCents(report.Totals["invoiced"]),
		)
		return nil
	}
}

// formatCents turns integer cents into a "128.50" string (no sign, no $).
func formatCents(c int64) string {
	neg := c < 0
	if neg {
		c = -c
	}
	whole := c / 100
	frac := c % 100
	if neg {
		return fmt.Sprintf("-%d.%02d", whole, frac)
	}
	return fmt.Sprintf("%d.%02d", whole, frac)
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
