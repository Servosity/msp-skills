// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command (handfixes.json warranty-novels).
// pp:data-source local

package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"warranty-pp-cli/internal/store"
	"warranty-pp-cli/internal/warranty"

	"github.com/spf13/cobra"
)

type importResult struct {
	File       string                `json:"file"`
	Customer   string                `json:"customer,omitempty"`
	Rows       int                   `json:"rows"`
	Created    int                   `json:"created"`
	Updated    int                   `json:"updated"`
	ByVendor   map[string]int        `json:"by_vendor"`
	NoVendor   []string              `json:"no_vendor"`
	Reassigned []importReassignment  `json:"reassigned"`
	Skipped    []warranty.ImportSkip `json:"skipped"`
	Saved      bool                  `json:"saved"`
}

type importReassignment struct {
	Serial string `json:"serial"`
	From   string `json:"from"`
	To     string `json:"to"`
}

func newNovelImportCmd(flags *rootFlags) *cobra.Command {
	var customer, format string
	cmd := &cobra.Command{
		Use:   "import <file>",
		Short: "Load a CSV or JSON device export (serial, optional manufacturer, model, hostname) into the mirror under a client name.",
		Long: `Use this command to load a client's device export into the local warranty
mirror, keyed by customer. Do NOT use this command to look serials up now; use
'lookup' instead.

Accepted columns (any case; spaces, dashes and underscores ignored):
  serial: Serial, Serial Number, Service Tag, SN, BIOS Serial Number
  manufacturer: Manufacturer, Vendor, Make, OEM (Dell, Lenovo, Apple; others
    are kept with no vendor)
  model: Model, Device Model, System Model
  hostname: Hostname, Computer Name, Device Name, Name
  customer: Customer, Client, Company, Organization, Site (--customer wins)
JSON is an array of objects with the same keys, or {"devices": [...]}. The
Datto RMM, NinjaOne and N-central device exports already carry Serial Number
and Manufacturer. Use - to read the file from stdin. Nothing is looked up: run
'recheck' afterwards. CLI only.`,
		Example: strings.Trim(`
  warranty-cli import devices.csv --customer "Acme Dental"
  warranty-cli import ninja-devices.json --json
  warranty-cli import - --customer "Acme Dental" < devices.csv`, "\n"),
		Annotations: map[string]string{"mcp:hidden": "true", "pp:data-source": "local", "pp:happy-args": "file=-;--customer=Acme Dental", "pp:happy-stdin": "Serial Number,Manufacturer,Model\n1ABC234,Dell Inc.,Latitude 5430\n"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if err := refuseUnderMCP("import"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "import devices into the local mirror")
			}
			if len(args) != 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("import takes exactly one file"))
			}
			customer = normalizeCustomer(customer)
			var src io.Reader = cmd.InOrStdin()
			if args[0] != "-" {
				f, err := os.Open(args[0]) // #nosec G304 -- the operator names the file on the CLI; refused over MCP
				if err != nil {
					return usageErr(fmt.Errorf("opening %s: %w", args[0], err))
				}
				defer f.Close()
				src = f
			}
			rows, skips, err := warranty.ParseImport(src, format, customer)
			if err != nil {
				return usageErr(err)
			}
			res := importResult{File: args[0], Customer: customer, Rows: len(rows), ByVendor: map[string]int{},
				NoVendor: []string{}, Reassigned: []importReassignment{}, Skipped: skips}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			db, err := openWarrantyWrite(ctx)
			if err != nil {
				return fmt.Errorf("opening the warranty mirror: %w", err)
			}
			defer db.Close()
			for _, r := range rows {
				up, err := db.UpsertDevice(ctx, store.DeviceUpsert{Serial: r.Serial, Vendor: r.Vendor, VendorSource: r.VendorSource,
					Customer: r.Customer, Source: store.DeviceSourceImport, Model: r.Model, Hostname: r.Hostname})
				if err != nil {
					return fmt.Errorf("saving %s: %w", r.Serial, err)
				}
				if up.Created {
					res.Created++
				} else {
					res.Updated++
				}
				if up.PrevCustomer != "" {
					res.Reassigned = append(res.Reassigned, importReassignment{Serial: r.Serial, From: up.PrevCustomer, To: r.Customer})
				}
				if r.Vendor == "" {
					res.NoVendor = append(res.NoVendor, r.Serial)
				} else {
					res.ByVendor[r.Vendor]++
				}
			}
			res.Saved = true
			for _, rr := range res.Reassigned {
				fmt.Fprintf(cmd.ErrOrStderr(), "note: %s moved from customer %q to %q\n", rr.Serial, rr.From, rr.To)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), res, flags)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Imported %d device(s) from %s: %d new, %d updated, %d skipped.\n",
				res.Rows, res.File, res.Created, res.Updated, len(res.Skipped))
			if len(res.NoVendor) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "%d device(s) have no vendor (add a manufacturer column or use lookup --vendor).\n", len(res.NoVendor))
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Next: warranty-cli recheck")
			return nil
		},
	}
	cmd.Flags().StringVar(&customer, "customer", "", "Customer every device in the file belongs to")
	cmd.Flags().StringVar(&format, "format", "", "File format: csv or json (default: detected)")
	return cmd
}
