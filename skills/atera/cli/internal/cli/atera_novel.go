// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Shared helpers for the hand-built transcendence commands (agents stale,
// agents inventory, tickets sla, tickets workload, customers book, alerts
// triage, since). These read the local SQLite store and answer cross-entity /
// time-window questions no single Atera API call can. Every consumer opens the
// store itself (so store access is visible per command) and then uses these
// decode/accessor helpers, which tolerate the json.Number-vs-string ambiguity
// real Atera payloads exhibit.

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"atera-pp-cli/internal/cliutil"
	"atera-pp-cli/internal/store"
)

// nvOpenRead opens the local mirror read-only for the read-only transcendence
// commands. Read-only handles take no write lock, so several novel commands can
// run concurrently against the same mirror without SQLITE_BUSY (the failure the
// scorecard's parallel sample probe surfaced). ok is false (nil store, nil err)
// when no mirror exists yet; nvLoad and the sync hints already tolerate a nil
// store, so callers render that as an empty "run sync first" result.
func nvOpenRead(dbPath string) (*store.Store, bool, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("atera-cli")
	}
	if _, err := os.Stat(dbPath); err != nil {
		return nil, false, nil
	}
	s, err := store.OpenReadOnly(dbPath)
	if err != nil {
		return nil, false, err
	}
	return s, true, nil
}

// nvObj is one decoded resource row: top-level JSON keys → raw values.
type nvObj map[string]json.RawMessage

// nvNow is overridable in tests; production uses the wall clock.
var nvNow = time.Now

// nvLoad reads every synced row of resourceType from the local store as decoded
// objects. Everything synced lands in the generic `resources` table (the typed
// tables are a parallel copy), so a single unbounded query covers agents,
// tickets, customers, contracts, alerts, etc. The query is deliberately
// LIMIT-free: fleet analytics need the whole estate, not List()'s 200-row cap.
// An empty or absent store yields an empty slice so callers emit [] / {} and
// exit 0 rather than erroring.
func nvLoad(s *store.Store, resourceType string) ([]nvObj, error) {
	if s == nil {
		return nil, nil // no mirror yet -> empty result, caller emits [] / {}
	}
	rows, err := s.Query(`SELECT data FROM resources WHERE resource_type = ?`, resourceType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]nvObj, 0)
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var o nvObj
		if err := json.Unmarshal([]byte(data), &o); err != nil {
			continue // skip a malformed row rather than abort the whole report
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// nvStr returns a string field. JSON strings are unquoted; other scalars are
// rendered as their raw text. Missing/null → "".
func nvStr(o nvObj, key string) string {
	raw, ok := o[key]
	if !ok || len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return strings.Trim(string(raw), `"`)
}

// nvInt returns an integer field, tolerating JSON numbers and string-encoded
// numbers (Atera occasionally quotes numeric IDs).
func nvInt(o nvObj, key string) (int64, bool) {
	return cliutil.ExtractInt(map[string]json.RawMessage(o), key)
}

// nvBool returns a boolean field; missing/unparseable → false.
func nvBool(o nvObj, key string) bool {
	raw, ok := o[key]
	if !ok {
		return false
	}
	var b bool
	_ = json.Unmarshal(raw, &b)
	return b
}

// nvTimeLayouts covers the ISO-8601 shapes Atera v3 emits — frequently without
// a timezone, which RFC3339 rejects, so we try several explicit layouts.
var nvTimeLayouts = []string{
	"2006-01-02T15:04:05.9999999Z07:00",
	"2006-01-02T15:04:05.999Z07:00",
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02T15:04:05.9999999",
	"2006-01-02T15:04:05.999",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

// nvTime parses an Atera date-time field to UTC. Returns ok=false on empty or
// unparseable input.
func nvTime(o nvObj, key string) (time.Time, bool) {
	s := nvStr(o, key)
	if s == "" {
		return time.Time{}, false
	}
	if t, ok := cliutil.ParseODataDate(s); ok { // handles /Date(ms)/ and RFC3339
		return t, true
	}
	for _, layout := range nvTimeLayouts {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// nvParseWindow parses a relative window like "24h", "90m", "7d", "2d12h".
// Go's time.ParseDuration lacks a day unit, so a leading "<n>d" is expanded to
// hours before delegating. Returns ok=false on an empty/invalid string.
func nvParseWindow(s string) (time.Duration, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, false
	}
	// Expand a leading day component: "7d" or "2d12h".
	if i := strings.IndexByte(s, 'd'); i > 0 {
		if days, err := strconv.Atoi(s[:i]); err == nil {
			d := time.Duration(days) * 24 * time.Hour
			rest := s[i+1:]
			if rest == "" {
				return d, true
			}
			if extra, err := time.ParseDuration(rest); err == nil {
				return d + extra, true
			}
			return 0, false
		}
	}
	if d, err := time.ParseDuration(s); err == nil && d > 0 {
		return d, true
	}
	return 0, false
}

// nvIsOpenTicket reports whether a ticket status counts as still-open work.
// Atera ships Open/Pending/Resolved/Closed; treat Resolved and Closed as done.
func nvIsOpenTicket(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "closed", "resolved":
		return false
	default:
		return true
	}
}

// nvEmit renders v as JSON (routing through --select/--csv/--compact/--quiet)
// when --json/--agent or a non-terminal stdout; otherwise it calls human().
func nvEmit(cmd *cobra.Command, flags *rootFlags, v any, human func()) error {
	w := cmd.OutOrStdout()
	if flags.asJSON || flags.agent || (!isTerminal(w) && !humanFriendly) {
		return printJSONFiltered(w, v, flags)
	}
	human()
	return nil
}

// nvTable prints an aligned text table for the human (terminal) output path.
func nvTable(w io.Writer, headers []string, rows [][]string) {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(headers, "\t"))
	for _, r := range rows {
		fmt.Fprintln(tw, strings.Join(r, "\t"))
	}
	// Flush errors on a terminal writer are non-actionable here; the human
	// table is best-effort. Explicitly ignore to satisfy errcheck/gosec G104.
	_ = tw.Flush()
}
