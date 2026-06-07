// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored helpers shared by the novel analytics commands.

package cli

import (
	"encoding/json"
	"time"

	"rocketcyber-pp-cli/internal/cliutil"
)

// parseEnvelope extracts the item array from a RocketCyber API response.
// v3 list endpoints wrap results as {totalCount,currentPage,totalPages,data:[...]};
// some surfaces return a bare array. total falls back to len(items) when the
// envelope carries no totalCount.
func parseEnvelope(data json.RawMessage) (items []json.RawMessage, total int64) {
	if err := json.Unmarshal(data, &items); err == nil {
		return items, int64(len(items))
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, 0
	}
	if d, ok := probe["data"]; ok {
		_ = json.Unmarshal(d, &items)
	}
	if t, ok := cliutil.ExtractInt(probe, "totalCount"); ok {
		total = t
	} else {
		total = int64(len(items))
	}
	return items, total
}

// extractString returns the string value for key from a JSON probe map,
// tolerating null and missing keys.
func extractString(probe map[string]json.RawMessage, key string) string {
	raw, ok := probe[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

// parseAPITime parses RocketCyber timestamps (RFC3339 with optional
// fractional seconds). Returns the zero time when unparseable.
func parseAPITime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
