// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel-feature logic (Printing Press transcendence). The CID-keyed
// fleet store that `fleet sync` populates and the `fleet *` rollups read.

package cli

import (
	"bytes"
	"context"
	"crowdstrike-pp-cli/internal/cliutil"
	"crowdstrike-pp-cli/internal/store"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const fleetSchema = `
CREATE TABLE IF NOT EXISTS fleet_entities (
  cid       TEXT NOT NULL,
  kind      TEXT NOT NULL,
  id        TEXT NOT NULL,
  name      TEXT,
  severity  TEXT,
  status    TEXT,
  last_seen TEXT,
  synced_at TEXT,
  data      TEXT,
  PRIMARY KEY (cid, kind, id)
);
CREATE INDEX IF NOT EXISTS idx_fleet_kind ON fleet_entities(kind);
CREATE INDEX IF NOT EXISTS idx_fleet_cid ON fleet_entities(cid);
CREATE TABLE IF NOT EXISTS fleet_snapshots (
  cid             TEXT NOT NULL,
  taken_at        TEXT NOT NULL,
  hosts           INTEGER DEFAULT 0,
  critical_alerts INTEGER DEFAULT 0,
  critical_vulns  INTEGER DEFAULT 0,
  PRIMARY KEY (cid, taken_at)
);
`

// fleetSyncStateKey is the sync_state resource_type row that fleet sync
// records on completion, so the generated hintIfUnsynced/hintIfStale helpers
// can warn fleet rollup readers about an unsynced or stale fleet store.
const fleetSyncStateKey = "fleet"

// openFleetStore opens (or creates) the local SQLite store and ensures the
// fleet_entities table exists. Callers must Close the returned store.
func openFleetStore(ctx context.Context, dbPath string) (*store.Store, error) {
	st, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		return nil, err
	}
	if _, err := st.DB().ExecContext(ctx, fleetSchema); err != nil {
		_ = st.Close() // best-effort cleanup; original error takes precedence
		return nil, fmt.Errorf("ensuring fleet schema: %w", err)
	}
	if err := ensureFleetOwnerColumn(ctx, st.DB()); err != nil {
		_ = st.Close() // best-effort cleanup; original error takes precedence
		return nil, fmt.Errorf("ensuring fleet schema: %w", err)
	}
	return st, nil
}

// ensureFleetOwnerColumn gives fleet_entities its owner column, in a store
// written before it too: the client a "self" row was read by (selfScopeOwner),
// "" for every other row. A "self" row written before the column has no owner:
// see fleetLegacySelfRow. crowdstrike handfix fleet-replace-only-a-proven-scope.
func ensureFleetOwnerColumn(ctx context.Context, db *sql.DB) error {
	has := func() (bool, error) {
		var n int
		err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('fleet_entities') WHERE name = 'owner'`).Scan(&n)
		return n > 0, err
	}
	if ok, err := has(); err != nil || ok {
		return err
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE fleet_entities ADD COLUMN owner TEXT NOT NULL DEFAULT ''`); err != nil {
		if ok, herr := has(); herr == nil && ok {
			return nil // another process added it first
		}
		return err
	}
	return nil
}

// fleetLegacySelfRow selects the rows written before the owner column for the
// authenticated CID: no reader can tell whose they are or whether they still
// hold, so they are not current posture, and the complete read of their kind
// deletes them (writeFleetEntities). The fabric also has cid "self" and no owner
// and is current, so the kinds are named: the per-CID kinds fleet sync reads.
// crowdstrike handfix fleet-legacy-rows-not-current.
const fleetLegacySelfRow = `cid = 'self' AND owner = '' AND kind IN ('host', 'alert', 'vuln', 'policy')`

// fleetLegacyRowsNote is the one line naming the rows fleetLegacySelfRow keeps
// out of every rollup, "" when there are none.
func fleetLegacyRowsNote(ctx context.Context, db *sql.DB) string {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM fleet_entities WHERE `+fleetLegacySelfRow).Scan(&n); err != nil || n == 0 {
		return ""
	}
	return fmt.Sprintf("%d row(s) written before this version are excluded from the fleet rollups; the next complete 'fleet sync' of that kind replaces them", n)
}

// hintIfFleetUnsynced is the fleet-family analogue of the generated
// hintIfUnsynced: the CID-keyed fleet store is populated by 'fleet sync', not
// the top-level 'sync', so fleet rollup readers must be pointed at the right
// command. Returns true when the unsynced hint was emitted.
func hintIfFleetUnsynced(cmd *cobra.Command, db *store.Store) bool {
	if cmd == nil || db == nil {
		return false
	}
	// Every rollup calls this first, so none drops rows without saying so. The
	// prefix is load-bearing: the MCP server forwards only hint: and warning:
	// lines of a tool's stderr (cobratree stderrHintLines).
	if note := fleetLegacyRowsNote(context.Background(), db.DB()); note != "" {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s.\n", note)
	}
	state, err := readSyncHintState(db, fleetSyncStateKey)
	if err != nil || state.hasState {
		return false
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "hint: fleet store has not been synced yet. Run 'crowdstrike-cli fleet sync' before trusting fleet results.\n")
	return true
}

// hintIfFleetStale is the fleet-family analogue of the generated hintIfStale,
// pointing the staleness refresh hint at 'fleet sync'.
func hintIfFleetStale(cmd *cobra.Command, db *store.Store, maxAge time.Duration) bool {
	if cmd == nil || db == nil || maxAge <= 0 {
		return false
	}
	state, err := readSyncHintState(db, fleetSyncStateKey)
	if err != nil || !state.hasState {
		return false
	}
	age := time.Since(state.lastSynced)
	if age <= maxAge {
		return false
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "hint: fleet store data is %s old, older than --max-age=%s. Run 'crowdstrike-cli fleet sync' to refresh.\n", syncHintRoundAge(age), maxAge)
	return true
}

// fleetScope is one kind of record for one CID: the unit a fleet sync read
// is complete or not for.
type fleetScope struct{ CID, Kind string }

// upsertFleetEntities writes a batch of entities in a single transaction.
// Existing rows (same cid+kind+id) are replaced so re-syncing a tenant
// refreshes rather than duplicates.
func upsertFleetEntities(ctx context.Context, db *sql.DB, ents []fleetEntity) (int, error) {
	return writeFleetEntities(ctx, db, ents, nil, "")
}

// writeFleetEntities is upsertFleetEntities that, given a scope the sync read
// completely, first removes every row of that scope in the same transaction:
// the scope then holds exactly what the read returned (an empty read leaves it
// empty). crowdstrike handfix fleet-refresh-completeness (Codex r1 of the
// 4.34.2 review, class stale records survive a complete refresh): rows were
// only ever upserted, so a vulnerability Spotlight no longer listed as open,
// a host removed from the tenant or a closed alert stayed in every fleet
// rollup. A read that failed or ended short replaces nothing (its rows are
// upserted over what the store held, and fleet sync warns). owner is the
// client the rows were read by ("" but for a "self" scope): every row written
// carries it, and a replacement given one removes only that client's rows of
// the scope and the pre-owner rows it supersedes, never another client's
// (crowdstrike handfixes fleet-replace-only-a-proven-scope,
// fleet-legacy-rows-not-current).
func writeFleetEntities(ctx context.Context, db *sql.DB, ents []fleetEntity, replace *fleetScope, owner string) (int, error) {
	if len(ents) == 0 && replace == nil {
		return 0, nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	if replace != nil {
		del, args := `DELETE FROM fleet_entities WHERE cid = ? AND kind = ?`, []any{replace.CID, replace.Kind}
		if owner != "" {
			del, args = del+` AND owner IN (?, '')`, append(args, owner)
		}
		if _, err := tx.ExecContext(ctx, del, args...); err != nil {
			_ = tx.Rollback()
			return 0, err
		}
	}
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO fleet_entities (cid, kind, id, name, severity, status, last_seen, synced_at, data, owner)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(cid, kind, id) DO UPDATE SET
		  name=excluded.name, severity=excluded.severity, status=excluded.status,
		  last_seen=excluded.last_seen, synced_at=excluded.synced_at, data=excluded.data, owner=excluded.owner`)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	defer stmt.Close()
	n := 0
	for _, e := range ents {
		var lastSeen string
		if !e.LastSeen.IsZero() {
			lastSeen = e.LastSeen.UTC().Format(time.RFC3339)
		}
		synced := e.SyncedAt
		if synced.IsZero() {
			synced = time.Now()
		}
		if _, err := stmt.ExecContext(ctx, e.CID, e.Kind, e.ID, e.Name, e.Severity, e.Status,
			lastSeen, synced.UTC().Format(time.RFC3339), fleetStoredRaw(e.Raw), owner); err != nil {
			_ = tx.Rollback()
			return n, err
		}
		n++
	}
	if err := tx.Commit(); err != nil {
		return n, err
	}
	return n, nil
}

// fleetStoredRaw is the payload fleet_entities keeps of raw: every value under
// a credential name redacted at any depth, containers included
// (cliutil.RedactSecretFields, the rule the generic store applies to every
// resource row in store/crowdstrike_secrets.go). fleet_entities is written by
// its own SQL, so the withholding driver alone stood between a child tenant's
// record and the file, and it withholds held values and credential-named
// string members only, never a list or object under a credential name. A
// payload that does not decode is not kept. crowdstrike handfix:
// store-never-keeps-secrets (fleet_entities).
func fleetStoredRaw(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return "{}"
	}
	out, err := json.Marshal(cliutil.RedactSecretFields(v))
	if err != nil {
		return "{}"
	}
	return string(out)
}

// loadFleetEntities reads entities from the store, optionally filtered to one
// kind (pass "" for all). Returned entities carry the typed columns the rollups
// need; Raw is left nil (the rollups don't read it).
func loadFleetEntities(ctx context.Context, dbPath, kind string) ([]fleetEntity, error) {
	st, err := openFleetStore(ctx, dbPath)
	if err != nil {
		return nil, err
	}
	defer st.Close()
	return loadFleetEntitiesFrom(ctx, st, kind, false)
}

// loadFleetEntitiesFrom reads entities through an already-open store handle so
// callers can emit sync hints and run several reads on one connection. When
// includeRaw is true the data column is loaded into Raw (needed by joins that
// read nested payload fields, e.g. fleet remediate and fleet tenants). Rows
// written before the owner column are not current: no reader sees them
// (fleetLegacySelfRow).
func loadFleetEntitiesFrom(ctx context.Context, st *store.Store, kind string, includeRaw bool) ([]fleetEntity, error) {
	cols := `cid, kind, id, name, severity, status, last_seen, synced_at`
	if includeRaw {
		cols += `, data`
	}
	q := `SELECT ` + cols + ` FROM fleet_entities WHERE NOT (` + fleetLegacySelfRow + `)`
	var rows *sql.Rows
	var err error
	if kind != "" {
		q += ` AND kind = ?`
		rows, err = st.DB().QueryContext(ctx, q, kind)
	} else {
		rows, err = st.DB().QueryContext(ctx, q)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []fleetEntity
	for rows.Next() {
		var e fleetEntity
		// name/severity/status are nullable columns (only cid/kind/id are NOT
		// NULL): hosts carry no severity, freshly-synced rows may omit status.
		// Scan them through sql.NullString so a NULL decodes to "" instead of
		// failing rows.Scan with "converting NULL to string is unsupported".
		var name, severity, status, lastSeen, synced sql.NullString
		dest := []any{&e.CID, &e.Kind, &e.ID, &name, &severity, &status, &lastSeen, &synced}
		var raw sql.NullString
		if includeRaw {
			dest = append(dest, &raw)
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		e.Name = name.String
		e.Severity = severity.String
		e.Status = status.String
		if includeRaw && raw.Valid && raw.String != "" {
			e.Raw = []byte(raw.String)
		}
		if lastSeen.Valid && lastSeen.String != "" {
			if t, perr := time.Parse(time.RFC3339, lastSeen.String); perr == nil {
				e.LastSeen = t
			}
		}
		if synced.Valid && synced.String != "" {
			if t, perr := time.Parse(time.RFC3339, synced.String); perr == nil {
				e.SyncedAt = t
			}
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// fleetSnapshot is one per-CID posture snapshot row, written by fleet sync and
// read by fleet trend.
type fleetSnapshot struct {
	CID            string    `json:"cid"`
	TakenAt        time.Time `json:"taken_at"`
	Hosts          int       `json:"hosts"`
	CriticalAlerts int       `json:"critical_alerts"`
	CriticalVulns  int       `json:"critical_vulns"`
}

// writeFleetSnapshots records one posture snapshot row per CID, all stamped
// with the same takenAt so fleet trend can group rows into sync generations.
func writeFleetSnapshots(ctx context.Context, db *sql.DB, snaps []fleetSnapshot, takenAt time.Time) error {
	if len(snaps) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO fleet_snapshots (cid, taken_at, hosts, critical_alerts, critical_vulns)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(cid, taken_at) DO UPDATE SET
		  hosts=excluded.hosts, critical_alerts=excluded.critical_alerts, critical_vulns=excluded.critical_vulns`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()
	// Nanosecond precision: the PK is (cid, taken_at), so two sync runs
	// landing in the same wall-clock second must still produce distinct
	// snapshot generations or fleet trend silently loses one.
	ts := takenAt.UTC().Format(time.RFC3339Nano)
	for _, s := range snaps {
		if _, err := stmt.ExecContext(ctx, s.CID, ts, s.Hosts, s.CriticalAlerts, s.CriticalVulns); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// loadFleetSnapshots reads every snapshot row ordered newest-first per CID.
func loadFleetSnapshots(ctx context.Context, st *store.Store) ([]fleetSnapshot, error) {
	rows, err := st.DB().QueryContext(ctx, `
		SELECT cid, taken_at, hosts, critical_alerts, critical_vulns
		FROM fleet_snapshots ORDER BY cid, taken_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []fleetSnapshot
	for rows.Next() {
		var s fleetSnapshot
		var taken string
		if err := rows.Scan(&s.CID, &taken, &s.Hosts, &s.CriticalAlerts, &s.CriticalVulns); err != nil {
			return nil, err
		}
		// RFC3339Nano parses both the nano-precision rows current syncs write
		// and the second-precision rows written before the collision fix.
		if t, perr := time.Parse(time.RFC3339Nano, taken); perr == nil {
			s.TakenAt = t
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// snapshotFromEntities computes per-CID posture counters from freshly-synced
// entities. Pure function; fleet sync calls it after the upsert loop.
func snapshotFromEntities(ents []fleetEntity) []fleetSnapshot {
	byCID := map[string]*fleetSnapshot{}
	order := []string{}
	for _, e := range ents {
		s, ok := byCID[e.CID]
		if !ok {
			s = &fleetSnapshot{CID: e.CID}
			byCID[e.CID] = s
			order = append(order, e.CID)
		}
		switch e.Kind {
		case kindHost:
			s.Hosts++
		case kindAlert:
			// Mirror fleet scorecard's open_critical_alerts: closed/resolved
			// criticals must not count, or trend deltas diverge from the
			// scorecard and mask genuine improvement. Case-insensitive so a
			// future fetcher storing "Critical" cannot silently undercount.
			if strings.EqualFold(e.Severity, "critical") && isOpenAlertStatus(e.Status) {
				s.CriticalAlerts++
			}
		case kindVuln:
			if strings.EqualFold(e.Severity, "critical") && isOpenVulnStatus(e.Status) {
				s.CriticalVulns++
			}
		}
	}
	out := make([]fleetSnapshot, 0, len(order))
	for _, cid := range order {
		out = append(out, *byCID[cid])
	}
	return out
}

// postureKinds are the kinds a posture snapshot counts: hosts, alerts and
// vulnerabilities.
var postureKinds = []string{kindHost, kindAlert, kindVuln}

// postureSnapshots is snapshotFromEntities over only the CIDs whose hosts,
// alerts and vulnerabilities this sync read completely (a CID read completely
// and found empty gets a zero row). crowdstrike handfix
// fleet-refresh-completeness (Codex r1 of the 4.34.2 review, class partial
// refresh recorded as complete posture): every sync recorded a snapshot from
// whatever kinds it fetched, so a sync of policies alone recorded zero
// critical vulnerabilities and fleet trend called the tenant "better", and a
// failed or short read did the same. skipped lists the synced CIDs that got
// no snapshot.
func postureSnapshots(ents []fleetEntity, complete map[fleetScope]bool, synced []string) (snaps []fleetSnapshot, skipped []string) {
	whole := map[string]bool{}
	for _, cid := range synced {
		ok := true
		for _, k := range postureKinds {
			ok = ok && complete[fleetScope{CID: cid, Kind: k}]
		}
		if ok {
			whole[cid] = true
		} else {
			skipped = append(skipped, cid)
		}
	}
	var kept []fleetEntity
	for _, e := range ents {
		if whole[e.CID] {
			kept = append(kept, e)
		}
	}
	snaps = snapshotFromEntities(kept)
	have := map[string]bool{}
	for _, s := range snaps {
		have[s.CID] = true
	}
	for _, cid := range synced {
		if whole[cid] && !have[cid] {
			have[cid] = true
			snaps = append(snaps, fleetSnapshot{CID: cid})
		}
	}
	return snaps, skipped
}
