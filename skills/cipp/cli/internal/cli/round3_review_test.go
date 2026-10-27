// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): regression tests for the Codex round-3 inputs,
// pinned by skills/cipp/handfixes.json under "token-error-text-allowlist",
// "cipp-results-failure-vocabulary", "tenant-list-message-is-not-a-fleet" and
// "bulk-checkpoint-per-file".

package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cipp-pp-cli/internal/store"
)

// TestAuthLoginPrintsNoTokenEndpointProse: Codex round 3 #1. The token is
// echoed in plain prose next to an AADSTS code; only the code may print.
func TestAuthLoginPrintsNoTokenEndpointProse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":"invalid_client","access_token":"minted_bearer_value","error_description":"AADSTS7000215: minted_bearer_value"}`)
	}))
	defer srv.Close()
	_, err := runAuthLogin(t, srv.URL, filepath.Join(t.TempDir(), "config.toml"))
	if err == nil {
		t.Fatal("a 400 from the token endpoint must surface as an error")
	}
	msg := err.Error()
	if strings.Contains(msg, "minted_bearer_value") {
		t.Fatalf("the echoed token printed:\n%s", msg)
	}
	if !strings.Contains(msg, "invalid_client") || !strings.Contains(msg, "AADSTS7000215") {
		t.Fatalf("the error code was lost:\n%s", msg)
	}
}

// TestCippResultsEveryFieldIsRead: Codex round 3 #2. An empty field never
// ends the scan; any field that signals failure makes the entry a failure.
func TestCippResultsEveryFieldIsRead(t *testing.T) {
	for body, wantFail := range map[string]bool{
		`{"Results":[{"resultText":"","message":"Failed to disable sign-in"}]}`:                                                                          true,
		`{"Results":[{"resultText":"Done","message":"","text":"Could not remove licenses"}]}`:                                                            true,
		`{"Results":[{"state":"success","resultText":"Failed to remove licenses"}]}`:                                                                     true,
		`{"Results":[{"resultText":"Offboarded u@t.example","error":"Throttled by Graph"}]}`:                                                             true,
		`{"Results":[{"Status":"Error","ResultText":""}]}`:                                                                                               true,
		`{"Results":[{"resultText":"Password reset accepted. It will fail if writeback is not enabled; this cannot be turned off.","state":"success"}]}`: true, // conflicting since the fresh round 1 (success state + failure text): an error, never resent
		`{"Results":[{"resultText":"","message":"Successfully disabled sign-in"}]}`:                                                                      false,
	} {
		if got := detectCippResultsFailure([]byte(body)) != nil; got != wantFail {
			t.Errorf("detectCippResultsFailure(%s) failed=%v, want %v", body, got, wantFail)
		}
	}
}

// TestTenantListWithFailureObjectIsRefused: Codex round 3 #3, end to end. A
// structured failure entry next to tenant rows refuses the whole list: fanout
// fails and no partial tenant list is saved.
func TestTenantListWithFailureObjectIsRefused(t *testing.T) {
	f, db := startFakeCIPP(t, map[string][]string{
		"/ListTenants":  {`{"Results":[{"defaultDomainName":"a.example"},{"state":"error","resultText":"Failed to retrieve remaining tenants"}]}`},
		"/ListMFAUsers": {`[{"id":"u1","UPN":"x@a.example"}]`},
	})
	if _, err := runCLI(t, "fanout", "--endpoint", "/ListMFAUsers", "--all-tenants", "--save", "--db", db, "--no-cache"); err == nil {
		t.Fatal("a tenant list carrying a failure entry was used as the fleet")
	}
	f.mu.Lock()
	for _, seen := range f.seen {
		if strings.HasPrefix(seen, "/ListMFAUsers") {
			t.Errorf("fanout ran against a partial fleet: %v", f.seen)
		}
	}
	f.mu.Unlock()
	if _, statErr := os.Stat(db); statErr == nil {
		s, err := store.Open(db)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		if rows, _ := s.List("tenants", 100); len(rows) != 0 {
			t.Fatalf("a partial tenant list was saved: %d rows", len(rows))
		}
	}
	for _, body := range []string{
		`[{"id":"r1","status":"active"},{"state":"error","resultText":"Failed to retrieve remaining relationships"}]`,
		`{"Results":[{"id":"r1"},{"customerTenantId":"no-id"}]}`,
	} {
		if rows, err := gdapRelationshipRows(json.RawMessage(body)); err == nil {
			t.Errorf("gdapRelationshipRows(%s) = %v, want an error", body, rows)
		}
	}
	if _, err := itemsFromResponse(json.RawMessage(`[{"id":"p1"},{"state":"error","resultText":"Failed to retrieve remaining policies"}]`)); err == nil {
		t.Error("a fanout response with a failure result object was accepted as rows")
	}
}

// TestBulkResumeEditedCSVSameSizeAndTime: Codex round 3 #4, end to end. An
// edited CSV with the same length and modification time starts from row 0.
func TestBulkResumeEditedCSVSameSizeAndTime(t *testing.T) {
	f, _ := startFakeCIPP(t, map[string][]string{
		"/ExecOffboardUser": {`{"Results":["Successfully offboarded"]}`},
	})
	csvPath := filepath.Join(t.TempDir(), "offboards.csv")
	write := func(user string) {
		body := "action,tenant,params_json\n" +
			`offboard,t.example,"{""tenantFilter"":""t.example"",""user"":[""` + user + `@t.example""],""DisableSignIn"":true}"` + "\n"
		if err := os.WriteFile(csvPath, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("ann")
	stamp := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(csvPath, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "bulk", "--from", csvPath, "--execute", "--resume"); err != nil {
		t.Fatalf("first run: %v", err)
	}
	write("bob")
	if err := os.Chtimes(csvPath, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "bulk", "--from", csvPath, "--execute", "--resume"); err != nil {
		t.Fatalf("second run: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	posts := 0
	for _, seen := range f.seen {
		if strings.HasPrefix(seen, "/ExecOffboardUser") {
			posts++
		}
	}
	if posts != 2 {
		t.Fatalf("the edited batch was skipped: %d offboard request(s), want 2", posts)
	}
}
