// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored shared helpers for the Kaseya BMS novel commands.
//
// BMS's swagger declares PascalCase property names (TicketNumber, QueueName)
// but the live API serializes camelCase (the unauthenticated error envelope
// returns {"success":false,...}). Every field read therefore tries the
// PascalCase schema name first and falls back to the camelCase wire name, so
// the novel commands work against either serialization.

package cli

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"kaseya-bms-pp-cli/internal/cliutil"
	"kaseya-bms-pp-cli/internal/store"

	"encoding/json"
)

// kbmsRows loads every row of one resource_type from the generic resources
// table and decodes the JSON payloads into maps for tolerant field access.
func kbmsRows(ctx context.Context, db *store.Store, resourceType string) ([]map[string]any, error) {
	rows, err := db.DB().QueryContext(ctx,
		`SELECT data FROM resources WHERE resource_type = ?`, resourceType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0, 256)
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil || m == nil {
			continue
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// kbmsLowerFirst converts a PascalCase schema name to its camelCase wire form.
func kbmsLowerFirst(key string) string {
	if key == "" {
		return key
	}
	return strings.ToLower(key[:1]) + key[1:]
}

// kbmsVal returns the value for a schema field, trying PascalCase then
// camelCase then all-lowercase.
func kbmsVal(m map[string]any, key string) (any, bool) {
	if v, ok := m[key]; ok && v != nil {
		return v, true
	}
	if v, ok := m[kbmsLowerFirst(key)]; ok && v != nil {
		return v, true
	}
	if v, ok := m[strings.ToLower(key)]; ok && v != nil {
		return v, true
	}
	return nil, false
}

// kbmsStr reads a string field; "" when absent or not a string.
func kbmsStr(m map[string]any, key string) string {
	v, ok := kbmsVal(m, key)
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// kbmsNum reads a numeric field, accepting JSON numbers and numeric strings
// (the streaming-feed lesson: number-as-string must not silently become 0).
func kbmsNum(m map[string]any, key string) (float64, bool) {
	v, ok := kbmsVal(m, key)
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case string:
		var f float64
		if _, err := jsonNumberParse(n, &f); err == nil {
			return f, true
		}
	}
	return 0, false
}

// jsonNumberParse parses a numeric string via the json package so locale
// quirks and exponent forms behave exactly like wire JSON numbers.
func jsonNumberParse(s string, out *float64) (bool, error) {
	if err := json.Unmarshal([]byte(strings.TrimSpace(s)), out); err != nil {
		return false, err
	}
	return true, nil
}

// kbmsBool reads a boolean field, tolerating bool, 0/1 numbers, and
// "true"/"false" strings.
func kbmsBool(m map[string]any, key string) bool {
	v, ok := kbmsVal(m, key)
	if !ok {
		return false
	}
	switch b := v.(type) {
	case bool:
		return b
	case float64:
		return b != 0
	case string:
		return strings.EqualFold(b, "true") || b == "1"
	}
	return false
}

// kbmsTimeLayouts covers the ISO-8601 shapes ASP.NET serializes, with and
// without zone or fractional seconds.
var kbmsTimeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.999999999",
	"2006-01-02T15:04:05",
	"2006-01-02",
}

// kbmsTime reads a datetime field. Uses cliutil.ParseODataDate first (which
// also handles RFC3339 and legacy /Date(ms)/ literals) then local layouts.
func kbmsTime(m map[string]any, key string) (time.Time, bool) {
	s := kbmsStr(m, key)
	if s == "" {
		return time.Time{}, false
	}
	if t, ok := cliutil.ParseODataDate(s); ok {
		return t, true
	}
	for _, layout := range kbmsTimeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// kbmsTicketOpen reports whether a ticket row represents open work: no
// CompletedDate and a status that does not read as a terminal state.
func kbmsTicketOpen(m map[string]any) bool {
	if _, done := kbmsTime(m, "CompletedDate"); done {
		return false
	}
	status := strings.ToLower(kbmsStr(m, "StatusName"))
	for _, terminal := range []string{"completed", "closed", "cancelled", "canceled", "resolved"} {
		if strings.Contains(status, terminal) {
			return false
		}
	}
	return true
}

// kbmsTicketActivity returns the best available last-activity timestamp for a
// ticket row: LastActivityUpdate, then ModifiedOn, then CreatedOn.
func kbmsTicketActivity(m map[string]any) (time.Time, bool) {
	for _, key := range []string{"LastActivityUpdate", "ModifiedOn", "CreatedOn"} {
		if t, ok := kbmsTime(m, key); ok {
			return t, true
		}
	}
	return time.Time{}, false
}

// kbmsHoursFromTimespent converts a raw Timespent value to hours. BMS reports
// Timespent in minutes by default; the --timespent-unit flag lets operators
// whose tenant serializes hours or seconds correct the math without a code
// change.
func kbmsHoursFromTimespent(raw float64, unit string) float64 {
	switch strings.ToLower(unit) {
	case "hours":
		return raw
	case "seconds":
		return raw / 3600
	default: // minutes
		return raw / 60
	}
}

// kbmsRound2 rounds to 2 decimal places for stable JSON output. math.Round
// keeps the rounding symmetric for negative values (credit/adjustment
// entries), which the truncate-toward-zero form got wrong.
func kbmsRound2(f float64) float64 {
	return math.Round(f*100) / 100
}

// kbmsSortedKeys returns map keys sorted for deterministic output.
func kbmsSortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// kbmsRejectLiveSource returns an error when --data-source live is requested
// for a local-only analytical command.
func kbmsRejectLiveSource(flags *rootFlags, command string) error {
	if flags != nil && flags.dataSource == "live" {
		return usageErr(errLiveSource(command))
	}
	return nil
}

type liveSourceError struct{ command string }

func (e liveSourceError) Error() string {
	return e.command + " reads the local mirror only and has no live equivalent; run 'sync' first and retry without --data-source live"
}

func errLiveSource(command string) error { return liveSourceError{command: command} }

// kbmsIDString returns a stable string form of a numeric-or-string ID field
// ("" when absent), so joins key on IDs instead of display names.
func kbmsIDString(m map[string]any, key string) string {
	if s := kbmsStr(m, key); s != "" {
		return s
	}
	if n, ok := kbmsNum(m, key); ok {
		return strconvFormatID(n)
	}
	return ""
}

func strconvFormatID(n float64) string {
	return strings.TrimSuffix(strings.TrimSuffix(fmtFloat(n), ".0"), ".00")
}

func fmtFloat(n float64) string {
	if n == math.Trunc(n) {
		return fmt.Sprintf("%.0f", n)
	}
	return fmt.Sprintf("%v", n)
}

// kbmsNormName canonicalizes a display name for fallback joining: lowercase,
// whitespace collapsed.
func kbmsNormName(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(name)), " ")
}

// kbmsOpenStore opens the local mirror for the read-only novel commands. It
// prefers a read-only handle when the schema already exists (read-only takes no
// write lock, so parallel novel commands don't collide with SQLITE_BUSY) and
// falls back to a read-write open that migrates when the DB is absent or not yet
// built, inside a short retry that rides out the first-run create/migrate race.
func kbmsOpenStore(ctx context.Context, dbPath string) (*store.Store, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("kaseya-bms-cli")
	}
	var lastErr error
	for attempt := 0; attempt < 8; attempt++ {
		if st, ok := kbmsTryReadOnlyMigrated(dbPath); ok {
			return st, nil
		}
		st, err := store.OpenWithContext(ctx, dbPath)
		if err == nil {
			return st, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		time.Sleep(time.Duration(20*(attempt+1)) * time.Millisecond)
	}
	return nil, lastErr
}

// kbmsTryReadOnlyMigrated opens read-only only when the file exists AND the
// resources table is present (a prior read-write open finished migrating).
func kbmsTryReadOnlyMigrated(path string) (*store.Store, bool) {
	if _, err := os.Stat(path); err != nil {
		return nil, false
	}
	st, err := store.OpenReadOnly(path)
	if err != nil {
		return nil, false
	}
	var one int
	if err := st.DB().QueryRow(`SELECT 1 FROM sqlite_master WHERE type='table' AND name='resources' LIMIT 1`).Scan(&one); err != nil {
		_ = st.Close()
		return nil, false
	}
	return st, true
}
