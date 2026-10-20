// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/saas-alerts/handfixes.json
// under the "test-event-delivery-failure-surfaced" entry.

package cli

import (
	"strings"
	"testing"
)

// TestSubscriptionsTestEventSurfacesDeliveryErrors: sendTestEvent answers 200
// with an IWhError list when delivery failed; that run is not a success.
func TestSubscriptionsTestEventSurfacesDeliveryErrors(t *testing.T) {
	api := newRQTAPI(t)
	rqtEnv(t, api)
	out, _, err := rqtRun(t, "subscriptions", "test-event", "--json")
	if err != nil || !strings.Contains(out, `"success": true`) {
		t.Fatalf("a delivered test event (true) must succeed: err=%v\n%s", err, out)
	}

	api.testEventResp = []any{map[string]any{"channelId": "CH_TEST", "message": "delivery failed"}}
	out, stderr, err := rqtRun(t, "subscriptions", "test-event", "--json")
	if rqtExit(err) != 6 || !strings.Contains(out, `"success": false`) || !strings.Contains(stderr, "test event delivery failed") {
		t.Fatalf("a delivery-error list must fail the run: exit=%d stderr=%q\n%s", rqtExit(err), stderr, out)
	}
}
