// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Shared helpers for the hand-authored Veeam transcendence commands
// (fleet-health, stale-backups, at-risk, alarms-triage, license-usage,
// company-overview, since). These read the local SQLite mirror that `sync`
// populates and assemble cross-tenant views the per-instance VSPC API never
// returns in one call.
package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"veeam-pp-cli/internal/cliutil"
	"veeam-pp-cli/internal/store"
)

// veeamRow is one decoded record from the local `resources` table.
type veeamRow map[string]any

// veeamOpenStore opens the local SQLite mirror read-write (creating and
// migrating it if needed), defaulting to the standard CLI database path when
// dbPath is empty. Used by commands that record local state (license-usage).
func veeamOpenStore(ctx context.Context, dbPath string) (*store.Store, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("veeam-cli")
	}
	return store.OpenWithContext(ctx, dbPath)
}

// veeamOpenStoreRead opens the local mirror read-only for read commands.
// Read-only handles do not take a write lock, so several read commands can run
// concurrently against the same mirror without SQLITE_BUSY. ok is false (with a
// nil store and nil error) when no mirror exists yet; callers render that as an
// empty "run sync first" result rather than an error.
func veeamOpenStoreRead(dbPath string) (*store.Store, bool, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("veeam-cli")
	}
	if _, err := os.Stat(dbPath); err != nil {
		return nil, false, nil
	}
	db, err := store.OpenReadOnly(dbPath)
	if err != nil {
		return nil, false, err
	}
	return db, true, nil
}

// veeamLoad returns every decoded record for the given resource_type keys.
// NULL/blank data rows are skipped rather than failing the whole query, and an
// empty result is a normal "nothing synced yet" state, not an error.
func veeamLoad(ctx context.Context, db *store.Store, types ...string) ([]veeamRow, error) {
	if db == nil || len(types) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(types))
	args := make([]any, len(types))
	for i, t := range types {
		placeholders[i] = "?"
		args[i] = t
	}
	q := "SELECT data FROM resources WHERE resource_type IN (" + strings.Join(placeholders, ",") + ")"
	rows, err := db.DB().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]veeamRow, 0)
	for rows.Next() {
		var data sql.NullString
		if err := rows.Scan(&data); err != nil {
			continue
		}
		if !data.Valid || strings.TrimSpace(data.String) == "" {
			continue
		}
		var m veeamRow
		if err := json.Unmarshal([]byte(data.String), &m); err != nil {
			continue
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// vget walks a dotted path through nested maps and returns the raw value.
func vget(m veeamRow, path string) any {
	var cur any = map[string]any(m)
	for _, seg := range strings.Split(path, ".") {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mm[seg]
	}
	return cur
}

// vstr returns a string for a dotted path, coercing numbers/bools sensibly.
func vstr(m veeamRow, path string) string {
	switch v := vget(m, path).(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	default:
		return ""
	}
}

// vnum returns a float64 for a dotted path; missing/non-numeric yields 0.
func vnum(m veeamRow, path string) float64 {
	switch v := vget(m, path).(type) {
	case float64:
		return v
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f
	default:
		return 0
	}
}

// vbool returns (value, present) for a boolean dotted path.
func vbool(m veeamRow, path string) (bool, bool) {
	if v, ok := vget(m, path).(bool); ok {
		return v, true
	}
	return false, false
}

// vtime parses a timestamp at a dotted path. ok is false when the value is
// absent or unparseable.
func vtime(m veeamRow, path string) (time.Time, bool) {
	return parseVeeamTime(vstr(m, path))
}

// parseVeeamTime parses the ISO8601/RFC3339 timestamps VSPC emits.
func parseVeeamTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.999999999",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// veeamCompanyNames builds an organizationUid -> company name map from synced
// companies so cross-tenant views can show names instead of GUIDs.
func veeamCompanyNames(ctx context.Context, db *store.Store) map[string]string {
	rows, _ := veeamLoad(ctx, db, "organizations-companies", "organizations-provider-companies")
	names := make(map[string]string, len(rows))
	for _, r := range rows {
		uid := strings.ToLower(vstr(r, "instanceUid"))
		name := vstr(r, "name")
		if uid != "" && name != "" {
			names[uid] = name
		}
	}
	return names
}

// veeamCompanyLabel resolves an org uid to a display name, falling back to the
// uid (or a placeholder when empty).
func veeamCompanyLabel(names map[string]string, uid string) string {
	if strings.TrimSpace(uid) == "" {
		return "(unassigned)"
	}
	if n, ok := names[strings.ToLower(uid)]; ok {
		return n
	}
	return uid
}

// veeamJobHealth normalizes a Veeam job/session status into a stable class.
func veeamJobHealth(status string) string {
	s := strings.ToLower(strings.TrimSpace(status))
	switch {
	case s == "":
		return "unknown"
	case strings.Contains(s, "success"):
		return "success"
	case strings.Contains(s, "warning"):
		return "warning"
	case strings.Contains(s, "fail"), strings.Contains(s, "error"):
		return "failed"
	case strings.Contains(s, "running"), strings.Contains(s, "working"),
		strings.Contains(s, "pending"), strings.Contains(s, "starting"),
		strings.Contains(s, "queued"):
		return "running"
	default:
		return "other"
	}
}

// veeamAgentOnline reports whether a backup-agent status string indicates the
// agent is reachable/online.
func veeamAgentOnline(status string) bool {
	s := strings.ToLower(strings.TrimSpace(status))
	return strings.Contains(s, "online") || strings.Contains(s, "healthy") ||
		strings.Contains(s, "active") || strings.Contains(s, "running")
}

// veeamSeverityRank orders alarm severities most-to-least urgent for sorting.
func veeamSeverityRank(sev string) int {
	switch strings.ToLower(strings.TrimSpace(sev)) {
	case "error":
		return 0
	case "warning":
		return 1
	case "info", "information":
		return 2
	case "resolved", "acknowledged":
		return 3
	default:
		return 4
	}
}

// veeamParseWindow parses a duration window like "24h", "3d", "1w" (via the
// loose parser), returning def for an empty input.
func veeamParseWindow(s string, def time.Duration) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return def, nil
	}
	return cliutil.ParseDurationLoose(s)
}

// veeamParseDays parses a day count flag (a plain number of days, e.g. "3", or
// a duration like "36h") into a duration, returning def days when empty.
func veeamParseDays(s string, defDays float64) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Duration(defDays * float64(24*time.Hour)), nil
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		if f < 0 {
			return 0, fmt.Errorf("--days must be non-negative")
		}
		return time.Duration(f * float64(24*time.Hour)), nil
	}
	return cliutil.ParseDurationLoose(s)
}

// veeamEmit renders a structured view as JSON (honoring --select/--compact/etc.)
// or, for an interactive terminal, a human table; emptyMsg is shown when the
// table form has no rows.
func veeamEmit(cmd *cobra.Command, flags *rootFlags, view any, table []map[string]any, emptyMsg string) error {
	if wantsHumanTable(cmd.OutOrStdout(), flags) {
		if len(table) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), emptyMsg)
			return nil
		}
		return printAutoTable(cmd.OutOrStdout(), table)
	}
	return printJSONFiltered(cmd.OutOrStdout(), view, flags)
}

// veeamHoursSince returns whole hours between t and now (>= 0).
func veeamHoursSince(now, t time.Time) int {
	d := now.Sub(t)
	if d < 0 {
		d = 0
	}
	return int(d.Hours())
}
