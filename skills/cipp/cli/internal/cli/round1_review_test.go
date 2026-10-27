// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/cipp/handfixes.json under the
// "export-tail-read-endpoints-only" and "tenant-list-message-is-not-a-fleet"
// entries.

package cli

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cipp-pp-cli/internal/cliutil/testenv"
	"cipp-pp-cli/internal/store"
)

// TestExportRefusesActionEndpoints: export is an MCP tool; it must never GET
// a CIPP action endpoint such as /ExecExtensionClearHIBPKey.
func TestExportRefusesActionEndpoints(t *testing.T) {
	f, _ := startFakeCIPP(t, map[string][]string{
		"/ExecExtensionClearHIBPKey": {`{"Results":"cleared"}`},
		"/ExecRunBackup":             {`{"Results":"started"}`},
	})
	for _, r := range []string{"exec-extension-clear-hibpkey", "exec-run-backup", "remove-standard"} {
		if _, err := runCLI(t, "export", r, "--no-cache"); err == nil {
			t.Errorf("export %s ran", r)
		}
		if _, err := runCLI(t, "tail", r, "--no-cache"); err == nil {
			t.Errorf("tail %s ran", r)
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.seen) != 0 {
		t.Fatalf("an action endpoint was called: %v", f.seen)
	}
}

// TestTenantListWithMessageIsRefused: CIPP appending a failure string to a
// partial tenant list must not become a fleet that looks complete.
func TestTenantListWithMessageIsRefused(t *testing.T) {
	for _, body := range []string{
		`{"Results":[{"defaultDomainName":"a.example"},"Failed to retrieve remaining tenants"]}`,
		`{"Results":"Access denied"}`,
		`[{"defaultDomainName":"a.example"},"Failed to retrieve remaining tenants"]`,
	} {
		if refs, err := parseTenantArray(json.RawMessage(body)); err == nil {
			t.Errorf("parseTenantArray(%s) = %v, want an error", body, refs)
		}
	}
	refs, err := parseTenantArray(json.RawMessage(`{"Results":[{"defaultDomainName":"a.example"},{"defaultDomainName":"b.example"}]}`))
	if err != nil || len(refs) != 2 {
		t.Fatalf("a clean tenant list failed: %v %v", refs, err)
	}
}

// TestFanoutRefusesAllMessageArray: a bare array of CIPP failure strings is
// not rows, so a tenant's stored rows survive it.
func TestFanoutRefusesAllMessageArray(t *testing.T) {
	_, db := startFakeCIPP(t, map[string][]string{
		"/ListConditionalAccessPolicies": {
			`[{"id":"p1","state":"enabled"},{"id":"p2","state":"enabled"}]`,
			`["Failed to retrieve policies"]`,
		},
	})
	args := []string{"fanout", "--endpoint", "/ListConditionalAccessPolicies", "--tenant", "t.example", "--save", "--db", db, "--no-cache"}
	if _, err := runCLI(t, args...); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, args...); err == nil {
		t.Fatal("a failure-only response was reported as success")
	}
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	rows, _ := readResourceRows(s, "conditionalaccesspolicies")
	if len(rows) != 2 {
		t.Fatalf("stored rows changed: %d left", len(rows))
	}
}

// TestAuthLoginTransportErrorWithholdsEncodedSecret: a token endpoint that
// redirects to another host with the client secret in the Location (base64)
// must not get it printed in the refused-redirect error.
func TestAuthLoginTransportErrorWithholdsEncodedSecret(t *testing.T) {
	echo := base64.StdEncoding.EncodeToString([]byte(loginTestSecret))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://other-host.invalid/?echo="+echo, http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	_, err := runAuthLogin(t, srv.URL, filepath.Join(t.TempDir(), "config.toml"))
	if err == nil {
		t.Fatal("a cross-host redirect must fail the login")
	}
	if msg := err.Error(); strings.Contains(msg, echo) || strings.Contains(msg, loginTestSecret) {
		t.Fatalf("the redirect error printed the client secret:\n%s", msg)
	}
}

// TestRowsCarryMessagesAllStringFailure: an all-string array is a failure only
// when an entry uses CIPP's failure vocabulary; other string lists are data.
func TestRowsCarryMessagesAllStringFailure(t *testing.T) {
	if _, err := itemsFromResponse(json.RawMessage(`["Failed to retrieve policies"]`)); err == nil {
		t.Fatal("a failure-only array was accepted as rows")
	}
	if _, err := itemsFromResponse(json.RawMessage(`["a.example","b.example"]`)); err != nil {
		t.Fatalf("a plain string list was refused: %v", err)
	}
}

// TestGDAPListWithMessageIsRefused: a bare mixed array (rows plus a CIPP
// failure string) must not become a relationship list that looks complete.
func TestGDAPListWithMessageIsRefused(t *testing.T) {
	for _, body := range []string{
		`[{"id":"r1","status":"active"},"Failed to retrieve remaining relationships"]`,
		`{"Results":[{"id":"r1"},"Failed to retrieve remaining relationships"]}`,
		`{"Results":"Access denied"}`,
		`"Access denied"`,
	} {
		if rows, err := gdapRelationshipRows(json.RawMessage(body)); err == nil {
			t.Errorf("gdapRelationshipRows(%s) = %v, want an error", body, rows)
		}
	}
}

// TestCippResultsFailureTrustsState: a structured Results entry's own state
// decides, and a "Successfully ..." line naming an "Exception" group is a
// success; plain failure text still fails.
func TestCippResultsFailureTrustsState(t *testing.T) {
	for body, wantFail := range map[string]bool{
		`{"Results":[{"resultText":"Password reset accepted. It will fail if writeback is not enabled; this cannot be turned off.","state":"success"}]}`: true, // conflicting since the fresh round 1 (success state + failure text): an error, never resent
		`{"Results":[{"resultText":"Password reset done","state":"error"}]}`:                                                                             true,
		`{"Results":["Successfully removed pat@t.example from group 'MFA Exception Group'"]}`:                                                            true, // conflicting since the fresh round 2: any failure word with any success word is an error, never resent
		`{"Results":["Failed to disable sign-in for u@t.example"]}`:                                                                                      true,
		`{"Results":"Access denied"}`:                                  true,
		`{"Results":[{"resultText":"Could not convert the mailbox"}]}`: true,
	} {
		if got := detectCippResultsFailure([]byte(body)) != nil; got != wantFail {
			t.Errorf("detectCippResultsFailure(%s) failed=%v, want %v", body, got, wantFail)
		}
	}
}

// TestBulkCheckpointPerFile: two CSVs with the same name in different folders
// (this week's and next week's offboards.csv) never share a checkpoint.
func TestBulkCheckpointPerFile(t *testing.T) {
	testenv.Isolate(t)
	a := filepath.Join(t.TempDir(), "offboards.csv")
	b := filepath.Join(t.TempDir(), "offboards.csv")
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, []byte("action,tenant,params_json\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	pa, err := bulkCheckpointPath(a, []byte("same-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	pb, err := bulkCheckpointPath(b, []byte("same-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	if pa == pb {
		t.Fatalf("two different CSV files share the checkpoint %s", pa)
	}
	again, _ := bulkCheckpointPath(a, []byte("same-bytes"))
	if again != pa {
		t.Fatalf("the same file got a different checkpoint: %s vs %s", pa, again)
	}
}
