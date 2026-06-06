// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored novel feature: quarter-over-quarter fleet diff. Compares two
// labeled snapshots (or a snapshot against the current store) to show devices
// added/removed and posture movement — the "what changed since the baseline"
// answer no Datto RMM API call provides.
package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type fleetDiffDevice struct {
	UID      string `json:"uid"`
	Hostname string `json:"hostname"`
	SiteName string `json:"siteName"`
}

type fleetPostureTotals struct {
	Devices         int `json:"devices"`
	Online          int `json:"online"`
	AVNotRunning    int `json:"avNotRunning"`
	PatchesMissing  int `json:"patchesMissing"`
	WarrantyExpired int `json:"warrantyExpired"`
}

type fleetDiffView struct {
	From           string             `json:"from"`
	To             string             `json:"to"`
	DevicesAdded   []fleetDiffDevice  `json:"devicesAdded"`
	DevicesRemoved []fleetDiffDevice  `json:"devicesRemoved"`
	FromTotals     fleetPostureTotals `json:"fromTotals"`
	ToTotals       fleetPostureTotals `json:"toTotals"`
}

// computePostureTotals rolls a device set into the posture counters used on
// both sides of the diff. Deleted devices are excluded.
func computePostureTotals(devices []fleetDevice, now time.Time) fleetPostureTotals {
	t := fleetPostureTotals{}
	for _, d := range devices {
		if d.Deleted {
			continue
		}
		t.Devices++
		if d.Online {
			t.Online++
		}
		if !avIsHealthy(d.Antivirus.AntivirusStatus) {
			t.AVNotRunning++
		}
		t.PatchesMissing += d.PatchManagement.PatchesApprovedPending
		if w, ok := parseWarranty(d.WarrantyDate); ok && w.Before(now) {
			t.WarrantyExpired++
		}
	}
	return t
}

// computeFleetDiff returns the device membership delta between two device
// sets plus posture totals for each side. Deleted devices are excluded from
// membership on both sides.
func computeFleetDiff(from, to []fleetDevice, now time.Time) ([]fleetDiffDevice, []fleetDiffDevice, fleetPostureTotals, fleetPostureTotals) {
	fromByUID := map[string]fleetDevice{}
	for _, d := range from {
		if !d.Deleted && d.UID != "" {
			fromByUID[d.UID] = d
		}
	}
	toByUID := map[string]fleetDevice{}
	for _, d := range to {
		if !d.Deleted && d.UID != "" {
			toByUID[d.UID] = d
		}
	}
	added := []fleetDiffDevice{}
	for uid, d := range toByUID {
		if _, ok := fromByUID[uid]; !ok {
			added = append(added, fleetDiffDevice{UID: uid, Hostname: d.Hostname, SiteName: d.SiteName})
		}
	}
	removed := []fleetDiffDevice{}
	for uid, d := range fromByUID {
		if _, ok := toByUID[uid]; !ok {
			removed = append(removed, fleetDiffDevice{UID: uid, Hostname: d.Hostname, SiteName: d.SiteName})
		}
	}
	sort.SliceStable(added, func(i, j int) bool { return added[i].Hostname < added[j].Hostname })
	sort.SliceStable(removed, func(i, j int) bool { return removed[i].Hostname < removed[j].Hostname })
	return added, removed, computePostureTotals(from, now), computePostureTotals(to, now)
}

func decodeFleetDevices(raws []json.RawMessage) []fleetDevice {
	out := make([]fleetDevice, 0, len(raws))
	for _, raw := range raws {
		var d fleetDevice
		if err := json.Unmarshal(raw, &d); err != nil {
			continue
		}
		out = append(out, d)
	}
	return out
}

// pp:data-source local
func newNovelFleetDiffCmd(flags *rootFlags) *cobra.Command {
	var fromLabel string
	var toLabel string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Show what changed between two fleet snapshots: devices in/out and posture deltas",
		Long: strings.TrimSpace(`
Use this command to show what changed between two fleet snapshots (devices
in/out, posture deltas). Do NOT use it to freeze a baseline; use
'fleet snapshot' instead. Do NOT use it for a single current-state card; use
'fleet scorecard' instead.

--to defaults to 'current', the live local store as of the last sync.`),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "--from=q1-acme",
		},
		Example: `  datto-rmm-cli fleet diff --from q1-acme --to q2-acme
  datto-rmm-cli fleet diff --from q1-acme --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would diff two fleet snapshots for device membership and posture deltas")
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if toLabel == "" {
				toLabel = "current"
			}
			if fromLabel == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--from <label> is required (take baselines with 'fleet snapshot')"))
			}
			db, err := openFleetStore(cmd, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			ctx := cmd.Context()
			loadSide := func(label string) ([]fleetDevice, error) {
				if label == "current" {
					if !hintIfUnsynced(cmd, db, fleetDevicesResource) {
						hintIfStale(cmd, db, fleetDevicesResource, flags.maxAge)
					}
					return loadFleetDevices(ctx, db)
				}
				ok, err := db.SnapshotExists(ctx, label)
				if err != nil {
					return nil, err
				}
				if !ok {
					return nil, fmt.Errorf("snapshot %q not found (see 'fleet snapshot --list')", label)
				}
				raws, err := db.SnapshotResourceData(ctx, label, fleetDevicesResource)
				if err != nil {
					return nil, err
				}
				return decodeFleetDevices(raws), nil
			}

			fromDevices, err := loadSide(fromLabel)
			if err != nil {
				return err
			}
			toDevices, err := loadSide(toLabel)
			if err != nil {
				return err
			}

			added, removed, fromTotals, toTotals := computeFleetDiff(fromDevices, toDevices, time.Now().UTC())
			view := fleetDiffView{
				From:           fromLabel,
				To:             toLabel,
				DevicesAdded:   added,
				DevicesRemoved: removed,
				FromTotals:     fromTotals,
				ToTotals:       toTotals,
			}
			if shouldPrintJSON(cmd, flags) {
				return flags.printJSON(cmd, view)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "fleet diff %s -> %s\n\n", fromLabel, toLabel)
			headers := []string{"METRIC", strings.ToUpper(fromLabel), strings.ToUpper(toLabel), "DELTA"}
			rows := [][]string{
				{"devices", strconv.Itoa(fromTotals.Devices), strconv.Itoa(toTotals.Devices), strconv.Itoa(toTotals.Devices - fromTotals.Devices)},
				{"online", strconv.Itoa(fromTotals.Online), strconv.Itoa(toTotals.Online), strconv.Itoa(toTotals.Online - fromTotals.Online)},
				{"av not running", strconv.Itoa(fromTotals.AVNotRunning), strconv.Itoa(toTotals.AVNotRunning), strconv.Itoa(toTotals.AVNotRunning - fromTotals.AVNotRunning)},
				{"patches missing", strconv.Itoa(fromTotals.PatchesMissing), strconv.Itoa(toTotals.PatchesMissing), strconv.Itoa(toTotals.PatchesMissing - fromTotals.PatchesMissing)},
				{"warranty expired", strconv.Itoa(fromTotals.WarrantyExpired), strconv.Itoa(toTotals.WarrantyExpired), strconv.Itoa(toTotals.WarrantyExpired - fromTotals.WarrantyExpired)},
			}
			if err := flags.printTable(cmd, headers, rows); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n%d devices added, %d removed\n", len(added), len(removed))
			return nil
		},
	}
	cmd.Flags().StringVar(&fromLabel, "from", "", "Baseline snapshot label")
	cmd.Flags().StringVar(&toLabel, "to", "current", "Comparison snapshot label, or 'current' for the live local store")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
