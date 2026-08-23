// Copyright 2026 geekbrownbear and contributors. Licensed under Apache-2.0. See LICENSE.

// `msp fleet` — one ranked view across every tenant.
//
// pp:data-source local
//
// Every Avanan view, in the portal and in the API alike, is scoped to a single
// tenant. Answering "which of my sixty customers is over their seat count" or
// "who had a bad phishing week" means one set of calls per tenant against a
// rate-limited API, and still leaves the ranking to be done by hand. Joining
// the mirrored tenants, licenses, and detection volume locally turns that into
// one query.

package cli

import (
	"fmt"
	"sort"
	"strings"

	"avanan-pp-cli/internal/avananmirror"

	"github.com/spf13/cobra"
)

type fleetRow struct {
	Tenant         string  `json:"tenant"`
	TenantID       string  `json:"tenant_id,omitempty"`
	Domain         string  `json:"domain,omitempty"`
	Status         string  `json:"status,omitempty"`
	Package        string  `json:"package,omitempty"`
	SeatsLicensed  int     `json:"seats_licensed"`
	SeatsUsed      int     `json:"seats_used"`
	SeatOverage    int     `json:"seat_overage"`
	Utilization    float64 `json:"utilization_pct"`
	Detections     int     `json:"detections"`
	Unresolved     int     `json:"unresolved_detections"`
	Addons         int     `json:"addons"`
	DetectionsSeat float64 `json:"detections_per_seat"`
}

type fleetReport struct {
	Tenants         int        `json:"tenants"`
	TotalSeats      int        `json:"total_seats_licensed"`
	TotalUsed       int        `json:"total_seats_used"`
	TotalDetections int        `json:"total_detections"`
	OverSeatCount   int        `json:"tenants_over_seat_count"`
	Rows            []fleetRow `json:"rows"`
	Note            string     `json:"note,omitempty"`
}

func newNovelMspFleetCmd(flags *rootFlags) *cobra.Command {
	var (
		dbPath   string
		sortBy   string
		limit    int
		overOnly bool
	)

	cmd := &cobra.Command{
		Use:   "fleet",
		Short: "One ranked table across every tenant joining licensed seats, add-ons, usage, and detection volume",
		Long: strings.Trim(`
A cross-tenant rollup built from the local mirror.

Joins mirrored MSP tenants and licenses against mirrored detection volume, then
ranks by seat overage or detection rate. The live equivalent is several calls
per tenant against a rate-limited API, and produces no ranking at all.

Populate the mirror first:

  avanan-cli sync --resources msp
  avanan-cli mirror --since 7d

Detection counts reflect whatever window was mirrored, so a shorter mirror
window produces smaller counts rather than an error.

Use this command for a cross-tenant fleet rollup joining license, add-on,
usage, and detection volume. Do NOT use this command to read or modify a
single tenant's record; use 'msp describe-tenant' or 'msp create-tenants'
instead.
`, "\n"),
		Example: strings.Trim(`
  avanan-cli msp fleet
  avanan-cli msp fleet --agent
  avanan-cli msp fleet --sort detections --limit 10 --json
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "msp fleet")
			}
			if flags.dataSource == "live" {
				return usageErr(fmt.Errorf("msp fleet has no live equivalent: it joins mirrored tenants, licenses, and detections; run 'sync --resources msp' and 'mirror' then retry without --data-source live"))
			}
			switch sortBy {
			case "", "overage", "detections", "seats", "name":
			default:
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("unknown --sort %q; valid values are overage, detections, seats, name", sortBy))
			}

			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			empty := fleetReport{Rows: []fleetRow{}}
			db, _, ok, err := openMirror(cmd, ctx, flags, dbPath, empty)
			if err != nil || !ok {
				return err
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, "msp") {
				hintIfStale(cmd, db, "msp", flags.maxAge)
			}

			// Sequential drains, never nested: SQLite holds one connection.
			tenants, err := loadResources(ctx, db, "msp")
			if err != nil {
				return err
			}
			events, err := loadResources(ctx, db, avananmirror.ResourceEvents)
			if err != nil {
				return err
			}

			// Detection volume per scope/tenant, computed once.
			detections := map[string]int{}
			unresolved := map[string]int{}
			for _, ev := range events {
				if ev.Obj == nil {
					continue
				}
				key := strings.ToLower(str(ev.Obj, scopeKeys...))
				if key == "" {
					continue
				}
				// Index the scope under both its full {farm}:{tenant} form and
				// its bare tenant suffix. Tenant records commonly carry the
				// bare id, and matching only one side leaves every row showing
				// zero detections while the mirror is full.
				detections[key]++
				if isUnresolved(str(ev.Obj, stateKeys...)) {
					unresolved[key]++
				}
				if idx := strings.Index(key, ":"); idx >= 0 && idx+1 < len(key) {
					suffix := key[idx+1:]
					detections[suffix]++
					if isUnresolved(str(ev.Obj, stateKeys...)) {
						unresolved[suffix]++
					}
				}
			}

			report := empty
			for _, t := range tenants {
				if t.Obj == nil {
					continue
				}
				name := str(t.Obj, "name", "tenantName", "customerName", "displayName")
				id := str(t.Obj, "id", "tenantId", "customerId")
				if name == "" && id == "" {
					continue
				}
				row := fleetRow{
					Tenant:   orDefault(name, id),
					TenantID: id,
					Domain:   str(t.Obj, "domain", "primaryDomain"),
					Status:   str(t.Obj, "status", "tenantStatus"),
					Package:  str(t.Obj, "package", "licensePackage", "packageName"),
				}
				row.SeatsLicensed = intField(t.Obj, "licensedUsers", "seats", "licenseCount", "numberOfLicenses")
				row.SeatsUsed = intField(t.Obj, "protectedUsers", "usedSeats", "activeUsers", "usersCount")
				row.Addons = countAddons(t.Obj)

				if row.SeatsUsed > row.SeatsLicensed && row.SeatsLicensed > 0 {
					row.SeatOverage = row.SeatsUsed - row.SeatsLicensed
				}
				if row.SeatsLicensed > 0 {
					row.Utilization = round1(float64(row.SeatsUsed) / float64(row.SeatsLicensed) * 100)
				}

				for _, key := range tenantKeys(name, id, row.Domain) {
					if n, ok := detections[key]; ok {
						row.Detections = n
						row.Unresolved = unresolved[key]
						break
					}
				}
				if row.SeatsUsed > 0 {
					row.DetectionsSeat = round2(float64(row.Detections) / float64(row.SeatsUsed))
				}

				report.Rows = append(report.Rows, row)
			}

			// Filter BEFORE aggregating, so the headline numbers describe the
			// rows actually shown. Counting all tenants above a filtered table
			// reads as a reporting bug to anyone checking the arithmetic.
			if overOnly {
				kept := make([]fleetRow, 0, len(report.Rows))
				for _, r := range report.Rows {
					if r.SeatOverage > 0 {
						kept = append(kept, r)
					}
				}
				report.Rows = kept
			}
			report.OverSeatCount = 0
			for _, r := range report.Rows {
				report.TotalSeats += r.SeatsLicensed
				report.TotalUsed += r.SeatsUsed
				report.TotalDetections += r.Detections
				if r.SeatOverage > 0 {
					report.OverSeatCount++
				}
			}
			report.Tenants = len(report.Rows)

			sortFleet(report.Rows, sortBy)
			if limit > 0 && len(report.Rows) > limit {
				report.Rows = report.Rows[:limit]
			}

			if report.Tenants == 0 {
				report.Note = "no MSP tenants in the local mirror; run 'avanan-cli sync --resources msp' first"
			} else if report.TotalDetections == 0 {
				report.Note = "tenants are mirrored but no detections are; run 'avanan-cli mirror --since 7d' to populate detection volume"
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), report, flags)
			}

			if len(report.Rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), orDefault(report.Note, "No tenants matched."))
				return nil
			}

			fmt.Fprintf(cmd.OutOrStdout(), "%d tenants — %d seats licensed, %d used, %d detections, %d over seat count\n\n",
				report.Tenants, report.TotalSeats, report.TotalUsed, report.TotalDetections, report.OverSeatCount)
			fmt.Fprintf(cmd.OutOrStdout(), "%-28s %8s %6s %7s %8s %6s %s\n",
				"TENANT", "LICENSED", "USED", "OVER", "DETECT", "UNRES", "PACKAGE")
			for _, r := range report.Rows {
				fmt.Fprintf(cmd.OutOrStdout(), "%-28s %8d %6d %7d %8d %6d %s\n",
					truncateCell(r.Tenant, 28), r.SeatsLicensed, r.SeatsUsed, r.SeatOverage, r.Detections, r.Unresolved, r.Package)
			}
			if report.Note != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "\nNote: %s\n", report.Note)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&sortBy, "sort", "overage", "Ranking: overage, detections, seats, name")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum tenants to return (0 = all)")
	cmd.Flags().BoolVar(&overOnly, "over-seats-only", false, "Show only tenants above their licensed seat count")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

func sortFleet(rows []fleetRow, by string) {
	sort.SliceStable(rows, func(i, j int) bool {
		switch by {
		case "detections":
			if rows[i].Detections != rows[j].Detections {
				return rows[i].Detections > rows[j].Detections
			}
		case "seats":
			if rows[i].SeatsLicensed != rows[j].SeatsLicensed {
				return rows[i].SeatsLicensed > rows[j].SeatsLicensed
			}
		case "name":
			return strings.ToLower(rows[i].Tenant) < strings.ToLower(rows[j].Tenant)
		default: // overage
			if rows[i].SeatOverage != rows[j].SeatOverage {
				return rows[i].SeatOverage > rows[j].SeatOverage
			}
			if rows[i].Detections != rows[j].Detections {
				return rows[i].Detections > rows[j].Detections
			}
		}
		return strings.ToLower(rows[i].Tenant) < strings.ToLower(rows[j].Tenant)
	})
}

// tenantKeys returns the identifiers a detection's scope field might carry for
// a tenant. Avanan's scope strings are {farm}:{tenant}, but events reference
// tenants inconsistently across payloads.
func tenantKeys(name, id, domain string) []string {
	keys := make([]string, 0, 6)
	for _, v := range []string{id, name, domain} {
		v = strings.ToLower(strings.TrimSpace(v))
		if v == "" {
			continue
		}
		keys = append(keys, v)
		if idx := strings.Index(v, ":"); idx >= 0 && idx+1 < len(v) {
			keys = append(keys, v[idx+1:])
		}
	}
	return keys
}

func intField(obj map[string]any, keys ...string) int {
	for _, k := range keys {
		if v, ok := obj[k]; ok {
			switch n := v.(type) {
			case float64:
				return int(n)
			case string:
				var parsed int
				if _, err := fmt.Sscanf(n, "%d", &parsed); err == nil {
					return parsed
				}
			}
		}
	}
	return 0
}

func countAddons(obj map[string]any) int {
	for _, k := range []string{"addons", "addOns", "additionalPackages"} {
		if v, ok := obj[k]; ok {
			if list, ok := v.([]any); ok {
				return len(list)
			}
		}
	}
	return 0
}

func round1(f float64) float64 { return float64(int(f*10+0.5)) / 10 }
func round2(f float64) float64 { return float64(int(f*100+0.5)) / 100 }
