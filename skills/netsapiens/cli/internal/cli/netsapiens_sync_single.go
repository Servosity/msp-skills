// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"strings"
)

// singleObjectDependentItem treats a per-parent response that is one JSON
// object (NetSapiens /domains/{domain}/billing) as a one-row page. It only
// fires when the object carries every part of the resource's declared ID, so
// error envelopes and wrapper objects are never stored as rows.
func singleObjectDependentItem(data json.RawMessage, resource string) (json.RawMessage, bool) {
	idField := resourceIDFieldOverrides[resource]
	if idField == "" {
		return nil, false
	}
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil || obj == nil {
		return nil, false
	}
	for _, part := range strings.Split(idField, "+") {
		v, ok := obj[part]
		if !ok || v == nil || v == "" {
			return nil, false
		}
	}
	return data, true
}
