// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: shared helpers for the IT Glue novel (transcendence) commands.
//
// IT Glue is a JSON:API service. The sync command stores each resource as the
// raw JSON:API resource object — `{"id":"123","type":"organizations",
// "attributes":{...},"relationships":{...}}` — so the local store stays
// byte-consistent with the live API. These helpers parse that shape into a
// clean, flat projection the transcendence commands (search, coverage, changes,
// org brief, contacts dupes, passwords stale) all share. The generated
// list/get/sync code is intentionally left untouched.

package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"itglue-pp-cli/internal/store"
)

// itgResourceTypes is the set of resource_type keys this CLI syncs, matching
// the spec's five resources and sync.go's resource list.
var itgResourceTypes = []string{"organizations", "contacts", "passwords", "configurations", "documents"}

// itgMaxScan bounds a full-store scan. MSP tenants are well under this; the cap
// only guards against an unbounded query.
const itgMaxScan = 1_000_000

// itgRecord is a parsed IT Glue JSON:API resource object from the local store.
type itgRecord struct {
	ID         string
	Type       string
	Attributes map[string]any
	Raw        json.RawMessage
}

// parseITGRecord unmarshals a stored record. When the record carries a JSON:API
// `attributes` object it is used as the attribute map; otherwise the top-level
// object is treated as the attributes (defensive against flat shapes).
func parseITGRecord(raw json.RawMessage) (itgRecord, bool) {
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return itgRecord{}, false
	}
	rec := itgRecord{Raw: raw, Attributes: map[string]any{}}
	rec.ID = itgString(obj["id"])
	if t, ok := obj["type"].(string); ok {
		rec.Type = t
	}
	if attrs, ok := obj["attributes"].(map[string]any); ok {
		rec.Attributes = attrs
	} else {
		rec.Attributes = obj
	}
	return rec, true
}

// itgString renders a JSON-decoded value as a string without scientific
// notation for integral numbers (IT Glue ids decode as float64).
func itgString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return t.String()
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

// attr returns the first non-empty attribute among keys, as a string.
func (r itgRecord) attr(keys ...string) string {
	for _, k := range keys {
		if v, ok := r.Attributes[k]; ok {
			if s := strings.TrimSpace(itgString(v)); s != "" {
				return s
			}
		}
	}
	return ""
}

// orgID resolves the owning organization id. For an organization record this is
// its own id; for child resources it is the `organization-id` attribute.
func (r itgRecord) orgID() string {
	if r.Type == "organizations" {
		return r.ID
	}
	return r.attr("organization-id", "organization_id")
}

// orgName resolves the owning organization name.
func (r itgRecord) orgName() string {
	if r.Type == "organizations" {
		return r.attr("name")
	}
	return r.attr("organization-name", "organization_name")
}

// displayName derives a human-facing name for the record by resource type.
func (r itgRecord) displayName() string {
	switch r.Type {
	case "contacts":
		if n := r.attr("name"); n != "" {
			return n
		}
		full := strings.TrimSpace(r.attr("first-name") + " " + r.attr("last-name"))
		if full != "" {
			return full
		}
	}
	return r.attr("name", "short-name", "title", "hostname", "asset-tag")
}

// contactEmail returns a contact's primary (or first) email. IT Glue stores
// emails as a `contact-emails` array of {value, primary} objects, but flatter
// shapes (an `email` attribute) are also honored.
func (r itgRecord) contactEmail() string {
	if e := r.attr("email", "primary-email"); e != "" {
		return e
	}
	raw, ok := r.Attributes["contact-emails"]
	if !ok {
		return ""
	}
	list, ok := raw.([]any)
	if !ok {
		return ""
	}
	first := ""
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		val := strings.TrimSpace(itgString(m["value"]))
		if val == "" {
			continue
		}
		if first == "" {
			first = val
		}
		if p, _ := m["primary"].(bool); p {
			return val
		}
	}
	return first
}

// updatedAtString returns the raw `updated-at` attribute string.
func (r itgRecord) updatedAtString() string {
	return r.attr("updated-at", "updated_at")
}

// updatedAt parses the record's `updated-at` attribute.
func (r itgRecord) updatedAt() (time.Time, bool) {
	return parseITGTime(r.updatedAtString())
}

// itgTimeLayouts covers the ISO-8601 shapes IT Glue emits plus a date-only
// fallback.
var itgTimeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.000Z07:00",
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05 UTC",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

// parseITGTime parses an IT Glue timestamp string, trying each known layout.
func parseITGTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range itgTimeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// parseSinceArg interprets a --since value as either an absolute timestamp
// (any itgTimeLayout) or a relative window ("7d", "24h", "30m", "90s"). The
// reference time `now` is passed in so callers stay deterministic/testable.
func parseSinceArg(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	if t, ok := parseITGTime(s); ok {
		return t, nil
	}
	// "Nd" → N days; otherwise defer to time.ParseDuration (h/m/s).
	if strings.HasSuffix(s, "d") {
		if n, err := strconv.Atoi(strings.TrimSuffix(s, "d")); err == nil {
			return now.Add(-time.Duration(n) * 24 * time.Hour), nil
		}
	}
	if d, err := time.ParseDuration(s); err == nil {
		return now.Add(-d), nil
	}
	return time.Time{}, fmt.Errorf("invalid --since %q: use a date (2006-01-02), an RFC3339 timestamp, or a window like 7d/24h", s)
}

// summary projects a record to a clean, flat map for agent/table output.
func (r itgRecord) summary() map[string]any {
	m := map[string]any{
		"resource_type": r.Type,
		"id":            r.ID,
		"name":          r.displayName(),
	}
	if org := r.orgName(); org != "" {
		m["organization"] = org
	}
	if oid := r.orgID(); oid != "" {
		m["organization_id"] = oid
	}
	if u := r.updatedAtString(); u != "" {
		m["updated_at"] = u
	}
	switch r.Type {
	case "configurations":
		itgAddIf(m, "serial_number", r.attr("serial-number"))
		itgAddIf(m, "primary_ip", r.attr("primary-ip"))
		itgAddIf(m, "hostname", r.attr("hostname"))
	case "contacts":
		itgAddIf(m, "email", r.contactEmail())
	case "passwords":
		itgAddIf(m, "username", r.attr("username"))
		itgAddIf(m, "url", r.attr("url", "resource-url"))
	}
	return m
}

// itgAddIf sets m[k]=v only when v is non-empty.
func itgAddIf(m map[string]any, k, v string) {
	if strings.TrimSpace(v) != "" {
		m[k] = v
	}
}

// normalizeDupeKey lowercases and collapses internal whitespace so
// "Jane  Doe" and "jane doe" compare equal for duplicate detection.
func normalizeDupeKey(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(s))), " ")
}

// ftsQuote wraps each whitespace-separated term as an FTS5 quoted phrase so
// query text containing FTS5 special characters — hyphens in serial numbers
// ("SN-ABC123"), colons, parentheses, the bareword "NOT/AND/OR" operators — is
// matched literally instead of crashing the MATCH parser with a SQL logic
// error. Multiple terms stay space-separated (implicit AND). An empty query
// yields an empty phrase that matches nothing rather than erroring.
func ftsQuote(q string) string {
	fields := strings.Fields(q)
	if len(fields) == 0 {
		return `""`
	}
	quoted := make([]string, len(fields))
	for i, f := range fields {
		quoted[i] = `"` + strings.ReplaceAll(f, `"`, `""`) + `"`
	}
	return strings.Join(quoted, " ")
}

// openITGStore opens the default local SQLite store with a clear "run sync"
// hint on failure.
func openITGStore(cmd *cobra.Command) (*store.Store, error) {
	dbPath := defaultDBPath("itglue-cli")
	db, err := store.OpenWithContext(cmd.Context(), dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening local database: %w\nRun 'itglue-cli sync --full' first to populate the local store", err)
	}
	return db, nil
}

// listITGRecords lists and parses every stored record of one resource type.
func listITGRecords(db *store.Store, resourceType string) ([]itgRecord, error) {
	raws, err := db.List(resourceType, itgMaxScan)
	if err != nil {
		return nil, err
	}
	recs := make([]itgRecord, 0, len(raws))
	for _, raw := range raws {
		if rec, ok := parseITGRecord(raw); ok {
			recs = append(recs, rec)
		}
	}
	return recs, nil
}
