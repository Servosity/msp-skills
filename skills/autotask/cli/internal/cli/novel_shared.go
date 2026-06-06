// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0.
//
// Shared helpers for the transcendence (novel) commands. These commands are
// cross-entity aggregations over the LOCAL SQLite store (synced via `sync`);
// no single Autotask API call returns them. They read with store.List, which
// returns `SELECT data FROM resources WHERE resource_type = ?` rows.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"autotask-pp-cli/internal/store"
)

// novelDBPath returns the DB path for a novel command: --db override or the
// default store location.
func novelDBPath(dbFlag string) string {
	if strings.TrimSpace(dbFlag) != "" {
		return dbFlag
	}
	return defaultDBPath("autotask-cli")
}

// openNovelStore opens the local store for a novel command (--db override or
// the default path), with a sync-pointing error when the open fails.
func openNovelStore(ctx context.Context, dbFlag string) (*store.Store, error) {
	db, err := store.OpenWithContext(ctx, novelDBPath(dbFlag))
	if err != nil {
		return nil, fmt.Errorf("opening local database: %w\nRun 'autotask-cli sync' first.", err)
	}
	return db, nil
}

// listEntity returns every synced record of a resource_type from an open
// store as decoded maps. resourceType is the kebab key the sync layer writes
// (e.g. "tickets", "time-entries", "contract-blocks").
//
// NOTE: store.List treats limit <= 0 as limit = 200 (the generated default for
// interactive reads). Novel aggregations must see EVERY synced row — a silent
// 200-row slice understates totals and corrupts rankings on any real tenant —
// so pass an explicit effectively-unbounded limit instead of 0.
func listEntity(db *store.Store, resourceType string) ([]map[string]any, error) {
	raws, err := db.List(resourceType, math.MaxInt32)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(raws))
	for _, r := range raws {
		var m map[string]any
		if json.Unmarshal(r, &m) == nil {
			out = append(out, m)
		}
	}
	return out, nil
}

// numAt extracts a numeric field as float64 regardless of how JSON decoded it
// (float64, json.Number, int, or numeric string). Guards the store-decode
// json.Number/UseNumber int trap: a naive float64 cast yields 0 for json.Number
// and would zero every aggregation.
func numAt(m map[string]any, keys ...string) (float64, bool) {
	for _, k := range keys {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		switch n := v.(type) {
		case float64:
			return n, true
		case float32:
			return float64(n), true
		case int:
			return float64(n), true
		case int64:
			return float64(n), true
		case json.Number:
			if f, err := n.Float64(); err == nil {
				return f, true
			}
		case string:
			if f, err := strconv.ParseFloat(strings.TrimSpace(n), 64); err == nil {
				return f, true
			}
		}
	}
	return 0, false
}

// intAt is numAt rounded to int64.
func intAt(m map[string]any, keys ...string) (int64, bool) {
	f, ok := numAt(m, keys...)
	return int64(f), ok
}

// strAt returns the first present string-able field value.
func strAt(m map[string]any, keys ...string) string {
	for _, k := range keys {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		switch s := v.(type) {
		case string:
			if strings.TrimSpace(s) != "" {
				return s
			}
		case json.Number:
			return s.String()
		case float64:
			return strconv.FormatFloat(s, 'f', -1, 64)
		case bool:
			return strconv.FormatBool(s)
		}
	}
	return ""
}

// boolAt returns a boolean field (handles bool, "true"/"false", and 1/0).
func boolAt(m map[string]any, keys ...string) bool {
	for _, k := range keys {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		switch b := v.(type) {
		case bool:
			return b
		case string:
			if pb, err := strconv.ParseBool(strings.TrimSpace(b)); err == nil {
				return pb
			}
		case float64:
			return b != 0
		case json.Number:
			if f, err := b.Float64(); err == nil {
				return f != 0
			}
		}
	}
	return false
}

var atTimeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.999",
	"2006-01-02T15:04:05",
	"2006-01-02",
}

// timeAt parses a timestamp field, ok=false when absent/unparseable.
func timeAt(m map[string]any, keys ...string) (time.Time, bool) {
	s := strAt(m, keys...)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range atTimeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// parseNovelDuration accepts Autotask-friendly windows: "24h", "7d", "2w",
// "30m", "90s", or a bare integer (days). Returns the duration.
func parseNovelDuration(arg string) (time.Duration, bool) {
	arg = strings.TrimSpace(strings.ToLower(arg))
	if arg == "" {
		return 0, false
	}
	if n, err := strconv.Atoi(arg); err == nil {
		return time.Duration(n) * 24 * time.Hour, true
	}
	if strings.HasSuffix(arg, "d") {
		if n, err := strconv.Atoi(strings.TrimSuffix(arg, "d")); err == nil {
			return time.Duration(n) * 24 * time.Hour, true
		}
		return 0, false
	}
	if strings.HasSuffix(arg, "w") {
		if n, err := strconv.Atoi(strings.TrimSuffix(arg, "w")); err == nil {
			return time.Duration(n) * 7 * 24 * time.Hour, true
		}
		return 0, false
	}
	if d, err := time.ParseDuration(arg); err == nil {
		return d, true
	}
	return 0, false
}

// ageBucketLabel classifies an age in hours into a service-desk aging bucket.
func ageBucketLabel(hours float64) string {
	switch {
	case hours < 24:
		return "0-1d"
	case hours < 72:
		return "1-3d"
	case hours < 168:
		return "3-7d"
	case hours < 336:
		return "7-14d"
	case hours < 720:
		return "14-30d"
	default:
		return "30d+"
	}
}

// ticketCreated returns the most relevant timestamp for aging a ticket: prefer
// createDate, fall back to lastActivityDate.
func ticketCreated(m map[string]any) (time.Time, bool) {
	if t, ok := timeAt(m, "createDate", "createdDate", "CreateDate"); ok {
		return t, true
	}
	return timeAt(m, "lastActivityDate", "LastActivityDate")
}

// isTicketOpen reports whether a ticket is in an open state. Autotask status 5
// is "Complete"; everything else is open. Missing status => open (never drop).
func isTicketOpen(m map[string]any) bool {
	if s, ok := intAt(m, "status"); ok {
		return s != 5
	}
	return true
}

// accrueBlock folds one contract-block's hours into purchased/used/remaining,
// honoring Autotask's hoursLeft|hoursRemaining vs hoursUsed|hoursApproved
// split: when the block reports what's left, used is derived; when it reports
// what's used, remaining is derived. This is the single source of truth for
// the field-name contract — contract-burn, reconcile, account-brief, and
// retainer all accumulate through it.
func accrueBlock(b map[string]any) (purchased, used, remaining float64) {
	purchased, _ = numAt(b, "hours")
	if r, ok := numAt(b, "hoursLeft", "hoursRemaining"); ok {
		return purchased, purchased - r, r
	}
	if u, ok := numAt(b, "hoursUsed", "hoursApproved"); ok {
		return purchased, u, purchased - u
	}
	return purchased, 0, 0
}
