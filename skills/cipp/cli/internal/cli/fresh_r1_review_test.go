// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): regression tests for the fresh Codex round-1
// inputs, pinned by skills/cipp/handfixes.json under
// "bulk-conflict-needs-review" and "tenant-list-message-is-not-a-fleet".

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cipp-pp-cli/internal/store"
)

func countSeen(f *fakeCIPP, prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, s := range f.seen {
		if strings.HasPrefix(s, prefix) {
			n++
		}
	}
	return n
}

// TestBulkConflictingWriteIsNeverResent: a success state beside failure text
// is an error for bulk, is never checkpointed as done, and is not resent by
// --resume (or by a plain re-run of the same CSV).
func TestBulkConflictingWriteIsNeverResent(t *testing.T) {
	f, _ := startFakeCIPP(t, map[string][]string{
		"/ExecOffboardUser": {`{"Results":[{"state":"success","resultText":"","message":"The operation failed: access denied"}]}`},
	})
	csvPath := filepath.Join(t.TempDir(), "offboards.csv")
	body := "action,tenant,params_json\n" +
		`offboard,t.example,"{""tenantFilter"":""t.example"",""user"":[""u@t.example""],""DisableSignIn"":true}"` + "\n"
	if err := os.WriteFile(csvPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "bulk", "--from", csvPath, "--execute", "--resume"); err == nil {
		t.Fatal("a conflicting write was reported as success")
	}
	if n := countSeen(f, "/ExecOffboardUser"); n != 1 {
		t.Fatalf("first run sent %d request(s), want 1", n)
	}
	if _, err := runCLI(t, "bulk", "--from", csvPath, "--execute", "--resume"); err == nil {
		t.Fatal("a row that needs review must keep the run non-zero")
	}
	if _, err := runCLI(t, "bulk", "--from", csvPath, "--execute"); err == nil {
		t.Fatal("a row that needs review must keep a plain re-run non-zero")
	}
	if n := countSeen(f, "/ExecOffboardUser"); n != 1 {
		t.Fatalf("the conflicting write was resent automatically: %d request(s)", n)
	}
}

// TestFanoutFailureObjectWithIDKeepsStoredRows: an element with an id and a
// failure state is not a row; stored p1 and p2 stay.
func TestFanoutFailureObjectWithIDKeepsStoredRows(t *testing.T) {
	_, db := startFakeCIPP(t, map[string][]string{
		"/ListConditionalAccessPolicies": {
			`[{"id":"p1","displayName":"Require MFA","state":"enabled"},{"id":"p2","displayName":"Block legacy auth","state":"enabled"}]`,
			`{"Results":[{"id":"p1","displayName":"Require MFA","state":"enabled"},{"id":"error-1","state":"error","resultText":"Failed to retrieve remaining policies"}]}`,
		},
	})
	args := []string{"fanout", "--endpoint", "/ListConditionalAccessPolicies", "--tenant", "t.example", "--save", "--db", db, "--no-cache"}
	if _, err := runCLI(t, args...); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, args...); err == nil {
		t.Fatal("a failure object with an id was accepted as a row")
	}
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	rows, _ := readResourceRows(s, "conditionalaccesspolicies")
	if len(rows) != 2 {
		t.Fatalf("stored rows changed: %d left, want p1 and p2", len(rows))
	}
}

// TestTenantEnvelopeFailureKeepsStoredTenants: an envelope-level error beside
// a partial Results list refuses the fleet; stored tenants a and b stay.
func TestTenantEnvelopeFailureKeepsStoredTenants(t *testing.T) {
	f, db := startFakeCIPP(t, map[string][]string{
		"/ListTenants": {
			`[{"defaultDomainName":"a.example","customerId":"a"},{"defaultDomainName":"b.example","customerId":"b"}]`,
			`{"Results":[{"defaultDomainName":"a.example","customerId":"a"}],"error":"Failed to retrieve remaining tenants"}`,
		},
		"/ListMFAUsers": {`[{"id":"u1","UPN":"x@example"}]`},
	})
	args := []string{"fanout", "--endpoint", "/ListMFAUsers", "--all-tenants", "--save", "--db", db, "--no-cache"}
	if _, err := runCLI(t, args...); err != nil {
		t.Fatal(err)
	}
	before := countSeen(f, "/ListMFAUsers")
	if _, err := runCLI(t, args...); err == nil {
		t.Fatal("a tenant list with an envelope error was used as the fleet")
	}
	if after := countSeen(f, "/ListMFAUsers"); after != before {
		t.Fatalf("fanout ran against a partial fleet (%d more read(s))", after-before)
	}
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	tenants, _ := s.List("tenants", 100)
	if len(tenants) != 2 {
		t.Fatalf("the stored tenant list changed: %d left, want a and b", len(tenants))
	}
}

// TestFanoutEnvelopeFailureKeepsStoredRows: an envelope-level failure beside
// the Results rows refuses the response; stored p1 and p2 stay.
func TestFanoutEnvelopeFailureKeepsStoredRows(t *testing.T) {
	_, db := startFakeCIPP(t, map[string][]string{
		"/ListConditionalAccessPolicies": {
			`[{"id":"p1","displayName":"Require MFA"},{"id":"p2","displayName":"Block legacy auth"}]`,
			`{"Results":[{"id":"p1","displayName":"Require MFA"}],"error":"Failed to retrieve remaining policies"}`,
		},
	})
	args := []string{"fanout", "--endpoint", "/ListConditionalAccessPolicies", "--tenant", "t.example", "--save", "--db", db, "--no-cache"}
	if _, err := runCLI(t, args...); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, args...); err == nil {
		t.Fatal("an envelope error beside partial rows was accepted")
	}
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	rows, _ := readResourceRows(s, "conditionalaccesspolicies")
	if len(rows) != 2 {
		t.Fatalf("stored rows changed: %d left, want p1 and p2", len(rows))
	}
}
