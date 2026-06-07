// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built novel feature: snapshot-to-snapshot status drift.
// pp:data-source local
package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"cove-pp-cli/internal/coverpc"
	"cove-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

type statusChangeItem struct {
	AccountID  int64  `json:"account_id"`
	DeviceName string `json:"device_name"`
	Customer   string `json:"customer"`
	FromStatus int64  `json:"from_status"`
	FromName   string `json:"from_name"`
	ToStatus   int64  `json:"to_status"`
	ToName     string `json:"to_name"`
	Kind       string `json:"kind"` // regression | recovery | change
}

type devicesChangesView struct {
	BaselineSnapshot int64              `json:"baseline_snapshot"`
	BaselineTakenAt  string             `json:"baseline_taken_at"`
	LatestSnapshot   int64              `json:"latest_snapshot"`
	LatestTakenAt    string             `json:"latest_taken_at"`
	Items            []statusChangeItem `json:"items"`
	Added            []string           `json:"added_devices,omitempty"`
	Removed          []string           `json:"removed_devices,omitempty"`
	Note             string             `json:"note,omitempty"`
}

func newNovelDevicesChangesCmd(flags *rootFlags) *cobra.Command {
	var since string
	var dbPath string
	cmd := &cobra.Command{
		Use:   "changes",
		Short: "Backup-status flips between local snapshots: regressions and recoveries",
		Long: strings.Trim(`
Compares the last-session status between two local snapshots — the newest
one and the newest one taken at or before --since ago (default: the previous
snapshot) — and surfaces every flip: healthy→failed regressions and
failed→healthy recoveries, plus devices added or removed.

Requires at least two snapshots: run 'cove-cli snapshot' periodically.

Use this command for backup-STATUS regressions/recoveries between snapshots.
Do NOT use it for storage-size trends; use 'storage growth' instead.
`, "\n"),
		Example: strings.Trim(`
  cove-cli devices changes --json
  cove-cli devices changes --since 7d --json`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":          "true",
			"pp:no-error-path-probe": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would diff backup status between the two latest local snapshots")
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			window, err := parseCoveSince(since)
			if err != nil {
				return err
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
				return flags.printJSON(cmd, devicesChangesView{Items: []statusChangeItem{}, Note: "no snapshots yet — " + snapshotPairNote})
			}
			var baseline store.CoveSnapshot
			var haveBase bool
			if window > 0 {
				baseline, haveBase, err = db.CoveSnapshotBefore(ctx, latest.TakenAt.Add(-window))
			} else {
				baseline, haveBase, err = db.CoveSnapshotBefore(ctx, latest.TakenAt.Add(-time.Second))
			}
			if err != nil {
				return err
			}
			if !haveBase || baseline.ID == latest.ID {
				return flags.printJSON(cmd, devicesChangesView{
					LatestSnapshot: latest.ID,
					LatestTakenAt:  latest.TakenAt.Format(time.RFC3339),
					Items:          []statusChangeItem{},
					Note:           "only one qualifying snapshot — " + snapshotPairNote,
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
			view := devicesChangesView{
				BaselineSnapshot: baseline.ID,
				BaselineTakenAt:  baseline.TakenAt.Format(time.RFC3339),
				LatestSnapshot:   latest.ID,
				LatestTakenAt:    latest.TakenAt.Format(time.RFC3339),
				Items:            []statusChangeItem{},
			}
			for id, nr := range newRows {
				br, existed := baseRows[id]
				if !existed {
					view.Added = append(view.Added, nr.DeviceName)
					continue
				}
				if br.LastStatus == nr.LastStatus {
					continue
				}
				kind := "change"
				wasBad := coverpc.BadSessionStatuses[int(br.LastStatus)] || br.LastStatus == 7
				isBad := coverpc.BadSessionStatuses[int(nr.LastStatus)] || nr.LastStatus == 7
				switch {
				case !wasBad && isBad:
					kind = "regression"
				case wasBad && !isBad:
					kind = "recovery"
				}
				view.Items = append(view.Items, statusChangeItem{
					AccountID:  id,
					DeviceName: nr.DeviceName,
					Customer:   nr.Customer,
					FromStatus: br.LastStatus,
					FromName:   coverpc.StatusName(int(br.LastStatus)),
					ToStatus:   nr.LastStatus,
					ToName:     coverpc.StatusName(int(nr.LastStatus)),
					Kind:       kind,
				})
			}
			for id, br := range baseRows {
				if _, still := newRows[id]; !still {
					view.Removed = append(view.Removed, br.DeviceName)
				}
			}
			sort.Slice(view.Items, func(i, j int) bool {
				// Regressions first, then customer/device for stable output.
				rank := map[string]int{"regression": 0, "change": 1, "recovery": 2}
				if rank[view.Items[i].Kind] != rank[view.Items[j].Kind] {
					return rank[view.Items[i].Kind] < rank[view.Items[j].Kind]
				}
				if view.Items[i].Customer != view.Items[j].Customer {
					return view.Items[i].Customer < view.Items[j].Customer
				}
				return view.Items[i].DeviceName < view.Items[j].DeviceName
			})
			sort.Strings(view.Added)
			sort.Strings(view.Removed)
			if len(view.Items) == 0 {
				view.Note = "no status flips between the compared snapshots"
			}
			return emitCoveJSON(cmd, flags, view, view.Items)
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "Compare against the newest snapshot at least this old (e.g. 7d); default: the previous snapshot")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: the CLI's local store)")
	return cmd
}
