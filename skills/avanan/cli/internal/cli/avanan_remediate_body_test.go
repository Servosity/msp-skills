// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): regression guard for handfixes.json entry
// "remediate-event-body-uses-event-field" (class wrong-result).

package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRemediateBodyNamesTheActionPerTargetKind: the event action contract
// (POST /v1.0/action/event, request_contracts.json action.post-event and the
// generated action_post-event.go) names the action requestData.eventActionName
// and requires it. remediate sent requestData.entityActionName for events too,
// so an event quarantine or restore carried no action name the endpoint
// reads. The entity path keeps entityActionName.
func TestRemediateBodyNamesTheActionPerTargetKind(t *testing.T) {
	for _, tc := range []struct {
		action, flag, target, path, want, wrong string
	}{
		{"quarantine", "--event", "ev-1", "/v1.0/action/event", "eventActionName", "entityActionName"},
		{"restore", "--event", "ev-2", "/v1.0/action/event", "eventActionName", "entityActionName"},
		{"quarantine", "--entity", "en-1", "/v1.0/action/entity", "entityActionName", "eventActionName"},
		{"restore", "--entity", "en-2", "/v1.0/action/entity", "entityActionName", "eventActionName"},
	} {
		t.Run(tc.action+" "+tc.flag, func(t *testing.T) {
			isolateAvananEnv(t)
			key := "POST " + tc.path
			s := &reviewServer{handlers: map[string]func(http.ResponseWriter, *http.Request){
				key: jsonReply(`{"responseEnvelope":{"responseCode":0},"responseData":[{"taskId":"T-9"}]}`),
			}}
			srv := httptest.NewServer(s)
			defer srv.Close()
			t.Setenv("AVANAN_BASE_URL", srv.URL)

			args := []string{"remediate", tc.action, tc.flag, tc.target, "--scope", "farm:A", "--yes", "--json"}
			if tc.flag == "--entity" {
				args = append(args, "--entity-type", "office365_emails_email")
			}
			stdout, stderr, err := executeCLI(t, args...)
			if err != nil {
				t.Fatalf("remediate: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
			}
			s.mu.Lock()
			bodies := append([]string(nil), s.bodies[key]...)
			s.mu.Unlock()
			if len(bodies) != 1 {
				t.Fatalf("%s received %d request(s), want 1", key, len(bodies))
			}
			var sent struct {
				RequestData map[string]any `json:"requestData"`
			}
			if jerr := json.Unmarshal([]byte(bodies[0]), &sent); jerr != nil {
				t.Fatalf("body is not JSON: %v: %s", jerr, bodies[0])
			}
			if got := sent.RequestData[tc.want]; got != tc.action {
				t.Errorf("requestData.%s = %v, want %q (body %s)", tc.want, got, tc.action, bodies[0])
			}
			if _, present := sent.RequestData[tc.wrong]; present {
				t.Errorf("requestData carries %s on the %s path (body %s)", tc.wrong, tc.path, bodies[0])
			}
		})
	}
}
