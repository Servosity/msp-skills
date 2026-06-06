// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"acronis-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

// atoiPositive parses a non-negative integer flag value.
func atoiPositive(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, err
	}
	if n < 0 {
		return 0, fmt.Errorf("want a non-negative integer")
	}
	return n, nil
}

// tenantNames builds a tenant_id -> name lookup from the tenants table. Returns
// an empty map (never nil) if the table is empty or missing.
func tenantNames(db *store.Store) map[string]string {
	names := map[string]string{}
	rows, err := db.Query(`SELECT id, name FROM tenants`)
	if err != nil {
		return names
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var name *string
		if rows.Scan(&id, &name) != nil {
			continue
		}
		names[id] = deref(name)
	}
	return names
}

// deref returns the value of a *string or "" if nil.
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// parseTime tries the timestamp formats SQLite / the sync layer emit.
func parseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// openNovelDB opens the local store for a novel read command, resolving the
// default DB path when dbPath is empty (the pm_load.go pattern). The caller is
// responsible for `defer db.Close()`.
func openNovelDB(cmd *cobra.Command, dbPath string) (*store.Store, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("acronis-cli")
	}
	db, err := store.OpenWithContext(cmd.Context(), dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening local database: %w\nRun 'acronis-cli sync' first.", err)
	}
	return db, nil
}

// wantJSON reports whether the command should emit JSON: explicit --json, or a
// non-terminal stdout (piped / captured), matching the task's "default to JSON
// when non-terminal" contract.
func wantJSON(flags *rootFlags, cmd *cobra.Command) bool {
	if flags != nil && flags.asJSON {
		return true
	}
	return !isTerminal(cmd.OutOrStdout())
}

// encodeJSON writes v as indented JSON honoring --select/--compact/--csv.
func encodeJSON(cmd *cobra.Command, flags *rootFlags, v any) error {
	return printJSONFiltered(cmd.OutOrStdout(), v, flags)
}

// jsonStr pulls the first present, non-empty string-ish value for any of keys
// out of a decoded JSON object.
func jsonStr(obj map[string]any, keys ...string) string {
	for _, k := range keys {
		v, ok := obj[k]
		if !ok || v == nil {
			continue
		}
		s := strings.TrimSpace(fmt.Sprintf("%v", v))
		if s != "" && s != "<nil>" {
			return s
		}
	}
	return ""
}

// jsonNum pulls the first present numeric value for any of keys; returns
// (value, true) on success.
func jsonNum(obj map[string]any, keys ...string) (float64, bool) {
	for _, k := range keys {
		v, ok := obj[k]
		if !ok || v == nil {
			continue
		}
		switch n := v.(type) {
		case float64:
			return n, true
		case json.Number:
			if f, err := n.Float64(); err == nil {
				return f, true
			}
		case int:
			return float64(n), true
		case int64:
			return float64(n), true
		}
	}
	return 0, false
}

// decodeData unmarshals a JSON data column into a map; returns nil on failure.
func decodeData(b []byte) map[string]any {
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return nil
	}
	return m
}

// taskOutcome classifies a task_manager row's state/result_code into one of
// "ok", "failed", "running", or "other".
func taskOutcome(state, resultCode string) string {
	s := strings.ToLower(strings.TrimSpace(state))
	rc := strings.ToLower(strings.TrimSpace(resultCode))
	// Failure signals win.
	for _, f := range []string{"fail", "error", "missed", "cancel", "abort"} {
		if strings.Contains(s, f) || strings.Contains(rc, f) {
			return "failed"
		}
	}
	for _, ok := range []string{"ok", "success", "succeed", "complete", "done"} {
		if strings.Contains(s, ok) || strings.Contains(rc, ok) {
			return "ok"
		}
	}
	for _, r := range []string{"run", "progress", "active", "started", "enqueu", "pending"} {
		if strings.Contains(s, r) {
			return "running"
		}
	}
	return "other"
}

// agentOffline reports whether an agent status string indicates offline.
func agentOffline(status string) bool {
	s := strings.ToLower(strings.TrimSpace(status))
	if s == "" {
		return false
	}
	for _, off := range []string{"offline", "unavailable", "disconnect", "down", "inactive", "lost"} {
		if strings.Contains(s, off) {
			return true
		}
	}
	return false
}

// usageNameOf extracts the offering/metric name from a usages data JSON object.
func usageNameOf(obj map[string]any) string {
	return jsonStr(obj, "name", "offering_item", "usage_name", "offeringItem")
}

// offeringNameOf extracts the offering name from an offering_items data JSON object.
func offeringNameOf(obj map[string]any) string {
	return jsonStr(obj, "name", "offering_item", "application_id", "offeringItem", "applicationId")
}

// usageValueOf extracts a numeric usage value from a usages data JSON object.
func usageValueOf(obj map[string]any) (float64, bool) {
	return jsonNum(obj, "value", "absolute_value", "absoluteValue", "amount", "quantity")
}

// usageEditionOf extracts the edition from a usages data JSON object.
func usageEditionOf(obj map[string]any) string {
	return jsonStr(obj, "edition", "offering_edition", "offeringEdition")
}

// agentOnline reports whether an agent status string indicates online.
func agentOnline(status string) bool {
	s := strings.ToLower(strings.TrimSpace(status))
	for _, on := range []string{"online", "connected", "active", "available", "ok", "ready"} {
		if strings.Contains(s, on) {
			return true
		}
	}
	return false
}
