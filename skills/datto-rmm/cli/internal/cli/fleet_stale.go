// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"sort"
	"strconv"
	"time"

	"github.com/spf13/cobra"
)

type staleView struct {
	Hostname  string `json:"hostname"`
	SiteName  string `json:"siteName"`
	LastSeen  string `json:"lastSeen"`
	DaysStale int    `json:"daysStale"`
	Online    bool   `json:"online"`
}

// computeStale returns devices whose lastSeen parses, are at least `days` days
// stale, and are not deleted. Sorted by DaysStale descending.
func computeStale(devices []fleetDevice, days int, now time.Time) []staleView {
	out := []staleView{}
	for _, d := range devices {
		if d.Deleted {
			continue
		}
		t, ok := parseDattoTime(d.LastSeen)
		if !ok {
			continue
		}
		ds := daysSince(t, now)
		if ds < days {
			continue
		}
		out = append(out, staleView{
			Hostname:  d.Hostname,
			SiteName:  d.SiteName,
			LastSeen:  t.Format(time.RFC3339),
			DaysStale: ds,
			Online:    d.Online,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].DaysStale > out[j].DaysStale })
	return out
}

// pp:data-source local
func newNovelFleetStaleCmd(flags *rootFlags) *cobra.Command {
	var days int
	var dbPath string

	cmd := &cobra.Command{
		Use:         "stale",
		Short:       "Lists every device across all customer sites that hasn't checked in within a chosen window",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example: `  datto-rmm-cli fleet stale --days 30
  datto-rmm-cli fleet stale --days 14 --json`,
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
			view := computeStale(devices, days, time.Now().UTC())

			if shouldPrintJSON(cmd, flags) {
				return flags.printJSON(cmd, view)
			}
			headers := []string{"HOSTNAME", "SITE", "LAST SEEN", "DAYS STALE", "ONLINE"}
			rows := make([][]string, 0, len(view))
			for _, v := range view {
				rows = append(rows, []string{v.Hostname, v.SiteName, v.LastSeen, strconv.Itoa(v.DaysStale), strconv.FormatBool(v.Online)})
			}
			return flags.printTable(cmd, headers, rows)
		},
	}
	cmd.Flags().IntVar(&days, "days", 30, "Minimum days since last check-in to flag")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
