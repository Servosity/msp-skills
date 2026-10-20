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

	// The spec does not pin the delivered wording, so an unrecognized string warns.
	api.testEventResp = "Test event sent successfully"
	out, stderr, err = rqtRun(t, "subscriptions", "test-event", "--json")
	if err != nil || !strings.Contains(stderr, "outcome unknown") {
		t.Fatalf("an unrecognized string must warn, not fail: err=%v stderr=%q\n%s", err, stderr, out)
	}
}
