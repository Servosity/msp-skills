// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package bound

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"ninjaone-pp-cli/internal/ninjapage"
)

// NinjaOne's two documented list shapes for MCP pages (the CLI's sync and export
// apply the same rules). Without them a bare-array keyset page and an object-cursor
// report page both came back with no next_cursor and count equal to the page size,
// which reads as the whole fleet.
//
//   - Keyset lists return a bare array paged by the last row's key (?after=<id>, or
//     ?anchorNaturalId=<naturalId> on /v2/ticketing/app-user-contact): a page
//     continues unless it is empty or shorter than the requested pageSize.
//   - Report lists carry {"cursor":{"name":..,"offset":..}}: the token stays the
//     same while the offset advances, so a repeated token is not the end; a short
//     or empty page is.
//
// Hand-fix (handfixes.json ninja-paging-every-reader).

// keysetNextCursor is the keyset continuation for a bare-array page, or "": the
// last row's key (internal/ninjapage) unless the page is empty or shorter than
// the requested pageSize.
func keysetNextCursor(items []json.RawMessage, opts PageOptions) string {
	k, ok := ninjapage.KeysetForParam(opts.CursorParam)
	if !ok || !ninjapage.Full(len(items), opts.PageSize) {
		return ""
	}
	id, _ := k.LastID(items)
	return id
}

// pageCursorAtPath reads the cursor at a dotted path: a string as-is, or an
// object's token. offset reports an object cursor that carries an offset.
func pageCursorAtPath(data json.RawMessage, path string) (token string, offset bool) {
	if s := extractStringPath(data, path); s != "" {
		return s, false
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return "", false
	}
	var node map[string]json.RawMessage
	if json.Unmarshal(data, &node) != nil {
		return "", false
	}
	parts := strings.Split(path, ".")
	for i, part := range parts {
		raw, ok := node[part]
		if !ok {
			return "", false
		}
		var next map[string]json.RawMessage
		if json.Unmarshal(raw, &next) != nil {
			return "", false
		}
		if i < len(parts)-1 {
			node = next
			continue
		}
		for _, key := range []string{"name", "next_cursor", "nextCursor", "next", "cursor", "token"} {
			var s string
			if v, ok := next[key]; ok && json.Unmarshal(v, &s) == nil && s != "" {
				_, hasOffset := next["offset"]
				return s, hasOffset
			}
		}
	}
	return "", false
}

// Class mcp-continuation-never-replays-a-stateful-cursor. When the result budget
// splits an API page, the next_cursor for the rest of that page used to resend the
// page's own upstream request. NinjaOne report cursors are stateful: the token name
// stays the same while NinjaOne advances the offset server-side, so the resend
// returned the NEXT page and the local offset then discarded rows of both pages.
// A split page is now retained here, keyed by a random id carried in the cursor
// (endpointCursor.Retained) and bound to the request it answered (RetainScope); its
// continuations are served from it without an upstream request. A continuation whose
// page is gone fails loudly instead of re-requesting.

const (
	retainedMaxPages = 32
	retainedMaxBytes = 64 << 20
	retainedTTL      = 30 * time.Minute
)

type retainedEntry struct {
	data  json.RawMessage
	scope string
	at    time.Time
}

var retained = struct {
	sync.Mutex
	pages map[string]*retainedEntry
	order []string
	bytes int
}{pages: map[string]*retainedEntry{}}

// ErrRetainedPageGone is returned for a continuation whose page is no longer held.
var ErrRetainedPageGone = errors.New("this next_cursor continues a page the MCP server no longer holds (it restarted, or the page expired); restart the listing without a cursor")

// ErrRetainedPageScope is returned for a continuation sent with a different request.
var ErrRetainedPageScope = errors.New("this next_cursor continues a different request; resend it with the same endpoint_id and params that returned it")

// retainPageFor returns the lazy retainer nextPageCursor calls when it emits a local
// continuation: the cursor's own key when it already continues a retained page, a
// new retention of original when the caller bound a scope, nil otherwise.
func retainPageFor(state endpointCursor, opts PageOptions, original json.RawMessage) func() string {
	if state.Retained != "" {
		key := state.Retained
		return func() string { return key }
	}
	if opts.RetainScope == "" {
		return nil
	}
	key := ""
	return func() string {
		if key == "" {
			key = retainPage(original, opts.RetainScope)
		}
		return key
	}
}

func retainPage(data json.RawMessage, scope string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	key := hex.EncodeToString(b[:])
	retained.Lock()
	defer retained.Unlock()
	retained.pages[key] = &retainedEntry{data: append(json.RawMessage(nil), data...), scope: scope, at: time.Now()}
	retained.order = append(retained.order, key)
	retained.bytes += len(data)
	for len(retained.order) > 1 && (len(retained.order) > retainedMaxPages || retained.bytes > retainedMaxBytes) {
		dropRetainedLocked(retained.order[0])
	}
	return key
}

func dropRetainedLocked(key string) {
	if e, ok := retained.pages[key]; ok {
		retained.bytes -= len(e.data)
		delete(retained.pages, key)
	}
	for i, k := range retained.order {
		if k == key {
			retained.order = append(retained.order[:i], retained.order[i+1:]...)
			break
		}
	}
}

// RetainedPage returns the page an MCP cursor continues, when it continues a retained
// one: ok reports that the cursor names a retained page (the caller must not request
// upstream), err that the page is gone or was fetched for a different request.
func RetainedPage(cursor, scope string) (data json.RawMessage, ok bool, err error) {
	if cursor == "" {
		return nil, false, nil
	}
	state, derr := decodeEndpointCursor(cursor)
	if derr != nil || state.Retained == "" {
		return nil, false, nil
	}
	retained.Lock()
	defer retained.Unlock()
	e, found := retained.pages[state.Retained]
	if !found || time.Since(e.at) > retainedTTL {
		if found {
			dropRetainedLocked(state.Retained)
		}
		return nil, true, ErrRetainedPageGone
	}
	if e.scope != scope {
		return nil, true, ErrRetainedPageScope
	}
	e.at = time.Now()
	return e.data, true, nil
}
