// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command (handfixes.json warranty-novels).
// pp:data-source live

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"warranty-pp-cli/internal/config"
	"warranty-pp-cli/internal/warranty"

	"github.com/spf13/cobra"
)

func newNovelLookupCmd(flags *rootFlags) *cobra.Command {
	var vendor, file, format, customer, within, asOf string
	var noSave, confirm bool
	cmd := &cobra.Command{
		Use:   "lookup [serial...]",
		Short: "Warranty for one or more serials, vendor detected from the serial or named",
		Long: `Use this command to look up warranty coverage for serials right now: Dell
service tags, Lenovo serials (or SERIAL.MACHINETYPE) and Apple serials. Do NOT
use this command to refresh devices already in the mirror; use 'recheck'
instead.

Each serial goes to its vendor (--vendor, the mirror's record, or the serial's
shape: 7 characters Dell, 8 Lenovo, 12 Apple). Dell takes 100 tags per call,
Lenovo 25 serials per call spaced 250 ms apart, Apple one serial per call.
Vendors whose credentials are not set are reported, not called. Results are
saved to the local mirror (skip with --no-save; over MCP only with --confirm)
and raise coverage events.
--file reads a CSV or JSON device list and is CLI-only. Exits 5 when every
vendor call failed (see fetch_failures).`,
		Example: strings.Trim(`
  warranty-cli lookup 1ABC234 PF1ABCDE --agent
  warranty-cli lookup C02XG0FDH7JY --vendor apple --customer "Acme Dental"
  warranty-cli lookup --file devices.csv --customer "Acme Dental" --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "mcp:read-only-unless-flags": "confirm", "pp:data-source": "live", "pp:typed-exit-codes": "0,5", "mcp:read-flags": "file"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if vendor != "" {
				v, ok := warranty.NormalizeVendor(vendor)
				if !ok {
					return usageErr(fmt.Errorf("--vendor must be dell, lenovo or apple"))
				}
				vendor = v
			}
			customer = normalizeCustomer(customer)
			targets := make([]lookupTarget, 0, len(args))
			for _, a := range args {
				s := warranty.NormalizeSerial(a)
				if !warranty.ValidSerial(s) {
					return usageErr(fmt.Errorf("%q is not a serial (4 to 40 letters, digits, dots or dashes)", a))
				}
				t := lookupTarget{Serial: s, Customer: customer}
				if vendor != "" {
					t.Vendor, t.VendorSource = vendor, warranty.SourceDeclared
				}
				targets = append(targets, t)
			}
			if dryRunOK(flags) && file == "" {
				cfg, _ := config.Load(flags.configPath)
				return writeDryRun(cmd.OutOrStdout(), flags, planText("lookup", resolveTargets(cmd.Context(), targets), vendorStates(cfg)))
			}
			if file != "" {
				if err := refuseUnderMCP("lookup --file"); err != nil {
					return err
				}
				if dryRunOK(flags) {
					return writeDryRun(cmd.OutOrStdout(), flags, "lookup every serial in "+file)
				}
				f, err := os.Open(filepath.Clean(file)) // the operator names the file on the CLI; refused over MCP
				if err != nil {
					return usageErr(fmt.Errorf("opening --file: %w", err))
				}
				rows, skips, err := warranty.ParseImport(f, format, customer)
				_ = f.Close()
				if err != nil {
					return usageErr(err)
				}
				for _, s := range skips {
					fmt.Fprintf(cmd.ErrOrStderr(), "skipped row %d: %s\n", s.Row, s.Reason)
				}
				for _, r := range rows {
					t := lookupTarget{Serial: r.Serial, Customer: r.Customer, Vendor: r.Vendor, VendorSource: r.VendorSource}
					if vendor != "" {
						t.Vendor, t.VendorSource = vendor, warranty.SourceDeclared
					}
					targets = append(targets, t)
				}
			}
			if len(targets) == 0 {
				return usageErr(fmt.Errorf("give one or more serials, or --file"))
			}
			windowDays, err := parseDays("--within", within)
			if err != nil {
				return err
			}
			day, err := today(asOf)
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			save := !noSave && mirrorWriteAllowed()
			run, runErr := runLookups(ctx, flags, resolveTargets(ctx, targets), lookupOpts{WindowDays: windowDays, AsOf: day, Save: save})
			if runErr != nil && len(run.Results) == 0 {
				return runErr
			}
			if !noSave && !save {
				run.Note = joinNote(run.Note, previewNote)
			}
			markSourceByCalls(flags, run.Calls)
			if run.FirstError != "" {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d vendor call(s) failed; see fetch_failures\n", len(run.FetchFailures))
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFiltered(cmd.OutOrStdout(), run, flags); err != nil {
					return err
				}
				return runErr
			}
			rows := make([][]string, 0, len(run.Results))
			for _, o := range run.Results {
				rows = append(rows, []string{o.Serial, o.Vendor, o.Result, o.Model, coverageCell(o.CoverageEnd, o.CoverageOpen), o.Status, o.Message})
			}
			if err := flags.printTable(cmd, []string{"SERIAL", "VENDOR", "RESULT", "MODEL", "COVERAGE END", "STATUS", "NOTE"}, rows); err != nil {
				return err
			}
			return runErr
		},
	}
	cmd.Flags().StringVar(&vendor, "vendor", "", "Vendor for every serial: dell, lenovo or apple (default: detected)")
	cmd.Flags().StringVar(&file, "file", "", "CSV or JSON device file to look up (CLI only)")
	cmd.Flags().StringVar(&format, "format", "", "Format of --file: csv or json (default: detected)")
	cmd.Flags().StringVar(&customer, "customer", "", "Customer these devices belong to (saved in the mirror)")
	cmd.Flags().StringVar(&within, "within", "90d", "Coverage ending within this window counts as expiring")
	cmd.Flags().StringVar(&asOf, "as-of", "", "Judge expiry as of this date (YYYY-MM-DD; default today)")
	cmd.Flags().BoolVar(&noSave, "no-save", false, "Do not save results to the local mirror")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Save results to the mirror when run over MCP (the CLI saves without it)")
	return cmd
}

func coverageCell(end string, open bool) string {
	switch {
	case open && end != "":
		return end + " (+ open-ended)"
	case open:
		return "open-ended"
	}
	return end
}
