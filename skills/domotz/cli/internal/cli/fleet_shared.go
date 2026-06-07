// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel-feature support: shared helpers for the cross-fleet
// (transcendence) commands. These commands answer questions the agent-scoped
// Domotz Public API cannot answer in a single call by reading the local
// SQLite mirror (store-backed commands) or by fanning out across every agent
// (live commands).

package cli

import (
	"context"
	"domotz-pp-cli/internal/client"
	"domotz-pp-cli/internal/store"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// addFleetDBFlag registers the shared --db flag used by every store-backed
// fleet command so they all resolve the same local mirror.
func addFleetDBFlag(cmd *cobra.Command, dbPath *string) {
	cmd.Flags().StringVar(dbPath, "db", "", "Local store path (default: ~/.local/share/domotz-cli/data.db)")
}

// openFleetStore opens the local SQLite mirror at dbPath, or the default path
// when empty. A non-existent file is created with the schema, so commands stay
// safe (and return empty results) before the first sync.
func openFleetStore(ctx context.Context, dbPath string) (*store.Store, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("domotz-cli")
	}
	return store.OpenWithContext(ctx, dbPath)
}

// queryFleetRows runs a read-only SELECT and returns the rows as column->value
// maps in result order. Always returns a non-nil slice so empty results
// marshal to [] rather than null.
func queryFleetRows(ctx context.Context, db *store.Store, query string, args ...any) ([]map[string]any, error) {
	rows, err := db.DB().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	out := make([]map[string]any, 0)
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := make(map[string]any, len(cols))
		for i, c := range cols {
			m[c] = normalizeFleetValue(vals[i])
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// normalizeFleetValue renders SQLite scalar values JSON-friendly: []byte (the
// driver's representation of TEXT/JSON columns) becomes a string.
func normalizeFleetValue(v any) any {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}

// asString coerces a scanned SQLite value to a string.
func asString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []byte:
		return string(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", t)
	}
}

// asInt coerces a scanned SQLite aggregate value (which may be int64, float64,
// or NULL) to an int.
func asInt(v any) int {
	switch t := v.(type) {
	case nil:
		return 0
	case int64:
		return int(t)
	case float64:
		return int(t)
	case int:
		return t
	case []byte:
		n, _ := strconv.Atoi(strings.TrimSpace(string(t)))
		return n
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(t))
		return n
	default:
		return 0
	}
}

// parseSinceCutoff turns a window like "24h", "90m", or "7d" into an absolute
// cutoff time relative to now. time.ParseDuration handles h/m/s; the "d"
// (days) suffix is handled here because Go durations have no day unit.
func parseSinceCutoff(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return now.Add(-24 * time.Hour), nil
	}
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil || days < 0 {
			return time.Time{}, fmt.Errorf("invalid --since %q: use forms like 24h, 90m, or 7d", s)
		}
		return now.AddDate(0, 0, -days), nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return time.Time{}, fmt.Errorf("invalid --since %q: use forms like 24h, 90m, or 7d", s)
	}
	return now.Add(-d), nil
}

// fleetAgentRef is a lightweight agent identity used by the live fan-out
// commands (events, ip-conflicts, speedtest, triggers).
type fleetAgentRef struct {
	ID   string `json:"agent_id"`
	Name string `json:"site"`
}

// fleetAgentsLive fetches the agent list directly from the API so live
// fan-out commands work without a prior sync. IDs are normalized to bare
// strings (the API encodes them as JSON numbers).
func fleetAgentsLive(ctx context.Context, c *client.Client) ([]fleetAgentRef, error) {
	data, err := c.Get(ctx, "/agent", map[string]string{})
	if err != nil {
		return nil, err
	}
	var raw []map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing agent list: %w", err)
	}
	refs := make([]fleetAgentRef, 0, len(raw))
	for _, a := range raw {
		ref := fleetAgentRef{ID: trimJSONString(a["id"])}
		if dn, ok := a["display_name"]; ok {
			ref.Name = trimJSONString(dn)
		}
		if ref.ID != "" {
			refs = append(refs, ref)
		}
	}
	return refs, nil
}

// trimJSONString renders a raw JSON scalar (number or quoted string) as a
// bare Go string: `12345` -> "12345", `"abc"` -> "abc".
func trimJSONString(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return ""
	}
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		var unq string
		if err := json.Unmarshal(raw, &unq); err == nil {
			return unq
		}
		return strings.Trim(s, "\"")
	}
	return s
}

// site reports the human site label for a fan-out agent ref: the display
// name when present, else the raw id.
func (a fleetAgentRef) site() string {
	if a.Name != "" {
		return a.Name
	}
	return a.ID
}
