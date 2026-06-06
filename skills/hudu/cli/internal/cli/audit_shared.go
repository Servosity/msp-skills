// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored transcendence support: shared helpers for the audit/onboard/reconcile
// features. These compute documentation-hygiene insights over the local SQLite mirror
// that no single Hudu API call returns.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"hudu-pp-cli/internal/store"
)

// openAuditStore opens the local SQLite mirror read-for-query. A missing
// database is created+migrated empty, so audits return empty results (not an
// error) before the first sync — keeping verify/--dry-run probes at exit 0.
func openAuditStore(ctx context.Context, dbPath string) (*store.Store, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("hudu-cli")
	}
	db, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening local database: %w\nRun 'hudu-cli sync' first.", err)
	}
	return db, nil
}

// emitAudit renders a structured result as JSON (when --json/--agent or piped)
// or via the supplied human renderer. Routing through printJSONFiltered picks
// up --select/--compact for free, per the agent build checklist.
func emitAudit(cmd *cobra.Command, flags *rootFlags, result any, human func(w io.Writer)) error {
	w := cmd.OutOrStdout()
	if flags.asJSON || (!isTerminal(w) && !flags.csv && !flags.quiet && !flags.plain) {
		return printJSONFiltered(w, result, flags)
	}
	human(w)
	return nil
}

// parseAgeDays parses a threshold like "180d", "26w", "6m", "2y", or a bare
// integer (interpreted as days). Months are approximated as 30 days, years as
// 365 — staleness thresholds don't need calendar precision.
func parseAgeDays(s string) (int, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	unit := s[len(s)-1]
	mult := 1
	numPart := s
	switch unit {
	case 'd':
		mult, numPart = 1, s[:len(s)-1]
	case 'w':
		mult, numPart = 7, s[:len(s)-1]
	case 'm':
		mult, numPart = 30, s[:len(s)-1]
	case 'y':
		mult, numPart = 365, s[:len(s)-1]
	default:
		// no recognized suffix: treat the whole string as a day count
	}
	n, err := strconv.Atoi(strings.TrimSpace(numPart))
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q (use e.g. 180d, 26w, 6m, 2y, or a number of days)", s)
	}
	if n < 0 {
		return 0, fmt.Errorf("duration must not be negative: %q", s)
	}
	return n * mult, nil
}

// parseHuduTime tolerantly parses the timestamp shapes Hudu emits.
func parseHuduTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05 -0700",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// ageDays returns whole days since the timestamp (negative if in the future).
func ageDays(iso string, now time.Time) (int, bool) {
	t, ok := parseHuduTime(iso)
	if !ok {
		return 0, false
	}
	return int(now.Sub(t).Hours() / 24), true
}

// daysUntil returns whole days until the timestamp (negative if already past).
func daysUntil(iso string, now time.Time) (int, bool) {
	t, ok := parseHuduTime(iso)
	if !ok {
		return 0, false
	}
	return int(t.Sub(now).Hours() / 24), true
}

// normLabel normalizes a field label for case/whitespace-insensitive matching.
func normLabel(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// isFilledValue reports whether a custom-field value counts as populated.
func isFilledValue(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(x) != ""
	case bool:
		return x
	case float64:
		return true
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	default:
		return true
	}
}

// filledLabels extracts the set of populated custom-field labels from an
// asset's `custom_fields`. Hudu has shipped two shapes over its versions:
//   - an array whose single element maps label -> value: [{"CPU":"i7","RAM":""}]
//   - an array of {label,value} objects: [{"label":"CPU","value":"i7"}]
//   - rarely, a bare object mapping label -> value
//
// All three are handled. The returned map is keyed by normalized label.
func filledLabels(raw json.RawMessage) map[string]bool {
	out := map[string]bool{}
	if len(raw) == 0 {
		return out
	}
	collectFromMap := func(m map[string]any) {
		// {label,value} object form
		if lv, ok := m["label"]; ok {
			label, _ := lv.(string)
			if label != "" {
				val := m["value"]
				out[normLabel(label)] = isFilledValue(val)
				return
			}
		}
		// label -> value map form
		for k, v := range m {
			out[normLabel(k)] = isFilledValue(v)
		}
	}
	var arr []map[string]any
	if err := json.Unmarshal(raw, &arr); err == nil {
		for _, m := range arr {
			collectFromMap(m)
		}
		return out
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil {
		collectFromMap(obj)
	}
	return out
}

type layoutField struct {
	Label    string
	Required bool
}

// layoutFields extracts the field schema from an asset layout's `fields` array.
// Each field is an object with at least a label; `required` defaults to false.
func layoutFields(raw json.RawMessage) []layoutField {
	var fields []layoutField
	if len(raw) == 0 {
		return fields
	}
	var arr []map[string]any
	if err := json.Unmarshal(raw, &arr); err != nil {
		return fields
	}
	for _, m := range arr {
		label := firstString(m, "label", "Label", "name", "field_name")
		if label == "" {
			continue
		}
		req := false
		if v, ok := m["required"].(bool); ok {
			req = v
		} else if v, ok := m["Required"].(bool); ok {
			req = v
		}
		fields = append(fields, layoutField{Label: label, Required: req})
	}
	return fields
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// queryDataRows runs a SELECT that returns a single `data` JSON column per row
// and returns the raw JSON messages. Used by audits that need the full object.
func queryDataRows(ctx context.Context, db *store.Store, query string, args ...any) ([]json.RawMessage, error) {
	rows, err := db.DB().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []json.RawMessage
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		out = append(out, json.RawMessage(data))
	}
	return out, rows.Err()
}

// asString coerces a JSON value (string or number) to a display string.
func asString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	default:
		return fmt.Sprintf("%v", x)
	}
}

// loadCompanyNames returns a map of company id -> name from the local mirror.
// Returns an empty map on any error so callers degrade to id-only display.
func loadCompanyNames(ctx context.Context, db *store.Store) map[int]string {
	names := map[int]string{}
	rows, err := db.DB().QueryContext(ctx, `SELECT id, name FROM companies`)
	if err != nil {
		return names
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			continue
		}
		n, _ := strconv.Atoi(id)
		names[n] = name
	}
	return names
}

// layoutSchema is the normalized field schema of one asset layout.
type layoutSchema struct {
	Name     string
	Required []string
	All      []string
}

// labelSet returns the layout's full label set for presence checks.
func (s layoutSchema) labelSet() map[string]bool {
	set := make(map[string]bool, len(s.All))
	for _, l := range s.All {
		set[l] = true
	}
	return set
}

// loadLayoutSchemas reads asset_layouts from the local mirror and normalizes
// each layout's field labels (required + all) keyed by layout id. Shared by
// the completeness, layout-drift, and summary audits so they agree on how a
// layout's schema is read.
func loadLayoutSchemas(ctx context.Context, db *store.Store) (map[int]layoutSchema, error) {
	rows, err := queryDataRows(ctx, db, `SELECT data FROM asset_layouts`)
	if err != nil {
		return nil, fmt.Errorf("reading asset layouts: %w", err)
	}
	schemas := map[int]layoutSchema{}
	for _, raw := range rows {
		var m map[string]any
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		id := intField(m, "id")
		if id == 0 {
			continue
		}
		fb, _ := json.Marshal(m["fields"])
		var req, all []string
		for _, f := range layoutFields(fb) {
			all = append(all, normLabel(f.Label))
			if f.Required {
				req = append(req, normLabel(f.Label))
			}
		}
		schemas[id] = layoutSchema{Name: asString(m["name"]), Required: req, All: all}
	}
	return schemas, nil
}
