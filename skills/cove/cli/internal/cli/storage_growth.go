// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built novel feature: used-storage drift from local snapshots.
// pp:data-source local
package cli

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"cove-pp-cli/internal/coverpc"
	"cove-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

type growthItem struct {
	AccountID  int64   `json:"account_id"`
	DeviceName string  `json:"device_name"`
	Customer   string  `json:"customer"`
	FromBytes  int64   `json:"from_bytes"`
	ToBytes    int64   `json:"to_bytes"`
	DeltaBytes int64   `json:"delta_bytes"`
	DeltaPct   float64 `json:"delta_pct,omitempty"`
}

type customerGrowth struct {
	Customer   string `json:"customer"`
	DeltaBytes int64  `json:"delta_bytes"`
}

type storageGrowthView struct {
	BaselineSnapshot int64            `json:"baseline_snapshot"`
	BaselineTakenAt  string           `json:"baseline_taken_at"`
	LatestSnapshot   int64            `json:"latest_snapshot"`
	LatestTakenAt    string           `json:"latest_taken_at"`
	Items            []growthItem     `json:"items"`
	ByCustomer       []customerGrowth `json:"by_customer"`
	// Compared is true only when two comparable snapshots were diffed. When
	// it is false no delta exists, so total_delta_bytes is absent, not 0.
	Compared        bool   `json:"compared"`
	TotalDeltaBytes *int64 `json:"total_delta_bytes,omitempty"`
	// Partial is true when either snapshot is not a known-complete scan.
	Partial bool `json:"partial"`
	// UnmeasuredDevices did not report used storage in one of the snapshots.
	UnmeasuredDevices int    `json:"unmeasured_devices,omitempty"`
	Note              string `json:"note,omitempty"`
}

func newNovelStorageGrowthCmd(flags *rootFlags) *cobra.Command {
	var since string
	var top int
	var dbPath string
	cmd := &cobra.Command{
		Use:   "growth",
		Short: "Used-storage growth per device and customer, from local snapshots",
		Long: strings.Trim(`
Diffs used storage (column I14) between two local snapshots of the same
partner tree and endpoint: the newest one and the newest one taken at or
before --since ago (default: the previous snapshot). The vendor console keeps
zero history; this trend exists only because 'cove-cli snapshot' persists
timestamped fleet state in SQLite. A device whose used storage was not
reported, or is not an exact non-negative integer, in either snapshot is left
out (unmeasured_devices), never read as zero, and partial is true when either
snapshot stopped at its scan cap. A snapshot with no recorded endpoint (taken
before endpoint tracking) is never compared: compared is false and no delta
is printed.

Use this command for NUMERIC used-storage deltas over time. Do NOT use it
for backup-status regressions; use 'devices changes' instead.
`, "\n"),
		Example: strings.Trim(`
  cove-cli storage growth --since 7d --json
  cove-cli storage growth --top 10 --csv`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":          "true",
			"pp:data-source":         "local",
			"pp:no-error-path-probe": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return coveDryRun(cmd, flags, "would diff used storage between local snapshots")
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			window, err := parseCoveSince(since)
			if err != nil {
				return err
			}
			if top <= 0 {
				top = 25
			}
			if dbPath == "" {
				dbPath = defaultDBPath("cove-cli")
			}
			db, err := store.Open(dbPath)
			if err != nil {
				return fmt.Errorf("opening local store: %w", err)
			}
			defer db.Close()
			ctx := cmd.Context()
			latest, ok, err := db.LatestCoveSnapshot(ctx)
			if err != nil {
				return err
			}
			if !ok {
				return flags.printJSON(cmd, storageGrowthView{Items: []growthItem{}, ByCustomer: []customerGrowth{}, Note: "no snapshots yet; " + snapshotPairNote})
			}
			baseline, refusal, err := snapshotBaseline(ctx, db, latest, window)
			if err != nil {
				return err
			}
			if refusal != "" {
				return flags.printJSON(cmd, storageGrowthView{
					LatestSnapshot: latest.ID,
					LatestTakenAt:  latest.TakenAt.Format(time.RFC3339),
					Items:          []growthItem{},
					ByCustomer:     []customerGrowth{},
					Note:           refusal,
				})
			}
			baseRows, err := db.CoveDeviceStats(ctx, baseline.ID)
			if err != nil {
				return err
			}
			newRows, err := db.CoveDeviceStats(ctx, latest.ID)
			if err != nil {
				return err
			}
			items := make([]growthItem, 0)
			perCustomer := map[string]int64{}
			var totalDelta int64
			unmeasured := 0
			for id, nr := range newRows {
				br, existed := baseRows[id]
				if !existed {
					continue // new devices have no baseline to trend against
				}
				// A storage column the server did not report is stored as 0;
				// reading it as a real zero would invent a growth or a shrink.
				from, fromOK := snapshotCount(br, coverpc.ColUsedStorage)
				to, toOK := snapshotCount(nr, coverpc.ColUsedStorage)
				if !fromOK || !toOK {
					unmeasured++
					continue
				}
				br.UsedStorage, nr.UsedStorage = from, to
				delta := nr.UsedStorage - br.UsedStorage
				if delta == 0 {
					continue
				}
				item := growthItem{
					AccountID:  id,
					DeviceName: nr.DeviceName,
					Customer:   nr.Customer,
					FromBytes:  br.UsedStorage,
					ToBytes:    nr.UsedStorage,
					DeltaBytes: delta,
				}
				if br.UsedStorage > 0 {
					item.DeltaPct = math.Trunc(float64(delta)/float64(br.UsedStorage)*1000) / 10
				}
				items = append(items, item)
				customerDelta := perCustomer[nr.Customer]
				if !addExact(&customerDelta, delta) || !addExact(&totalDelta, delta) {
					return errTotalOverflow("the storage delta total", id)
				}
				perCustomer[nr.Customer] = customerDelta
			}
			sort.Slice(items, func(i, j int) bool { return items[i].DeltaBytes > items[j].DeltaBytes })
			if len(items) > top {
				items = items[:top]
			}
			byCustomer := make([]customerGrowth, 0, len(perCustomer))
			for _, k := range sortedKeys(perCustomer) {
				byCustomer = append(byCustomer, customerGrowth{Customer: k, DeltaBytes: perCustomer[k]})
			}
			sort.Slice(byCustomer, func(i, j int) bool { return byCustomer[i].DeltaBytes > byCustomer[j].DeltaBytes })
			view := storageGrowthView{
				BaselineSnapshot: baseline.ID,
				BaselineTakenAt:  baseline.TakenAt.Format(time.RFC3339),
				LatestSnapshot:   latest.ID,
				LatestTakenAt:    latest.TakenAt.Format(time.RFC3339),
				Items:            items,
				ByCustomer:       byCustomer,
				Compared:         true,
				TotalDeltaBytes:  &totalDelta,
			}
			view.Partial = !latest.KnownComplete() || !baseline.KnownComplete()
			view.UnmeasuredDevices = unmeasured
			var notes []string
			if len(items) == 0 {
				if unmeasured > 0 {
					notes = append(notes, "no storage deltas among the devices measured in both snapshots")
				} else {
					notes = append(notes, "no storage deltas between the compared snapshots")
				}
			}
			if unmeasured > 0 {
				notes = append(notes, fmt.Sprintf("%d devices did not report used storage in one of the snapshots and are left out of every total", unmeasured))
			}
			if view.Partial {
				notes = append(notes, partialSnapshotNote)
			}
			view.Note = strings.Join(notes, "; ")
			return emitCoveJSON(cmd, flags, view, view.Items)
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "Compare against the newest snapshot at least this old (e.g. 7d); default: the previous snapshot")
	cmd.Flags().IntVar(&top, "top", 25, "Maximum device rows to return (per-customer rollup is always complete)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: the CLI's local store)")
	return cmd
}
