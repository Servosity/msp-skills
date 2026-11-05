// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command (handfixes.json warranty-novels).
// pp:data-source live

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"warranty-pp-cli/internal/cliutil"
	"warranty-pp-cli/internal/config"
	"warranty-pp-cli/internal/store"
	"warranty-pp-cli/internal/warranty"

	"github.com/spf13/cobra"
)

// recheckSelect picks the devices due a lookup: never checked first, then
// coverage ending within the expiring window (soonest first), then stale
// devices oldest first. Devices with no vendor are left for 'unresolved'.
func recheckSelect(devices []store.Device, vendor, asOf string, staleBefore, recentBefore time.Time, expiringDays, limit int) []store.Device {
	type pick struct {
		d    store.Device
		rank int
	}
	picks := []pick{}
	for _, d := range devices {
		if d.Vendor == "" || (vendor != "" && d.Vendor != vendor) {
			continue
		}
		checked, err := time.Parse(time.RFC3339, d.LastChecked)
		switch {
		case d.LastChecked == "" || err != nil:
			picks = append(picks, pick{d, 0})
		case expiringDays > 0 && d.CoverageEnd != "" && !d.CoverageOpen && checked.Before(recentBefore):
			if n := warranty.DaysBetween(asOf, d.CoverageEnd); n != nil && *n <= expiringDays {
				picks = append(picks, pick{d, 1})
				continue
			}
			if checked.Before(staleBefore) {
				picks = append(picks, pick{d, 2})
			}
		case checked.Before(staleBefore):
			picks = append(picks, pick{d, 2})
		}
	}
	sort.SliceStable(picks, func(i, j int) bool {
		a, b := picks[i], picks[j]
		if a.rank != b.rank {
			return a.rank < b.rank
		}
		if a.rank == 1 && a.d.CoverageEnd != b.d.CoverageEnd {
			return a.d.CoverageEnd < b.d.CoverageEnd
		}
		return a.d.LastChecked < b.d.LastChecked
	})
	out := make([]store.Device, 0, len(picks))
	for _, p := range picks {
		if limit > 0 && len(out) >= limit {
			break
		}
		out = append(out, p.d)
	}
	return out
}

// holdBackAppleAcrossCustomers drops Apple devices when no customer was named
// and they belong to more than one customer: one Apple Business API account
// sees one organization, so the others would all read not_in_org.
func holdBackAppleAcrossCustomers(due []store.Device, customer string) ([]store.Device, int) {
	if customer != "" {
		return due, 0
	}
	owners := map[string]bool{}
	for _, d := range due {
		if d.Vendor == warranty.Apple {
			owners[strings.ToLower(d.Customer)] = true
		}
	}
	if len(owners) <= 1 {
		return due, 0
	}
	kept, held := make([]store.Device, 0, len(due)), 0
	for _, d := range due {
		if d.Vendor == warranty.Apple {
			held++
			continue
		}
		kept = append(kept, d)
	}
	return kept, held
}

func newNovelRecheckCmd(flags *rootFlags) *cobra.Command {
	var customer, vendor, olderThan, expiring, within, asOf string
	var maxCalls, limit int
	var confirm bool
	cmd := &cobra.Command{
		Use:   "recheck",
		Short: "Re-check the serials already in the mirror, never-checked and soon-expiring first",
		Long: `Use this command to refresh coverage for devices already in the mirror,
chosen by last-checked age, client, or upcoming expiry. Do NOT use this command
for a serial or file that has not been imported; use 'lookup' instead.

Picks never-checked devices first, then devices whose coverage ends within
--expiring (not checked in the last day), then devices last checked before
--older-than, oldest first. Each vendor is called with its own batching (Dell
100 tags a call, Lenovo 25 serials a call, Apple 2 calls a serial); vendors
without credentials are skipped and named. --max-calls caps vendor calls for
the run; --dry-run shows the plan without calling anything. Apple Business
accounts belong to one customer organization: pass --customer and --vendor
apple with that customer's Apple variables set. Over MCP the run is a preview
unless --confirm is passed.`,
		Example: strings.Trim(`
  warranty-cli recheck --older-than 30d --expiring 120d --max-calls 50 --dry-run
  warranty-cli recheck --customer "Acme Dental" --agent
  warranty-cli recheck --vendor apple --customer "Beta LLC"`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "mcp:read-only-unless-flags": "confirm", "pp:data-source": "live", "pp:happy-args": "--older-than=30d;--max-calls=5"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help() // a bare call would start a long, mirror-writing run
			}
			if vendor != "" {
				v, ok := warranty.NormalizeVendor(vendor)
				if !ok {
					return usageErr(fmt.Errorf("--vendor must be dell, lenovo or apple"))
				}
				vendor = v
			}
			staleDays, err := parseDays("--older-than", olderThan)
			if err != nil {
				return err
			}
			expDays, err := parseDays("--expiring", expiring)
			if err != nil {
				return err
			}
			windowDays, err := parseDays("--within", within)
			if err != nil {
				return err
			}
			day, err := today(asOf)
			if err != nil {
				return err
			}
			if maxCalls < 0 || limit < 0 {
				return usageErr(fmt.Errorf("--max-calls and --limit must be 0 (no cap) or more"))
			}
			if (cliutil.IsDogfoodEnv() || cliutil.IsMCPChildEnv()) && (maxCalls == 0 || maxCalls > 10) {
				maxCalls = 10 // one bounded unit of work under a harness or an MCP call
			}
			customer = normalizeCustomer(customer)
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			devices := []store.Device{}
			db, err := openWarrantyRead(ctx)
			if err != nil {
				return err
			}
			if db != nil {
				devices, err = db.ListDevices(ctx, customer)
				_ = db.Close() // read-only handle: nothing to flush, so a close error cannot lose data
				if err != nil {
					return err
				}
			}
			now := time.Now().UTC()
			due := recheckSelect(devices, vendor, day, now.Add(-time.Duration(staleDays)*24*time.Hour), now.Add(-24*time.Hour), expDays, limit)
			due, heldApple := holdBackAppleAcrossCustomers(due, customer)
			targets := make([]lookupTarget, 0, len(due))
			for _, d := range due {
				targets = append(targets, lookupTarget{Serial: d.Serial, Vendor: d.Vendor, VendorSource: d.VendorSource})
			}
			if vendor == "" {
				noVendor := 0
				for _, d := range devices {
					if d.Vendor == "" {
						noVendor++
					}
				}
				if noVendor > 0 {
					noun := "devices have"
					if noVendor == 1 {
						noun = "device has"
					}
					// "hint:" lines reach MCP callers too (the shell-out forwards them).
					fmt.Fprintf(cmd.ErrOrStderr(), "hint: %d %s no known vendor and will not be rechecked; see 'warranty-cli unresolved --reason vendor_unknown'\n", noVendor, noun)
				}
			}
			if heldApple > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "note: %d Apple device(s) span several customers and were held back: "+
					"Apple Business accounts belong to one organization, so pass --customer with that customer's Apple variables set\n", heldApple)
			}
			if dryRunOK(flags) {
				cfg, _ := config.Load(flags.configPath)
				return writeDryRun(cmd.OutOrStdout(), flags, planText(fmt.Sprintf("recheck %d of %d device(s)", len(due), len(devices)), targets, vendorStates(cfg)))
			}
			if len(targets) == 0 {
				view := lookupRun{Results: []lookupOutcome{}, Calls: map[string]int{}, FetchFailures: []fetchFailure{},
					Note: fmt.Sprintf("nothing due: %d device(s) in the mirror were checked within %s", len(devices), olderThan)}
				if len(devices) == 0 {
					view.Note = "the warranty mirror is empty: run import first"
				}
				return flags.printJSON(cmd, view)
			}
			save := mirrorWriteAllowed()
			run, runErr := runLookups(ctx, flags, targets, lookupOpts{MaxCalls: maxCalls, WindowDays: windowDays, AsOf: day, Save: save})
			if runErr != nil && len(run.Results) == 0 {
				return runErr
			}
			if !save {
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
			counts := map[string]int{}
			for _, o := range run.Results {
				counts[o.Result]++
			}
			keys := make([]string, 0, len(counts))
			for k := range counts {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			parts := []string{}
			for _, k := range keys {
				parts = append(parts, fmt.Sprintf("%s %d", k, counts[k]))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Rechecked %d device(s): %s. Vendor calls: dell %d, lenovo %d, apple %d. Events raised: %d.\n",
				len(run.Results), strings.Join(parts, ", "), run.Calls["dell"], run.Calls["lenovo"], run.Calls["apple"], run.EventsRaised)
			if run.BudgetExhausted {
				fmt.Fprintln(cmd.OutOrStdout(), "Stopped at --max-calls; run recheck again for the rest.")
			}
			return runErr
		},
	}
	cmd.Flags().StringVar(&customer, "customer", "", "Only this customer's devices")
	cmd.Flags().StringVar(&vendor, "vendor", "", "Only this vendor's devices: dell, lenovo or apple")
	cmd.Flags().StringVar(&olderThan, "older-than", "30d", "Recheck devices last checked before this age")
	cmd.Flags().StringVar(&expiring, "expiring", "120d", "Also recheck devices whose coverage ends within this window (0d to skip)")
	cmd.Flags().StringVar(&within, "within", "90d", "Coverage ending within this window raises coverage.expiring")
	cmd.Flags().StringVar(&asOf, "as-of", "", "Judge expiry as of this date (YYYY-MM-DD; default today)")
	cmd.Flags().IntVar(&maxCalls, "max-calls", 0, "Cap vendor API calls for this run (0 for no cap)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Cap devices rechecked (0 for no cap)")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Save results to the mirror when run over MCP (the CLI saves without it)")
	return cmd
}
