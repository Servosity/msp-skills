// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built transcendence layer for the Action1 CLI.
//
// Action1's REST API is organization-siloed: nearly every endpoint requires an
// {orgId} path parameter and returns only that one organization's data, with no
// fleet-wide aggregation and no history. These fleet commands read the locally
// synced SQLite store, join and rank across every synced organization, and
// (for patch-drift) keep a time-series the API itself cannot provide.
//
// Shared store-read + coercion + output helpers live here so each fleet command
// stays small. All fleet commands are read-only and tolerate an empty store by
// returning an empty result with exit 0.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"action1-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

// fleetEnvOrg returns the default organization filter from ACTION1_ORG_ID, used
// as the default value of every fleet --org flag so an MSP working a single
// client can scope the whole fleet view with one env var. An explicit --org
// still overrides it.
func fleetEnvOrg() string {
	return strings.TrimSpace(os.Getenv("ACTION1_ORG_ID"))
}

// fleetOpenStore opens the local SQLite store at the default path (or the
// supplied --db override).
func fleetOpenStore(ctx context.Context, dbPath string) (*store.Store, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("action1-cli")
	}
	db, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening local database: %w\nRun 'action1-cli sync' first.", err)
	}
	return db, nil
}

// fleetLoadAll loads and decodes every synced record of a resource_type from the
// generic resources table. Returns an empty slice (not an error) when nothing is
// synced. Undecodable rows are skipped rather than aborting the whole rollup.
func fleetLoadAll(ctx context.Context, db *store.Store, resourceType string) ([]map[string]any, error) {
	rows, err := db.DB().QueryContext(ctx, `SELECT data FROM resources WHERE resource_type = ?`, resourceType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		obj, err := store.DecodeJSONObject(json.RawMessage(data))
		if err != nil {
			continue
		}
		out = append(out, obj)
	}
	return out, rows.Err()
}

// fleetField resolves a field value trying snake/camel/Pascal casing. Unlike
// store.LookupFieldValue (which flattens nested objects/arrays to JSON strings
// for SQLite columns), this returns the RAW decoded value, so nested summaries
// like missing_updates stay map[string]any and numbers stay json.Number.
// A report-data row (InstalledSoftwareReportDataFields and every other
// /reportdata and /installed-software row) carries its columns under "fields"
// with display names ("Name", "Update Status"): a key the row does not carry
// at the top level is read there too (action1 handfix
// fleet-reads-documented-shapes; Codex r1 action1 class wrong field read from
// documented response).
func fleetField(obj map[string]any, key string) any {
	if v := fleetTopField(obj, key); v != nil {
		return v
	}
	if fields, ok := obj["fields"].(map[string]any); ok {
		if v := fleetTopField(fields, key); v != nil {
			return v
		}
		if v, ok := fields[fleetDisplayName(key)]; ok {
			return v
		}
	}
	return nil
}

// fleetDisplayName is a report column's display name: words capitalized and
// joined by spaces (update_status is "Update Status").
func fleetDisplayName(key string) string {
	parts := strings.Split(key, "_")
	for i, p := range parts {
		if p != "" {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

func fleetTopField(obj map[string]any, key string) any {
	if v, ok := obj[key]; ok {
		return v
	}
	parts := strings.Split(key, "_")
	for i := 1; i < len(parts); i++ {
		if parts[i] == "" {
			continue
		}
		parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
	}
	camel := strings.Join(parts, "")
	if v, ok := obj[camel]; ok {
		return v
	}
	if parts[0] != "" {
		pascal := strings.ToUpper(parts[0][:1]) + parts[0][1:] + strings.Join(parts[1:], "")
		if v, ok := obj[pascal]; ok {
			return v
		}
	}
	return nil
}

// fleetNum coerces a json.Number / float64 / int / numeric-string to float64.
// The store decodes JSON with UseNumber(), so numeric fields arrive as
// json.Number; Action1 also returns some numbers as JSON strings ("9.8"). Both
// must be handled or numeric aggregation silently drops to zero.
func fleetNum(v any) (float64, bool) {
	switch n := v.(type) {
	case nil:
		return 0, false
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case string:
		s := strings.TrimSpace(n)
		if s == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(s, 64)
		return f, err == nil
	}
	return 0, false
}

// fleetNumField is fleetNum applied to a resolved field.
func fleetNumField(obj map[string]any, key string) float64 {
	f, _ := fleetNum(fleetField(obj, key))
	return f
}

// fleetStr coerces a field value to a trimmed string.
func fleetStr(v any) string {
	switch s := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(s)
	case json.Number:
		return s.String()
	case bool:
		if s {
			return "true"
		}
		return "false"
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", s))
	}
}

// fleetStrField resolves a field and returns it as a trimmed string.
func fleetStrField(obj map[string]any, key string) string {
	return fleetStr(fleetField(obj, key))
}

// fleetBoolish reports whether a field reads as truthy across the shapes
// Action1 uses (bool, "Yes"/"true"/"1", non-zero number).
func fleetBoolish(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "yes", "true", "1", "required", "y":
			return true
		}
		return false
	case json.Number:
		f, _ := t.Float64()
		return f != 0
	case float64:
		return t != 0
	}
	return false
}

// fleetNested returns a nested object field as a map, or nil.
func fleetNested(obj map[string]any, key string) map[string]any {
	if m, ok := fleetField(obj, key).(map[string]any); ok {
		return m
	}
	return nil
}

// fleetOrgID extracts an organization identifier. A row sync fetched by
// walking the organizations (action1 handfix org-fan-out-sync) carries the
// organization it was read from as parent_id; automation instances and
// installed software carry no organization field of their own. Otherwise
// organization_id, then the first entry of organization_ids.
func fleetOrgID(obj map[string]any) string {
	if id := fleetStrField(obj, "parent_id"); id != "" {
		return id
	}
	if id := fleetStrField(obj, "organization_id"); id != "" {
		return id
	}
	if arr, ok := fleetField(obj, "organization_ids").([]any); ok && len(arr) > 0 {
		return fleetStr(arr[0])
	}
	return ""
}

// fleetTimeLayouts covers the timestamp shapes Action1 returns across endpoints.
// The first is the one spec.json documents for last_seen, start_time,
// published_date and remediation_deadline (YYYY-MM-DD_HH-mm-ss, Endpoint
// last_seen example "2022-08-29_12-22-11"; Codex r1 action1 class wrong
// timestamp parsing from documented response). RFC 3339 reads the ISO times
// with fractional seconds ("2024-06-27T10:20:40.000Z").
var fleetTimeLayouts = []string{
	"2006-01-02_15-04-05",
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02",
	"2006/01/02 15:04:05",
	"2006/01/02",
	"01/02/2006 15:04:05",
	"01/02/2006",
}

// fleetParseTime parses a timestamp string trying the known Action1 layouts.
func fleetParseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, l := range fleetTimeLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// fleetLastSeen reads an endpoint's last check-in: known when it parses,
// unreadable when a value is set that no documented layout reads (it is then
// never taken as recent: the dark-agent and staleness views count it).
func fleetLastSeen(e map[string]any) (t time.Time, known, unreadable bool) {
	s := fleetStrField(e, "last_seen")
	if s == "" {
		return time.Time{}, false, false
	}
	if t, ok := fleetParseTime(s); ok {
		return t, true, false
	}
	return time.Time{}, false, true
}

// fleetOnlineStatusOffline interprets Endpoint.online_status as spec.json
// documents it (#/components/schemas/OnlineStatus: SUCCESS = last seen within
// 7 days, WARNING = 8 to 30 days ago, ERROR = more than 30 days ago; Codex r1
// action1 class wrong status interpretation from documented response). The
// connection words of other shapes (Online, Connected, Up) read as online. Any
// other value is reported, never taken as online; an empty one is unknown.
func fleetOnlineStatusOffline(s string) bool {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "", "SUCCESS", "ONLINE", "CONNECTED", "UP":
		return false
	}
	return true
}

// fleetCounts reads one of an endpoint's documented count summaries
// (missing_updates and vulnerabilities: {critical, other}, which Action1
// returns only when the endpoint was read with fields=* or that field). known
// is false when the summary is absent or holds neither count: an unknown
// posture, never zero (action1 handfix fleet-absent-is-unknown; Codex r1
// action1 class missing endpoint reported as remediated).
func fleetCounts(e map[string]any, key string) (critical, other int, known bool) {
	m := fleetNested(e, key)
	if m == nil {
		return 0, 0, false
	}
	c, okC := fleetNum(m["critical"])
	o, okO := fleetNum(m["other"])
	return int(c), int(o), okC || okO
}

// fleetEmit writes rows as field-filtered JSON (the default and --agent path, and
// whenever stdout is not a terminal) or a human table. jsonRows should be a slice
// or struct that marshals cleanly; header/matrix drive the human table.
func fleetEmit(cmd *cobra.Command, flags *rootFlags, jsonRows any, header []string, matrix [][]string) error {
	out := cmd.OutOrStdout()
	if flags.asJSON || (!isTerminal(out) && !humanFriendly) {
		return printJSONFiltered(out, jsonRows, flags)
	}
	if len(matrix) == 0 {
		fmt.Fprintln(out, "No matching records. Run 'action1-cli sync' first if the local store is empty.")
		return nil
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(header, "\t"))
	for _, r := range matrix {
		fmt.Fprintln(tw, strings.Join(r, "\t"))
	}
	return tw.Flush()
}

// fleetItoa renders a float that is really an integer count without a trailing .0.
func fleetItoa(f float64) string {
	return strconv.FormatInt(int64(f), 10)
}

// fmtFloat renders a score with up to one decimal place, trimming trailing zeros.
func fmtFloat(f float64) string {
	s := strconv.FormatFloat(f, 'f', 1, 64)
	return strings.TrimSuffix(s, ".0")
}
