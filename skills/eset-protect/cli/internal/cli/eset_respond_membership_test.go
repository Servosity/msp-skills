// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"eset-protect-pp-cli/internal/cliutil/testenv"
)

// TestRespondCustomerWaitUsesLiveMembership (handfixes.json:
// respond-wait-needs-complete-membership): a customer task targets the live
// group, so `respond scan --customer ... --yes --wait` must wait for the
// group's live members (subgroups included), not the mirror's. A device
// enrolled after the last sync, or one an interrupted sync missed, used to let
// --wait report complete=true as soon as the mirrored devices finished.
func TestRespondCustomerWaitUsesLiveMembership(t *testing.T) {
	const enrolledLater = "44444444-4444-4444-8444-444444444444"
	finished := func(uuids ...string) string {
		parts := make([]string, 0, len(uuids))
		for _, u := range uuids {
			parts = append(parts, `{"deviceUuid":"`+u+`","status":"TASK_RUN_STATUS_FINISHED"}`)
		}
		return `{"taskRuns":[` + strings.Join(parts, ",") + `],"nextPageToken":""}`
	}
	run := func(t *testing.T, members string, membersStatus int, runs string) (map[string]any, string) {
		testenv.Isolate(t)
		respondClearHarness(t)
		db := respondSeedMirror(t)
		respondFakeESET(t, func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodPost:
				_, _ = io.WriteString(w, `{"task":{"uuid":"task-cust"}}`)
			case r.URL.Path == "/v1/device_groups/"+respondTestGroup+"/devices":
				if r.URL.Query().Get("recurseSubgroups") != "true" {
					t.Errorf("live membership must include subgroups: %s", r.URL.RawQuery)
				}
				w.WriteHeader(membersStatus)
				_, _ = io.WriteString(w, members)
			default:
				_, _ = io.WriteString(w, runs)
			}
		})
		got, text, err := respondRunRoot(t, "respond", "scan", "--customer", "Acme Dental", "--yes", "--wait", "1s", "--db", db, "--json")
		if err != nil && got == nil {
			t.Fatalf("respond: %v\n%s", err, text)
		}
		return got, text
	}
	both := `{"devices":[{"uuid":"` + respondTestDevice + `"},{"uuid":"22222222-2222-4222-8222-222222222222"}],"nextPageToken":""}`
	t.Run("every live member finished", func(t *testing.T) {
		got, _ := run(t, both, http.StatusOK, finished(respondTestDevice, "22222222-2222-4222-8222-222222222222"))
		if got["complete"] != true || got["expected_runs"] != float64(2) {
			t.Fatalf("want complete=true with 2 expected runs: %v", got)
		}
	})
	t.Run("a device the mirror never saw has not run", func(t *testing.T) {
		live := `{"devices":[{"uuid":"` + respondTestDevice + `"},{"uuid":"22222222-2222-4222-8222-222222222222"},{"uuid":"` + enrolledLater + `"}],"nextPageToken":""}`
		got, _ := run(t, live, http.StatusOK, finished(respondTestDevice, "22222222-2222-4222-8222-222222222222"))
		if got["complete"] == true {
			t.Fatalf("a live member without a run must keep the task incomplete: %v", got)
		}
	})
	t.Run("a member row without a UUID", func(t *testing.T) {
		bad := `{"devices":[{"uuid":"` + respondTestDevice + `"},null,{"displayName":"no uuid"}],"nextPageToken":""}`
		got, text := run(t, bad, http.StatusOK, finished(respondTestDevice))
		if got["complete"] == true || !strings.Contains(text, "completion stays unknown") {
			t.Fatalf("an unreadable member must leave completion unknown: %v\n%s", got, text)
		}
	})
	t.Run("membership cannot be listed", func(t *testing.T) {
		got, text := run(t, `{}`, http.StatusInternalServerError, finished(respondTestDevice, "22222222-2222-4222-8222-222222222222"))
		if got["complete"] == true || !strings.Contains(text, "completion stays unknown") {
			t.Fatalf("an unlisted membership must leave completion unknown: %v\n%s", got, text)
		}
	})
}
