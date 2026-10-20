// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/saas-alerts/handfixes.json
// under the "test-event-delivery-failure-surfaced" entry.

package cli

import "strings"

// TestEventFailure reads a POST /misc/sendTestEvent 200 answer (anyOf boolean |
// string | IWhError[]) for the CLI command and the MCP execute tool alike, with
// the same rule as subscriptions health: it returns why the test event was not
// delivered (false, an error or unrecognized string, any IWhError), or "" when
// it was (true, an empty list, a recognized success string).
func TestEventFailure(path string, data []byte) string {
	if !strings.HasSuffix(path, "/misc/sendTestEvent") {
		return ""
	}
	if ok, why, _ := shTestEventOutcome(data, nil); !ok {
		return why
	}
	return ""
}
