// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/saas-alerts/handfixes.json
// under the "test-event-delivery-failure-surfaced" entry.

package cli

import (
	"encoding/json"
	"strings"
)

// TestEventFailure reads a POST /misc/sendTestEvent 200 answer (anyOf boolean |
// string | IWhError[]) for the CLI command and the MCP execute tool alike. It
// returns why the test event was not delivered, or "" when it was; unknown is a
// string answer whose wording is not recognized, which callers warn on rather
// than fail, since the spec does not pin the delivered text.
func TestEventFailure(path string, data []byte) (why string, unknown bool) {
	if !strings.HasSuffix(path, "/misc/sendTestEvent") {
		return "", false
	}
	var answer string
	if json.Unmarshal(data, &answer) == nil && shClassifyTestEventString(answer) == "unknown" {
		return "", true
	}
	if ok, why, _ := shTestEventOutcome(data, nil); !ok {
		return why, false
	}
	return "", false
}
