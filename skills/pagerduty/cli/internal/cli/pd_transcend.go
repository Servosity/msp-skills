// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Shared helpers for PagerDuty transcendence commands (pulse, oncall who/hours,
// audit coverage, insights mttr/responders/noisy, incidents timeline). These
// commands read the local SQLite mirror only — they never call the live API —
// so they exit 0 with an empty result when the store has not been synced yet.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"pagerduty-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

// pdStorePath resolves the local store path the sync command writes to.
func pdStorePath() string {
	return defaultDBPath("pagerduty-cli")
}

// pdLoadResource reads every row of a per-resource store table and decodes each
// row's `data` JSON column into a map. It returns an empty slice (never an
// error) when the store file does not exist yet or the table is empty/missing,
// so every transcendence command exits 0 on a machine that has never synced.
//
// `table` is always a fixed internal identifier chosen by the caller (one of
// the generated table names) — never user input — so direct interpolation into
// the identifier position is safe.
func pdLoadResource(ctx context.Context, table string) ([]map[string]any, error) {
	dbPath := pdStorePath()
	if _, err := os.Stat(dbPath); err != nil {
		return nil, nil // no store yet → empty
	}
	st, err := store.OpenReadOnly(dbPath)
	if err != nil {
		return nil, err
	}
	defer st.Close()

	rows, err := st.DB().QueryContext(ctx, `SELECT data FROM "`+table+`"`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil // older/partial store → empty
		}
		return nil, err
	}
	defer rows.Close()

	var out []map[string]any
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		obj, derr := store.DecodeJSONObject(json.RawMessage(data))
		if derr != nil {
			continue // skip an undecodable row rather than fail the whole report
		}
		out = append(out, obj)
	}
	return out, rows.Err()
}

// pdHintSync emits the framework's unsynced/stale stderr hints for the primary
// resource backing a transcendence command. Transcendence commands open the
// store per-load via pdLoadResource, so this opens a short-lived read-only
// handle just for hint state. A missing store file emits the unsynced hint
// directly (there is no handle to inspect yet).
func pdHintSync(cmd *cobra.Command, flags *rootFlags, resourceType string) {
	dbPath := pdStorePath()
	if _, err := os.Stat(dbPath); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "hint: local store has not been synced yet. Run 'pagerduty-cli sync' before trusting local results.\n")
		return
	}
	st, err := store.OpenReadOnly(dbPath)
	if err != nil {
		return
	}
	defer st.Close()
	if !hintIfUnsynced(cmd, st, resourceType) {
		var maxAge time.Duration
		if flags != nil {
			maxAge = flags.maxAge
		}
		hintIfStale(cmd, st, resourceType, maxAge)
	}
}

// ---- field accessors over DecodeJSONObject maps (numbers are json.Number) ----

func pdString(obj map[string]any, key string) string {
	if obj == nil {
		return ""
	}
	v, ok := obj[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", t)
	}
}

// pdMap returns a nested object (PagerDuty "reference" objects like
// service/escalation_policy/user/agent are nested maps).
func pdMap(obj map[string]any, key string) map[string]any {
	if obj == nil {
		return nil
	}
	if m, ok := obj[key].(map[string]any); ok {
		return m
	}
	return nil
}

// pdSlice returns a nested array of objects (escalation_rules, targets,
// assignments, acknowledgements, teams, ...).
func pdSlice(obj map[string]any, key string) []map[string]any {
	if obj == nil {
		return nil
	}
	raw, ok := obj[key].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(raw))
	for _, e := range raw {
		if m, ok := e.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// pdRefLabel returns a human label for a PagerDuty reference object, preferring
// summary, then name, then id.
func pdRefLabel(ref map[string]any) string {
	if ref == nil {
		return ""
	}
	for _, k := range []string{"summary", "name", "id"} {
		if s := pdString(ref, k); s != "" {
			return s
		}
	}
	return ""
}

// ---- time helpers ----

// pdParseTime parses a PagerDuty RFC3339 timestamp. ok is false on empty or
// unparseable input.
func pdParseTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05Z07:00", "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// pdWindow is a resolved [since, until] time window.
type pdWindow struct {
	Since time.Time
	Until time.Time
}

func (w pdWindow) contains(t time.Time) bool {
	if !w.Since.IsZero() && t.Before(w.Since) {
		return false
	}
	if !w.Until.IsZero() && t.After(w.Until) {
		return false
	}
	return true
}

// pdParseWindow resolves --since / --until flag values. Each accepts a relative
// duration suffixed with d/h/m (e.g. "30d", "24h", "90m") or an absolute
// RFC3339 timestamp. An empty --since defaults to 30 days before now; an empty
// --until defaults to now. `now` is injected for deterministic tests.
func pdParseWindow(sinceFlag, untilFlag string, now time.Time) (pdWindow, error) {
	w := pdWindow{Until: now}
	if untilFlag != "" {
		u, err := pdParseMoment(untilFlag, now)
		if err != nil {
			return w, fmt.Errorf("invalid --until %q: %w", untilFlag, err)
		}
		w.Until = u
	}
	if sinceFlag == "" {
		w.Since = now.AddDate(0, 0, -30)
	} else {
		s, err := pdParseMoment(sinceFlag, now)
		if err != nil {
			return w, fmt.Errorf("invalid --since %q: %w", sinceFlag, err)
		}
		w.Since = s
	}
	return w, nil
}

// pdParseMoment resolves one --since/--until value to an absolute time. A bare
// relative duration ("30d") is interpreted as "now minus that duration".
func pdParseMoment(s string, now time.Time) (time.Time, error) {
	if t, ok := pdParseTime(s); ok {
		return t, nil
	}
	if d, ok := pdParseRelative(s); ok {
		return now.Add(-d), nil
	}
	return time.Time{}, fmt.Errorf("not an RFC3339 time or NNd/NNh/NNm duration")
}

// pdParseRelative parses "30d", "24h", "90m", "45s" into a Duration.
func pdParseRelative(s string) (time.Duration, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	unit := s[len(s)-1]
	num := s[:len(s)-1]
	n, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0, false
	}
	switch unit {
	case 'd', 'D':
		return time.Duration(n * float64(24*time.Hour)), true
	case 'h', 'H':
		return time.Duration(n * float64(time.Hour)), true
	case 'm', 'M':
		return time.Duration(n * float64(time.Minute)), true
	case 's', 'S':
		return time.Duration(n * float64(time.Second)), true
	default:
		return 0, false
	}
}

// pdHumanDur renders a duration as a compact "1h 5m" / "3d 2h" / "45s" string.
func pdHumanDur(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		h := int(d / time.Hour)
		m := int((d % time.Hour) / time.Minute)
		if m == 0 {
			return fmt.Sprintf("%dh", h)
		}
		return fmt.Sprintf("%dh %dm", h, m)
	}
	days := int(d / (24 * time.Hour))
	h := int((d % (24 * time.Hour)) / time.Hour)
	if h == 0 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%dd %dh", days, h)
}

// pdIsOffHours reports whether a timestamp falls on a night (before 08:00 or at/
// after 20:00) or a weekend, in the timestamp's own location.
func pdIsOffHours(t time.Time) bool {
	wd := t.Weekday()
	if wd == time.Saturday || wd == time.Sunday {
		return true
	}
	h := t.Hour()
	return h < 8 || h >= 20
}

// ---- output routing ----

// pdEmit prints the machine value (honoring --json/--agent/--select/--csv/
// --compact) or, in a plain terminal context, the human rendering.
func pdEmit(cmd *cobra.Command, flags *rootFlags, v any, human func(w io.Writer)) error {
	if flags != nil && (flags.asJSON || flags.agent || flags.csv || flags.compact || flags.selectFields != "") {
		return flags.printJSON(cmd, v)
	}
	human(cmd.OutOrStdout())
	return nil
}

// pdRound2 rounds a float to 2 decimals for stable JSON/report output.
func pdRound2(f float64) float64 {
	return float64(int64(f*100+0.5)) / 100
}
