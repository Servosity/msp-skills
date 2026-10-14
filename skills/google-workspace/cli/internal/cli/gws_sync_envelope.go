// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (handfixes.json sync-google-empty-list-page and
// sync-google-last-page-proven): Google list-envelope rules the
// Discovery-sourced sync tables do not encode.

package cli

import "encoding/json"

// googleListItemsKey is the array each synced Google list response carries
// its records under.
var googleListItemsKey = map[string]string{
	"applications":  "applications",
	"groups":        "groups",
	"subscriptions": "subscriptions",
	"transfers":     "dataTransfers",
	"users":         "users",
}

// googleListPageIsEmpty reports an empty page of a synced list resource.
// Google omits the items array when a list is empty and returns only kind and
// etag; that envelope must not be stored as a record.
func googleListPageIsEmpty(resource string, data json.RawMessage) bool {
	key, ok := googleListItemsKey[resource]
	if !ok {
		return false
	}
	var env map[string]json.RawMessage
	if json.Unmarshal(data, &env) != nil {
		return false
	}
	if _, google := env["kind"]; !google {
		return false
	}
	raw, present := env[key]
	if !present || isJSONNull(raw) {
		return true
	}
	var items []json.RawMessage
	return json.Unmarshal(raw, &items) == nil && len(items) == 0
}

// googleListEnvelopeEnded reports a Google list envelope (it carries kind)
// with no nextPageToken: Google's list contract makes that the last page.
func googleListEnvelopeEnded(data json.RawMessage) bool {
	var env map[string]json.RawMessage
	if json.Unmarshal(data, &env) != nil {
		return false
	}
	if _, google := env["kind"]; !google {
		return false
	}
	tok, present := env["nextPageToken"]
	if !present || isJSONNull(tok) {
		return true
	}
	var s string
	return json.Unmarshal(tok, &s) == nil && s == ""
}
