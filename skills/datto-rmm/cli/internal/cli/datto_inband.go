// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"strings"
)

// envelopeCarriesError reports a populated error field on a page (handfix
// inband-error-field-is-a-failure, Codex review r2 class incomplete sync reported
// healthy). Datto documents ActivityLogsPage.error on HTTP 200 next to the page's
// records, and the page still carries pageDetails, so nothing else marks the read
// failed. Any spelling, any non-blank value; a value that is not JSON fails closed.
func envelopeCarriesError(envelope map[string]json.RawMessage) bool {
	for k, raw := range envelope {
		if strings.EqualFold(k, "error") || strings.EqualFold(k, "errors") {
			if jsonValuePopulated(raw) {
				return true
			}
		}
	}
	return false
}

func jsonValuePopulated(raw json.RawMessage) bool {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return true
	}
	switch t := v.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(t) != ""
	case bool:
		return t
	case float64:
		return t != 0
	case []any:
		return len(t) > 0
	case map[string]any:
		return len(t) > 0
	}
	return true
}
