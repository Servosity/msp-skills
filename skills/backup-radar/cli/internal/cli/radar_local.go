// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"backup-radar-pp-cli/internal/cliutil"
	"backup-radar-pp-cli/internal/config"
	"backup-radar-pp-cli/internal/radar"
	"backup-radar-pp-cli/internal/store"
)

// The default store scope hashes only the v3 credential. Fold the v2 key in
// too, so two Backup Radar v2 accounts on one machine never share a local
// database. With no v2 key the default (v3-only) scope is unchanged.
func init() {
	config.StoreScopeIdentity = func(c *config.Config) string {
		v2 := strings.TrimSpace(cliutil.EnvOverride("BACKUP_RADAR_V2_API_KEY"))
		if v2 == "" {
			return ""
		}
		return "v3=" + c.StoreScopeLegacyCredential() + "|v2=" + v2
	}
}

// radarResources are the synced resource types the hand-written reports read.
var radarResources = []string{"devices", "health", "backups", "backups-inactive", "backups-retired"}

// loadRadarSnapshot reads every synced Backup Radar row from the local store.
// ok=false means no local mirror exists yet; a sync hint was printed to stderr
// and the caller should emit an empty result.
func loadRadarSnapshot(cmd *cobra.Command, flags *rootFlags, dbPath string) (radar.Snapshot, bool, error) {
	snap := radar.Snapshot{Synced: map[string]bool{}}
	if dbPath == "" {
		dbPath = defaultDBPath("backup-radar-cli")
	}
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Fprintf(cmd.ErrOrStderr(), "no local mirror at %s\nrun: backup-radar-cli sync --db %s\n", dbPath, shellQuote(dbPath))
		return snap, false, nil
	}
	db, err := store.OpenReadOnlyContext(cmd.Context(), dbPath)
	if err != nil {
		return snap, false, fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()
	if !hintIfUnsynced(cmd, db, "") {
		hintIfStale(cmd, db, "", flags.maxAge)
	}

	type raw struct {
		typ  string
		data []byte
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(radarResources)), ",")
	args := make([]any, len(radarResources))
	for i, r := range radarResources {
		args[i] = r
	}
	rows, err := db.DB().QueryContext(cmd.Context(), `SELECT resource_type, data FROM resources WHERE resource_type IN (`+placeholders+`)`, args...)
	if err != nil {
		return snap, false, fmt.Errorf("query local store: %w", err)
	}
	var raws []raw
	for rows.Next() {
		var r raw
		if err := rows.Scan(&r.typ, &r.data); err != nil {
			_ = rows.Close()
			return snap, false, fmt.Errorf("scan row: %w", err)
		}
		raws = append(raws, r)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return snap, false, fmt.Errorf("iterate rows: %w", err)
	}
	if err := rows.Close(); err != nil {
		return snap, false, fmt.Errorf("close rows: %w", err)
	}

	// A sync_state row is written when an attempt starts. A collection counts
	// as synced after a completed attempt, or when a sync attempt left rows
	// (kept across a later failed retry). Rows from a filtered live read with
	// no sync attempt do not count.
	attempted := map[string]bool{}
	if stateRows, err := db.DB().QueryContext(cmd.Context(), `SELECT resource_type, last_attempt_complete FROM sync_state`); err == nil {
		for stateRows.Next() {
			var t string
			var complete int
			if stateRows.Scan(&t, &complete) == nil {
				attempted[t] = true
				snap.Synced[t] = snap.Synced[t] || complete == 1
			}
		}
		if err := stateRows.Err(); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: reading sync state: %v\n", err)
		}
		_ = stateRows.Close()
	} else {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: reading sync state: %v\n", err)
	}

	retired := map[int64]bool{}
	var backups []radar.Backup
	var inactive []radar.Inactive
	for _, r := range raws {
		if attempted[r.typ] {
			snap.Synced[r.typ] = true
		}
		var err error
		switch r.typ {
		case "devices":
			var d radar.Device
			if err = json.Unmarshal(r.data, &d); err == nil {
				snap.Devices = append(snap.Devices, d)
			}
		case "health":
			var h radar.ClientHealth
			if err = json.Unmarshal(r.data, &h); err == nil {
				snap.Health = append(snap.Health, h)
			}
		case "backups":
			var b radar.Backup
			// Only real ApiBackup rows (they always carry a status); anything
			// else cached under "backups" is not an active backup.
			if err = json.Unmarshal(r.data, &b); err == nil && b.BackupID != 0 && b.Status.Name != "" {
				backups = append(backups, b)
			}
		case "backups-inactive":
			var in radar.Inactive
			if err = json.Unmarshal(r.data, &in); err == nil {
				inactive = append(inactive, in)
			}
		case "backups-retired":
			var ret radar.Inactive // same backupId field as a retired backup
			if err = json.Unmarshal(r.data, &ret); err == nil && ret.BackupID != 0 {
				retired[ret.BackupID] = true
			}
		}
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: skipping unreadable %s row: %v\n", r.typ, err)
		}
	}
	// v2 rows of retired backups drop out by backupId (a plain sync never
	// deletes v2 rows). v3 rows carry no backupId; a retired backup leaves the
	// v3 device list and the next un-narrowed sync prunes it
	// (HAND-FIX(sync-prune-snapshots)). Names cannot identify it: a new backup
	// may reuse them.
	for _, b := range backups {
		if !retired[b.BackupID] {
			snap.Backups = append(snap.Backups, b)
		}
	}
	// A backup activated since the last --full sync is still stored as
	// inactive; an active v2 backup with the same id supersedes it. Names
	// alone are not enough: two backups can share them (different methods).
	activeIDs := map[int64]bool{}
	for _, b := range snap.Backups {
		activeIDs[b.BackupID] = true
	}
	for _, in := range inactive {
		if !retired[in.BackupID] && !activeIDs[in.BackupID] {
			snap.Inactive = append(snap.Inactive, in)
		}
	}
	return snap, true, nil
}

// radarSources reports which collections fed a report.
type radarSources struct {
	V3Devices  int    `json:"v3_devices"`
	V3Clients  int    `json:"v3_clients"`
	V2Backups  int    `json:"v2_backups"`
	V2Inactive int    `json:"v2_inactive"`
	Note       string `json:"note,omitempty"`
}

// sourcesOf counts every collection, but the "not synced" note names only the
// collections in uses (all four when uses is empty), so a report never asks
// for a key it does not read.
func sourcesOf(s radar.Snapshot, uses ...string) radarSources {
	src := radarSources{V3Devices: len(s.Devices), V3Clients: len(s.Health), V2Backups: len(s.Backups), V2Inactive: len(s.Inactive)}
	var missing []string
	for _, c := range []struct{ name, why string }{
		{"devices", "v3 devices (BACKUP_RADAR_API_KEY): SLA history and ScalePad mapping"},
		{"health", "v3 client health (BACKUP_RADAR_API_KEY): compliance rates"},
		{"backups", "v2 backups (BACKUP_RADAR_V2_API_KEY): status, tickets, policy flags"},
		{"backups-inactive", "v2 inactive backups (BACKUP_RADAR_V2_API_KEY)"},
	} {
		if len(uses) > 0 && !slices.Contains(uses, c.name) {
			continue
		}
		if !s.Synced[c.name] {
			missing = append(missing, c.why)
		}
	}
	switch {
	case !s.AnySynced():
		src.Note = "nothing synced yet; run 'backup-radar-cli sync'"
	case len(missing) > 0:
		src.Note = "not synced: " + strings.Join(missing, "; ") + ". Set the key and run 'backup-radar-cli sync'"
	}
	return src
}

// readPathOrStdin reads a file, or stdin when path is "-". Its callers
// (push --from, coverage diff --devices) declare those flags in
// mcp:write-flags, so MCP callers cannot choose the path.
func readPathOrStdin(cmd *cobra.Command, path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(cmd.InOrStdin())
	}
	// The path is opened exactly as given: Clean would resolve "dir/.." lexically and,
	// through a symlinked dir, read a different file than the shell would.
	return os.ReadFile(path) // #nosec G304 -- user-specified input file is this flag's documented purpose; MCP blocks the flag.
}

func splitCSVFlag(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// printRadarTable renders rows with a fixed column order and no truncation;
// the generic auto-table keeps six columns and breaks ties by map order.
func printRadarTable(w io.Writer, headers []string, rows [][]string) error {
	tw := newTabWriter(w)
	fmt.Fprintln(tw, strings.ToUpper(scrubRadarCells(headers)))
	for _, r := range rows {
		fmt.Fprintln(tw, scrubRadarCells(r))
	}
	return tw.Flush()
}

// scrubRadarCells joins one table row, dropping terminal control characters
// from every cell. Company, device and job names, tags and notes come from
// the tenant, and an ESC, BEL or OSC sequence inside one would otherwise reach
// the terminal untouched (the C1 writer only covers U+0080-U+009F). Tabs are
// the column separators, so they are stripped from cells too.
// HAND-FIX(radar-human-output-scrub-terminal)
func scrubRadarCells(cells []string) string {
	out := make([]string, len(cells))
	for i, c := range cells {
		out[i] = scrubRadarText(c)
	}
	return strings.Join(out, "\t")
}

// scrubRadarText drops terminal control characters (C0, DEL, C1) and tabs
// from one human-output string; newlines become spaces.
func scrubRadarText(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r':
			return ' '
		case r < 0x20, r == 0x7f, r >= 0x80 && r <= 0x9f:
			return -1
		}
		return r
	}, s)
}

// radarKeep lists the JSON keys of a report row type so --agent/--compact
// keep every field of these curated views instead of only "gravity" names.
func radarKeep(row any) []string {
	t := reflect.TypeOf(row)
	keys := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		if name := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]; name != "" && name != "-" {
			keys = append(keys, name)
		}
	}
	return keys
}
