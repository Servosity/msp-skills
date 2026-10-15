// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"watchguard-pp-cli/internal/store"
)

const fireboxDevicesResource = "firebox_devices"

// flexString accepts a JSON string, bool or number: the spec types cloud_managed
// as "yes"/"no" but its own examples show true.
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch t := v.(type) {
	case nil:
		*f = ""
	case string:
		*f = flexString(t)
	case bool:
		*f = map[bool]flexString{true: "yes", false: "no"}[t]
	default:
		*f = flexString(fmt.Sprint(t))
	}
	return nil
}

type fleetDevice struct {
	ID           string     `json:"id"`
	Name         flexString `json:"name,omitempty"`
	Model        flexString `json:"short_model_name,omitempty"`
	Serial       flexString `json:"serial_number,omitempty"`
	Version      flexString `json:"version,omitempty"`
	State        flexString `json:"state,omitempty"`
	CloudManaged flexString `json:"cloud_managed,omitempty"`
	OfflineSince flexString `json:"offline_since,omitempty"`
	Uptime       flexString `json:"uptime,omitempty"`
}

// readFleet drains device rows (drain-first: no follow-up query while rows is
// open) and counts rows that are not a device object.
// querier is satisfied by *sql.DB and *sql.Tx.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func readFleet(ctx context.Context, db querier, query string, args ...any) (map[string]fleetDevice, int, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	out := map[string]fleetDevice{}
	skipped := 0
	for rows.Next() {
		var id, data string
		if err := rows.Scan(&id, &data); err != nil {
			_ = rows.Close()
			return nil, 0, err
		}
		var d fleetDevice
		if json.Unmarshal([]byte(data), &d) != nil {
			skipped++
			continue
		}
		if d.ID == "" {
			d.ID = id
		}
		out[d.ID] = d
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, 0, err
	}
	return out, skipped, rows.Close()
}

func warnSkipped(cmd *cobra.Command, n int) {
	if n > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d synced device row(s) could not be decoded and were skipped\n", n)
	}
}

const noMirrorNote = "no local mirror: run 'watchguard-cli sync --resources firebox_devices --full' first"

// openFleetStore returns (nil, false, nil) when the database file does not exist yet.
func openFleetStore(cmd *cobra.Command, flags *rootFlags, dbPath string, readOnly bool) (*store.Store, bool, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("watchguard-cli")
	}
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Fprintf(cmd.ErrOrStderr(), "hint: %s (database %s)\n", noMirrorNote, dbPath)
		return nil, false, nil
	}
	var db *store.Store
	var err error
	if readOnly {
		db, err = store.OpenReadOnlyContext(cmd.Context(), dbPath)
	} else {
		db, err = store.OpenWithContext(cmd.Context(), dbPath)
	}
	if err != nil {
		return nil, false, fmt.Errorf("opening database: %w", err)
	}
	if !hintIfUnsynced(cmd, db, fireboxDevicesResource) {
		hintIfStale(cmd, db, fireboxDevicesResource, flags.maxAge)
	}
	return db, true, nil
}

type fleetChange struct {
	DeviceID     string `json:"device_id"`
	Name         string `json:"name,omitempty"`
	Change       string `json:"change"`
	From         string `json:"from,omitempty"`
	To           string `json:"to,omitempty"`
	OfflineSince string `json:"offline_since,omitempty"`
}

// changeOrder puts the changes that need action first.
var changeOrder = map[string]int{"went_offline": 0, "removed": 1, "state_changed": 2, "firmware_changed": 3, "cloud_managed_changed": 4, "new": 5, "came_online": 6}

// diffFleet compares two snapshots keyed by device ID.
func diffFleet(prev, cur map[string]fleetDevice) []fleetChange {
	out := make([]fleetChange, 0)
	for _, id := range sortedMapKeys(cur) {
		c := cur[id]
		p, ok := prev[id]
		if !ok {
			out = append(out, fleetChange{DeviceID: id, Name: string(c.Name), Change: "new", To: string(c.State)})
			continue
		}
		if p.State != c.State {
			ch := fleetChange{DeviceID: id, Name: string(c.Name), Change: "state_changed", From: string(p.State), To: string(c.State), OfflineSince: string(c.OfflineSince)}
			switch {
			case c.State == "online":
				ch.Change, ch.OfflineSince = "came_online", string(p.OfflineSince)
			case c.State == "offline":
				ch.Change = "went_offline"
			}
			out = append(out, ch)
		}
		if p.Version != c.Version {
			out = append(out, fleetChange{DeviceID: id, Name: string(c.Name), Change: "firmware_changed", From: string(p.Version), To: string(c.Version)})
		}
		if p.CloudManaged != c.CloudManaged {
			out = append(out, fleetChange{DeviceID: id, Name: string(c.Name), Change: "cloud_managed_changed", From: string(p.CloudManaged), To: string(c.CloudManaged)})
		}
	}
	for _, id := range sortedMapKeys(prev) {
		if _, ok := cur[id]; !ok {
			out = append(out, fleetChange{DeviceID: id, Name: string(prev[id].Name), Change: "removed", From: string(prev[id].State)})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return changeOrder[out[i].Change] < changeOrder[out[j].Change] })
	return out
}

type changesView struct {
	ComparedTo      string        `json:"compared_to_snapshot,omitempty"`
	CurrentSnapshot string        `json:"current_snapshot,omitempty"`
	Devices         int           `json:"devices_scanned"`
	Changes         []fleetChange `json:"changes"`
	Note            string        `json:"note,omitempty"`
}

// Snapshots form a chain: each run names its parent, and parent is UNIQUE, so two
// concurrent invocations cannot both record a successor to the same snapshot.
var fleetSnapshotDDL = []string{
	`CREATE TABLE IF NOT EXISTS wg_fleet_runs (
		snapshot_at TEXT PRIMARY KEY,
		parent TEXT NOT NULL UNIQUE,
		fingerprint TEXT NOT NULL,
		devices INTEGER NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS wg_fleet_devices (
		snapshot_at TEXT NOT NULL,
		device_id TEXT NOT NULL,
		data JSON NOT NULL,
		PRIMARY KEY (snapshot_at, device_id)
	)`,
}

const keepFleetSnapshots = 30 // ponytail: fixed retention; make it a flag if anyone needs longer history

type snapshotRef struct {
	at, fingerprint string
}

// fleetFingerprint identifies a fleet state; uptime is left out so a re-sync of
// an unchanged fleet is not a new state.
func fleetFingerprint(fleet map[string]fleetDevice) string {
	h := sha256.New()
	for _, id := range sortedMapKeys(fleet) {
		d := fleet[id]
		d.Uptime = ""
		b, _ := json.Marshal(d)
		h.Write(b)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// recentSnapshots returns the two newest snapshots (newest first); a missing
// table means no history yet.
func recentSnapshots(ctx context.Context, db querier) ([]snapshotRef, error) {
	rows, err := db.QueryContext(ctx, `SELECT snapshot_at, fingerprint FROM wg_fleet_runs ORDER BY rowid DESC LIMIT 2`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	var out []snapshotRef
	for rows.Next() {
		var r snapshotRef
		if err := rows.Scan(&r.at, &r.fingerprint); err != nil {
			_ = rows.Close()
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	return out, rows.Close()
}

// recordSnapshot stores cur as the successor of parent; it returns false when a
// concurrent invocation already recorded that successor.
func recordSnapshot(ctx context.Context, db *sql.DB, parent, fp string, cur map[string]fleetDevice) (string, bool, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", false, err
	}
	res, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO wg_fleet_runs (snapshot_at, parent, fingerprint, devices) VALUES (?, ?, ?, ?)`, now, parent, fp, len(cur))
	if err != nil {
		_ = tx.Rollback()
		return "", false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		_ = tx.Rollback()
		return "", false, nil
	}
	// The insert holds the write lock: if another run recorded after we read the
	// head, this fleet view is stale and must not become the new head.
	var head string
	if err := tx.QueryRowContext(ctx, `SELECT snapshot_at FROM wg_fleet_runs WHERE snapshot_at != ? ORDER BY rowid DESC LIMIT 1`, now).Scan(&head); err != nil && err != sql.ErrNoRows {
		_ = tx.Rollback()
		return "", false, err
	}
	if head != parent {
		_ = tx.Rollback()
		return "", false, nil
	}
	for _, id := range sortedMapKeys(cur) {
		data, _ := json.Marshal(cur[id])
		if _, err := tx.ExecContext(ctx, `INSERT INTO wg_fleet_devices (snapshot_at, device_id, data) VALUES (?, ?, ?)`, now, id, string(data)); err != nil {
			_ = tx.Rollback()
			return "", false, err
		}
	}
	for _, q := range []string{
		`DELETE FROM wg_fleet_runs WHERE rowid NOT IN (SELECT rowid FROM wg_fleet_runs ORDER BY rowid DESC LIMIT ?)`,
		`DELETE FROM wg_fleet_devices WHERE snapshot_at NOT IN (SELECT snapshot_at FROM wg_fleet_runs) AND ? > 0`,
	} {
		if _, err := tx.ExecContext(ctx, q, keepFleetSnapshots); err != nil {
			_ = tx.Rollback()
			return "", false, err
		}
	}
	return now, true, tx.Commit()
}

func newNovelFireboxDevicesChangesCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var noRecord bool
	var limit int
	cmd := &cobra.Command{
		Use:   "changes",
		Short: "Show which synced Fireboxes, FireClusters and APs went offline, came back, appeared, disappeared or changed firmware since the last snapshot",
		Long: "Use this command to see what changed in the Firebox/FireCluster/AP fleet since the last snapshot (went offline, came back, new or removed devices, firmware changed). " +
			"Do NOT use this command to find devices running older firmware than their model peers; use 'firebox-devices firmware-lag' instead.\n\n" +
			"Reads the locally synced device list, so sync first (sync includes Subscriber devices by default; use --full so devices that disappeared are dropped and reported as removed). " +
			"A snapshot is recorded whenever the synced fleet looks different from the last one; running the command again before the next sync repeats the same comparison. " +
			"Change kinds: went_offline, removed, state_changed, firmware_changed, cloud_managed_changed, new, came_online. The first run records a baseline.",
		Example: strings.Trim(`
  watchguard-cli sync --resources firebox_devices --full
  watchguard-cli firebox-devices changes --agent
  watchguard-cli firebox-devices changes --no-record --json`, "\n"),
		Annotations: map[string]string{"mcp:local-write": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "firebox-devices changes")
			}
			view := changesView{Changes: make([]fleetChange, 0)}
			db, ok, err := openFleetStore(cmd, flags, dbPath, noRecord)
			if err != nil {
				return err
			}
			if !ok {
				view.Note = noMirrorNote
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			defer db.Close()
			ctx := cmd.Context()
			sqldb := db.DB()
			if !noRecord {
				for _, ddl := range fleetSnapshotDDL {
					if _, err := sqldb.ExecContext(ctx, ddl); err != nil {
						return fmt.Errorf("creating snapshot table: %w", err)
					}
				}
			}
			// One read transaction: the fleet and the snapshot head come from the
			// same database state, so a fleet read before a newer recorded snapshot
			// can never be appended after it.
			rtx, err := sqldb.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
			if err != nil {
				return err
			}
			defer func() { _ = rtx.Rollback() }()
			cur, skipped, err := readFleet(ctx, rtx, `SELECT id, data FROM resources WHERE resource_type = ?`, fireboxDevicesResource)
			if err != nil {
				return fmt.Errorf("reading synced devices: %w", err)
			}
			warnSkipped(cmd, skipped)
			view.Devices = len(cur)
			var syncRuns int
			if err := rtx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sync_state WHERE resource_type = ?`, fireboxDevicesResource).Scan(&syncRuns); err != nil {
				syncRuns = 0
			}
			if len(cur) == 0 && syncRuns == 0 {
				view.Note = "no synced devices; " + strings.TrimPrefix(noMirrorNote, "no local mirror: ")
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			snaps, err := recentSnapshots(ctx, rtx)
			if err != nil {
				return fmt.Errorf("reading snapshots: %w", err)
			}
			fp := fleetFingerprint(cur)
			// A fleet that looks like the newest snapshot is compared with the one
			// before it, so repeating the command repeats the same answer.
			record := len(snaps) == 0 || snaps[0].fingerprint != fp
			var base *snapshotRef
			switch {
			case record && len(snaps) > 0:
				base = &snaps[0]
			case !record && len(snaps) > 1:
				base = &snaps[1]
			}
			if base != nil {
				view.ComparedTo = base.at
				prev, _, err := readFleet(ctx, rtx, `SELECT device_id, data FROM wg_fleet_devices WHERE snapshot_at = ?`, base.at)
				if err != nil {
					return fmt.Errorf("reading previous snapshot: %w", err)
				}
				view.Changes = diffFleet(prev, cur)
			}
			_ = rtx.Rollback()
			if record && !noRecord {
				parent := ""
				if len(snaps) > 0 {
					parent = snaps[0].at
				}
				at, stored, err := recordSnapshot(ctx, sqldb, parent, fp, cur)
				if err != nil {
					return fmt.Errorf("recording snapshot: %w", err)
				}
				if stored {
					view.CurrentSnapshot = at
				}
			}
			if base == nil {
				if record && !noRecord {
					view.Note = "first run: recorded a baseline snapshot of the synced fleet; run again after the next sync to see changes"
				} else {
					view.Note = "no earlier snapshot to compare with; run again after the next sync"
				}
			}
			if limit > 0 && len(view.Changes) > limit {
				view.Changes = view.Changes[:limit]
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if view.Note != "" {
				fmt.Fprintln(cmd.OutOrStdout(), view.Note)
			}
			if len(view.Changes) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No changes across %d device(s).\n", view.Devices)
				return nil
			}
			tw := newTabWriter(cmd.OutOrStdout())
			fmt.Fprintln(tw, "CHANGE\tDEVICE\tNAME\tFROM\tTO")
			for _, c := range view.Changes {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", c.Change, c.DeviceID, c.Name, c.From, c.To)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local SQLite database path (default: the CLI's data directory)")
	cmd.Flags().BoolVar(&noRecord, "no-record", false, "Compare only: open the database read-only and record no snapshot")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum changes to return (0 = all)")
	return cmd
}
