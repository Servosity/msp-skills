// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: pinned by skills/sophos-central/handfixes.json under
// "single-read-keeps-the-object".
//
// The generator gives single-object GET commands the first array property of
// the response schema as a "response path" and unwraps it. On Sophos that
// turned `alerts get` into the alert's allowedActions strings, `tenants get`
// into its products, and `endpoints get-tamper-protection` into the INVALIDATED
// previous passwords (dropping the current password and the enabled flag). The
// path is only a list wrapper when every other top-level key is paging
// metadata ({items, pages}, {has_more, items, next_cursor}); otherwise the
// whole object is the answer. List commands keep the generated unwrap.

package cli

import (
	"encoding/json"
	"strings"
)

var responsePathPagingKeys = map[string]bool{
	"pages": true, "has_more": true, "next_cursor": true, "nextKey": true,
	"next": true, "total": true, "totalCount": true, "page": true, "pageSize": true,
}

func applyEntityResponsePath(data json.RawMessage, responsePath string, isList bool) json.RawMessage {
	key := strings.TrimPrefix(strings.TrimSpace(responsePath), "$.")
	// List commands (licenses list: {organization, tenant, licenses}) keep
	// printing rows; only single-object reads keep the object.
	if isList || key == "" || strings.Contains(key, ".") {
		return applyResponsePath(data, responsePath)
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(data, &root) != nil {
		return applyResponsePath(data, responsePath)
	}
	for k := range root {
		if k != key && !responsePathPagingKeys[k] {
			return data
		}
	}
	return applyResponsePath(data, responsePath)
}
