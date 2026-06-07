// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built transcendence feature for levelio-cli. Reads the local store.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"levelio-pp-cli/internal/store"
)

type rebootDueDevice struct {
	Hostname           string  `json:"hostname"`
	DeviceID           string  `json:"device_id"`
	Online             bool    `json:"online"`
	MaintenanceMode    bool    `json:"maintenance_mode"`
	UpdatesSinceReboot int     `json:"updates_since_reboot"`
	OldestInstall      string  `json:"oldest_install"`
	WaitingDays        float64 `json:"waiting_days"`
	LastReboot         string  `json:"last_reboot"`
}

type rebootDueResult struct {
	MinDays        int               `json:"min_days"`
	DevicesChecked int               `json:"devices_checked"`
	Skipped        int               `json:"skipped_no_reboot_time"`
	Devices        []rebootDueDevice `json:"devices"`
}

// lvlComputeRebootDue correlates installed updates against each device's
// last_reboot_time: an update installed AFTER the last reboot means the device
// has not rebooted since the install — the reboot-debt list. Devices without a
// parseable last_reboot_time are counted as skipped, not guessed.
func lvlComputeRebootDue(devices []lvlDevice, updates []lvlUpdate, minDays int, now time.Time) rebootDueResult {
	res := rebootDueResult{MinDays: minDays}

	installedByDevice := map[string][]time.Time{}
	for _, u := range updates {
		t, ok := lvlParseTime(u.InstalledOn)
		if !ok {
			continue
		}
		installedByDevice[u.DeviceID] = append(installedByDevice[u.DeviceID], t)
	}

	for _, d := range devices {
		res.DevicesChecked++
		lastReboot, ok := lvlParseTime(d.LastRebootTime)
		if !ok {
			res.Skipped++
			continue
		}
		var since []time.Time
		for _, t := range installedByDevice[d.ID] {
			if t.After(lastReboot) {
				since = append(since, t)
			}
		}
		if len(since) == 0 {
			continue
		}
		oldest := since[0]
		for _, t := range since[1:] {
			if t.Before(oldest) {
				oldest = t
			}
		}
		waiting := round1(now.Sub(oldest).Hours() / 24.0)
		if waiting < float64(minDays) {
			continue
		}
		res.Devices = append(res.Devices, rebootDueDevice{
			Hostname: lvlDeviceLabel(d), DeviceID: d.ID,
			Online: d.Online, MaintenanceMode: d.MaintenanceMode,
			UpdatesSinceReboot: len(since),
			OldestInstall:      oldest.Format(time.RFC3339),
			WaitingDays:        waiting,
			LastReboot:         lastReboot.Format(time.RFC3339),
		})
	}

	sort.SliceStable(res.Devices, func(i, j int) bool {
		if res.Devices[i].WaitingDays != res.Devices[j].WaitingDays {
			return res.Devices[i].WaitingDays > res.Devices[j].WaitingDays
		}
		return res.Devices[i].Hostname < res.Devices[j].Hostname
	})
	return res
}

// pp:data-source local
func newNovelRebootDueCmd(flags *rootFlags) *cobra.Command {
	var minDays int
	var dbPath string

	cmd := &cobra.Command{
		Use:         "reboot-due",
		Short:       "Devices with updates installed since their last reboot — the reboot-debt list",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `List devices whose installed OS updates landed AFTER the device's last
reboot — patches that are on disk but not yet effective until the machine
restarts. For each device: how many installs are waiting, the oldest install
date, and how many days it has been waiting. Computed offline by correlating
the synced updates table with each device's last_reboot_time. Devices without
a reboot timestamp are reported as skipped, never guessed. Use --days to only
flag devices waiting at least N days.

Use this command to list devices waiting on a REBOOT to finalize installed
patches. Do NOT use it for fleet patch-EXPOSURE aggregation (pending vs
installed by category); use 'patch-posture' instead.

Run 'levelio-cli sync' first to populate the local store.`,
		Example: strings.Trim(`
  # Every device carrying reboot debt
  levelio-cli reboot-due

  # Only devices waiting 3+ days, JSON for agents
  levelio-cli reboot-due --days 3 --agent
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
			if !hintIfUnsynced(cmd, db, "updates") {
				hintIfStale(cmd, db, "updates", flags.maxAge)
			}

			devices, err := lvlDevices(db)
			if err != nil {
				return fmt.Errorf("loading devices: %w", err)
			}
			updates, err := lvlUpdates(db)
			if err != nil {
				return fmt.Errorf("loading updates: %w", err)
			}
			res := lvlComputeRebootDue(devices, updates, minDays, time.Now().UTC())

			if flags.asJSON {
				return printJSONFiltered(out, res, flags)
			}
			fmt.Fprintf(out, "%d device(s) carrying reboot debt (of %d checked, %d without reboot timestamp)\n",
				len(res.Devices), res.DevicesChecked, res.Skipped)
			if len(res.Devices) == 0 {
				return nil
			}
			fmt.Fprintln(out, "WAITING_DAYS\tINSTALLS\tONLINE\tDEVICE")
			for _, d := range res.Devices {
				fmt.Fprintf(out, "%.1f\t%d\t%t\t%s\n", d.WaitingDays, d.UpdatesSinceReboot, d.Online, d.Hostname)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&minDays, "days", 0, "Only flag devices whose oldest waiting install is at least N days old")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
