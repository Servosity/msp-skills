// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
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
	// Compared is true only when two comparable snapshots were diffed; when
	// it is false an empty items list means nothing was checked, not "no
	// flips".
	Compared bool `json:"compared"`
	// Partial is true when either snapshot is not a known-complete scan:
	// added/removed devices are then not reported.
	Partial bool `json:"partial"`
	// StatusUnreported counts devices present in both snapshots whose status
	// was not reported (or is not an exact integer) in one or both of them,
	// so no flip could be judged.
	StatusUnreported int    `json:"status_unreported,omitempty"`
	Note             string `json:"note,omitempty"`
}

func newNovelDevicesChangesCmd(flags *rootFlags) *cobra.Command {
	var since string
	var dbPath string
	cmd := &cobra.Command{
		Use:   "changes",
		Short: "Backup-status flips between local snapshots: regressions and recoveries",
		Long: strings.Trim(`
Compares the last-session status between two local snapshots of the same
partner tree and endpoint (the newest one and the newest one taken at or
before --since ago; default: the previous snapshot) and surfaces every flip:
regressions into an adverse or never-run status, recoveries (an adverse or
never-run status to Completed; nothing else counts as a recovery), other
changes, plus devices added or removed. Added and removed devices are only
reported when both snapshots are complete scans. A device whose status is
missing in either snapshot, or in both, is counted in status_unreported. A
snapshot with no recorded endpoint (taken before endpoint tracking) is never
compared: compared is false and no flips are reported.

Requires at least two snapshots: run 'cove-cli snapshot' periodically.

Use this command for backup-STATUS regressions/recoveries between snapshots.
Do NOT use it for storage-size trends; use 'storage growth' instead.
`, "\n"),
		Example: strings.Trim(`
  cove-cli devices changes --json
  cove-cli devices changes --since 7d --json`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":          "true",
			"pp:data-source":         "local",
			"pp:no-error-path-probe": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return coveDryRun(cmd, flags, "would diff backup status between the two latest local snapshots")
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
				return flags.printJSON(cmd, devicesChangesView{Items: []statusChangeItem{}, Note: "no snapshots yet; " + snapshotPairNote})
			}
			baseline, refusal, err := snapshotBaseline(ctx, db, latest, window)
			if err != nil {
				return err
			}
			if refusal != "" {
				return flags.printJSON(cmd, devicesChangesView{
					LatestSnapshot: latest.ID,
					LatestTakenAt:  latest.TakenAt.Format(time.RFC3339),
					Items:          []statusChangeItem{},
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
			view := devicesChangesView{
				BaselineSnapshot: baseline.ID,
				BaselineTakenAt:  baseline.TakenAt.Format(time.RFC3339),
				LatestSnapshot:   latest.ID,
				LatestTakenAt:    latest.TakenAt.Format(time.RFC3339),
				Items:            []statusChangeItem{},
				Compared:         true,
				Partial:          !latest.KnownComplete() || !baseline.KnownComplete(),
			}
			for id, nr := range newRows {
				br, existed := baseRows[id]
				if !existed {
					view.Added = append(view.Added, nr.DeviceName)
					continue
				}
				from, fromOK := snapshotSetting(br, coverpc.ColTotalLastStatus)
				to, toOK := snapshotSetting(nr, coverpc.ColTotalLastStatus)
				if !fromOK || !toOK {
					// Missing on either side, or on both: no flip can be
					// judged, and silence must not read as "no flips".
					view.StatusUnreported++
					continue
				}
				if from == to {
					continue
				}
				// One classification for every command: a recovery needs a
				// Completed session, not merely a status that is no longer bad.
				fromClass, toClass := coverpc.ClassifyStatus(from, true), coverpc.ClassifyStatus(to, true)
				kind := "change"
				switch {
				case !fromClass.Bad() && toClass.Bad():
					kind = "regression"
				case fromClass.Bad() && toClass == coverpc.StatusSucceeded:
					kind = "recovery"
				}
				view.Items = append(view.Items, statusChangeItem{
					AccountID:  id,
					DeviceName: nr.DeviceName,
					Customer:   nr.Customer,
					FromStatus: from,
					FromName:   coverpc.StatusName(int(from)),
					ToStatus:   to,
					ToName:     coverpc.StatusName(int(to)),
					Kind:       kind,
				})
			}
			for id, br := range baseRows {
				if _, still := newRows[id]; !still {
					view.Removed = append(view.Removed, br.DeviceName)
				}
			}
			if view.Partial {
				// A device missing from a partial scan was not removed, and
				// one beyond the baseline's cap was not added.
				view.Added, view.Removed = nil, nil
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
			var notes []string
			if len(view.Items) == 0 {
				if view.StatusUnreported > 0 {
					notes = append(notes, "no status flips among the devices that reported a status in both snapshots")
				} else {
					notes = append(notes, "no status flips between the compared snapshots")
				}
			}
			if view.StatusUnreported > 0 {
				notes = append(notes, fmt.Sprintf("%d devices did not report a status in one or both of the snapshots, so no flip was judged for them", view.StatusUnreported))
			}
			if view.Partial {
				notes = append(notes, partialSnapshotNote)
			}
			view.Note = strings.Join(notes, "; ")
			return emitCoveJSON(cmd, flags, view, view.Items)
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "Compare against the newest snapshot at least this old (e.g. 7d); default: the previous snapshot")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: the CLI's local store)")
	return cmd
}
