// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"auvik-pp-cli/internal/cliutil"
	"auvik-pp-cli/internal/store"
)

type deviceSnapshotRow struct {
	DeviceID    string
	TenantID    string
	DeviceName  string
	DeviceType  string
	MakeModel   string
	Serial      string
	Firmware    string
	Software    string
	Fingerprint string
}

type inventoryChange struct {
	Change     string `json:"change"`
	Client     string `json:"client"`
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	DeviceType string `json:"device_type,omitempty"`
	MakeModel  string `json:"make_model,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

type inventoryDiffReport struct {
	BaselineAt    string            `json:"baseline_at,omitempty"`
	CurrentAt     string            `json:"current_at"`
	Counts        map[string]int    `json:"counts"`
	DevicesNow    int               `json:"devices_now"`
	DevicesThen   int               `json:"devices_then"`
	Changes       []inventoryChange `json:"changes"`
	SnapshotSaved bool              `json:"snapshot_saved"`
	Note          string            `json:"note,omitempty"`
}

func newNovelInventoryDiffCmd(flags *rootFlags) *cobra.Command {
	var (
		dbPath   string
		sinceIn  string
		snapshot bool
		change   string
		client   string
		limit    int
	)

	cmd := &cobra.Command{
		Use:   "diff",
		Short: "List devices added, removed, or changed fleet-wide since the last sync, attributed to each client.",
		Long: strings.Trim(`
Diffs the currently synced device inventory against the most recent stored
snapshot and reports devices added, removed, and changed, per client.

This exists because Auvik's API emits NO deletion event. filter[modifiedAfter]
surfaces additions and changes only; a decommissioned device simply stops
appearing in list responses. Removals are only detectable by keeping our own
prior view of the fleet, which --snapshot records.

Use this command for devices added, removed, or changed fleet-wide between
syncs. Do NOT use this command for the change history of one specific device;
use 'changes' instead.

Reads the local mirror. Typical loop:
  auvik-cli sync --resources tenants,inventory --full
  auvik-cli inventory diff --snapshot
`, "\n"),
		Example: strings.Trim(`
  # What changed since the last recorded snapshot, and record a new one
  auvik-cli inventory diff --snapshot --agent

  # Only devices that vanished (the case the API cannot tell you)
  auvik-cli inventory diff --change removed --agent

  # Compare against a snapshot at least a week old
  auvik-cli inventory diff --since 7d
`, "\n"),
		Annotations: map[string]string{
			"mcp:local-write": "true",
			"pp:happy-args":   "--limit=5",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "inventory diff")
			}
			if change != "" && change != "added" && change != "removed" && change != "changed" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--change must be one of added, removed, changed"))
			}
			var before time.Time
			if sinceIn != "" {
				d, err := cliutil.ParseDurationLoose(sinceIn)
				if err != nil {
					_ = cmd.Usage()
					return usageErr(fmt.Errorf("--since %q: %w", sinceIn, err))
				}
				before = time.Now().UTC().Add(-d)
			}

			resolved := dbPath
			if resolved == "" {
				resolved = defaultDBPath("auvik-cli")
			}
			empty := inventoryDiffReport{Counts: map[string]int{}, Changes: []inventoryChange{}}
			if _, statErr := os.Stat(resolved); os.IsNotExist(statErr) {
				fmt.Fprintf(cmd.ErrOrStderr(),
					"no local mirror at %s\nrun: auvik-cli sync --resources tenants,inventory --db %s\n",
					resolved, resolved)
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), empty, flags)
				}
				return nil
			}

			ctx := cmd.Context()
			// Only --snapshot writes. Taking a read-write handle for a plain
			// read was a real hazard: the driver mmaps the store, so a
			// concurrent writer growing the file faults a peer's mapping with
			// SIGBUS below SQLite's busy handling. Readers now stay read-only,
			// and the write path is serialized by withSnapshotLock.
			var db *store.Store
			var err error
			if snapshot {
				// Hold the lock across the OPEN as well: opening read-write runs
				// migrations, which write and can grow the file, faulting a
				// peer's mmap before any of our own INSERTs run.
				release, lockErr := acquireSnapshotLock(resolved)
				if lockErr != nil {
					return lockErr
				}
				defer release()
				db, err = store.OpenWithContext(ctx, resolved)
			} else {
				db, err = store.OpenReadOnlyContext(ctx, resolved)
			}
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, "inventory") {
				hintIfStale(cmd, db, "inventory", flags.maxAge)
			}

			devices, err := loadResources(ctx, db, rtDevices)
			if err != nil {
				return err
			}
			names := tenantNames(ctx, db)
			current := currentDeviceSnapshot(devices)

			baselineAt, baseline, err := loadLatestSnapshot(ctx, db, before)
			if err != nil {
				return err
			}

			report := buildInventoryDiff(baseline, current, names)
			report.CurrentAt = time.Now().UTC().Format(time.RFC3339)
			report.DevicesNow = len(current)
			report.DevicesThen = len(baseline)
			if !baselineAt.IsZero() {
				report.BaselineAt = baselineAt.Format(time.RFC3339)
			}

			if len(baseline) == 0 {
				report.Changes = []inventoryChange{}
				report.Counts = map[string]int{"added": 0, "removed": 0, "changed": 0}
				report.Note = "no prior snapshot to compare against; re-run with --snapshot to record the first baseline"
			}

			if snapshot {
				if err := saveDeviceSnapshot(ctx, db, current); err != nil {
					return fmt.Errorf("saving snapshot: %w", err)
				}
				report.SnapshotSaved = true
			}

			if change != "" {
				filtered := make([]inventoryChange, 0, len(report.Changes))
				for _, c := range report.Changes {
					if c.Change == change {
						filtered = append(filtered, c)
					}
				}
				report.Changes = filtered
			}
			if client != "" {
				needle := strings.ToLower(client)
				filtered := make([]inventoryChange, 0, len(report.Changes))
				for _, c := range report.Changes {
					if strings.Contains(strings.ToLower(c.Client), needle) {
						filtered = append(filtered, c)
					}
				}
				report.Changes = filtered
			}
			if limit > 0 && len(report.Changes) > limit {
				report.Changes = report.Changes[:limit]
			}

			done, err := emitLocalReport(cmd, flags, report, report.Changes)
			if err != nil || done {
				return err
			}

			out := cmd.OutOrStdout()
			if report.BaselineAt == "" {
				fmt.Fprintln(out, report.Note)
				if report.SnapshotSaved {
					fmt.Fprintf(out, "recorded baseline snapshot of %d devices\n", report.DevicesNow)
				}
				return nil
			}
			fmt.Fprintf(out, "Inventory diff vs snapshot %s\n", report.BaselineAt)
			fmt.Fprintf(out, "  added: %d   removed: %d   changed: %d   (%d devices then, %d now)\n\n",
				report.Counts["added"], report.Counts["removed"], report.Counts["changed"],
				report.DevicesThen, report.DevicesNow)
			if len(report.Changes) == 0 {
				fmt.Fprintln(out, "No differences match the given filters.")
			} else {
				fmt.Fprintf(out, "%-9s %-22s %-28s %s\n", "CHANGE", "CLIENT", "DEVICE", "DETAIL")
				for _, c := range report.Changes {
					fmt.Fprintf(out, "%-9s %-22s %-28s %s\n",
						c.Change, truncate(c.Client, 22), truncate(c.DeviceName, 28), c.Detail)
				}
			}
			if report.SnapshotSaved {
				fmt.Fprintf(out, "\nrecorded new snapshot of %d devices\n", report.DevicesNow)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	cmd.Flags().StringVar(&sinceIn, "since", "", "Compare against the newest snapshot at least this old (e.g. 24h, 7d, 4w)")
	cmd.Flags().BoolVar(&snapshot, "snapshot", false, "Record the current inventory as a new snapshot after diffing")
	cmd.Flags().StringVar(&change, "change", "", "Only one change kind: added, removed, changed")
	cmd.Flags().StringVar(&client, "client", "", "Only show changes whose client name contains this substring")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum rows to return (0 = all)")
	return cmd
}

func currentDeviceSnapshot(devices []auvikRow) map[string]deviceSnapshotRow {
	out := make(map[string]deviceSnapshotRow, len(devices))
	for _, d := range devices {
		row := deviceSnapshotRow{
			DeviceID:   d.ID,
			TenantID:   d.rel("tenant"),
			DeviceName: firstNonEmpty(d.attrString(fDeviceName.Field), d.ID),
			DeviceType: d.attrString(fDeviceType.Field),
			MakeModel:  d.attrString(fDeviceMakeModel.Field),
			Serial:     d.attrString(fDeviceSerial.Field),
			Firmware:   d.attrString(fDeviceFirmware.Field),
			Software:   d.attrString(fDeviceSoftware.Field),
		}
		row.Fingerprint = deviceFingerprint(row)
		out[d.ID] = row
	}
	return out
}

// deviceFingerprint covers the identity-bearing fields whose drift is worth
// reporting, and ONLY fields the snapshot table persists -- otherwise a change
// can be detected but never described.
//
// Deliberately excluded: lastSeenTime and onlineStatus (volatile telemetry that
// would mark a healthy fleet as churning) and ipAddresses (a JSON array whose
// order is not guaranteed, and which changes on every DHCP re-lease).
func deviceFingerprint(row deviceSnapshotRow) string {
	h := sha256.New()
	for _, part := range []string{
		row.DeviceName, row.DeviceType, row.MakeModel, row.TenantID,
		row.Serial, row.Firmware, row.Software,
	} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// loadLatestSnapshot returns the newest snapshot at or before `before`
// (or simply the newest when before is zero). Drain-first: the parent rows are
// fully scanned and closed before any follow-up query runs.
func loadLatestSnapshot(ctx context.Context, db *store.Store, before time.Time) (time.Time, map[string]deviceSnapshotRow, error) {
	var at time.Time
	q := `SELECT MAX(snapshot_at) FROM auvik_device_snapshots`
	args := []any{}
	if !before.IsZero() {
		q += ` WHERE snapshot_at <= ?`
		args = append(args, before.Format(time.RFC3339))
	}
	var raw sql.NullString
	if err := db.DB().QueryRowContext(ctx, q, args...).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return at, map[string]deviceSnapshotRow{}, nil
		}
		// A locked or corrupt store must not masquerade as "no baseline yet".
		return at, nil, fmt.Errorf("reading snapshot index: %w", err)
	}
	if !raw.Valid || raw.String == "" {
		return at, map[string]deviceSnapshotRow{}, nil
	}
	at, _ = parseAuvikTime(raw.String)

	rows, err := db.DB().QueryContext(ctx,
		`SELECT device_id, tenant_id, device_name, device_type, make_model,
		        serial_number, firmware_version, software_version, fingerprint
		   FROM auvik_device_snapshots WHERE snapshot_at = ?`, raw.String)
	if err != nil {
		return at, nil, fmt.Errorf("reading snapshot: %w", err)
	}
	out := map[string]deviceSnapshotRow{}
	for rows.Next() {
		var (
			id                                      string
			tenant, name, dtype, model, fingerprint sql.NullString
			serial, firmware, software              sql.NullString
		)
		if err := rows.Scan(&id, &tenant, &name, &dtype, &model,
			&serial, &firmware, &software, &fingerprint); err != nil {
			_ = rows.Close()
			return at, nil, fmt.Errorf("scanning snapshot: %w", err)
		}
		out[id] = deviceSnapshotRow{
			DeviceID: id, TenantID: tenant.String, DeviceName: name.String,
			DeviceType: dtype.String, MakeModel: model.String,
			Serial: serial.String, Firmware: firmware.String, Software: software.String,
			Fingerprint: fingerprint.String,
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return at, nil, fmt.Errorf("iterating snapshot: %w", err)
	}
	if err := rows.Close(); err != nil {
		return at, nil, fmt.Errorf("closing snapshot rows: %w", err)
	}
	return at, out, nil
}

func saveDeviceSnapshot(ctx context.Context, db *store.Store, current map[string]deviceSnapshotRow) error {
	stamp := time.Now().UTC().Format(time.RFC3339)
	tx, err := db.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT OR REPLACE INTO auvik_device_snapshots
		   (snapshot_at, device_id, tenant_id, device_name, device_type, make_model,
		    serial_number, firmware_version, software_version, fingerprint)
		 VALUES (?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	ids := make([]string, 0, len(current))
	for id := range current {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		r := current[id]
		if _, err := stmt.ExecContext(ctx, stamp, r.DeviceID, r.TenantID,
			r.DeviceName, r.DeviceType, r.MakeModel,
			r.Serial, r.Firmware, r.Software, r.Fingerprint); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// buildInventoryDiff is the pure comparison core, split out for table tests.
func buildInventoryDiff(before, after map[string]deviceSnapshotRow, tenants map[string]string) inventoryDiffReport {
	report := inventoryDiffReport{
		Counts:  map[string]int{"added": 0, "removed": 0, "changed": 0},
		Changes: make([]inventoryChange, 0),
	}
	if len(before) == 0 {
		return report
	}

	ids := map[string]bool{}
	for id := range before {
		ids[id] = true
	}
	for id := range after {
		ids[id] = true
	}
	ordered := make([]string, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)

	for _, id := range ordered {
		old, hadOld := before[id]
		cur, hadCur := after[id]
		switch {
		case !hadOld && hadCur:
			report.Changes = append(report.Changes, inventoryChange{
				Change: "added", Client: tenantLabel(tenants, cur.TenantID),
				DeviceID: id, DeviceName: cur.DeviceName, DeviceType: cur.DeviceType,
				MakeModel: cur.MakeModel, Detail: "new since snapshot",
			})
			report.Counts["added"]++
		case hadOld && !hadCur:
			report.Changes = append(report.Changes, inventoryChange{
				Change: "removed", Client: tenantLabel(tenants, old.TenantID),
				DeviceID: id, DeviceName: old.DeviceName, DeviceType: old.DeviceType,
				MakeModel: old.MakeModel, Detail: "no longer returned by the API",
			})
			report.Counts["removed"]++
		case old.Fingerprint != cur.Fingerprint:
			report.Changes = append(report.Changes, inventoryChange{
				Change: "changed", Client: tenantLabel(tenants, cur.TenantID),
				DeviceID: id, DeviceName: cur.DeviceName, DeviceType: cur.DeviceType,
				MakeModel: cur.MakeModel, Detail: describeDeviceDelta(old, cur),
			})
			report.Counts["changed"]++
		}
	}
	return report
}

func describeDeviceDelta(old, cur deviceSnapshotRow) string {
	parts := make([]string, 0, 3)
	if old.DeviceName != cur.DeviceName {
		parts = append(parts, fmt.Sprintf("name %q -> %q", old.DeviceName, cur.DeviceName))
	}
	if old.DeviceType != cur.DeviceType {
		parts = append(parts, fmt.Sprintf("type %q -> %q", old.DeviceType, cur.DeviceType))
	}
	if old.MakeModel != cur.MakeModel {
		parts = append(parts, fmt.Sprintf("model %q -> %q", old.MakeModel, cur.MakeModel))
	}
	if old.Serial != cur.Serial {
		parts = append(parts, fmt.Sprintf("serial %q -> %q", old.Serial, cur.Serial))
	}
	if old.Firmware != cur.Firmware {
		parts = append(parts, fmt.Sprintf("firmware %q -> %q", old.Firmware, cur.Firmware))
	}
	if old.Software != cur.Software {
		parts = append(parts, fmt.Sprintf("software %q -> %q", old.Software, cur.Software))
	}
	if old.TenantID != cur.TenantID {
		parts = append(parts, "moved client")
	}
	if len(parts) == 0 {
		return "identity fields changed"
	}
	return strings.Join(parts, "; ")
}
