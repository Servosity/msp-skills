// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): regression tests for the batch-1b Codex
// round-1 inputs, pinned by skills/cipp/handfixes.json under
// "cipp-results-failure-vocabulary", "tenant-list-message-is-not-a-fleet",
// "fanout-save-integrity" and "token-error-text-allowlist".

package cli

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// fanoutUsersTwice stores u1/u2 for t.example, then runs the same fanout
// --save against second and reports the error and the users left.
func fanoutUsersTwice(t *testing.T, second string) (error, int) {
	t.Helper()
	_, db := startFakeCIPP(t, map[string][]string{
		"/ListUsers": {`[{"id":"u1"},{"id":"u2"}]`, second},
	})
	args := []string{"fanout", "--endpoint", "/ListUsers", "--tenant", "t.example", "--save", "--db", db, "--no-cache"}
	if _, err := runCLI(t, args...); err != nil {
		t.Fatal(err)
	}
	_, err := runCLI(t, args...)
	return err, storedRows(t, db, "users")
}

// TestFanoutLoneFailureObjectKeepsStoredUsers: Codex's input. A lone
// {"id":"error-1","error":"HTTP 403"} is not the user list; u1 and u2 stay.
func TestFanoutLoneFailureObjectKeepsStoredUsers(t *testing.T) {
	err, n := fanoutUsersTwice(t, `{"id":"error-1","error":"HTTP 403"}`)
	if err == nil || n != 2 {
		t.Fatalf("a lone failure object replaced the stored users: err=%v, %d left", err, n)
	}
}

// TestFanoutLoneObjectIsNotSaved: a singleton object with no failure signal
// is shown but not stored; u1 and u2 stay.
func TestFanoutLoneObjectIsNotSaved(t *testing.T) {
	err, n := fanoutUsersTwice(t, `{"id":"u3","displayName":"Only one"}`)
	if err == nil || n != 2 {
		t.Fatalf("a lone object replaced the stored users: err=%v, %d left", err, n)
	}
}

// TestFanoutSuccessFalseElementKeepsStoredUsers: Codex's input. An element
// with "success":false marks a partial list; u2 is not deleted.
func TestFanoutSuccessFalseElementKeepsStoredUsers(t *testing.T) {
	err, n := fanoutUsersTwice(t, `{"Results":[{"id":"u1"},{"id":"partial","success":false}]}`)
	if err == nil || n != 2 {
		t.Fatalf("a success:false element was accepted: err=%v, %d left", err, n)
	}
}

// TestBulkEnvelopeErrorBesideSuccessIsHeld: Codex's input through bulk. A
// success in Results beside an envelope error is conflicting: an error, sent
// once, never checkpointed as done.
func TestBulkEnvelopeErrorBesideSuccessIsHeld(t *testing.T) {
	f, _ := startFakeCIPP(t, map[string][]string{
		"/ExecOffboardUser": {`{"Results":["success"],"error":"HTTP 403"}`},
	})
	csvPath := writeOffboardCSV(t, "u")
	for i := 0; i < 2; i++ {
		if _, err := runCLI(t, "bulk", "--from", csvPath, "--execute", "--resume"); err == nil {
			t.Fatal("a success beside an envelope error was recorded as success")
		}
	}
	if n := countSeen(f, "/ExecOffboardUser"); n != 1 {
		t.Fatalf("the conflicting write was resent: %d request(s)", n)
	}
}

// TestWalkerKeepsTenantRecordsAndOutlookKeys: a tenant's own Graph error
// fields are left out only when validating the tenant list; in a write result
// or any other list they are failures. A false under a key that only contains
// the letters "ok" is not a failure.
func TestWalkerKeepsTenantRecordsAndOutlookKeys(t *testing.T) {
	tenant := `[{"defaultDomainName":"a.example","LastGraphError":"invalid_client:AADSTS7000215: Invalid client secret provided.","GraphErrorCount":3}]`
	if _, err := cippListRows([]byte(tenant), "tenant list", tenantRecordFields, "defaultDomainName"); err != nil {
		t.Fatalf("the tenant list refused a tenant's own Graph error fields: %v", err)
	}
	if detectCippResultsFailure([]byte(tenant)) == nil {
		t.Error("a write result carrying LastGraphError was not read as a failure")
	}
	if _, err := cippListRows([]byte(`[{"id":"r1","LastGraphError":"Failed"}]`), "GDAP relationship list", nil, "id"); err == nil {
		t.Error("a non-tenant list carrying LastGraphError was accepted")
	}
	if r := detectCippResultsFailure([]byte(`{"Results":[{"id":"u1","isOutlookMobileEnabled":false,"tokenLifetimePolicy":false}]}`)); r != nil {
		t.Errorf("an outlook/token key holding false read as a failure: %s", r.Message)
	}
	for _, body := range []string{`{"ok":false}`, `{"Results":[{"isOk":"false"}]}`, `{"requestSucceeded":false}`} {
		if detectCippResultsFailure([]byte(body)) == nil {
			t.Errorf("%s was not read as a failure", body)
		}
	}
}

// TestLoginTokenDecodeErrorQuotesNothing: Codex's input. HTTP 200 whose
// expires_in overflows int: the error names the kind, never the input.
func TestLoginTokenDecodeErrorQuotesNothing(t *testing.T) {
	const token = "123456789012345678901234567890"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"` + token + `","expires_in":` + token + `}`))
	}))
	defer srv.Close()
	out, err := runAuthLogin(t, srv.URL, filepath.Join(t.TempDir(), "config.toml"))
	if err == nil {
		t.Fatal("an undecodable token response was accepted")
	}
	if strings.Contains(err.Error()+out, token[:12]) {
		t.Fatalf("the token response was quoted in the error: %v %s", err, out)
	}
	if !strings.Contains(err.Error(), "could not be decoded") {
		t.Fatalf("the error does not name the kind: %v", err)
	}
}

// TestFanoutNamesWithFailureWordsAreRows: in a list, failure wording counts
// only under a status-like key, so a user named "Error Reporting" and a group
// named "Denied Access" are stored as rows.
func TestFanoutNamesWithFailureWordsAreRows(t *testing.T) {
	_, db := startFakeCIPP(t, map[string][]string{
		"/ListUsers":  {`{"Results":[{"id":"u1","displayName":"Error Reporting","userPrincipalName":"er@t.example"},{"id":"u2","displayName":"Ana"}]}`},
		"/ListGroups": {`[{"id":"g1","displayName":"Denied Access","description":"Blocked from sign-in"}]`},
	})
	for endpoint, want := range map[string]int{"/ListUsers": 2, "/ListGroups": 1} {
		if _, err := runCLI(t, "fanout", "--endpoint", endpoint, "--tenant", "t.example", "--save", "--db", db, "--no-cache"); err != nil {
			t.Fatalf("%s: a row whose name holds a failure word was refused: %v", endpoint, err)
		}
		if n := storedRows(t, db, resourceTypeForEndpoint(endpoint)); n != want {
			t.Fatalf("%s: %d rows stored, want %d", endpoint, n, want)
		}
	}
}

// TestFanoutStatusFailedElementKeepsStoredUsers: an element whose status is
// "failed" refuses the list; u1 and u2 stay.
func TestFanoutStatusFailedElementKeepsStoredUsers(t *testing.T) {
	err, n := fanoutUsersTwice(t, `{"Results":[{"id":"u1"},{"id":"u2","status":"failed"}]}`)
	if err == nil || n != 2 {
		t.Fatalf("an element with status failed was accepted: err=%v, %d left", err, n)
	}
}

// TestFanoutStatusKeyWordingKeepsStoredUsers: failure wording under a
// status-like key (message) refuses the list; u1 and u2 stay.
func TestFanoutStatusKeyWordingKeepsStoredUsers(t *testing.T) {
	err, n := fanoutUsersTwice(t, `[{"id":"u1"},{"id":"u2","message":"Failed to retrieve remaining users"}]`)
	if err == nil || n != 2 {
		t.Fatalf("failure wording in a message field was accepted: err=%v, %d left", err, n)
	}
}
