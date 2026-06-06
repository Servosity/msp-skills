// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel feature. Not generated.

package cli

import (
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"
	"n-central-pp-cli/internal/cliutil"
	"n-central-pp-cli/internal/store"
)

// maintGap is a device with no maintenance window covering the target date.
type maintGap struct {
	CustomerName string `json:"customerName"`
	SiteName     string `json:"siteName"`
	DeviceID     string `json:"deviceId"`
	LongName     string `json:"longName"`
}

// maintCustomerReport groups maintenance gaps by customer with a summary.
type maintCustomerReport struct {
	CustomerName string     `json:"customerName"`
	Total        int        `json:"total"`
	Covered      int        `json:"covered"`
	Uncovered    int        `json:"uncovered"`
	CoveragePct  float64    `json:"coveragePct"`
	Gaps         []maintGap `json:"gaps"`
}

func newNovelMaintCoverageCmd(flags *rootFlags) *cobra.Command {
	var flagBefore string
	var flagCustomer int
	var flagLimit int

	cmd := &cobra.Command{
		Use:   "coverage",
		Short: "List devices and sites with no maintenance window before a reboot/patch wave, so nothing reboots in business hours.",
		Long: `Find devices that have no maintenance window scheduled on or before a target
date. For each device in the local mirror, fetch its maintenance windows live
and check whether any window's start (or next-run) is on or before --before.
Devices with no covering window are reported, grouped by customer.

--before defaults to 7 days from now. Requires live API access.`,
		Example: `  n-central-cli maint coverage --before 2026-06-15
  n-central-cli maint coverage --customer 100 --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if flagLimit <= 0 {
				flagLimit = 200
			}

			before := time.Now().AddDate(0, 0, 7)
			if flagBefore != "" {
				parsed, ok := parseDateFlag(flagBefore)
				if !ok {
					return usageErr(fmt.Errorf("invalid --before %q: expected YYYY-MM-DD", flagBefore))
				}
				before = parsed
			}

			if cliutil.IsVerifyEnv() {
				return flags.printJSON(cmd, []maintCustomerReport{})
			}

			devices, err := maintScanDevices(cmd, flagCustomer, flagLimit)
			if err != nil {
				return err
			}
			if len(devices) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "no devices in the local store to audit; run 'n-central-cli sync' first")
				return flags.printJSON(cmd, []maintCustomerReport{})
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}

			byCust := map[string]*maintCustomerReport{}
			for _, d := range devices {
				rep := byCust[d.customerName]
				if rep == nil {
					rep = &maintCustomerReport{CustomerName: d.customerName}
					byCust[d.customerName] = rep
				}
				rep.Total++

				path := "/devices/" + d.deviceID + "/maintenance-windows"
				raw, gerr := c.Get(cmd.Context(), path, nil)
				if gerr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: maintenance-windows fetch failed for device %s: %v\n", d.deviceID, classifyAPIError(gerr, flags))
					rep.Uncovered++
					rep.Gaps = append(rep.Gaps, maintGap{CustomerName: d.customerName, SiteName: d.siteName, DeviceID: d.deviceID, LongName: d.longName})
					continue
				}
				if maintHasCoveringWindow(raw, before) {
					rep.Covered++
				} else {
					rep.Uncovered++
					rep.Gaps = append(rep.Gaps, maintGap{CustomerName: d.customerName, SiteName: d.siteName, DeviceID: d.deviceID, LongName: d.longName})
				}
			}

			out := make([]maintCustomerReport, 0, len(byCust))
			for _, r := range byCust {
				if r.Total > 0 {
					r.CoveragePct = float64(r.Covered) / float64(r.Total) * 100
				}
				out = append(out, *r)
			}
			sort.SliceStable(out, func(i, j int) bool {
				if out[i].Uncovered != out[j].Uncovered {
					return out[i].Uncovered > out[j].Uncovered
				}
				return out[i].CustomerName < out[j].CustomerName
			})

			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				tw := newTabWriter(cmd.OutOrStdout())
				fmt.Fprintln(tw, bold("CUSTOMER")+"\t"+bold("COVERED")+"\t"+bold("UNCOVERED")+"\t"+bold("COVERAGE"))
				for _, r := range out {
					name := r.CustomerName
					if name == "" {
						name = "(unknown)"
					}
					fmt.Fprintf(tw, "%s\t%d\t%d\t%.0f%%\n", name, r.Covered, r.Uncovered, r.CoveragePct)
				}
				return tw.Flush()
			}
			return flags.printJSON(cmd, out)
		},
	}
	cmd.Flags().StringVar(&flagBefore, "before", "", "Coverage cutoff date YYYY-MM-DD (default: 7 days from now)")
	cmd.Flags().IntVar(&flagCustomer, "customer", 0, "Scope to a single customer ID")
	cmd.Flags().IntVar(&flagLimit, "limit", 200, "Maximum devices to scan")
	return cmd
}

// maintScanDevice is a device row pulled from the local mirror.
type maintScanDevice struct {
	deviceID     string
	longName     string
	customerName string
	siteName     string
}

func maintScanDevices(cmd *cobra.Command, customerID, limit int) ([]maintScanDevice, error) {
	dbPath := defaultDBPath("n-central-cli")
	db, err := store.OpenWithContext(cmd.Context(), dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening local database: %w\nRun 'n-central-cli sync' first.", err)
	}
	defer db.Close()

	query := `SELECT device_id, long_name, customer_name, site_name FROM devices WHERE device_id IS NOT NULL`
	var args []any
	if customerID > 0 {
		query += ` AND customer_id = ?`
		args = append(args, customerID)
	}
	query += ` ORDER BY customer_name, device_id LIMIT ?`
	args = append(args, limit)

	rows, err := db.DB().QueryContext(cmd.Context(), query, args...)
	if err != nil {
		return nil, fmt.Errorf("reading devices: %w", err)
	}
	defer rows.Close()

	var out []maintScanDevice
	for rows.Next() {
		var deviceID sql.NullInt64
		var longName, customerName, siteName sql.NullString
		if err := rows.Scan(&deviceID, &longName, &customerName, &siteName); err != nil {
			return nil, fmt.Errorf("scanning device: %w", err)
		}
		if !deviceID.Valid {
			continue
		}
		out = append(out, maintScanDevice{
			deviceID:     fmt.Sprintf("%d", deviceID.Int64),
			longName:     longName.String,
			customerName: customerName.String,
			siteName:     siteName.String,
		})
	}
	return out, rows.Err()
}

// maintHasCoveringWindow reports whether a maintenance-windows response
// contains at least one window whose start/next-run is on or before `before`.
// Tolerant: if no window date can be parsed but at least one window exists,
// presence is treated as coverage (better to under-report gaps than to flag a
// device whose window simply uses an unfamiliar date field).
func maintHasCoveringWindow(raw []byte, before time.Time) bool {
	windows := unwrapData(raw)
	if len(windows) == 0 {
		return false
	}
	anyParsed := false
	for _, w := range windows {
		obj := decodeObj(w)
		if obj == nil {
			continue
		}
		for _, key := range []string{"startTime", "start", "startDate", "nextRun", "nextRunTime", "next_run", "scheduledTime", "windowStart"} {
			if v := firstField(obj, key); v != nil {
				if t, ok := parseFlexibleTime(v); ok {
					anyParsed = true
					if !t.After(before) {
						return true
					}
				}
			}
		}
	}
	// No parseable date on any window — treat presence as coverage.
	return !anyParsed
}
