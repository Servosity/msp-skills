// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/saas-alerts/handfixes.json
// under the "rule-triggers-list-all-follows-array-cursor" entry.

package cli

import (
	"encoding/json"
	"net/url"
	"strings"
)

// respondArrayCursor returns Respond's pagination.cursor pair
// ([triggeredAt, id]) joined with a comma, or "" when raw is not that pair.
// cursorValue (sync.go) owns the two-element rule.
func respondArrayCursor(raw json.RawMessage) string {
	var parts []json.RawMessage
	if json.Unmarshal(raw, &parts) != nil {
		return ""
	}
	return cursorValue(raw)
}

// respondCursorRequest sends a comma-joined startAfter cursor as the repeated
// startAfter keys GET /rule-triggers documents, replacing any startAfter the
// path already carried (an operator's --start-after on the first page).
func respondCursorRequest(path string, params map[string]string, cursorParam string) (string, map[string]string) {
	v := params[cursorParam]
	if cursorParam != "startAfter" || !strings.Contains(v, ",") {
		return path, params
	}
	rest := make(map[string]string, len(params))
	for k, val := range params {
		if k != cursorParam {
			rest[k] = val
		}
	}
	if u, err := url.Parse(path); err == nil && u.RawQuery != "" {
		q := u.Query()
		q.Del(cursorParam)
		u.RawQuery = q.Encode()
		path = u.String()
	}
	return appendArrayQueryParam(path, cursorParam, v, "form", true), rest
}

// respondEndOfPages reports whether a Respond page says, with a literal
// pagination.hasNextPage false, that it is the last one. Respond also returns
// a cursor on that page; following it would cost an extra request that can
// repeat the page. Only the literal false is authoritative (null or a string
// is unknown), matching the sync hand-fix.
func respondEndOfPages(obj map[string]json.RawMessage, cursorParam, hasMoreField string) bool {
	if cursorParam != "startAfter" || hasMoreField == "" {
		return false
	}
	raw, ok := rawAtPath(obj, hasMoreField)
	return ok && strings.TrimSpace(string(raw)) == "false"
}
