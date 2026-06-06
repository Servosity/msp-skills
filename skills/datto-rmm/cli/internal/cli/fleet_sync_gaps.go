// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored novel feature: sync completeness check. Every fleet analytic
// is only as trustworthy as the sync that fed it. This command audits each
// synced resource — stored rows vs the count recorded at sync time, plus sync
// age — and cross-checks the device table against the per-site device counts
// the API itself reported (sites carry devicesStatus.numberOfDevices).
package cli

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"datto-rmm-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

type syncGapRow struct {
	Resource      string `json:"resource"`
	StoredRows    int    `json:"storedRows"`
	RecordedCount int    `json:"recordedCount"`
	Drift         int    `json:"drift"`
	LastSyncedAt  string `json:"lastSyncedAt,omitempty"`
	Status        string `json:"status"`
}

type deviceCrossCheck struct {
	ExpectedFromSites int `json:"expectedFromSites"`
	SyncedDevices     int `json:"syncedDevices"`
	Gap               int `json:"gap"`
}

type syncGapsView struct {
	Resources   []syncGapRow     `json:"resources"`
	DeviceCheck deviceCrossCheck `json:"deviceCrossCheck"`
	Gaps        int              `json:"gaps"`
	Note        string           `json:"note,omitempty"`
}

type syncStateRow struct {
	lastSyncedAt string
	totalCount   int
}

// computeSyncGaps merges stored row counts with sync_state records into the
// per-resource audit rows. A resource is a gap when it was never synced or
// when stored rows drifted from the count recorded at sync time.
func computeSyncGaps(rowCounts map[string]int, states map[string]syncStateRow) []syncGapRow {
	names := map[string]struct{}{}
	for n := range rowCounts {
		names[n] = struct{}{}
	}
	for n := range states {
		names[n] = struct{}{}
	}
	out := make([]syncGapRow, 0, len(names))
	for n := range names {
		row := syncGapRow{Resource: n, StoredRows: rowCounts[n]}
		st, synced := states[n]
		if !synced {
			row.Status = "never-synced"
			out = append(out, row)
			continue
		}
		row.RecordedCount = st.totalCount
		row.LastSyncedAt = st.lastSyncedAt
		row.Drift = row.StoredRows - st.totalCount
		if row.Drift == 0 {
			row.Status = "ok"
		} else {
			row.Status = "drift"
		}
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Resource < out[j].Resource })
	return out
}

func loadResourceRowCounts(ctx context.Context, db *store.Store) (map[string]int, error) {
	rows, err := db.DB().QueryContext(ctx, `SELECT resource_type, COUNT(*) FROM resources GROUP BY resource_type`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var rt string
		var n int
		if err := rows.Scan(&rt, &n); err != nil {
			return nil, err
		}
		out[rt] = n
	}
	return out, rows.Err()
}

func loadSyncStates(ctx context.Context, db *store.Store) (map[string]syncStateRow, error) {
	rows, err := db.DB().QueryContext(ctx, `SELECT resource_type, COALESCE(last_synced_at, ''), COALESCE(total_count, 0) FROM sync_state`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]syncStateRow{}
	for rows.Next() {
		var rt, at string
		var n int
		if err := rows.Scan(&rt, &at, &n); err != nil {
			return nil, err
		}
		out[rt] = syncStateRow{lastSyncedAt: at, totalCount: n}
	}
	return out, rows.Err()
}

// pp:data-source local
func newNovelFleetSyncGapsCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "sync-gaps",
		Short: "Verify every synced resource is complete before trusting fleet numbers",
		Long: strings.TrimSpace(`
Use this command to verify a sync pulled every page (stored rows vs the count
recorded at sync time, plus a device cross-check against the per-site device
counts the API reported). Do NOT use it to refresh data; use 'sync' instead.
Do NOT use it to find stale data by age; use 'stale' instead.`),
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example: `  datto-rmm-cli fleet sync-gaps
  datto-rmm-cli fleet sync-gaps --json --select gaps,deviceCrossCheck`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would audit stored row counts against sync-time totals and per-site device counts")
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

			ctx := cmd.Context()
			rowCounts, err := loadResourceRowCounts(ctx, db)
			if err != nil {
				return err
			}
			states, err := loadSyncStates(ctx, db)
			if err != nil {
				return err
			}
			resources := computeSyncGaps(rowCounts, states)

			// Device cross-check: the API itself reports per-site device
			// counts on the sites payload; their sum is the expected fleet
			// size the synced device table should match.
			sites, err := loadFleetSites(ctx, db)
			if err != nil {
				return err
			}
			expected := 0
			for _, s := range sites {
				expected += s.DevicesStatus.NumberOfDevices
			}
			check := deviceCrossCheck{
				ExpectedFromSites: expected,
				SyncedDevices:     rowCounts[fleetDevicesResource],
			}
			check.Gap = check.ExpectedFromSites - check.SyncedDevices

			gaps := 0
			for _, r := range resources {
				if r.Status != "ok" {
					gaps++
				}
			}
			view := syncGapsView{Resources: resources, DeviceCheck: check, Gaps: gaps}
			if gaps > 0 || (expected > 0 && check.Gap != 0) {
				view.Note = "gaps detected: re-run 'sync' (or 'sync --full') before trusting fleet analytics"
			}
			if shouldPrintJSON(cmd, flags) {
				return flags.printJSON(cmd, view)
			}
			headers := []string{"RESOURCE", "ROWS", "RECORDED", "DRIFT", "LAST SYNCED", "STATUS"}
			rows := make([][]string, 0, len(resources))
			for _, r := range resources {
				rows = append(rows, []string{r.Resource, strconv.Itoa(r.StoredRows), strconv.Itoa(r.RecordedCount), strconv.Itoa(r.Drift), r.LastSyncedAt, r.Status})
			}
			if err := flags.printTable(cmd, headers, rows); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\ndevices: %d synced vs %d expected from site counts (gap %d)\n",
				check.SyncedDevices, check.ExpectedFromSites, check.Gap)
			if view.Note != "" {
				fmt.Fprintln(cmd.OutOrStdout(), view.Note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
