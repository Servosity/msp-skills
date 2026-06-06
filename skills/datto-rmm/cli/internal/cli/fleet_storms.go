// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"sort"
	"strconv"
	"time"

	"github.com/spf13/cobra"
)

type stormView struct {
	DeviceName string `json:"deviceName"`
	SiteName   string `json:"siteName"`
	AlertCount int    `json:"alertCount"`
	DeviceUID  string `json:"deviceUid"`
}

// computeStorms groups alerts by device and counts those within the last
// `days`. Alerts with an unparseable timestamp are always counted; the window
// filter only applies when the timestamp parses. Sorted by AlertCount desc,
// then takes the top N (top<=0 means all).
func computeStorms(alerts []fleetAlert, days, top int, now time.Time) []stormView {
	type agg struct {
		name  string
		site  string
		uid   string
		count int
	}
	groups := map[string]*agg{}
	cutoff := now.AddDate(0, 0, -days)

	for _, a := range alerts {
		if t, ok := parseDattoTime(a.Timestamp); ok {
			if t.Before(cutoff) {
				continue
			}
		}
		key := a.AlertSourceInfo.DeviceUID
		if key == "" {
			key = a.AlertSourceInfo.DeviceName
		}
		if key == "" {
			key = "(unknown)"
		}
		g := groups[key]
		if g == nil {
			g = &agg{
				name: a.AlertSourceInfo.DeviceName,
				site: a.AlertSourceInfo.SiteName,
				uid:  a.AlertSourceInfo.DeviceUID,
			}
			groups[key] = g
		}
		g.count++
	}

	out := make([]stormView, 0, len(groups))
	for _, g := range groups {
		out = append(out, stormView{
			DeviceName: g.name,
			SiteName:   g.site,
			AlertCount: g.count,
			DeviceUID:  g.uid,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].AlertCount != out[j].AlertCount {
			return out[i].AlertCount > out[j].AlertCount
		}
		return out[i].DeviceName < out[j].DeviceName
	})
	if top > 0 && len(out) > top {
		out = out[:top]
	}
	return out
}

// pp:data-source local
func newNovelFleetStormsCmd(flags *rootFlags) *cobra.Command {
	var days int
	var top int
	var dbPath string

	cmd := &cobra.Command{
		Use:         "storms",
		Short:       "Ranks the noisiest devices by alert volume over a recent window so the NOC can tune monitors",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example: `  datto-rmm-cli fleet storms --days 7 --top 20
  datto-rmm-cli fleet storms --json`,
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

			if !hintIfUnsynced(cmd, db, fleetAlertsOpenResource) {
				hintIfStale(cmd, db, fleetAlertsOpenResource, flags.maxAge)
			}

			alerts, err := loadFleetAlerts(cmd.Context(), db)
			if err != nil {
				return err
			}
			view := computeStorms(alerts, days, top, time.Now().UTC())

			if shouldPrintJSON(cmd, flags) {
				return flags.printJSON(cmd, view)
			}
			headers := []string{"DEVICE", "SITE", "ALERTS", "DEVICE UID"}
			rows := make([][]string, 0, len(view))
			for _, v := range view {
				rows = append(rows, []string{v.DeviceName, v.SiteName, strconv.Itoa(v.AlertCount), v.DeviceUID})
			}
			return flags.printTable(cmd, headers, rows)
		},
	}
	cmd.Flags().IntVar(&days, "days", 7, "Window in days to count alerts within")
	cmd.Flags().IntVar(&top, "top", 20, "Show only the top N noisiest devices (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
