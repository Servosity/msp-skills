// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command (handfixes.json warranty-novels).
// pp:data-source local

package cli

import (
	"fmt"
	"strings"
	"time"

	"warranty-pp-cli/internal/config"
	"warranty-pp-cli/internal/store"
	"warranty-pp-cli/internal/warranty"

	"github.com/spf13/cobra"
)

// Unresolved reasons, most blocking first.
var unresolvedReasons = []string{
	warranty.ResultVendorUnknown, warranty.ResultVendorUnconfigured, warranty.ResultAmbiguous,
	warranty.ResultInvalid, warranty.ResultNotFound, warranty.ResultNotInOrg, warranty.ResultError,
	"never_checked", "stale", warranty.ResultDuplicated, "no_coverage",
}

var unresolvedFix = map[string]string{
	warranty.ResultVendorUnknown:      "add a manufacturer column and re-import, or lookup <serial> --vendor dell|lenovo|apple",
	warranty.ResultVendorUnconfigured: "set this vendor's credentials (warranty-cli doctor lists them)",
	warranty.ResultAmbiguous:          "Lenovo needs SERIAL.MACHINETYPE: lookup <serial>.<machine type> --vendor lenovo",
	warranty.ResultInvalid:            "Dell does not recognize this service tag: check it for typos",
	warranty.ResultNotFound:           "the vendor has no record for this serial: check it, or the vendor",
	warranty.ResultNotInOrg:           "not in the configured Apple Business organization: set that customer's Apple variables",
	warranty.ResultError:              "the last lookup failed: run recheck again",
	"never_checked":                   "run warranty-cli recheck",
	"no_coverage":                     "the vendor answered with no coverage dates: check the serial with the vendor",
	"stale":                           "run warranty-cli recheck --older-than <age>",
	warranty.ResultDuplicated:         "Dell lists this tag more than once; its lines were merged",
}

type unresolvedDevice struct {
	Serial      string `json:"serial"`
	Customer    string `json:"customer"`
	Vendor      string `json:"vendor"`
	Reason      string `json:"reason"`
	Detail      string `json:"detail,omitempty"`
	Fix         string `json:"fix"`
	LastChecked string `json:"last_checked"`
}

// unresolvedReason picks the one reason a device's coverage cannot be
// trusted, or "" when it can.
func unresolvedReason(d store.Device, states map[string]vendorState, staleBefore time.Time) string {
	switch {
	case d.Vendor == "":
		return warranty.ResultVendorUnknown
	case !states[d.Vendor].Configured:
		return warranty.ResultVendorUnconfigured
	}
	switch d.LastResult {
	case warranty.ResultAmbiguous, warranty.ResultInvalid, warranty.ResultNotFound, warranty.ResultNotInOrg, warranty.ResultError:
		return d.LastResult
	}
	if d.LastChecked == "" {
		return "never_checked"
	}
	if t, err := time.Parse(time.RFC3339, d.LastChecked); err != nil || t.Before(staleBefore) {
		return "stale"
	}
	if d.LastResult == warranty.ResultDuplicated {
		return warranty.ResultDuplicated
	}
	// report counts this device unverified: the vendor answered with no coverage.
	if d.CoverageEnd == "" && !d.CoverageOpen {
		return "no_coverage"
	}
	return ""
}

func newNovelUnresolvedCmd(flags *rootFlags) *cobra.Command {
	var customer, olderThan, reason string
	cmd := &cobra.Command{
		Use:   "unresolved",
		Short: "Every device whose coverage the report cannot speak for, each with one reason: never checked, stale, vendor unknown",
		Long: `Use this command to list devices whose coverage the mirror cannot vouch for,
with a reason per device. Do NOT use this command to check which vendor
credentials are configured; use 'doctor' instead. Do NOT use it to list devices
that are expiring or expired; use 'report' instead.

Reasons, most blocking first: vendor_unknown (no manufacturer and the serial's
shape names none; HP lands here), vendor_unconfigured, ambiguous (Lenovo 101),
invalid (Dell), not_found, not_in_org (Apple), error, never_checked, stale
(last checked before --older-than), duplicated (Dell), no_coverage (the
vendor answered with no coverage dates). Each row carries the
fix. Reads the local mirror only.`,
		Example: strings.Trim(`
  warranty-cli unresolved --customer "Acme Dental" --agent
  warranty-cli unresolved --older-than 60d --reason stale --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "list unresolved devices from the local mirror")
			}
			days, err := parseDays("--older-than", olderThan)
			if err != nil {
				return err
			}
			if reason != "" && unresolvedFix[reason] == "" {
				return usageErr(fmt.Errorf("--reason must be one of %s", strings.Join(unresolvedReasons, ", ")))
			}
			customer = normalizeCustomer(customer)
			out := map[string]any{"devices": []unresolvedDevice{}, "counts": map[string]int{}, "checked": 0}
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
				// The saved columns were judged on the lookup's date; judge the lines today, as report does.
				if err = judgeCoverageOn(cmd.Context(), db, devices, time.Now().UTC().Format("2006-01-02")); err != nil {
					return err
				}
			}
			cfg, _ := config.Load(flags.configPath)
			states := vendorStates(cfg)
			staleBefore := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
			rows := []unresolvedDevice{}
			counts := map[string]int{}
			for _, d := range devices {
				r := unresolvedReason(d, states, staleBefore)
				if r == "" || (reason != "" && r != reason) {
					continue
				}
				counts[r]++
				detail := d.LastMessage
				if r == warranty.ResultVendorUnconfigured {
					detail = "set " + strings.Join(states[d.Vendor].Needs, ", ")
				}
				rows = append(rows, unresolvedDevice{Serial: d.Serial, Customer: d.Customer, Vendor: d.Vendor, Reason: r,
					Detail: detail, Fix: unresolvedFix[r], LastChecked: d.LastChecked})
			}
			out["devices"], out["counts"], out["checked"] = rows, counts, len(devices)
			// Zero devices is not an all-clear: say why there is nothing to vouch for.
			note := ""
			switch {
			case db == nil:
				note = "the warranty mirror is empty: run import, then recheck"
			case len(devices) == 0:
				note = "no devices in the mirror for this filter: run import, then recheck"
			}
			if note != "" {
				out["note"] = note
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), out, flags)
			}
			if note != "" {
				fmt.Fprintln(cmd.OutOrStdout(), "No devices to check.")
				fmt.Fprintln(cmd.ErrOrStderr(), note)
				return nil
			}
			if len(rows) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "All %d device(s) have current coverage data.\n", len(devices))
				return nil
			}
			table := make([][]string, 0, len(rows))
			for _, r := range rows {
				table = append(table, []string{r.Customer, r.Serial, r.Vendor, r.Reason, r.Fix})
			}
			return flags.printTable(cmd, []string{"CUSTOMER", "SERIAL", "VENDOR", "REASON", "FIX"}, table)
		},
	}
	cmd.Flags().StringVar(&customer, "customer", "", "Only this customer's devices")
	cmd.Flags().StringVar(&olderThan, "older-than", "30d", "Devices last checked before this age are stale")
	cmd.Flags().StringVar(&reason, "reason", "", "Only this reason (e.g. never_checked, stale, vendor_unknown)")
	return cmd
}
