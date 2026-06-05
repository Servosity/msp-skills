// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Shared logic for the hand-written novel commands (registry, pull, export,
// snapshot, diff, trend, describe): alias resolution against the local
// registry, the --where predicate compiler that targets MSPbots'
// comma-encoded operator DSL, response-envelope row extraction, per-row
// identity keys, and column type inference.

package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"mspbots-pp-cli/internal/client"
	"mspbots-pp-cli/internal/store"
)

// resolvedResource is the outcome of alias-or-ID resolution.
type resolvedResource struct {
	Alias        string `json:"alias,omitempty"`
	ResourceID   string `json:"resource_id"`
	ResourceType string `json:"resource_type"`
}

const (
	resourceTypeDataset = "dataset"
	resourceTypeWidget  = "widget"
)

// validateResourceType gates the shared --type flag. allowEmpty covers the
// commands where empty means "defer to the registry / default".
func validateResourceType(t string, allowEmpty bool) error {
	if t == "" {
		if allowEmpty {
			return nil
		}
		return fmt.Errorf("--type must be %s or %s", resourceTypeDataset, resourceTypeWidget)
	}
	if t != resourceTypeDataset && t != resourceTypeWidget {
		return fmt.Errorf("--type must be %s or %s, got %q", resourceTypeDataset, resourceTypeWidget, t)
	}
	return nil
}

// rawResourceIDRe matches bare MSPbots resource IDs (long numeric snowflakes)
// so users can bypass the registry with a raw ID plus --type.
var rawResourceIDRe = regexp.MustCompile(`^[0-9]{10,25}$`)

// openMspbotsStore opens the local SQLite store (creating the novel-command
// tables when missing). dbPath falls back to the CLI's default store path.
func openMspbotsStore(ctx context.Context, dbPath string) (*store.Store, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("mspbots-cli")
	}
	db, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening local store: %w", err)
	}
	if err := db.EnsureMspbotsSchema(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// resolveResource maps a positional <alias-or-id> argument to a concrete
// resource. Registry aliases win; bare numeric IDs are accepted directly with
// typeOverride (default dataset). Unknown aliases return a not-found error
// listing what is registered.
func resolveResource(ctx context.Context, db *store.Store, arg, typeOverride string) (resolvedResource, error) {
	entry, err := db.RegistryGet(ctx, arg)
	switch {
	case err == nil:
		rt := entry.ResourceType
		if typeOverride != "" {
			rt = typeOverride
		}
		// Belt over the registry-add gate: a row written by another process or
		// a hand-edited DB must not smuggle a non-snowflake into the URL path.
		if !rawResourceIDRe.MatchString(entry.ResourceID) {
			return resolvedResource{}, fmt.Errorf("registry entry %q has malformed resource ID %q; remove and re-add it", entry.Alias, entry.ResourceID)
		}
		return resolvedResource{Alias: entry.Alias, ResourceID: entry.ResourceID, ResourceType: rt}, nil
	case errors.Is(err, sql.ErrNoRows):
		if rawResourceIDRe.MatchString(arg) {
			rt := typeOverride
			if rt == "" {
				rt = resourceTypeDataset
			}
			return resolvedResource{ResourceID: arg, ResourceType: rt}, nil
		}
		known, listErr := db.RegistryList(ctx)
		if listErr != nil || len(known) == 0 {
			return resolvedResource{}, notFoundErr(fmt.Errorf("alias %q is not registered and is not a raw resource ID; register it first: mspbots-cli registry add %s <resourceId> --type dataset", arg, arg))
		}
		names := make([]string, 0, len(known))
		for _, e := range known {
			names = append(names, e.Alias)
		}
		return resolvedResource{}, notFoundErr(fmt.Errorf("alias %q is not registered (known aliases: %s)", arg, strings.Join(names, ", ")))
	default:
		return resolvedResource{}, fmt.Errorf("looking up alias %q: %w", arg, err)
	}
}

// resourceEndpointPath returns the Public API path for a resolved resource.
func resourceEndpointPath(r resolvedResource) string {
	return "/api/" + r.ResourceType + "/" + r.ResourceID
}

// compileWhere translates readable predicates into MSPbots' comma-encoded
// operator DSL. One predicate per --where flag, shape: "Column OP value".
//
//	"Status = Open"                  → Status=Open        (equals / contains)
//	"Update Date >= 2026-05-01"      → Update Date=2026-05-01,
//	"Price <= 56.3"                  → Price=,56.3
//	"Price between 12.6,56.3"        → Price=12.6,56.3
//	"Name contains Tod"              → Name=Tod
//	"Status is empty"                → Status=
//
// Column names may contain spaces; the operator is found by scanning for the
// first recognized token. URL encoding happens in the HTTP client.
func compileWhere(wheres []string) (map[string]string, error) {
	params := make(map[string]string, len(wheres))
	for _, w := range wheres {
		col, val, err := compileOneWhere(w)
		if err != nil {
			return nil, err
		}
		if _, dup := params[col]; dup {
			return nil, fmt.Errorf("column %q appears in more than one --where; combine them into a single predicate (e.g. \"%s between A,B\")", col, col)
		}
		params[col] = val
	}
	return params, nil
}

var whereOpRe = regexp.MustCompile(`(?i)\s+(>=|<=|=|between|contains|is\s+empty)\s*`)

func compileOneWhere(w string) (column, value string, err error) {
	trimmed := strings.TrimSpace(w)
	if trimmed == "" {
		return "", "", fmt.Errorf("empty --where predicate")
	}
	loc := whereOpRe.FindStringSubmatchIndex(trimmed)
	if loc == nil {
		return "", "", fmt.Errorf("cannot parse --where %q: expected \"Column >= value\", \"Column <= value\", \"Column = value\", \"Column between A,B\", \"Column contains value\", or \"Column is empty\"", w)
	}
	column = strings.TrimSpace(trimmed[:loc[2]])
	op := strings.ToLower(strings.Join(strings.Fields(trimmed[loc[2]:loc[3]]), " "))
	rest := strings.TrimSpace(trimmed[loc[1]:])
	if column == "" {
		return "", "", fmt.Errorf("cannot parse --where %q: missing column name", w)
	}
	switch op {
	case "is empty":
		if rest != "" {
			return "", "", fmt.Errorf("cannot parse --where %q: \"is empty\" takes no value", w)
		}
		return column, "", nil
	case "=", "contains":
		if rest == "" {
			return "", "", fmt.Errorf("cannot parse --where %q: missing value (use \"is empty\" to match empty values)", w)
		}
		return column, rest, nil
	case ">=":
		if rest == "" {
			return "", "", fmt.Errorf("cannot parse --where %q: missing value", w)
		}
		return column, rest + ",", nil
	case "<=":
		if rest == "" {
			return "", "", fmt.Errorf("cannot parse --where %q: missing value", w)
		}
		return column, "," + rest, nil
	case "between":
		parts := strings.Split(rest, ",")
		if len(parts) < 2 || len(parts)%2 != 0 {
			return "", "", fmt.Errorf("cannot parse --where %q: between needs an even number of comma-separated bounds (A,B or A,B,C,D)", w)
		}
		for i, p := range parts {
			parts[i] = strings.TrimSpace(p)
			if parts[i] == "" {
				return "", "", fmt.Errorf("cannot parse --where %q: empty bound in between", w)
			}
		}
		return column, strings.Join(parts, ","), nil
	default:
		return "", "", fmt.Errorf("cannot parse --where %q: unsupported operator %q", w, op)
	}
}

// extractRows pulls the row array out of a Public API response. The success
// envelope is undocumented, so this tries the shapes MyBatis-Plus-style
// backends commonly emit (bare array; records/rows/list/items/data arrays,
// possibly nested one level under "data") and falls back to nil rows with the
// raw payload preserved so no response shape is ever silently dropped.
func extractRows(raw json.RawMessage) ([]json.RawMessage, bool) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil, false
	}
	if strings.HasPrefix(trimmed, "[") {
		var arr []json.RawMessage
		if err := json.Unmarshal(raw, &arr); err == nil {
			return arr, true
		}
		return nil, false
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, false
	}
	for _, key := range []string{"records", "rows", "list", "items", "data"} {
		v, ok := obj[key]
		if !ok {
			continue
		}
		inner := strings.TrimSpace(string(v))
		if strings.HasPrefix(inner, "[") {
			var arr []json.RawMessage
			if err := json.Unmarshal(v, &arr); err == nil {
				return arr, true
			}
		}
		// One nested level: {"data":{"records":[...]}}
		if key == "data" && strings.HasPrefix(inner, "{") {
			if arr, ok := extractRows(v); ok {
				return arr, true
			}
		}
	}
	return nil, false
}

// rowIdentityKey computes a stable per-row identity for diffing: an id-shaped
// field when one exists, else a content hash over the full row JSON.
func rowIdentityKey(row json.RawMessage) string {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(row, &obj); err == nil {
		for _, k := range []string{"id", "Id", "ID", "uuid", "Uuid", "UUID"} {
			if v, ok := obj[k]; ok {
				s := strings.Trim(strings.TrimSpace(string(v)), `"`)
				if s != "" && s != "null" {
					return "id:" + s
				}
			}
		}
	}
	sum := sha256.Sum256(canonicalRowJSON(row))
	return "hash:" + hex.EncodeToString(sum[:8])
}

// canonicalRowJSON re-marshals a row with sorted keys so hash identity is
// stable across servers that reorder fields between responses. UseNumber
// keeps numbers as their raw text — MSPbots rows carry 19-digit snowflake
// IDs that float64 would silently round, which would collide hash identities
// and mask changed rows in diff.
func canonicalRowJSON(row json.RawMessage) []byte {
	dec := json.NewDecoder(bytes.NewReader(row))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return row
	}
	out, err := json.Marshal(v) // encoding/json sorts map keys; json.Number re-emits verbatim
	if err != nil {
		return row
	}
	return out
}

// fetchResourcePage performs one Public API page fetch for a resolved
// resource, merging pagination and compiled filter params.
//
// Empty-valued filters (the DSL's "is empty" operator encodes as `Column=`)
// are embedded in the path's query string instead of the params map: the
// generated client drops empty params from the map, but pre-embedded query
// keys survive its parse-and-re-encode untouched.
func fetchResourcePage(ctx context.Context, c *client.Client, r resolvedResource, page, size int, filters map[string]string) (json.RawMessage, error) {
	params := map[string]string{
		"current": strconv.Itoa(page),
		"size":    strconv.Itoa(size),
	}
	path := resourceEndpointPath(r)
	var emptyKeys []string
	for k, v := range filters {
		if v == "" {
			emptyKeys = append(emptyKeys, k)
			continue
		}
		params[k] = v
	}
	if len(emptyKeys) > 0 {
		sort.Strings(emptyKeys) // deterministic URLs for tests and caching
		pairs := make([]string, len(emptyKeys))
		for i, k := range emptyKeys {
			pairs[i] = url.QueryEscape(k) + "="
		}
		path += "?" + strings.Join(pairs, "&")
	}
	raw, err := c.Get(ctx, path, params)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

// walkResourcePages fetches pages 1..maxPages, invoking fn on each page's
// rows, terminating on an empty/short page. Returns pages fetched and
// whether the cap stopped the walk with full pages still coming — callers
// surface that so a partial capture is never mistaken for a complete one.
func walkResourcePages(ctx context.Context, c *client.Client, r resolvedResource, maxPages, pageSize int, filters map[string]string, fn func(rows []json.RawMessage) error) (pages int, maxPagesHit bool, err error) {
	if maxPages <= 0 {
		return 0, false, nil
	}
	maxPagesHit = true
	for page := 1; page <= maxPages; page++ {
		raw, err := fetchResourcePage(ctx, c, r, page, pageSize, filters)
		if err != nil {
			return pages, false, fmt.Errorf("fetching page %d: %w", page, err)
		}
		rows, ok := extractRows(raw)
		if !ok || len(rows) == 0 {
			maxPagesHit = false
			break
		}
		pages++
		if err := fn(rows); err != nil {
			return pages, false, err
		}
		if len(rows) < pageSize {
			maxPagesHit = false
			break
		}
	}
	return pages, maxPagesHit, nil
}

// columnInfo is one inferred column in a describe report.
type columnInfo struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	NonNull   int    `json:"non_null"`
	Sampled   int    `json:"sampled"`
	Example   string `json:"example,omitempty"`
	WhereHint string `json:"where_hint,omitempty"`
}

// inferColumns derives column names and types from sampled rows. Mechanical
// only: JSON type plus date-shape sniffing on strings; no LLM involved.
func inferColumns(rows []json.RawMessage) []columnInfo {
	type acc struct {
		types   map[string]int
		nonNull int
		example string
	}
	cols := map[string]*acc{}
	for _, row := range rows {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(row, &obj); err != nil {
			continue
		}
		for k, v := range obj {
			a := cols[k]
			if a == nil {
				a = &acc{types: map[string]int{}}
				cols[k] = a
			}
			t := jsonScalarType(v)
			a.types[t]++
			if t != "null" {
				a.nonNull++
				if a.example == "" {
					a.example = truncateExample(string(v))
				}
			}
		}
	}
	names := make([]string, 0, len(cols))
	for k := range cols {
		names = append(names, k)
	}
	sort.Strings(names)
	out := make([]columnInfo, 0, len(names))
	for _, name := range names {
		a := cols[name]
		out = append(out, columnInfo{
			Name:      name,
			Type:      dominantType(a.types),
			NonNull:   a.nonNull,
			Sampled:   len(rows),
			Example:   a.example,
			WhereHint: whereHintForType(name, dominantType(a.types)),
		})
	}
	return out
}

var dateShapeRe = regexp.MustCompile(`^"\d{4}-\d{2}-\d{2}([T ]\d{2}:\d{2}(:\d{2})?)?`)

func jsonScalarType(v json.RawMessage) string {
	s := strings.TrimSpace(string(v))
	switch {
	case s == "null" || s == "":
		return "null"
	case s == "true" || s == "false":
		return "boolean"
	case strings.HasPrefix(s, `"`):
		if dateShapeRe.MatchString(s) {
			return "date"
		}
		// Numeric strings are common in BI exports; surface them as numbers
		// so trend/--where hints point at the right operators.
		if _, err := strconv.ParseFloat(strings.Trim(s, `"`), 64); err == nil {
			return "number"
		}
		return "text"
	case strings.HasPrefix(s, "{"):
		return "object"
	case strings.HasPrefix(s, "["):
		return "array"
	default:
		if _, err := strconv.ParseFloat(s, 64); err == nil {
			return "number"
		}
		return "text"
	}
}

func dominantType(counts map[string]int) string {
	best, bestN := "", 0
	// Deterministic tie-break: fixed type-priority order so equal counts
	// resolve consistently across runs.
	for _, t := range []string{"number", "date", "boolean", "text", "object", "array"} {
		if n := counts[t]; n > bestN {
			best, bestN = t, n
		}
	}
	if bestN == 0 {
		return "null"
	}
	return best
}

func whereHintForType(name, t string) string {
	switch t {
	case "number":
		return fmt.Sprintf("--where %q", name+" >= 10")
	case "date":
		return fmt.Sprintf("--where %q", name+" >= 2026-01-01")
	case "boolean":
		return fmt.Sprintf("--where %q", name+" = true")
	case "text":
		return fmt.Sprintf("--where %q", name+" contains foo")
	default:
		return ""
	}
}

func truncateExample(s string) string {
	const max = 60
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// numericValue extracts a float from a row column, accepting JSON numbers and
// number-shaped strings (the BI-export norm).
func numericValue(row json.RawMessage, column string) (float64, bool) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(row, &obj); err != nil {
		return 0, false
	}
	v, ok := obj[column]
	if !ok {
		return 0, false
	}
	s := strings.TrimSpace(string(v))
	s = strings.Trim(s, `"`)
	if s == "" || s == "null" {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}
