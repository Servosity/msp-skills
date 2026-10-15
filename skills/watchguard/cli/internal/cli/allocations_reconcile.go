// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"watchguard-pp-cli/internal/client"
	"watchguard-pp-cli/internal/cliutil"
)

type hardwareFinding struct {
	Serial       string `json:"serial"`
	Model        string `json:"model,omitempty"`
	AccountID    string `json:"account_id,omitempty"`
	AccountName  string `json:"account_name,omitempty"`
	DeviceID     string `json:"device_id,omitempty"`
	DeviceName   string `json:"device_name,omitempty"`
	State        string `json:"state,omitempty"`
	OfflineSince string `json:"offline_since,omitempty"`
}

type hardwareReconcile struct {
	AllocatedNotReporting []hardwareFinding `json:"allocated_not_reporting"`
	AllocatedOffline      []hardwareFinding `json:"allocated_offline"`
	ReportingUnallocated  []hardwareFinding `json:"reporting_unallocated"`
	ReportingUntracked    []hardwareFinding `json:"reporting_untracked"`
	Skipped               string            `json:"skipped,omitempty"`
}

// productUsage keeps quantities nil when the API does not report them:
// subscription rows carry subscriptionTotal (which may be "unlimited") and usage instead.
type productUsage struct {
	ResourceType      string `json:"resource_type"`
	Product           string `json:"product"`
	AllocationType    string `json:"allocation_type,omitempty"`
	Total             *int   `json:"total,omitempty"`
	Allocated         *int   `json:"allocated,omitempty"`
	Available         *int   `json:"available,omitempty"`
	SubscriptionTotal string `json:"subscription_total,omitempty"`
	Usage             *int   `json:"usage,omitempty"`
	ExpiryDate        string `json:"expiry_date,omitempty"`
}

func optInt(m map[string]any, k string) *int {
	if n, ok := intField(m, k); ok {
		return &n
	}
	return nil
}

func productFromRow(resourceType string, m map[string]any) productUsage {
	return productUsage{ResourceType: resourceType, Product: strField(m, "productName"), AllocationType: strField(m, "allocationType"),
		Total: optInt(m, "quantityTotal"), Allocated: optInt(m, "quantityAllocated"), Available: optInt(m, "quantityAvailable"),
		SubscriptionTotal: strField(m, "subscriptionTotal"), Usage: optInt(m, "usage"), ExpiryDate: strField(m, "allocationExpiryDate")}
}

func showInt(p *int) string {
	if p == nil {
		return "-"
	}
	return fmt.Sprint(*p)
}

type reconcileView struct {
	Hardware      hardwareReconcile `json:"hardware"`
	Products      []productUsage    `json:"products"`
	Expiring      []expiringItem    `json:"expiring"`
	Truncated     []string          `json:"truncated_lookups,omitempty"`
	FetchFailures []fetchFailure    `json:"fetch_failures,omitempty"`
}

func emptyHardware() hardwareReconcile {
	return hardwareReconcile{
		AllocatedNotReporting: make([]hardwareFinding, 0), AllocatedOffline: make([]hardwareFinding, 0),
		ReportingUnallocated: make([]hardwareFinding, 0), ReportingUntracked: make([]hardwareFinding, 0),
	}
}

// reconcileHardware matches Firebox allocation rows to the devices reporting in.
func reconcileHardware(allocated, unallocated, devices []map[string]any, offlineAfter time.Duration, now time.Time) hardwareReconcile {
	out := emptyHardware()
	bySerial := map[string]map[string]any{}
	for _, d := range devices {
		if s := serialKey(strField(d, "serial_number")); s != "" {
			bySerial[s] = d
		}
	}
	tracked := map[string]bool{}
	for _, a := range allocated {
		s := serialKey(strField(a, "serialOrLicense"))
		if s == "" {
			continue
		}
		tracked[s] = true
		f := hardwareFinding{Serial: s, Model: strField(a, "modelName"), AccountID: strField(a, "accountId"), AccountName: strField(a, "accountName")}
		d, ok := bySerial[s]
		if !ok {
			out.AllocatedNotReporting = append(out.AllocatedNotReporting, f)
			continue
		}
		f.DeviceID, f.DeviceName, f.State, f.OfflineSince = strField(d, "id"), strField(d, "name"), strField(d, "state"), strField(d, "offline_since")
		if isOffline(f.State) {
			since, ok := parseTime(f.OfflineSince)
			if f.State == "never connected" || !ok || now.Sub(since) >= offlineAfter {
				out.AllocatedOffline = append(out.AllocatedOffline, f)
			}
		}
	}
	unalloc := map[string]bool{}
	for _, u := range unallocated {
		if s := serialKey(strField(u, "serialOrLicense")); s != "" {
			unalloc[s] = true
			tracked[s] = true
		}
	}
	for _, s := range sortedMapKeys(bySerial) {
		d := bySerial[s]
		f := hardwareFinding{Serial: s, Model: strField(d, "short_model_name"), DeviceID: strField(d, "id"), DeviceName: strField(d, "name"), State: strField(d, "state")}
		if unalloc[s] {
			out.ReportingUnallocated = append(out.ReportingUnallocated, f)
		} else if !tracked[s] {
			out.ReportingUntracked = append(out.ReportingUntracked, f)
		}
	}
	return out
}

func allocationSummary(ctx context.Context, c *client.Client, own, resourceType, status string, maxPages int) ([]map[string]any, bool, error) {
	extra := map[string]string{}
	if status != "" {
		extra["allocationStatus"] = status
	}
	return pageLimitOffset(ctx, c, "/rest/platform/allocation/v2/"+url.PathEscape(own)+"/assets/summary/"+url.PathEscape(resourceType), extra, nil, "data", 100, maxPages)
}

func newNovelAllocationsReconcileCmd(flags *rootFlags) *cobra.Command {
	var types, expiring, offline string
	var maxPages int
	cmd := &cobra.Command{
		Use:   "reconcile",
		Short: "Match Firebox allocations against the Fireboxes that actually report in, and summarize software license usage and expiry",
		Long: "Use this command to compare owned, allocated and available licenses and to match hardware allocations against Fireboxes actually reporting in. " +
			"Do NOT use this command for a cross-product per-customer summary; use 'accounts health' instead.\n\n" +
			"Needs a WatchGuard Service Provider account (allocation APIs). Hardware: allocated serials with no reporting Firebox, allocated Fireboxes offline " +
			"longer than --offline-for, reporting Fireboxes still unallocated, and reporting Fireboxes with no allocation record; skipped with a reason if a " +
			"lookup fails. Products: total, allocated and available quantity per software product in the Service Provider's inventory. " +
			"Expiring: Firebox allocations and products that run out within --expiring-within (already-expired ones flagged expired).\n\n" +
			"Exit codes: 0 every lookup succeeded, 6 some lookup failed (listed in fetch_failures; --allow-partial-failure exits 0), 5 the allocation lookup itself failed.",
		Example: strings.Trim(`
  watchguard-cli allocations reconcile --agent
  watchguard-cli allocations reconcile --expiring-within 30d --json
  watchguard-cli allocations reconcile --types endPoint,authPoint --offline-for 14d`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:typed-exit-codes": "0,6"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "allocations reconcile")
			}
			window, err := cliutil.ParseDurationLoose(expiring)
			if err != nil || window < 0 {
				return usageErr(fmt.Errorf("--expiring-within: %q is not a duration like 30d", expiring))
			}
			offlineAfter, err := cliutil.ParseDurationLoose(offline)
			if err != nil || offlineAfter < 0 {
				return usageErr(fmt.Errorf("--offline-for: %q is not a duration like 7d", offline))
			}
			pages := listPages(maxPages)
			ctx := cmd.Context()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			own, err := ownAccount(c)
			if err != nil {
				return err
			}
			now := time.Now()
			view := reconcileView{Hardware: emptyHardware(), Products: make([]productUsage, 0), Expiring: make([]expiringItem, 0), FetchFailures: make([]fetchFailure, 0)}
			lookups := 0
			fail := func(source string, err error) {
				view.FetchFailures = append(view.FetchFailures, fetchFailure{Source: source, Error: err.Error()})
			}
			trunc := func(source string, t bool) {
				if t {
					view.Truncated = append(view.Truncated, source)
				}
			}

			allocated, t1, err := allocationSummary(ctx, c, own, "firebox", "allocated", pages)
			if err != nil {
				return classifyAPIErrorOnly(err)
			}
			lookups += 3 // allocated, unallocated, devices
			trunc("firebox allocated summary", t1)
			unallocated, t2, errU := allocationSummary(ctx, c, own, "firebox", "unallocated", pages)
			if errU != nil {
				fail("firebox unallocated summary", errU)
			}
			trunc("firebox unallocated summary", t2)
			devData, errD := c.Get(ctx, "/rest/firebox/management/v1/info/"+url.PathEscape(own)+"/devices", map[string]string{"include_hierarchy": "true", "device_type": "FB"})
			if errD != nil {
				fail("firebox devices", errD)
			}
			switch {
			case errU != nil || errD != nil:
				view.Hardware.Skipped = "hardware matching skipped: a Firebox lookup failed (see fetch_failures); partial data would report false findings"
			case t1 || t2:
				view.Hardware.Skipped = "hardware matching skipped: an allocation list was cut short by --max-pages; raise it so every serial is matched"
			default:
				view.Hardware = reconcileHardware(allocated, unallocated, asMaps(devData, "data"), offlineAfter, now)
			}
			view.Expiring = append(view.Expiring, expiringWithin("firebox", allocated, window, now)...)

			for _, rt := range strings.Split(types, ",") {
				if rt = strings.TrimSpace(rt); rt == "" {
					continue
				}
				lookups++
				rows, t, err := allocationSummary(ctx, c, own, rt, "", pages)
				if err != nil {
					fail(rt+" summary", err)
					continue
				}
				trunc(rt+" summary", t)
				for _, m := range rows {
					view.Products = append(view.Products, productFromRow(rt, m))
				}
				view.Expiring = append(view.Expiring, expiringWithin(rt, rows, window, now)...)
			}
			sort.SliceStable(view.Expiring, func(i, j int) bool { return view.Expiring[i].DaysLeft < view.Expiring[j].DaysLeft })
			if len(view.FetchFailures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d lookups failed; see fetch_failures\n", len(view.FetchFailures))
			}
			if len(view.Truncated) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: --max-pages cut %s short\n", strings.Join(view.Truncated, ", "))
			}
			// Hand-fix (handfixes.json fanout-partial-data-exit): a failed lookup
			// exits 6 like every other fan-out, not 0 with a fetch_failures list.
			verdict := readVerdict(cmd.ErrOrStderr(), flags, lookups-len(view.FetchFailures), len(view.FetchFailures), "allocation lookups")
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFiltered(cmd.OutOrStdout(), view, flags); err != nil {
					return err
				}
				return verdict
			}
			h := view.Hardware
			if h.Skipped != "" {
				fmt.Fprintln(cmd.OutOrStdout(), h.Skipped)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Fireboxes: %d allocated but not reporting, %d allocated and offline, %d reporting but unallocated, %d reporting with no allocation record\n",
					len(h.AllocatedNotReporting), len(h.AllocatedOffline), len(h.ReportingUnallocated), len(h.ReportingUntracked))
			}
			tw := newTabWriter(cmd.OutOrStdout())
			fmt.Fprintln(tw, "TYPE\tPRODUCT\tALLOCATION\tTOTAL\tALLOCATED\tAVAILABLE\tSUBSCRIPTION\tUSAGE")
			for _, p := range view.Products {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", p.ResourceType, p.Product, p.AllocationType, showInt(p.Total), showInt(p.Allocated), showInt(p.Available), p.SubscriptionTotal, showInt(p.Usage))
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			for _, e := range view.Expiring {
				fmt.Fprintf(cmd.OutOrStdout(), "expires in %d day(s): %s %s %s %s\n", e.DaysLeft, e.ResourceType, e.Product, e.Serial, e.AccountName)
			}
			return verdict
		},
	}
	cmd.Flags().StringVar(&types, "types", "endPoint,authPoint,threatSync,ndr,fireCloud", "Software resource types to summarize: endPoint, authPoint, threatSync, ndr, fireCloud, managedServices, fireboxRetention, cloudDR")
	cmd.Flags().StringVar(&expiring, "expiring-within", "30d", "Report allocations and licenses that expire within this window")
	cmd.Flags().StringVar(&offline, "offline-for", "7d", "Report allocated Fireboxes offline at least this long")
	cmd.Flags().IntVar(&maxPages, "max-pages", 20, fmt.Sprintf("Maximum pages of 100 rows per allocation lookup (1-%d)", maxListPages))
	return cmd
}
