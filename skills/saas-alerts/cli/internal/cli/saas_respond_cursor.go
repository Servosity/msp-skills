// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/saas-alerts/handfixes.json
// under the "rule-triggers-list-all-follows-array-cursor" entry.

package cli

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
)

// respondArrayCursor returns an array cursor - Respond's pagination.cursor
// pair ([triggeredAt, id]) or Unify's meta.sort - as a compact JSON array of
// strings, or "" when raw is not a non-empty array of scalars.
// respondCursorRequest re-expands it into repeated query keys.
func respondArrayCursor(raw json.RawMessage) string {
	return unifySortCursor(raw)
}

// respondCursorRequest sends an array cursor (JSON array or comma list) as the
// repeated startAfter keys GET /rule-triggers documents, or the repeated
// searchAfter keys Unify v2 documents, replacing any value the path already
// carried (an operator's --start-after or --search-after on the first page).
func respondCursorRequest(path string, params map[string]string, cursorParam string) (string, map[string]string) {
	v := params[cursorParam]
	if (cursorParam != "startAfter" && cursorParam != "searchAfter") || !(strings.HasPrefix(v, "[") || strings.Contains(v, ",")) {
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

// unifyMaxPageSize is the Manage 1.2.0 spec's documented pageSize maximum for
// the Unify v2 device lists.
const unifyMaxPageSize = 1000

// unifyFullBarePage reports whether a bare-array Unify page (no meta.sort to
// page from) filled the requested page, so more devices may exist. The spec's
// example response is a bare array while its searchAfter description implies
// an envelope; until a live tenant settles it, a full bare page is reported as
// truncated, never complete. With no --page-size the server default is
// unknown, so 100 (what the CLI used to request) is assumed.
func unifyFullBarePage(pageSizeParam string, n int) bool {
	limit, err := strconv.Atoi(strings.TrimSpace(pageSizeParam))
	if err != nil || limit <= 0 {
		limit = 100
	}
	return n >= limit
}
