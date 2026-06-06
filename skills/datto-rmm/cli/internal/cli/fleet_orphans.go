// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/spf13/cobra"
)

type orphanView struct {
	Hostname  string `json:"hostname"`
	SiteName  string `json:"siteName"`
	Suspended bool   `json:"suspended"`
	Deleted   bool   `json:"deleted"`
	LastSeen  string `json:"lastSeen"`
	DaysStale int    `json:"daysStale"`
	Reason    string `json:"reason"`
}

// computeOrphans returns devices that are suspended/deleted OR stale (lastSeen
// parses and is at least staleDays old), AND have no alert in the last
// staleDays days. Sorted by DaysStale descending.
func computeOrphans(devices []fleetDevice, alerts []fleetAlert, staleDays int, now time.Time) []orphanView {
	cutoff := now.AddDate(0, 0, -staleDays)

	// Devices with a recent alert.
	recentlyAlerted := map[string]struct{}{}
	for _, a := range alerts {
		uid := a.AlertSourceInfo.DeviceUID
		if uid == "" {
			continue
		}
		if t, ok := parseDattoTime(a.Timestamp); ok {
			if t.Before(cutoff) {
				continue
			}
			recentlyAlerted[uid] = struct{}{}
		} else {
			// Unparseable timestamp: treat as recent (conservative: don't orphan it).
			recentlyAlerted[uid] = struct{}{}
		}
	}

	out := []orphanView{}
	for _, d := range devices {
		daysStale := -1
		isStale := false
		if t, ok := parseDattoTime(d.LastSeen); ok {
			daysStale = daysSince(t, now)
			if daysStale >= staleDays {
				isStale = true
			}
		}
		if !(d.Suspended || d.Deleted || isStale) {
			continue
		}
		if d.UID != "" {
			if _, ok := recentlyAlerted[d.UID]; ok {
				continue
			}
		}

		var reason string
		switch {
		case d.Deleted:
			reason = "deleted"
		case d.Suspended:
			reason = "suspended"
		default:
			reason = fmt.Sprintf("stale %dd, no recent alerts", daysStale)
		}

		lastSeen := ""
		if t, ok := parseDattoTime(d.LastSeen); ok {
			lastSeen = t.Format(time.RFC3339)
		}
		out = append(out, orphanView{
			Hostname:  d.Hostname,
			SiteName:  d.SiteName,
			Suspended: d.Suspended,
			Deleted:   d.Deleted,
			LastSeen:  lastSeen,
			DaysStale: daysStale,
			Reason:    reason,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].DaysStale > out[j].DaysStale })
	return out
}

// pp:data-source local
func newNovelFleetOrphansCmd(flags *rootFlags) *cobra.Command {
	var staleDays int
	var dbPath string

	cmd := &cobra.Command{
		Use:         "orphans",
		Short:       "Finds long-stale, suspended, or deleted devices with no recent alerts that may still count against a site",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example: `  datto-rmm-cli fleet orphans
  datto-rmm-cli fleet orphans --stale-days 120 --json`,
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

			ctx := cmd.Context()
			devices, err := loadFleetDevices(ctx, db)
			if err != nil {
				return err
			}
			alerts, err := loadFleetAlerts(ctx, db)
			if err != nil {
				return err
			}
			view := computeOrphans(devices, alerts, staleDays, time.Now().UTC())

			if shouldPrintJSON(cmd, flags) {
				return flags.printJSON(cmd, view)
			}
			headers := []string{"HOSTNAME", "SITE", "SUSPENDED", "DELETED", "LAST SEEN", "DAYS STALE", "REASON"}
			rows := make([][]string, 0, len(view))
			for _, v := range view {
				rows = append(rows, []string{
					v.Hostname, v.SiteName,
					strconv.FormatBool(v.Suspended), strconv.FormatBool(v.Deleted),
					v.LastSeen, strconv.Itoa(v.DaysStale), v.Reason,
				})
			}
			return flags.printTable(cmd, headers, rows)
		},
	}
	cmd.Flags().IntVar(&staleDays, "stale-days", 90, "Days without check-in (and without alerts) to consider an orphan")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
