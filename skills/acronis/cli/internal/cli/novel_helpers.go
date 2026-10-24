// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

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

// tenantNames builds a tenant_id -> name lookup over every mirrored tenant.
// A query or scan failure is an error: rollups use this map as the tenant set,
// so a silently short map would drop tenants from their results.
func tenantNames(db *store.Store) (map[string]string, error) {
	names := map[string]string{}
	rows, err := db.Query(`SELECT id, name FROM tenants`)
	if err != nil {
		return nil, fmt.Errorf("querying tenants: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var name *string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("reading tenants: %w", err)
		}
		names[id] = deref(name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading tenants: %w", err)
	}
	return names, nil
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
	// A JSON number stored in a DATETIME column: Unix seconds or milliseconds.
	if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
		if n > 1e11 {
			return time.UnixMilli(n).UTC(), true
		}
		return time.Unix(n, 0).UTC(), true
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
	unlock, err := acronisLockMirror(db.Path())
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	db.ReleaseOnClose(unlock)
	if err := acronisRequireCompleteMirror(db); err != nil {
		_ = db.Close()
		return nil, err
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
// "ok", "warning", "failed", "running", or "other". It matches whole values,
// never substrings: only an explicit successful result ("ok") on a task whose
// state is "completed" establishes success. Every terminal result that is not ok or warning -
// including timedout, abandoned, cancelled and any code this connector does not
// know - is a failure, so a new vendor result code can never read as success.
func taskOutcome(state, resultCode string) string {
	s := strings.ToLower(strings.TrimSpace(state))
	rc := strings.ToLower(strings.TrimSpace(resultCode))
	switch s {
	case "enqueued", "assigned", "started", "paused", "running", "queued", "pending", "in_progress":
		if rc == "" {
			return "running"
		}
	}
	switch rc {
	case "ok":
		if s == "completed" {
			return "ok"
		}
		return "other"
	case "warning":
		if s == "completed" {
			return "warning"
		}
		return "other"
	case "":
		switch s {
		case "completed":
			// Finished without a result: nothing proves it succeeded.
			return "failed"
		case "failed", "error", "cancelled", "canceled", "abandoned", "timedout":
			return "failed"
		}
		return "other"
	}
	// Any other result code, known (error, cancelled, abandoned, timedout) or
	// not, is a failure signal.
	return "failed"
}

// taskBackupKind classifies a task row's type: "backup" for an explicit
// backup task, "other" for a type known not to be a backup (restore,
// replication, validation, ...), and "unknown" for anything else - no type, or
// a type this connector does not recognize. Success evidence requires
// "backup"; failure evidence accepts "backup" or "unknown", so a failure is
// hidden only when its type is known not to be a backup.
func taskBackupKind(typ string) string {
	t := strings.ToLower(strings.TrimSpace(typ))
	if t == "backup" {
		return "backup"
	}
	if taskNonBackupTypes[t] {
		return "other"
	}
	return "unknown"
}

// taskNonBackupTypes are task types that are never a backup run.
var taskNonBackupTypes = map[string]bool{
	"restore": true, "recovery": true, "recover": true,
	"replication": true, "replicate": true,
	"validation": true, "validate": true,
	"cleanup": true, "retention": true,
	"conversion": true, "migration": true,
	"discovery": true, "inventory": true, "scan": true,
	"update": true, "install": true, "patch": true,
}

// taskTimeColumns is the SELECT list (in rollupTime order) every task rollup
// uses to date a task_manager row: completion first, then the progressively
// earlier lifecycle stamps.
const taskTimeColumns = `completed_at, updated_at, started_at, enqueued_at, created_at`

// rollupEarliestTime is the earliest stamp a rollup accepts as a real time.
var rollupEarliestTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// rollupTime returns the first non-empty timestamp among candidates, in order.
// known is false when every candidate is empty. A non-empty value that does not
// parse is an error: a rollup must never silently reinterpret or drop it.
func rollupTime(candidates ...string) (t time.Time, known bool, err error) {
	for _, c := range candidates {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if parsed, ok := parseTime(c); ok {
			// A zero or pre-2000 stamp is a placeholder, not a time: reading
			// it as known would push a failure out of every window.
			if parsed.Before(rollupEarliestTime) {
				return time.Time{}, false, nil
			}
			return parsed, true, nil
		}
		return time.Time{}, false, fmt.Errorf("unrecognized timestamp %q in the local mirror; refusing to report from it", c)
	}
	return time.Time{}, false, nil
}

// agentStatusOf normalizes an agent status to "online", "offline" or
// "unknown" by exact value. The store derives status from the API's online
// boolean (store.LookupFieldValue) or its documented online/offline enum; any
// other value is unknown and is never evidence of a working agent.
func agentStatusOf(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "online":
		return "online"
	case "offline":
		return "offline"
	default:
		return "unknown"
	}
}

// agentOnline reports whether an agent status proves the agent is online.
func agentOnline(status string) bool { return agentStatusOf(status) == "online" }

// agentNotOnline reports whether an agent status fails to prove the agent is
// online (offline or unknown).
func agentNotOnline(status string) bool { return !agentOnline(status) }

// offeringIdentity is the full identity of a usage or offering item: the same
// fields the store keys these rows by (store.ExtractResourceID). Rows that
// differ in any field are different licenses or metrics and are never merged.
type offeringIdentity struct {
	ApplicationID string `json:"application_id,omitempty"`
	Name          string `json:"offering"`
	Edition       string `json:"edition,omitempty"`
	InfraID       string `json:"infra_id,omitempty"`
}

// offeringIdentityOf extracts the identity from a usages or offering_items
// data JSON object.
func offeringIdentityOf(obj map[string]any) offeringIdentity {
	return offeringIdentity{
		ApplicationID: jsonStr(obj, "application_id", "applicationId"),
		Name:          jsonStr(obj, "name", "offering_item", "offeringItem", "usage_name", "usageName"),
		Edition:       jsonStr(obj, "edition", "offering_edition", "offeringEdition"),
		InfraID:       jsonStr(obj, "infra_id", "infraId"),
	}
}

// offeringStatusOf reads an offering item's status: active (1), inactive (0)
// or unknown (absent or any other value). Only an explicit active status makes
// an offering item a license.
func offeringStatusOf(obj map[string]any) string {
	v, ok := obj["status"]
	if !ok || v == nil {
		return "unknown"
	}
	switch s := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", v))); s {
	case "1", "active", "true", "enabled":
		return "active"
	case "0", "inactive", "false", "disabled":
		return "inactive"
	default:
		return "unknown"
	}
}

// usageValueOf extracts a numeric usage value from a usages data JSON object.
func usageValueOf(obj map[string]any) (float64, bool) {
	return jsonNum(obj, "value", "absolute_value", "absoluteValue", "amount", "quantity")
}

// decodeRow decodes a data column, failing loudly: a row the rollup cannot read
// must not silently disappear from its result.
func decodeRow(table string, data []byte) (map[string]any, error) {
	obj := decodeData(data)
	if obj == nil {
		return nil, fmt.Errorf("undecodable %s row in the local mirror; re-run 'acronis-cli sync --full'", table)
	}
	return obj, nil
}

// activeOfferingCounts counts, per tenant, the offering items whose status
// proves them active (offeringStatusOf). tenantID, when non-empty, restricts
// the scan to that tenant.
func activeOfferingCounts(db *store.Store, tenantID string) (map[string]int, error) {
	query := `SELECT tenants_id, data FROM offering_items`
	var args []any
	if tenantID != "" {
		query += ` WHERE tenants_id = ?`
		args = append(args, tenantID)
	}
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying offering_items: %w", err)
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var tid string
		var data []byte
		if err := rows.Scan(&tid, &data); err != nil {
			return nil, fmt.Errorf("reading offering_items: %w", err)
		}
		obj, err := decodeRow("offering_items", data)
		if err != nil {
			return nil, err
		}
		if offeringStatusOf(obj) == "active" {
			counts[tid]++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading offering_items: %w", err)
	}
	return counts, nil
}

// countActiveOfferings is activeOfferingCounts for one tenant.
func countActiveOfferings(db *store.Store, tenantID string) (int, error) {
	counts, err := activeOfferingCounts(db, tenantID)
	if err != nil {
		return 0, err
	}
	return counts[tenantID], nil
}
