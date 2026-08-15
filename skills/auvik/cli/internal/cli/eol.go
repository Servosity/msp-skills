// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// eolRow is one device's support-lifecycle exposure.
type eolRow struct {
	Bucket        string `json:"bucket"`
	DaysRemaining *int   `json:"days_remaining"`
	Client        string `json:"client"`
	DeviceID      string `json:"device_id"`
	DeviceName    string `json:"device_name"`
	DeviceType    string `json:"device_type,omitempty"`
	MakeModel     string `json:"make_model,omitempty"`

	// Dates (v2 lifecycle + warranty). Empty when that resource is unsynced.
	LastSupportDate       string `json:"last_support_date,omitempty"`
	SalesAvailabilityDate string `json:"sales_availability_date,omitempty"`
	WarrantyExpiration    string `json:"warranty_expiration,omitempty"`

	// Status enums (v1 lifecycle + warranty). Never dates.
	LastSupportStatus           string `json:"last_support_status,omitempty"`
	SalesAvailability           string `json:"sales_availability,omitempty"`
	SoftwareMaintenance         string `json:"software_maintenance,omitempty"`
	SecuritySoftwareMaintenance string `json:"security_software_maintenance,omitempty"`
	WarrantyStatus              string `json:"warranty_status,omitempty"`

	Source string `json:"source"`
}

type eolReport struct {
	GeneratedAt  string         `json:"generated_at"`
	AsOf         string         `json:"as_of"`
	Buckets      map[string]int `json:"buckets"`
	DevicesTotal int            `json:"devices_total"`
	DevicesDated int            `json:"devices_with_dates"`
	Rows         []eolRow       `json:"rows"`
	Note         string         `json:"note,omitempty"`
}

// Bucket names, ordered most-urgent first.
const (
	bucketExpired   = "expired"
	bucket90        = "expiring_90d"
	bucket365       = "expiring_365d"
	bucketSupported = "supported"
)

var eolBucketOrder = map[string]int{
	bucketExpired:   0,
	bucket90:        1,
	bucket365:       2,
	bucketSupported: 3,
}

func newNovelEolCmd(flags *rootFlags) *cobra.Command {
	var (
		dbPath  string
		bucket  string
		client  string
		within  int
		limit   int
		asOfStr string
	)

	cmd := &cobra.Command{
		Use:   "eol",
		Short: "See every device approaching or past end-of-support across all your clients at once, bucketed by urgency.",
		Long: strings.Trim(`
Buckets every locally synced device into expired / expiring-90d / expiring-365d /
supported using the earliest of its lifecycle dates (end-of-support, end-of-life,
end-of-sale) and its warranty expiry, grouped by client.

Use this command for hardware support-lifecycle exposure -- end-of-life,
end-of-sale, and warranty expiry across clients. Do NOT use this command for
counting billable devices or reconciling invoice counts; use 'usage reconcile'
instead.

Reads the local mirror. Run this first:
  auvik-cli sync --resources tenants,inventory,auvik-inventory-lifecycle,inventory-device-lifecycle,inventory-device-warranty --full
`, "\n"),
		Example: strings.Trim(`
  # Everything aging out, most urgent first
  auvik-cli eol --agent

  # Only what is already past support, for one client
  auvik-cli eol --bucket expired --client acme --agent

  # Anything expiring inside two quarters, trimmed to slide fields
  auvik-cli eol --within 180 --agent --select rows.client,rows.device_name,rows.make_model,rows.last_support_date
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "--limit=5",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "eol")
			}
			if bucket != "" && eolBucketOrder[bucket] == 0 && bucket != bucketExpired {
				if _, ok := eolBucketOrder[bucket]; !ok {
					_ = cmd.Usage()
					return usageErr(fmt.Errorf("--bucket must be one of expired, expiring_90d, expiring_365d, supported"))
				}
			}

			asOf := time.Now().UTC()
			if asOfStr != "" {
				t, ok := parseAuvikTime(asOfStr)
				if !ok {
					_ = cmd.Usage()
					return usageErr(fmt.Errorf("--as-of must be a date like 2026-08-14"))
				}
				asOf = t
			}

			empty := eolReport{Buckets: map[string]int{}, Rows: []eolRow{}}
			db, handled, err := openLocalMirror(cmd, flags, dbPath, empty)
			if err != nil || handled {
				return err
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, "auvik-inventory-lifecycle") {
				hintIfStale(cmd, db, "auvik-inventory-lifecycle", flags.maxAge)
			}

			ctx := cmd.Context()
			devices, err := loadResources(ctx, db, rtDevices)
			if err != nil {
				return err
			}
			lifecyclesV1, err := loadResources(ctx, db, rtDeviceLifecycle)
			if err != nil {
				return err
			}
			lifecyclesV2, err := loadResources(ctx, db, rtDeviceLifecycleV2)
			if err != nil {
				return err
			}
			warranties, err := loadResources(ctx, db, rtDeviceWarranty)
			if err != nil {
				return err
			}
			names := tenantNames(ctx, db)

			report := buildEolReport(devices, lifecyclesV1, lifecyclesV2, warranties, names, asOf)

			// Filters apply after bucketing so the bucket counts always describe
			// the whole fleet, not the filtered slice.
			rows := report.Rows
			if bucket != "" {
				rows = filterEol(rows, func(r eolRow) bool { return r.Bucket == bucket })
			}
			if client != "" {
				needle := strings.ToLower(client)
				rows = filterEol(rows, func(r eolRow) bool {
					return strings.Contains(strings.ToLower(r.Client), needle)
				})
			}
			if within > 0 {
				rows = filterEol(rows, func(r eolRow) bool {
					return r.DaysRemaining != nil && *r.DaysRemaining <= within
				})
			}
			if limit > 0 && len(rows) > limit {
				rows = rows[:limit]
			}
			report.Rows = rows

			if report.DevicesTotal == 0 {
				report.Note = "no devices in the local mirror; run 'auvik-cli sync --resources inventory --full'"
			} else if report.DevicesDated == 0 {
				report.Note = "devices are synced but no end-of-support dates were found. Auvik's v1 lifecycle resource carries STATUS values only; the dates live on the v2 resource. Run: auvik-cli sync --resources auvik-inventory-lifecycle,inventory-device-warranty --full"
			}

			done, err := emitLocalReport(cmd, flags, report, report.Rows)
			if err != nil || done {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "End-of-support exposure as of %s\n", asOf.Format("2006-01-02"))
			fmt.Fprintf(out, "  expired: %d   expiring 90d: %d   expiring 365d: %d   supported: %d   (of %d devices, %d dated)\n\n",
				report.Buckets[bucketExpired], report.Buckets[bucket90],
				report.Buckets[bucket365], report.Buckets[bucketSupported],
				report.DevicesTotal, report.DevicesDated)
			if len(report.Rows) == 0 {
				if report.Note != "" {
					fmt.Fprintln(out, report.Note)
				} else {
					fmt.Fprintln(out, "No devices match the given filters.")
				}
				return nil
			}
			fmt.Fprintf(out, "%-14s %-6s %-22s %-26s %-22s %s\n", "BUCKET", "DAYS", "CLIENT", "DEVICE", "MAKE/MODEL", "DATE")
			for _, r := range report.Rows {
				days := "-"
				if r.DaysRemaining != nil {
					days = fmt.Sprintf("%d", *r.DaysRemaining)
				}
				fmt.Fprintf(out, "%-14s %-6s %-22s %-26s %-22s %s\n",
					r.Bucket, days, truncate(r.Client, 22), truncate(r.DeviceName, 26),
					truncate(r.MakeModel, 22), r.Source)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	cmd.Flags().StringVar(&bucket, "bucket", "", "Only show one bucket: expired, expiring_90d, expiring_365d, supported")
	cmd.Flags().StringVar(&client, "client", "", "Only show devices whose client name contains this substring")
	cmd.Flags().IntVar(&within, "within", 0, "Only show devices whose support ends within this many days")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum rows to return (0 = all)")
	cmd.Flags().StringVar(&asOfStr, "as-of", "", "Evaluate dates against this date instead of today (YYYY-MM-DD)")
	return cmd
}

func filterEol(rows []eolRow, keep func(eolRow) bool) []eolRow {
	out := make([]eolRow, 0, len(rows))
	for _, r := range rows {
		if keep(r) {
			out = append(out, r)
		}
	}
	return out
}

// buildEolReport is the pure core: devices joined to lifecycle + warranty,
// bucketed against asOf. Split out so it is table-testable without SQLite.
//
// FIELD SOURCING (this is where the first cut went wrong):
//   - v1 deviceLifecycleAttributes carries STATUS ENUMS ONLY
//     (lastSupportStatus, salesAvailability, softwareMaintenanceStatus,
//     securitySoftwareMaintenanceStatus) -- no dates at all.
//   - The end-of-support / end-of-sale DATES live on the v2 resource
//     deviceLifecycleAttributesV2 (lastSupportDate, salesAvailabilityDate).
//   - Warranty carries warrantyExpirationDate.
//
// A device with only v1 lifecycle synced can still be bucketed `expired` from
// its status enum, but it will have no date -- which is reported honestly
// rather than being dressed up as a date.
func buildEolReport(devices, lifecyclesV1, lifecyclesV2, warranties []auvikRow, tenants map[string]string, asOf time.Time) eolReport {
	type facts struct {
		lastSupportDate string
		salesAvailDate  string
		warrantyDate    string
		lastSupportStat string
		salesAvailStat  string
		swMaintStat     string
		secSwMaintStat  string
		warrantyStat    string
	}
	byDevice := map[string]*facts{}
	get := func(id string) *facts {
		f, ok := byDevice[id]
		if !ok {
			f = &facts{}
			byDevice[id] = f
		}
		return f
	}

	for _, l := range lifecyclesV1 {
		f := get(l.ID)
		f.lastSupportStat = l.attrString(fLifecycleLastSupportStatus.Field)
		f.salesAvailStat = l.attrString(fLifecycleSalesAvail.Field)
		f.swMaintStat = l.attrString(fLifecycleSwMaint.Field)
		f.secSwMaintStat = l.attrString(fLifecycleSecSwMaint.Field)
	}
	for _, l := range lifecyclesV2 {
		f := get(l.ID)
		f.lastSupportDate = l.attrString(fLifecycleV2LastSupportDate.Field)
		f.salesAvailDate = l.attrString(fLifecycleV2SalesAvailDate.Field)
	}
	for _, w := range warranties {
		f := get(w.ID)
		f.warrantyDate = w.attrString(fWarrantyExpiration.Field)
		f.warrantyStat = firstNonEmpty(
			w.attrString(fWarrantyCoverageStatus.Field),
			w.attrString(fServiceCoverageStatus.Field),
		)
	}

	report := eolReport{
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		AsOf:         asOf.Format("2006-01-02"),
		Buckets:      map[string]int{bucketExpired: 0, bucket90: 0, bucket365: 0, bucketSupported: 0},
		DevicesTotal: len(devices),
		Rows:         make([]eolRow, 0, len(devices)),
	}

	for _, dev := range devices {
		f := byDevice[dev.ID]
		if f == nil {
			f = &facts{}
		}
		earliest, label, hasDate := earliestDate(map[string]string{
			"last_support_date":       f.lastSupportDate,
			"sales_availability_date": f.salesAvailDate,
			"warranty_expiration":     f.warrantyDate,
		})

		row := eolRow{
			Client:     tenantLabel(tenants, dev.rel("tenant")),
			DeviceID:   dev.ID,
			DeviceName: firstNonEmpty(dev.attrString(fDeviceName.Field), dev.ID),
			DeviceType: dev.attrString(fDeviceType.Field),
			MakeModel:  dev.attrString(fDeviceMakeModel.Field),

			LastSupportDate:       f.lastSupportDate,
			SalesAvailabilityDate: f.salesAvailDate,
			WarrantyExpiration:    f.warrantyDate,

			LastSupportStatus:           f.lastSupportStat,
			SalesAvailability:           f.salesAvailStat,
			SoftwareMaintenance:         f.swMaintStat,
			SecuritySoftwareMaintenance: f.secSwMaintStat,
			WarrantyStatus:              f.warrantyStat,
		}

		// Status enums are a second, independent signal. A device Auvik already
		// calls "expired" is expired even when no date was synced.
		statusExpired := statusMeansExpired(f.lastSupportStat) ||
			statusMeansExpired(f.swMaintStat) ||
			statusMeansExpired(f.warrantyStat)
		securityOnly := statusMeansSecurityOnly(f.lastSupportStat) ||
			statusMeansSecurityOnly(f.swMaintStat)

		switch {
		case hasDate:
			report.DevicesDated++
			days := int(math.Floor(earliest.Sub(asOf).Hours() / 24))
			row.DaysRemaining = &days
			row.Source = label
			switch {
			case days < 0:
				row.Bucket = bucketExpired
			case days <= 90:
				row.Bucket = bucket90
			case days <= 365:
				row.Bucket = bucket365
			default:
				row.Bucket = bucketSupported
			}
			// A date in the future cannot override Auvik's own "expired" verdict.
			if statusExpired && row.Bucket != bucketExpired {
				row.Bucket = bucketExpired
				row.Source = label + " (status: expired)"
			}
		case statusExpired:
			row.Bucket = bucketExpired
			row.Source = "status only (no date synced)"
		case securityOnly:
			row.Bucket = bucket90
			row.Source = "status securityOnly (no date synced)"
		default:
			row.Bucket = bucketSupported
			if f.lastSupportStat == "" && f.swMaintStat == "" && f.warrantyStat == "" {
				row.Source = "no lifecycle or warranty data"
			} else {
				row.Source = "status covered/available (no date synced)"
			}
		}
		report.Buckets[row.Bucket]++
		report.Rows = append(report.Rows, row)
	}

	sort.SliceStable(report.Rows, func(i, j int) bool {
		a, b := report.Rows[i], report.Rows[j]
		if eolBucketOrder[a.Bucket] != eolBucketOrder[b.Bucket] {
			return eolBucketOrder[a.Bucket] < eolBucketOrder[b.Bucket]
		}
		switch {
		case a.DaysRemaining != nil && b.DaysRemaining != nil && *a.DaysRemaining != *b.DaysRemaining:
			return *a.DaysRemaining < *b.DaysRemaining
		case a.DaysRemaining != nil && b.DaysRemaining == nil:
			return true
		case a.DaysRemaining == nil && b.DaysRemaining != nil:
			return false
		}
		if a.Client != b.Client {
			return a.Client < b.Client
		}
		return a.DeviceName < b.DeviceName
	})
	return report
}

// earliestDate returns the soonest parseable date and which field it came from.
func earliestDate(candidates map[string]string) (time.Time, string, bool) {
	var best time.Time
	var label string
	found := false
	// Deterministic iteration so ties resolve the same way every run.
	keys := make([]string, 0, len(candidates))
	for k := range candidates {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t, ok := parseAuvikTime(candidates[k])
		if !ok {
			continue
		}
		if !found || t.Before(best) {
			best, label, found = t, k, true
		}
	}
	return best, label, found
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
