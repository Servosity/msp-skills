// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built transcendence feature for levelio-cli. Reads the local store.

package cli

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"levelio-pp-cli/internal/store"
)

type staleDevice struct {
	Hostname        string  `json:"hostname"`
	Nickname        string  `json:"nickname,omitempty"`
	DeviceID        string  `json:"device_id"`
	GroupID         string  `json:"group_id,omitempty"`
	LastSeenAt      string  `json:"last_seen_at,omitempty"`
	DaysDark        float64 `json:"days_dark"`
	HasLastSeen     bool    `json:"has_last_seen"`
	Online          bool    `json:"online"`
	MaintenanceMode bool    `json:"maintenance_mode"`
}

type staleResult struct {
	ThresholdDays     float64       `json:"threshold_days"`
	ExcludeMaintained bool          `json:"exclude_maintenance"`
	TotalDevices      int           `json:"total_devices"`
	Count             int           `json:"count"`
	Devices           []staleDevice `json:"devices"`
}

func round1(x float64) float64 { return math.Round(x*10) / 10 }

// lvlComputeStale returns devices that have not checked in for >= days, plus
// devices with no last_seen that are currently offline.
func lvlComputeStale(devices []lvlDevice, days float64, excludeMaint bool, now time.Time) staleResult {
	res := staleResult{ThresholdDays: days, ExcludeMaintained: excludeMaint, TotalDevices: len(devices)}
	for _, d := range devices {
		if excludeMaint && d.MaintenanceMode {
			continue
		}
		dd, ok := lvlDaysDark(d, now)
		switch {
		case ok && dd >= days:
			res.Devices = append(res.Devices, mkStale(d, round1(dd), true))
		case !ok && !d.Online:
			// Never reported a last_seen and is offline -> dark with unknown age.
			res.Devices = append(res.Devices, mkStale(d, -1, false))
		}
	}
	sort.SliceStable(res.Devices, func(i, j int) bool {
		return res.Devices[i].DaysDark > res.Devices[j].DaysDark
	})
	res.Count = len(res.Devices)
	return res
}

func mkStale(d lvlDevice, daysDark float64, hasLastSeen bool) staleDevice {
	return staleDevice{
		Hostname:        lvlDeviceLabel(d),
		Nickname:        d.Nickname,
		DeviceID:        d.ID,
		GroupID:         d.GroupID,
		LastSeenAt:      d.LastSeenAt,
		DaysDark:        daysDark,
		HasLastSeen:     hasLastSeen,
		Online:          d.Online,
		MaintenanceMode: d.MaintenanceMode,
	}
}

// pp:data-source local
func newNovelStaleCmd(flags *rootFlags) *cobra.Command {
	var days float64
	var excludeMaint bool
	var dbPath string

	cmd := &cobra.Command{
		Use:         "stale",
		Short:       "List devices that have gone dark (not seen in N days)",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `List devices that have not checked in for at least --days days, computed
offline from the locally synced store. Devices with no last-seen timestamp that
are currently offline are also reported (days_dark = -1, unknown age). Use
--exclude-maintenance to ignore machines intentionally in maintenance mode.

Run 'levelio-cli sync' first to populate the local store.`,
		Example: strings.Trim(`
  # Devices not seen in 7+ days
  levelio-cli stale --days 7

  # Ignore maintenance-mode machines, JSON for agents
  levelio-cli stale --days 14 --exclude-maintenance --agent
`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			out := cmd.OutOrStdout()
			if dbPath == "" {
				dbPath = defaultDBPath("levelio-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local store: %w\nRun 'levelio-cli sync' first.", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "devices") {
				hintIfStale(cmd, db, "devices", flags.maxAge)
			}

			devices, err := lvlDevices(db)
			if err != nil {
				return fmt.Errorf("loading devices: %w", err)
			}
			res := lvlComputeStale(devices, days, excludeMaint, time.Now().UTC())

			if flags.asJSON {
				return printJSONFiltered(out, res, flags)
			}
			fmt.Fprintf(out, "%d of %d device(s) dark for >= %.0f day(s)\n", res.Count, res.TotalDevices, days)
			if res.Count == 0 {
				return nil
			}
			fmt.Fprintln(out, "DAYS_DARK\tONLINE\tLAST_SEEN\tHOSTNAME")
			for _, d := range res.Devices {
				dd := fmt.Sprintf("%.1f", d.DaysDark)
				if !d.HasLastSeen {
					dd = "unknown"
				}
				fmt.Fprintf(out, "%s\t%t\t%s\t%s\n", dd, d.Online, d.LastSeenAt, d.Hostname)
			}
			return nil
		},
	}
	cmd.Flags().Float64Var(&days, "days", 7, "Mark devices not seen in at least this many days")
	cmd.Flags().BoolVar(&excludeMaint, "exclude-maintenance", false, "Skip devices in maintenance mode")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
