// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"quotewerks-pp-cli/internal/qw"
)

// requoteHeaderColumns are copied from the source document when present.
var requoteHeaderColumns = func() []string {
	cols := []string{"SalesRep"}
	for _, prefix := range []string{"SoldTo", "ShipTo"} {
		for _, suffix := range []string{"Company", "Contact", "Address1", "Address2", "Address3", "City", "State", "PostalCode", "Country", "Phone", "Fax", "Email"} {
			cols = append(cols, prefix+suffix)
		}
	}
	return append(cols, "SoldToCMCompanyRecID")
}()

// parseQtyOverrides reads --qty PART=N flags keyed by normalized part. One
// value may also carry several comma-separated pairs, which is how the MCP
// mirror passes a repeatable flag.
func parseQtyOverrides(values []string) (map[string]float64, error) {
	out := map[string]float64{}
	var pairs []string
	for _, v := range values {
		pairs = append(pairs, strings.Split(v, ",")...)
	}
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		n, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if !ok || qw.NormalizePart(k) == "" || err != nil || n <= 0 || math.IsInf(n, 0) || math.IsNaN(n) {
			return nil, usageErr(fmt.Errorf("--qty %q: use PART=QUANTITY with a positive quantity", p))
		}
		out[qw.NormalizePart(k)] = n
	}
	return out, nil
}

type requoteFailure struct {
	Line  int    `json:"line"`
	Part  string `json:"part,omitempty"`
	Error string `json:"error"`
}

func newNovelRequoteCmd(flags *rootFlags) *cobra.Command {
	var qtyPairs []string
	var only, rep, name, status, productDB string
	var keepPrices, confirm bool

	cmd := &cobra.Command{
		Use:   "requote [DocNo-or-RecGUID]",
		Short: "Copy a prior quote or order into a new quote with today's product prices; previews unless --confirm",
		Long: `Copy an existing document's customer (SoldTo/ShipTo fields), sales rep and lines into
a new QUOTE dated today. Product lines are refreshed from the product databases (UnitCost,
UnitPrice, UnitList) unless --keep-prices; --qty PART=N changes quantities and --only keeps
just the listed parts. Nothing is written until --confirm; then the header is created
first and each line is posted with its DocRecGUID. If a line fails, the new header and the
lines that succeeded remain, and the receipt lists exactly which lines failed.

Use this command to copy an existing document's customer and lines into a new QUOTE, with
optional new quantities and product-database prices refreshed. Do NOT use it to start an
empty quote; use 'docs create' instead. Do NOT use it to add one line to an existing
quote; use 'items add' instead.`,
		Example: strings.Trim(`
  quotewerks-cli requote 1042 --qty 20UES0X100=12 --json
  quotewerks-cli requote 1042 --only 20UES0X100 --keep-prices --confirm`, "\n"),
		Annotations: map[string]string{"pp:data-source": "live", "mcp:read-only-unless-flags": "confirm", "pp:happy-args": "doc=1042"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "requote")
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("the source document number or RecGUID is required"))
			}
			qtys, err := parseQtyOverrides(qtyPairs)
			if err != nil {
				return err
			}
			onlySet := partCSVSet(only)
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			src, err := resolveDoc(ctx, c, args[0])
			if err != nil {
				return err
			}
			srcLines, err := fetchItems(ctx, c, src.ID)
			if err != nil {
				return err
			}
			header := map[string]any{"DocType": "QUOTE", "DocDate": time.Now().Format("2006-01-02") + "T00:00:00"}
			for _, col := range requoteHeaderColumns {
				if v := qw.Str(src.Attributes[col]); v != "" {
					header[col] = src.Attributes[col]
				}
			}
			header["DocName"] = fmt.Sprintf("Requote of %s", qw.Str(src.Attributes["DocNo"]))
			if n := qw.Str(src.Attributes["DocName"]); n != "" {
				header["DocName"] = fmt.Sprintf("Requote of %s: %s", qw.Str(src.Attributes["DocNo"]), n)
			}
			if name != "" {
				header["DocName"] = name
			}
			if rep != "" {
				header["SalesRep"] = rep
			}
			if status != "" {
				header["DocStatus"] = status
			}
			// Copy every column a client may set, so percent discounts, charges,
			// custom fields and the like survive; the schema says which those are.
			writable, err := writableColumns(ctx, c, qw.TableItems)
			if err != nil {
				return err
			}
			var idx *qw.ProductIndex
			if !keepPrices {
				tables, err := productTables(ctx, c, productDB)
				if err != nil {
					return err
				}
				found, err := catalogLookup(ctx, c, tables, lineParts(srcLines))
				if err != nil {
					return err
				}
				idx = qw.NewProductIndex(found)
			}
			lines := make([]map[string]any, 0, len(srcLines))
			sources := make([]string, 0, len(srcLines))
			usedQty := map[string]bool{}
			for _, l := range srcLines {
				product := qw.IsProductLine(l)
				key := qw.NormalizePart(qw.Str(l["ManufacturerPartNumber"]))
				if len(onlySet) > 0 && (!product || !onlySet[key]) {
					continue
				}
				attrs := map[string]any{}
				for col, v := range l {
					if writable[col] && v != nil {
						attrs[col] = v
					}
				}
				source := "copied"
				if product {
					if q, ok := qtys[key]; ok {
						attrs["QtyBase"] = q
						usedQty[key] = true
					}
					if idx != nil && qw.HasPricingFactor(l) {
						source = "copied (line priced per unit of pricing other than one; not refreshed)"
					} else if idx != nil {
						if p, ok := idx.Match(qw.Str(l["Manufacturer"]), qw.Str(l["ManufacturerPartNumber"]), qw.Str(l["VendorPartNumber"])); ok {
							if p.HasCost {
								attrs["UnitCost"] = p.Cost
							}
							if p.HasPrice {
								attrs["UnitPrice"] = p.Price
							}
							if p.HasList {
								attrs["UnitList"] = p.List
							}
							if p.HasCost || p.HasPrice || p.HasList {
								source = "product_db:" + p.DB
							} else {
								source = "copied (catalog row in " + p.DB + " has no usable price)"
							}
						}
					}
				}
				lines = append(lines, attrs)
				sources = append(sources, source)
			}
			if len(lines) == 0 || (len(onlySet) > 0 && !hasProductLine(lines)) {
				return usageErr(fmt.Errorf("nothing to copy from document %s with the given --only parts", qw.Str(src.Attributes["DocNo"])))
			}
			for k := range qtys {
				if !usedQty[k] {
					return usageErr(fmt.Errorf("--qty part %s is not on document %s", k, qw.Str(src.Attributes["DocNo"])))
				}
			}
			preview := make([]map[string]any, 0, len(lines))
			for i, l := range lines {
				row := map[string]any{"price_source": sources[i], "line_type": qw.LineTypeName(l["LineType"])}
				for k, v := range l {
					row[k] = v
				}
				if qw.IsProductLine(l) {
					if m := lineMargin(l); m != nil {
						row["margin_pct"] = *m
					}
				}
				preview = append(preview, row)
			}
			view := map[string]any{
				"source": project(src.Flat(), []string{"id", "DocNo", "DocType", "DocName", "SoldToCompany"}),
				"header": header,
				"lines":  preview,
				"totals": sumLines(lines),
			}
			// Prove the whole plan and its receipt serialize before anything is
			// printed or written (an overflowing quantity makes totals
			// infinite), so a bad value cannot fail after the header exists.
			if _, err := json.Marshal(view); err != nil {
				return usageErr(fmt.Errorf("the new quote cannot be encoded (check quantities and prices): %w", err))
			}
			if !confirm {
				return printWriteResult(cmd.OutOrStdout(), writeModeEnvelope(false, view), flags)
			}
			created, err := qwWrite(ctx, c, "POST", qw.TableHeaders, "", header)
			if err != nil {
				return err
			}
			failures := make([]requoteFailure, 0)
			written := 0
			for i, l := range lines {
				l["DocRecGUID"] = created.ID
				if _, err := qwWrite(ctx, c, "POST", qw.TableItems, "", l); err != nil {
					failures = append(failures, requoteFailure{Line: i + 1, Part: qw.Str(l["ManufacturerPartNumber"]), Error: err.Error()})
					continue
				}
				written++
			}
			view["created"] = project(created.Flat(), qwHeaderFields)
			view["lines_written"] = written
			view["line_failures"] = failures
			if err := printWriteResult(cmd.OutOrStdout(), writeModeEnvelope(true, view), flags); err != nil {
				return err
			}
			if len(failures) > 0 {
				// The summary rides in the error too: MCP clients only see stderr on a non-zero exit.
				nums := make([]string, 0, len(failures))
				for _, f := range failures {
					nums = append(nums, fmt.Sprintf("%d (%s)", f.Line, f.Part))
				}
				return partialFailureErr(fmt.Errorf("new quote %s (DocNo %s) was created but %d of %d lines failed: lines %s; do not re-run requote (it would create another quote); add the missing lines with 'items add %s'",
					created.ID, qw.Str(created.Attributes["DocNo"]), len(failures), len(lines), strings.Join(nums, ", "), created.ID))
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&qtyPairs, "qty", nil, "New quantity for a part as PART=QUANTITY; repeatable")
	cmd.Flags().StringVar(&only, "only", "", "Copy only these manufacturer part numbers (comma-separated)")
	cmd.Flags().BoolVar(&keepPrices, "keep-prices", false, "Keep the source document's cost and price instead of refreshing from the product databases")
	cmd.Flags().StringVar(&rep, "rep", "", "Sales rep for the new quote; must be an existing QuoteWerks Web user (default: source rep)")
	cmd.Flags().StringVar(&name, "name", "", "Document name for the new quote (default: 'Requote of <DocNo>: <name>')")
	cmd.Flags().StringVar(&status, "status", "", "Document status for the new quote as your tenant names it")
	cmd.Flags().StringVar(&productDB, "product-db", "", "Refresh prices from one product database only")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Create the new quote and its lines; without it the command only previews")
	return cmd
}

func hasProductLine(lines []map[string]any) bool {
	for _, l := range lines {
		if qw.IsProductLine(l) {
			return true
		}
	}
	return false
}
