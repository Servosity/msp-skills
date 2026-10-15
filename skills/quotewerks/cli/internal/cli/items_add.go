// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"quotewerks-pp-cli/internal/qw"
)

func newNovelItemsAddCmd(flags *rootFlags) *cobra.Command {
	var part, description, manufacturer, vendorPart, productDB string
	var qty, price, cost, list float64
	var lineType int
	var noLookup, confirm bool

	cmd := &cobra.Command{
		Use:   "add [DocNo-or-RecGUID]",
		Short: "Add a line to a quote, filled from your product databases; previews unless --confirm",
		Long: `Add one DocumentItems line to an existing document. With --part, the part is looked
up across every Products_<Db>_Products table and Manufacturer, Description, UnitPrice,
UnitCost and UnitList are filled from the match (a raw POST leaves them blank).
Explicit --price, --cost, --list and --description win over the product database.
By default the command previews the request; add --confirm to write it.

Use this command to add one line to an existing document. Do NOT use it to copy a whole
prior quote; use 'requote' instead.`,
		Example: strings.Trim(`
  quotewerks-cli items add 1042 --part 20UES0X100 --qty 12 --json
  quotewerks-cli items add 1042 --description "Onsite setup" --qty 4 --price 150 --cost 90 --confirm`, "\n"),
		Annotations: map[string]string{"pp:data-source": "live", "mcp:read-only-unless-flags": "confirm", "pp:happy-args": "doc=1042;--part=20UES0X100"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "items add")
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a document number or RecGUID is required"))
			}
			if part == "" && description == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--part or --description is required"))
			}
			if qty <= 0 {
				return usageErr(fmt.Errorf("--qty must be greater than zero"))
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			doc, err := resolveDoc(ctx, c, args[0])
			if err != nil {
				return err
			}
			attrs := map[string]any{"DocRecGUID": doc.ID, "LineType": lineType, "QtyBase": qty}
			var match *qw.Product
			warnings := make([]string, 0)
			if part != "" {
				attrs["ManufacturerPartNumber"] = part
				if !noLookup {
					tables, err := productTables(ctx, c, productDB)
					if err != nil {
						return err
					}
					found, err := catalogLookup(ctx, c, tables, []string{part})
					if err != nil {
						return err
					}
					if p, ok := qw.NewProductIndex(found).Match(manufacturer, part, part); ok {
						match = &p
						if p.Manufacturer != "" {
							attrs["Manufacturer"] = p.Manufacturer
						}
						if p.ManufacturerPartNumber != "" {
							attrs["ManufacturerPartNumber"] = p.ManufacturerPartNumber
						}
						if p.VendorPartNumber != "" {
							attrs["VendorPartNumber"] = p.VendorPartNumber
						}
						if p.Description != "" {
							attrs["Description"] = p.Description
						}
						if p.HasPrice {
							attrs["UnitPrice"] = p.Price
						}
						if p.HasCost {
							attrs["UnitCost"] = p.Cost
						}
						if p.HasList {
							attrs["UnitList"] = p.List
						}
						if p.PricingFactor {
							warnings = append(warnings, fmt.Sprintf("part %s in %s is priced per unit of pricing other than one; its amounts were not copied, set --price and --cost", part, p.DB))
						} else if !p.HasPrice || !p.HasCost {
							warnings = append(warnings, fmt.Sprintf("part %s matched in %s but that row has no price or cost; set --price and --cost", part, p.DB))
						}
					} else {
						warnings = append(warnings, fmt.Sprintf("part %s was not found in any product database; set --price and --cost or the line lands without pricing", part))
					}
				}
			}
			for col, v := range map[string]string{"Description": description, "Manufacturer": manufacturer, "VendorPartNumber": vendorPart} {
				if v != "" {
					attrs[col] = v
				}
			}
			for col, f := range map[string]string{"UnitPrice": "price", "UnitCost": "cost", "UnitList": "list"} {
				if cmd.Flags().Changed(f) {
					attrs[col] = map[string]float64{"price": price, "cost": cost, "list": list}[f]
				}
			}
			view := map[string]any{"document": project(doc.Flat(), []string{"id", "DocNo", "DocType", "SoldToCompany"})}
			if match != nil {
				view["product_match"] = match
			}
			if m := marginOf(attrs["UnitPrice"], attrs["UnitCost"]); m != nil {
				view["margin_pct"] = *m
			}
			if len(warnings) > 0 {
				view["warnings"] = warnings
			}
			if !confirm {
				view["would"] = []any{jsonapiBody("POST", qw.TableItems, "", attrs)}
				return printWriteResult(cmd.OutOrStdout(), writeModeEnvelope(false, view), flags)
			}
			created, err := qwWrite(ctx, c, "POST", qw.TableItems, "", attrs)
			if err != nil {
				return err
			}
			view["created"] = decorateLine(created.Flat(), false)
			return printWriteResult(cmd.OutOrStdout(), writeModeEnvelope(true, view), flags)
		},
	}
	cmd.Flags().StringVar(&part, "part", "", "Manufacturer part number to look up and quote (ManufacturerPartNumber)")
	cmd.Flags().Float64Var(&qty, "qty", 1, "Quantity (QtyBase)")
	cmd.Flags().Float64Var(&price, "price", 0, "Unit sell price; overrides the product database (UnitPrice)")
	cmd.Flags().Float64Var(&cost, "cost", 0, "Unit cost; overrides the product database (UnitCost)")
	cmd.Flags().Float64Var(&list, "list", 0, "Unit list price; overrides the product database (UnitList)")
	cmd.Flags().StringVar(&description, "description", "", "Line description; overrides the product database (Description)")
	cmd.Flags().StringVar(&manufacturer, "manufacturer", "", "Manufacturer name; also prefers this manufacturer when a part matches several")
	cmd.Flags().StringVar(&vendorPart, "vendor-part", "", "Vendor part number (VendorPartNumber)")
	cmd.Flags().IntVar(&lineType, "line-type", 1, "QuoteWerks LineType: 1 product/service, 2 comment, 256 heading")
	cmd.Flags().StringVar(&productDB, "product-db", "", "Look the part up in one product database only")
	cmd.Flags().BoolVar(&noLookup, "no-lookup", false, "Do not look the part up in the product databases")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Write the line; without it the command only previews the request")
	return cmd
}
