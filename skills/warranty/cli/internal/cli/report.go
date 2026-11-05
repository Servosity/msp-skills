// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command (handfixes.json warranty-novels).
// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"

	"warranty-pp-cli/internal/store"
	"warranty-pp-cli/internal/warranty"

	"github.com/spf13/cobra"
)

type reportDevice struct {
	Serial       string `json:"serial"`
	Customer     string `json:"customer"`
	Vendor       string `json:"vendor"`
	Model        string `json:"model"`
	Hostname     string `json:"hostname,omitempty"`
	CoverageEnd  string `json:"coverage_end"`
	Status       string `json:"status"`
	DaysLeft     int    `json:"days_left"`
	Renewable    bool   `json:"renewable"`
	LastChecked  string `json:"last_checked"`
	LastCoverage string `json:"last_coverage,omitempty"`
}

type reportView struct {
	AsOf       string         `json:"as_of"`
	WithinDays int            `json:"within_days"`
	Customer   string         `json:"customer,omitempty"`
	Devices    []reportDevice `json:"devices"`
	Counts     map[string]int `json:"counts"`
	Unverified int            `json:"unverified"`
	Note       string         `json:"note,omitempty"`
}

func newNovelReportCmd(flags *rootFlags) *cobra.Command {
	var within, customer, status, asOf string
	var limit int
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Devices out of warranty or expiring within N days, per client, from the mirror.",
		Long: `Use this command to list the individual devices whose hardware coverage ends
within N days or has already ended, per customer. Do NOT use this command for
per-quarter counts or refresh budgets; use 'lifecycle' instead. Do NOT use it
to find devices with no coverage data; use 'unresolved' instead.

Reads the local mirror only. A device's coverage end is the latest end among
its active hardware lines: Dell lines with service levels D, DL, PJ or PR,
canceled or inactive lines, and cancelled contracts never count. Devices with
open-ended coverage (AppleCare+ for Business) are covered. renewable is true
when the vendor shows a renewal path (AppleCare isRenewable, Lenovo upgrade).
unverified counts devices the mirror has no coverage for.`,
		Example: strings.Trim(`
  warranty-cli report --within 90d --agent
  warranty-cli report --customer "Acme Dental" --status expired --csv`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "report from the local mirror")
			}
			windowDays, err := parseDays("--within", within)
			if err != nil {
				return err
			}
			day, err := today(asOf)
			if err != nil {
				return err
			}
			switch status {
			case "all", "expiring", "expired":
			default:
				return usageErr(fmt.Errorf("--status must be all, expiring or expired"))
			}
			if limit < 0 {
				return usageErr(fmt.Errorf("--limit must be 0 (no limit) or more"))
			}
			customer = normalizeCustomer(customer)
			view := reportView{AsOf: day, WithinDays: windowDays, Customer: customer, Devices: []reportDevice{},
				Counts: map[string]int{"expiring": 0, "expired": 0}}
			db, err := openWarrantyRead(cmd.Context())
			if err != nil {
				return err
			}
			devices := []store.Device{}
			if db != nil {
				defer db.Close()
				if devices, err = db.ListDevices(cmd.Context(), customer); err != nil {
					return err
				}
				if err = judgeCoverageOn(cmd.Context(), db, devices, day); err != nil {
					return err
				}
			}
			for _, d := range devices {
				sum := warranty.Summary{End: d.CoverageEnd, Open: d.CoverageOpen}
				st, days := warranty.Status(sum, day, windowDays)
				if !coverageTrusted(d) {
					st = "unknown" // the last vendor answer was not a success: any failure signal wins
				}
				if st == "unknown" {
					view.Unverified++
					continue
				}
				if st == "covered" || (status != "all" && st != status) {
					continue
				}
				view.Counts[st]++
				lines, err := db.Lines(cmd.Context(), d.Serial)
				if err != nil {
					return err
				}
				rd := reportDevice{Serial: d.Serial, Customer: d.Customer, Vendor: d.Vendor, Model: d.Model, Hostname: d.Hostname,
					CoverageEnd: d.CoverageEnd, Status: st, LastChecked: d.LastChecked}
				if days != nil {
					rd.DaysLeft = *days
				}
				for _, l := range lines {
					if l.Active && l.Hardware && l.Renewable {
						rd.Renewable = true
					}
					if l.Hardware && l.End == d.CoverageEnd && (rd.LastCoverage == "" || l.Active) {
						rd.LastCoverage = l.Description
					}
				}
				view.Devices = append(view.Devices, rd)
			}
			sortReport(view.Devices)
			if limit > 0 && len(view.Devices) > limit {
				view.Devices = view.Devices[:limit]
			}
			if db == nil {
				view.Note = "the warranty mirror is empty: run import, then recheck"
			} else if len(devices) == 0 {
				view.Note = "no devices in the mirror for this filter: run import, then recheck"
			} else if view.Unverified > 0 {
				view.Note = fmt.Sprintf("%d device(s) have no coverage data yet; see 'warranty-cli unresolved'", view.Unverified)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			rows := make([][]string, 0, len(view.Devices))
			for _, d := range view.Devices {
				rows = append(rows, []string{d.Customer, d.Serial, d.Vendor, d.Model, d.CoverageEnd, d.Status, fmt.Sprint(d.DaysLeft)})
			}
			if len(rows) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No devices expiring within %d days or expired as of %s.\n", windowDays, day)
			} else if err := flags.printTable(cmd, []string{"CUSTOMER", "SERIAL", "VENDOR", "MODEL", "COVERAGE END", "STATUS", "DAYS"}, rows); err != nil {
				return err
			}
			if view.Note != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), view.Note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&within, "within", "90d", "Coverage ending within this window counts as expiring (e.g. 90d, 12w)")
	cmd.Flags().StringVar(&customer, "customer", "", "Only this customer's devices")
	cmd.Flags().StringVar(&status, "status", "all", "Which devices: all (expiring and expired), expiring or expired")
	cmd.Flags().StringVar(&asOf, "as-of", "", "Judge expiry as of this date (YYYY-MM-DD; default today)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum devices to list (0 for all)")
	return cmd
}

// sortReport orders by days left (expired first, most overdue first), then
// customer and serial.
func sortReport(ds []reportDevice) {
	sort.SliceStable(ds, func(i, j int) bool { return reportLess(ds[i], ds[j]) })
}

func reportLess(a, b reportDevice) bool {
	if a.DaysLeft != b.DaysLeft {
		return a.DaysLeft < b.DaysLeft
	}
	if !strings.EqualFold(a.Customer, b.Customer) {
		return strings.ToLower(a.Customer) < strings.ToLower(b.Customer)
	}
	return a.Serial < b.Serial
}
