// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"
	"ninjaone-pp-cli/internal/store"
)

// driftRow is one org's change on a metric vs the previous snapshot.
type driftRow struct {
	OrgID     string  `json:"orgId"`
	Org       string  `json:"org"`
	Previous  float64 `json:"previous"`
	Current   float64 `json:"current"`
	Delta     float64 `json:"delta"`
	Direction string  `json:"direction"` // better | worse | flat | new
}

const driftSnapshotDDL = `CREATE TABLE IF NOT EXISTS pp_drift_snapshots (
	metric    TEXT NOT NULL,
	org_id    TEXT NOT NULL,
	value     REAL NOT NULL,
	taken_at  TEXT NOT NULL
)`

// pp:data-source local
func newNovelDriftCmd(flags *rootFlags) *cobra.Command {
	var metric string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "drift",
		Short: "Week-over-week change per org vs the previous snapshot (patch|backup|stale)",
		Long: `Computes a per-org metric now and compares it against the most recent stored
snapshot, then records the current values as a new snapshot. This is the only
week-over-week answer — NinjaOne and competing tools keep no history. Run it on
a cadence (e.g. after each weekly sync) to build the trend.

Metrics:
  patch   compliance percent per org (higher is better)
  backup  coverage percent per org   (higher is better)
  stale   stale device count per org (lower is better)

Reads and writes the local store; run 'ninjaone-cli sync' first.`,
		Example: `  # Patch-compliance drift since the last snapshot
  ninjaone-cli drift --metric patch --agent

  # Backup-coverage drift
  ninjaone-cli drift --metric backup`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			switch metric {
			case "patch", "backup", "stale":
			default:
				return fmt.Errorf("invalid --metric %q: use patch, backup, or stale", metric)
			}
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openNovelStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, "") {
				hintIfStale(cmd, db, "", flags.maxAge)
			}

			if _, err := db.DB().Exec(driftSnapshotDDL); err != nil {
				return fmt.Errorf("preparing snapshot table: %w", err)
			}

			devices, err := buildDeviceIndex(db)
			if err != nil {
				return fmt.Errorf("loading devices: %w", err)
			}
			orgNames, err := buildOrgNames(db)
			if err != nil {
				return fmt.Errorf("loading organizations: %w", err)
			}
			current, err := computeDriftMetric(db, devices, metric)
			if err != nil {
				return err
			}

			// Load the most recent prior snapshot for this metric.
			prev, _, err := loadLatestSnapshot(db, metric)
			if err != nil {
				return fmt.Errorf("loading prior snapshot: %w", err)
			}

			// stale counts devices: lower is better. patch/backup are
			// percentages: higher is better.
			rows := buildDriftRows(current, prev, orgNames, metric == "stale")

			// Persist the current values as a new snapshot.
			takenAt := time.Now().UTC().Format(time.RFC3339Nano)
			if err := saveSnapshot(db, metric, current, takenAt); err != nil {
				return fmt.Errorf("saving snapshot: %w", err)
			}

			baseline := len(prev) == 0
			if wantsStructured(flags) {
				return flags.printJSON(cmd, rows)
			}
			if note := emptyStoreNote(devices); note != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), note)
			}
			if baseline {
				fmt.Fprintf(cmd.OutOrStdout(), "No prior snapshot for %q — baseline of %d org(s) saved. Re-run after the next sync to see drift.\n", metric, len(current))
			}
			if len(rows) == 0 {
				return nil
			}
			tableRows := make([][]string, 0, len(rows))
			for _, r := range rows {
				tableRows = append(tableRows, []string{
					r.Org,
					fmt.Sprintf("%.1f", r.Previous),
					fmt.Sprintf("%.1f", r.Current),
					fmt.Sprintf("%+.1f", r.Delta),
					r.Direction,
				})
			}
			return flags.printTable(cmd, []string{"ORG", "PREVIOUS", "CURRENT", "DELTA", "DIRECTION"}, tableRows)
		},
	}
	cmd.Flags().StringVar(&metric, "metric", "patch", "Metric to track: patch, backup, or stale")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: local data store)")
	return cmd
}

// loadLatestSnapshot returns the per-org values from the most recent snapshot
// of a metric, plus that snapshot's timestamp. Empty map when none exists.
func loadLatestSnapshot(db *store.Store, metric string) (map[string]float64, string, error) {
	var takenAt string
	row := db.DB().QueryRow(`SELECT taken_at FROM pp_drift_snapshots WHERE metric = ? ORDER BY taken_at DESC LIMIT 1`, metric)
	if err := row.Scan(&takenAt); err != nil {
		// No prior snapshot.
		return map[string]float64{}, "", nil
	}
	rows, err := db.DB().Query(`SELECT org_id, value FROM pp_drift_snapshots WHERE metric = ? AND taken_at = ?`, metric, takenAt)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	out := map[string]float64{}
	for rows.Next() {
		var orgID string
		var val float64
		if err := rows.Scan(&orgID, &val); err != nil {
			continue
		}
		out[orgID] = val
	}
	return out, takenAt, rows.Err()
}

// saveSnapshot writes the current per-org values as a new snapshot batch.
// The batch is one transaction: a partial snapshot sharing taken_at would be
// read back by loadLatestSnapshot as a complete (skewed) baseline.
func saveSnapshot(db *store.Store, metric string, values map[string]float64, takenAt string) error {
	tx, err := db.DB().Begin()
	if err != nil {
		return err
	}
	for orgID, v := range values {
		if _, err := tx.Exec(
			`INSERT INTO pp_drift_snapshots (metric, org_id, value, taken_at) VALUES (?, ?, ?, ?)`,
			metric, orgID, v, takenAt,
		); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// computeDriftMetric returns the current per-org value for a metric.
func computeDriftMetric(db *store.Store, devices map[string]nvDevice, metric string) (map[string]float64, error) {
	orgNames := map[string]string{} // not needed for raw values
	switch metric {
	case "patch":
		stats, err := computePatchStats(db, devices)
		if err != nil {
			return nil, err
		}
		vals := map[string]float64{}
		for orgID, s := range stats {
			vals[orgID] = s.compliancePct()
		}
		return vals, nil
	case "backup":
		gaps, _, err := computeBackupCoverage(db, devices, orgNames)
		if err != nil {
			return nil, err
		}
		devCount := map[string]int{}
		for _, d := range devices {
			devCount[d.OrgID]++
		}
		gapCount := map[string]int{}
		for _, g := range gaps {
			gapCount[g.OrgID]++
		}
		vals := map[string]float64{}
		for orgID, n := range devCount {
			if n == 0 {
				vals[orgID] = 100
				continue
			}
			vals[orgID] = roundPct(float64(n-gapCount[orgID]) / float64(n) * 100)
		}
		return vals, nil
	case "stale":
		stale := computeStaleDevices(devices, orgNames, 14, time.Now().UTC())
		vals := map[string]float64{}
		for _, d := range devices {
			if _, ok := vals[d.OrgID]; !ok {
				vals[d.OrgID] = 0
			}
		}
		for _, s := range stale {
			vals[s.OrgID]++
		}
		return vals, nil
	}
	return nil, fmt.Errorf("unknown metric %q", metric)
}

// buildDriftRows diffs current vs previous per-org values. Split out for tests.
// lowerIsBetter flips the better/worse labels for count-shaped metrics
// (stale device counts) where a positive delta is a regression.
func buildDriftRows(current, prev map[string]float64, orgNames map[string]string, lowerIsBetter bool) []driftRow {
	var rows []driftRow
	for orgID, cur := range current {
		p, had := prev[orgID]
		r := driftRow{OrgID: orgID, Org: orgLabel(orgNames, orgID), Previous: p, Current: cur}
		if !had {
			r.Direction = "new"
			r.Delta = 0
		} else {
			r.Delta = roundPct(cur - p)
			improved := r.Delta > 0
			if lowerIsBetter {
				improved = r.Delta < 0
			}
			switch {
			case r.Delta == 0:
				r.Direction = "flat"
			case improved:
				r.Direction = "better"
			default:
				r.Direction = "worse"
			}
		}
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Delta != rows[j].Delta {
			return rows[i].Delta < rows[j].Delta
		}
		return rows[i].Org < rows[j].Org
	})
	return rows
}
