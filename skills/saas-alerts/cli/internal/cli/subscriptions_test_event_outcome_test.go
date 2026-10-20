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

	// An answer is never a subscription: even an allowed failure is not stored.
	if _, _, err := rqtRun(t, "subscriptions", "test-event", "--allow-partial-failure", "--json"); err != nil {
		t.Fatalf("--allow-partial-failure: %v", err)
	}
	if out, _, _ := rqtRun(t, "subscriptions", "list", "--data-source", "local", "--json"); strings.Contains(out, "CH_TEST") {
		t.Fatalf("a test event answer was stored as a subscription:\n%s", out)
	}

	// Like subscriptions health, a string it does not recognize as delivered
	// (a failure wording without a known failure word included) is not a success.
	for _, answer := range []string{"Test event couldn't be delivered", "Success: false", "0 events sent"} {
		api.testEventResp = answer
		out, _, err = rqtRun(t, "subscriptions", "test-event", "--json")
		if rqtExit(err) != 6 || !strings.Contains(out, `"success": false`) {
			t.Fatalf("%q must fail the run: exit=%d\n%s", answer, rqtExit(err), out)
		}
	}
}
