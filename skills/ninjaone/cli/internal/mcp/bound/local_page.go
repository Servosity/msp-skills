// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package bound

// Hand-fix (handfixes.json unpaged-list-mcp-continuation, issue #704): a GET
// endpoint with no upstream paging returns its whole list in one answer, and
// the result budget used to cut it at MaxItems/MaxBytes with nothing to
// continue from. A cut answer now carries a local cursor (an offset into the
// list); the caller passes it back as params._pp_cursor, the list is fetched
// again and read from that offset. The cursor never reaches the API. An answer
// that is not cut renders byte for byte as EndpointResponse does.

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// LocalCursorParam is the params key that carries a local continuation cursor.
const LocalCursorParam = "_pp_cursor"

const (
	localListNote     = "Typed MCP endpoint response was bounded for MCP output. To read the rest of this same answer, repeat the call with the same params plus next_cursor as params._pp_cursor: the request is sent again and read from where this answer stopped, so a list that changes between calls can shift. Paging parameters the endpoint itself takes, if any, reach records beyond this answer."
	localObjectNote   = "Typed MCP endpoint response was bounded for MCP output. To read the rest of this same answer, repeat the call with the same params plus _pp_next_cursor as params._pp_cursor: the request is sent again and read from where this answer stopped, so a list that changes between calls can shift. Paging parameters the endpoint itself takes, if any, reach records beyond this answer."
	localOversizeNote = "One list item does not fit the MCP result budget, so it is shown only as a preview (item_too_large). Pass the cursor back as params._pp_cursor with the same params to continue after it; read the whole item with a command-mirror tool and --select."
)

// localCursor is a continuation within one request's answer. Scope binds it
// to that request (endpoint, path and query), so a cursor replayed against
// another request is refused instead of silently skipping its first items.
type localCursor struct {
	Version int    `json:"v"`
	Offset  int    `json:"o"`
	Scope   string `json:"k"`
}

// LocalScope names one request for local continuation cursors.
func LocalScope(endpointID, path string, query map[string]string) string {
	keys := make([]string, 0, len(query))
	for key := range query {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%q %q", endpointID, path)
	for _, key := range keys {
		_, _ = fmt.Fprintf(h, " %q=%q", key, query[key])
	}
	return hex.EncodeToString(h.Sum(nil)[:12])
}

func encodeLocalCursor(offset int, scope string) string {
	data, err := json.Marshal(localCursor{Version: 1, Offset: offset, Scope: scope})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(data)
}

// LocalPageCursor validates a local continuation cursor for the request
// scope names and returns its offset.
func LocalPageCursor(cursor, scope string) (int, error) {
	data, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, fmt.Errorf("invalid MCP cursor")
	}
	var state localCursor
	if err := json.Unmarshal(data, &state); err != nil || state.Version != 1 || state.Offset < 0 || state.Scope == "" {
		return 0, fmt.Errorf("invalid MCP cursor")
	}
	if state.Scope != scope {
		return 0, fmt.Errorf("the cursor belongs to another request; repeat the call with the same endpoint and params")
	}
	return state.Offset, nil
}

// EndpointLocalPageResponse renders a GET answer from an endpoint with no
// upstream paging, from offset (validated with LocalPageCursor) for the
// request scope names. A bare list, or an object whose single list is over
// the budget (or is being continued), can be read page by page. Everything
// else renders exactly as EndpointResponse does.
func EndpointLocalPageResponse(method string, data json.RawMessage, offset int, scope string) string {
	if !strings.EqualFold(method, "GET") || offset < 0 {
		return EndpointResponse(method, data)
	}
	start := offset
	trimmed := strings.TrimSpace(string(data))
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var items []json.RawMessage
		if json.Unmarshal(data, &items) == nil {
			return localPage(data, items, start, scope, nil, "items")
		}
	}
	if start > 0 || len(data) > MaxBytes {
		var obj map[string]json.RawMessage
		if json.Unmarshal(data, &obj) == nil {
			if field, items, ok := singleArrayField(obj); ok {
				return localPage(data, items, start, scope, obj, field)
			}
		}
	}
	return EndpointResponse(method, data)
}

// localPage renders items from start within the budget. base is nil for a
// bare list (the {count, items} envelope) or the vendor object holding the
// list in field (its other members are kept; this side's metadata is
// _pp_-prefixed so it never overwrites a vendor member). When the vendor's
// other members leave no room for an item, a page leaves them out and names
// them under _pp_omitted_members.
func localPage(data json.RawMessage, items []json.RawMessage, start int, scope string, base map[string]json.RawMessage, field string) string {
	if items == nil {
		items = []json.RawMessage{}
	}
	if start > len(items) {
		start = len(items)
	}
	cursorAt := func(offset int) string {
		if offset >= len(items) {
			return ""
		}
		return encodeLocalCursor(offset, scope)
	}
	var siblings []string
	for key := range base {
		if key != field {
			siblings = append(siblings, key)
		}
	}
	sort.Strings(siblings)
	// build renders n items from start; tooLarge renders the item at start as
	// a preview of previewLimit bytes instead; bare leaves the vendor's other
	// members out.
	build := func(n int, tooLarge bool, previewLimit int, bare bool) []byte {
		end := start + n
		if tooLarge {
			end = start + 1
		}
		next := cursorAt(end)
		out := make(map[string]any, len(base)+10)
		if !bare {
			for _, key := range siblings {
				out[key] = base[key]
			}
		}
		out[field] = items[start : start+n]
		if base == nil {
			out["count"] = len(items)
		}
		if bare && len(siblings) > 0 {
			out["_pp_omitted_members"] = siblings
		}
		// A page that leaves out a vendor member is never a whole answer.
		if start > 0 || end < len(items) || tooLarge || (bare && len(siblings) > 0) {
			meta := map[string]any{
				"truncated":      true,
				"returned_count": n,
				"original_bytes": len(data),
				"max_bytes":      MaxBytes,
			}
			note := localListNote
			if base != nil {
				note = localObjectNote
				meta["total_count"] = len(items)
			}
			if tooLarge {
				note = localOversizeNote
				meta["item_too_large"] = map[string]any{"index": start, "bytes": len(items[start]), "preview": previewString(items[start], previewLimit)}
			}
			meta["note"] = note
			if start > 0 {
				meta["offset"] = start
			}
			if next != "" {
				meta["next_cursor"] = next
			}
			for key, value := range meta {
				if base == nil {
					out[key] = value
				} else {
					out["_pp_"+key] = value
				}
			}
			out["_pp_truncated"] = true
		}
		encoded, err := json.Marshal(out)
		if err != nil {
			return nil
		}
		return encoded
	}

	limit := len(items) - start
	if limit > MaxItems {
		limit = MaxItems
	}
	for _, bare := range []bool{false, true} {
		if bare && len(siblings) == 0 {
			continue
		}
		for n := limit; n > 0; n-- {
			if out := build(n, false, 0, bare); out != nil && len(out) <= MaxBytes {
				return string(out)
			}
		}
	}
	if start < len(items) {
		for _, bare := range []bool{false, true} {
			for _, previewLimit := range []int{maxPreviewBytes, 2048, 512, 0} {
				if out := build(0, true, previewLimit, bare); out != nil && len(out) <= MaxBytes {
					return string(out)
				}
			}
		}
		// The vendor object's other members alone fill the budget. A cursor
		// at this same offset would make no progress, so offer none.
		return previewEnvelope(data, endpointPreviewNote)
	}
	if out := build(0, false, 0, false); out != nil && len(out) <= MaxBytes {
		return string(out)
	}
	return previewEnvelope(data, endpointPreviewNote)
}

// singleArrayField returns the one array-valued member of obj; ok is false
// when there is none or more than one.
func singleArrayField(obj map[string]json.RawMessage) (string, []json.RawMessage, bool) {
	field := ""
	var items []json.RawMessage
	for key, raw := range obj {
		trimmed := strings.TrimSpace(string(raw))
		if len(trimmed) == 0 || trimmed[0] != '[' {
			continue
		}
		var candidate []json.RawMessage
		if json.Unmarshal(raw, &candidate) != nil {
			continue
		}
		if field != "" {
			return "", nil, false
		}
		field, items = key, candidate
	}
	return field, items, field != ""
}
