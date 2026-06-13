// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"syncro-pp-cli/internal/store"
)

// parseAgeDuration parses duration-ish flag strings used by the novel
// analytics commands. It accepts a "Nd" suffix (days → N*24h) on top of the
// standard time.ParseDuration grammar (e.g. "48h", "90m", "30s"). Plain
// integers without a unit are rejected so a typo fails loudly rather than
// being silently treated as nanoseconds.
func parseAgeDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	if strings.HasSuffix(s, "d") {
		nStr := strings.TrimSuffix(s, "d")
		n, err := strconv.ParseFloat(nStr, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid day duration %q: %w", s, err)
		}
		return time.Duration(n * float64(24*time.Hour)), nil
	}
	return time.ParseDuration(s)
}

// novelJSONFieldString returns the string form of a JSON value extracted from
// a map, trying the snake_case key first then a few common variants. Returns
// "" when absent. Numbers are rendered without scientific notation.
func novelJSONString(obj map[string]any, keys ...string) string {
	for _, k := range keys {
		v, ok := obj[k]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case string:
			return t
		case float64:
			if t == float64(int64(t)) {
				return strconv.FormatInt(int64(t), 10)
			}
			return strconv.FormatFloat(t, 'f', -1, 64)
		case json.Number:
			return t.String()
		case bool:
			return strconv.FormatBool(t)
		default:
			return fmt.Sprintf("%v", t)
		}
	}
	return ""
}

// novelParseTimestamp attempts to parse a timestamp string commonly emitted by
// the Syncro API into a time.Time. Returns ok=false when the value is empty or
// unparseable. Handles RFC3339, the SQLite "2006-01-02 15:04:05" form, and a
// bare date.
func novelParseTimestamp(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05Z07:00",
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05 -0700",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, true
		}
	}
	// Unix epoch seconds fallback.
	if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
		return time.Unix(n, 0).UTC(), true
	}
	return time.Time{}, false
}

// novelSyncHint writes a one-line "run sync first" hint to stderr for the
// non-JSON path of the analytics commands. It is intentionally advisory only:
// the commands always exit 0 with an empty result on an empty store.
func novelSyncHint(w interface{ Write([]byte) (int, error) }, msg string) {
	if w == nil {
		return
	}
	fmt.Fprintln(w, msg)
}

// syncroOpenStore opens the local mirror for the read-only novel commands:
// read-only when the schema exists (no write lock, so parallel reads don't
// collide with SQLITE_BUSY), else read-write-migrate, with a short retry to
// ride out the first-run create/migrate race. Non-nil on a nil error.
func syncroOpenStore(ctx context.Context, dbPath string) (*store.Store, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("syncro-cli")
	}
	var lastErr error
	for attempt := 0; attempt < 8; attempt++ {
		if st, ok := syncroTryReadOnlyMigrated(dbPath); ok {
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

func syncroTryReadOnlyMigrated(path string) (*store.Store, bool) {
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
