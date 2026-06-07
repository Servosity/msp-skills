// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
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
func fleetField(obj map[string]any, key string) any {
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

// fleetOrgID extracts an organization identifier, preferring organization_id
// and falling back to the first entry of organization_ids.
func fleetOrgID(obj map[string]any) string {
	if id := fleetStrField(obj, "organization_id"); id != "" {
		return id
	}
	if arr, ok := fleetField(obj, "organization_ids").([]any); ok && len(arr) > 0 {
		return fleetStr(arr[0])
	}
	return ""
}

// fleetTimeLayouts covers the timestamp shapes Action1 returns across endpoints.
var fleetTimeLayouts = []string{
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
