// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"comet-backup-pp-cli/internal/comet"
	"comet-backup-pp-cli/internal/store"
)

// ponytail: one row per snapshot holding the whole credential-free projection
// as JSON; diffs run in memory. Split into per-entity tables if fleets grow
// past what a single JSON document per snapshot handles comfortably.
const snapshotTableDDL = `CREATE TABLE IF NOT EXISTS comet_snapshots (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	taken_at INTEGER NOT NULL,
	server_version TEXT NOT NULL DEFAULT '',
	data TEXT NOT NULL
)`

type snapshotSummary struct {
	SnapshotID    int64  `json:"snapshot_id"`
	TakenAt       string `json:"taken_at"`
	ServerVersion string `json:"server_version"`
	Tenants       int    `json:"tenants"`
	Devices       int    `json:"devices"`
	Items         int    `json:"protected_items"`
	Problems      int    `json:"problems"`
	Kept          int    `json:"snapshots_kept"`
	DB            string `json:"db"`
}

func newNovelSnapshotCmd(flags *rootFlags) *cobra.Command {
	var flagKeep int
	var flagDB, flagGrace, flagUnscheduled string

	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Capture the current fleet into a local SQLite store so changes can compare runs",
		Long: `Capture the current fleet into a local SQLite store so changes can compare runs.

Use this command to capture the current fleet into the local store so 'changes' can diff it later; run it on a schedule. Do NOT use this command to read the fleet's current backup state; use 'health' instead.

A snapshot stores, per tenant, device, protected item and Storage Vault, only the fields the reports use: no password hashes, TOTP keys, recovery codes or Storage Vault credentials. --keep prunes older snapshots.`,
		Example: strings.Trim(`
  comet-backup-cli snapshot
  comet-backup-cli snapshot --keep 90 --json`, "\n"),
		// mcp:local-write, not read-only: it writes and prunes (--keep) the
		// CLI's own snapshot store. --db is a blocked MCP destination flag.
		Annotations: map[string]string{"mcp:local-write": "true", "pp:data-source": "live"},
		RunE: guardRunE(flags, func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "snapshot")
			}
			if flagKeep < 1 {
				return usageErr(fmt.Errorf("--keep must be at least 1"))
			}
			grace, err := durationFlag("grace", flagGrace)
			if err != nil {
				return err
			}
			unscheduled, err := durationFlag("unscheduled-interval", flagUnscheduled)
			if err != nil {
				return err
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			version, err := serverVersion(ctx, c)
			if err != nil {
				return fleetError(cmd, flags, c, err)
			}
			f, err := loadFleet(ctx, c)
			if err != nil {
				return fleetError(cmd, flags, c, err)
			}
			now := time.Now()
			snap := comet.Snapshot{
				TakenAt: now.Unix(), ServerVersion: version,
				Items:   comet.Health(f.profiles, f.live, comet.HealthOptions{Now: now, Grace: grace, MaxAge: unscheduled}),
				Devices: comet.Devices(f.profiles, f.live, now),
				Tenants: comet.Usage(f.profiles, false),
				Vaults:  comet.Usage(f.profiles, true),
			}
			dbPath := flagDB
			if dbPath == "" {
				dbPath = defaultDBPath(cometDBName)
			}
			id, kept, err := saveSnapshot(ctx, dbPath, snap, flagKeep)
			if err != nil {
				return err
			}
			sum := snapshotSummary{SnapshotID: id, TakenAt: fmtTime(snap.TakenAt), ServerVersion: version, Tenants: len(snap.Tenants),
				Devices: len(snap.Devices), Items: len(snap.Items), Kept: kept, DB: dbPath}
			for _, it := range snap.Items {
				if comet.IsProblem(it.State) {
					sum.Problems++
				}
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), sum, flags)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Snapshot %d: %d tenants, %d devices, %d protected items (%d need attention). %d snapshots kept in %s\n",
				sum.SnapshotID, sum.Tenants, sum.Devices, sum.Items, sum.Problems, sum.Kept, dbPath)
			return nil
		}),
	}
	cmd.Flags().IntVar(&flagKeep, "keep", 90, "Number of snapshots to keep; older ones are deleted")
	cmd.Flags().StringVar(&flagDB, "db", "", "SQLite database path (default: the CLI's local data directory)")
	cmd.Flags().StringVar(&flagGrace, "grace", fleetDefaultGrace, "Overdue slack recorded with each item's health state (same as health --grace)")
	cmd.Flags().StringVar(&flagUnscheduled, "unscheduled-interval", fleetDefaultMaxAge, "Expected interval for unscheduled items (same as health --unscheduled-interval)")
	return cmd
}

// saveSnapshot writes one snapshot row and prunes to keep rows in a single
// transaction, then records the sync checkpoint outside it (SaveSyncState
// opens its own write).
func saveSnapshot(ctx context.Context, dbPath string, snap comet.Snapshot, keep int) (int64, int, error) {
	data, err := json.Marshal(snap)
	if err != nil {
		return 0, 0, err
	}
	db, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		return 0, 0, fmt.Errorf("opening local store %s: %w", dbPath, err)
	}
	defer db.Close()
	tx, err := db.DB().BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit
	if _, err := tx.ExecContext(ctx, snapshotTableDDL); err != nil {
		return 0, 0, fmt.Errorf("creating snapshot table: %w", err)
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO comet_snapshots (taken_at, server_version, data) VALUES (?, ?, ?)`, snap.TakenAt, snap.ServerVersion, string(data))
	if err != nil {
		return 0, 0, fmt.Errorf("saving snapshot: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, 0, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM comet_snapshots WHERE id NOT IN (SELECT id FROM comet_snapshots ORDER BY id DESC LIMIT ?)`, keep); err != nil {
		return 0, 0, fmt.Errorf("pruning snapshots: %w", err)
	}
	var kept int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM comet_snapshots`).Scan(&kept); err != nil {
		return 0, 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("committing snapshot: %w", err)
	}
	if err := db.SaveSyncState("snapshots", fmt.Sprint(id), len(snap.Items)); err != nil {
		return 0, 0, fmt.Errorf("recording snapshot time: %w", err)
	}
	return id, kept, nil
}

type storedSnapshot struct {
	ID   int64
	Snap comet.Snapshot
}

// loadSnapshots returns the newest snapshot plus the comparison base: the
// newest snapshot taken at or before cutoff, or the one before the newest
// when cutoff is zero. Rows are drained before decoding.
func loadSnapshots(ctx context.Context, db *sql.DB, cutoff int64) (*storedSnapshot, *storedSnapshot, bool, error) {
	var exists int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='comet_snapshots'`).Scan(&exists); err != nil || exists == 0 {
		return nil, nil, false, err
	}
	load := func(query string, args ...any) (*storedSnapshot, error) {
		var id int64
		var data string
		err := db.QueryRowContext(ctx, query, args...).Scan(&id, &data)
		if err == sql.ErrNoRows {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		var s comet.Snapshot
		if err := json.Unmarshal([]byte(data), &s); err != nil {
			return nil, fmt.Errorf("decoding snapshot %d: %w", id, err)
		}
		return &storedSnapshot{ID: id, Snap: s}, nil
	}
	latest, err := load(`SELECT id, data FROM comet_snapshots ORDER BY id DESC LIMIT 1`)
	if err != nil || latest == nil {
		return nil, nil, false, err
	}
	var base *storedSnapshot
	shortHistory := false
	if cutoff > 0 {
		base, err = load(`SELECT id, data FROM comet_snapshots WHERE taken_at <= ? AND id < ? ORDER BY id DESC LIMIT 1`, cutoff, latest.ID)
		if err == nil && base == nil {
			// Nothing is old enough: compare with the oldest snapshot, and say so.
			base, err = load(`SELECT id, data FROM comet_snapshots WHERE id < ? ORDER BY id ASC LIMIT 1`, latest.ID)
			shortHistory = base != nil
		}
	} else {
		base, err = load(`SELECT id, data FROM comet_snapshots WHERE id < ? ORDER BY id DESC LIMIT 1`, latest.ID)
	}
	return latest, base, shortHistory, err
}
