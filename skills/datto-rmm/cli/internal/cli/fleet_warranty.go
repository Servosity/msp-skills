// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"sort"
	"strconv"
	"time"

	"github.com/spf13/cobra"
)

type warrantyView struct {
	Hostname     string `json:"hostname"`
	SiteName     string `json:"siteName"`
	WarrantyDate string `json:"warrantyDate"`
	DaysLeft     int    `json:"daysLeft"`
	DeviceType   string `json:"deviceType"`
}

// computeWarranty returns devices whose warrantyDate parses and expires within
// `within` days from now (including already-expired). DaysLeft can be negative.
// Sorted by DaysLeft ascending (most-overdue first).
func computeWarranty(devices []fleetDevice, within int, now time.Time) []warrantyView {
	out := []warrantyView{}
	limit := now.AddDate(0, 0, within)
	for _, d := range devices {
		exp, ok := parseWarranty(d.WarrantyDate)
		if !ok {
			continue
		}
		if exp.After(limit) {
			continue
		}
		daysLeft := int(exp.Sub(now).Hours() / 24)
		out = append(out, warrantyView{
			Hostname:     d.Hostname,
			SiteName:     d.SiteName,
			WarrantyDate: exp.Format("2006-01-02"),
			DaysLeft:     daysLeft,
			DeviceType:   d.DeviceType.Type,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].DaysLeft < out[j].DaysLeft })
	return out
}

// pp:data-source local
func newNovelFleetWarrantyCmd(flags *rootFlags) *cobra.Command {
	var within int
	var dbPath string

	cmd := &cobra.Command{
		Use:         "warranty",
		Short:       "Surfaces every device whose hardware warranty expires within a window, sorted by days remaining",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example: `  datto-rmm-cli fleet warranty --within 60
  datto-rmm-cli fleet warranty --within 30 --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openFleetStore(cmd, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, fleetDevicesResource) {
				hintIfStale(cmd, db, fleetDevicesResource, flags.maxAge)
			}

			devices, err := loadFleetDevices(cmd.Context(), db)
			if err != nil {
				return err
			}
			view := computeWarranty(devices, within, time.Now().UTC())

			if shouldPrintJSON(cmd, flags) {
				return flags.printJSON(cmd, view)
			}
			headers := []string{"HOSTNAME", "SITE", "WARRANTY", "DAYS LEFT", "TYPE"}
			rows := make([][]string, 0, len(view))
			for _, v := range view {
				rows = append(rows, []string{v.Hostname, v.SiteName, v.WarrantyDate, strconv.Itoa(v.DaysLeft), v.DeviceType})
			}
			return flags.printTable(cmd, headers, rows)
		},
	}
	cmd.Flags().IntVar(&within, "within", 60, "Flag warranties expiring within this many days (includes expired)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
