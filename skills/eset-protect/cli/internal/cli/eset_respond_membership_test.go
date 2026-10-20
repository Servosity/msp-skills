// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"io"
	"net/http"
	"testing"

	"eset-protect-pp-cli/internal/cliutil/testenv"
)

// TestRespondCustomerWaitNeedsCompleteMembership (handfixes.json:
// respond-wait-needs-complete-membership): a customer scan's expected runs
// come from the mirrored member list. After an interrupted devices sync that
// list may miss devices, so `respond scan --customer ... --yes --wait` must not
// report complete=true just because the mirrored members finished; with a
// complete sync it still does.
func TestRespondCustomerWaitNeedsCompleteMembership(t *testing.T) {
	run := func(t *testing.T, devicesComplete bool) map[string]any {
		testenv.Isolate(t)
		respondClearHarness(t)
		db := respondSeedMirror(t)
		mirrorHintMarkSync(t, db, true, "device-groups")
		mirrorHintMarkSync(t, db, devicesComplete, "devices")
		respondFakeESET(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				_, _ = io.WriteString(w, `{"task":{"uuid":"task-cust"}}`)
				return
			}
			_, _ = io.WriteString(w, `{"taskRuns":[`+
				`{"deviceUuid":"`+respondTestDevice+`","status":"TASK_RUN_STATUS_FINISHED"},`+
				`{"deviceUuid":"22222222-2222-4222-8222-222222222222","status":"TASK_RUN_STATUS_FINISHED"}],"nextPageToken":""}`)
		})
		got, text, err := respondRunRoot(t, "respond", "scan", "--customer", "Acme Dental", "--yes", "--wait", "1s", "--db", db, "--json")
		if err != nil && got == nil {
			t.Fatalf("respond: %v\n%s", err, text)
		}
		return got
	}
	t.Run("complete sync", func(t *testing.T) {
		if got := run(t, true); got["complete"] != true {
			t.Fatalf("every mirrored member finished on a complete mirror, want complete=true: %v", got)
		}
	})
	t.Run("interrupted sync", func(t *testing.T) {
		if got := run(t, false); got["complete"] == true {
			t.Fatalf("an interrupted devices sync must not let --wait report the customer task complete: %v", got)
		}
	})
}
