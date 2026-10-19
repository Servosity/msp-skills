// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import "encoding/json"

// jsmHasTopLevelKey reports whether raw is a JSON object with key at the top.
// The request-replace hook uses it so only sync's SLA-bearing request rows
// replace the stored copy; a live `request get` (no sla) still merges.
func jsmHasTopLevelKey(raw json.RawMessage, key string) bool {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return false
	}
	_, ok := m[key]
	return ok
}
