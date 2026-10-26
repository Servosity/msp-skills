// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package ninjapage is the one rule every NinjaOne paging reader shares: sync (both loops),
// export, tail, the endpoint `--all` paginator and MCP pages. Hand-fix, class
// ninja-paging-every-reader.
//
// NinjaOne lists come in two documented shapes (spec.yaml):
//
//   - Keyset lists return a bare JSON array and page by sending the last row's key back:
//     ?after=<id> on /v2/devices, /v2/devices-detailed, /v2/organizations(-detailed),
//     /v2/locations and /v2/organization/{id}/devices ("Last Node ID from previous page");
//     ?anchorNaturalId=<naturalId> on /v2/ticketing/app-user-contact ("Last User Identifier
//     from previous page").
//   - Report lists (/v2/queries/*, /v2/backup/*) return {"cursor":{"name","offset",...},
//     "results":[...]}: the token name repeats while NinjaOne advances the offset.
//
// A page ends a walk only when it is empty or shorter than every page size it could have been
// served at; when that size is unknown, only an empty page ends it.
package ninjapage

import (
	"encoding/json"
	"strings"
)

// AfterParam is the keyset parameter of the after-id lists.
const AfterParam = "after"

// Keyset is how a bare-array list endpoint pages: the request parameter and the row field whose
// last value it takes. An empty Field reads the id chain (id, uid, entityId, deviceId).
type Keyset struct {
	Param string
	Field string
}

// keysetByPath lists endpoints whose keyset parameter the generated code never modelled.
var keysetByPath = map[string]Keyset{
	"/v2/ticketing/app-user-contact": {Param: "anchorNaturalId", Field: "naturalId"},
}

// KeysetForPath reports the keyset an endpoint path pages by, beyond the after-id lists the
// generated code already names by their `after` parameter.
func KeysetForPath(path string) (Keyset, bool) {
	k, ok := keysetByPath[CleanPath(path)]
	return k, ok
}

// KeysetForParam reports the keyset a cursor parameter names: `after` (id chain) or a
// parameter from the path table. Callers apply it to bare-array pages only: /v2/activities
// also has an `after` parameter, but it is a date and its response is an object.
func KeysetForParam(param string) (Keyset, bool) {
	if param == AfterParam {
		return Keyset{Param: AfterParam}, true
	}
	for _, k := range keysetByPath {
		if k.Param == param {
			return k, true
		}
	}
	return Keyset{}, false
}

// CleanPath drops a query string and a trailing slash.
func CleanPath(path string) string {
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	if len(path) > 1 {
		path = strings.TrimRight(path, "/")
	}
	return path
}

// LastID is the keyset value of a page's last row, false when it carries none (the walk
// must then stop rather than resend a cursor it cannot advance).
func (k Keyset) LastID(items []json.RawMessage) (string, bool) {
	if len(items) == 0 {
		return "", false
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(items[len(items)-1], &obj) != nil {
		return "", false
	}
	fields := []string{"id", "uid", "entityId", "deviceId"}
	if k.Field != "" {
		fields = []string{k.Field}
	}
	for _, key := range fields {
		raw, ok := obj[key]
		if !ok {
			continue
		}
		var n json.Number
		if json.Unmarshal(raw, &n) == nil {
			if s := n.String(); s != "" && s != "0" {
				return s, true
			}
		}
		var s string
		if json.Unmarshal(raw, &s) == nil && s != "" {
			return s, true
		}
	}
	return "", false
}

// IsBareArray reports a top-level JSON array.
func IsBareArray(data json.RawMessage) bool {
	t := strings.TrimSpace(string(data))
	if t == "" || t[0] != '[' {
		return false
	}
	var arr []json.RawMessage
	return json.Unmarshal(data, &arr) == nil
}

// Limit is the page size a continuation decision compares a page against: the smaller of the
// requested and the default size when both are known (an out-of-range request NinjaOne clamps
// then never ends a walk early), whichever is known otherwise, 0 when neither is.
func Limit(requested, def int) int {
	switch {
	case requested > 0 && def > 0:
		if requested < def {
			return requested
		}
		return def
	case requested > 0:
		return requested
	case def > 0:
		return def
	}
	return 0
}

// Full reports whether a page of n rows may have a successor under limit: it is non-empty and
// not shorter than the limit (any non-empty page when the limit is unknown).
func Full(n, limit int) bool {
	return n > 0 && (limit <= 0 || n >= limit)
}

// ReportCursor reads a report page's cursor: the token to resend and a progress key that
// includes the offset, so a repeated token with an advancing offset is progress. ok is false
// when the page carries no cursor.
func ReportCursor(raw json.RawMessage) (token, progress string, ok bool) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, s, s != ""
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return "", "", false
	}
	for _, key := range []string{"name", "next_cursor", "nextCursor", "next", "cursor", "token"} {
		if json.Unmarshal(obj[key], &s) == nil && s != "" {
			token = s
			break
		}
	}
	if token == "" {
		return "", "", false
	}
	progress = token
	if off, ok := obj["offset"]; ok {
		progress = token + "\x00" + strings.TrimSpace(string(off))
	}
	return token, progress, true
}
