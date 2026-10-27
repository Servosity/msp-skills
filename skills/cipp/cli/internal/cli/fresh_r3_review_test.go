// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): regression tests for the fresh Codex round-3
// inputs, pinned by skills/cipp/handfixes.json under
// "tenant-list-message-is-not-a-fleet", "cipp-results-failure-vocabulary" and
// "bulk-conflict-needs-review".

package cli

import (
	"crypto/sha256"
	"os"
	"runtime"
	"testing"

	"cipp-pp-cli/internal/store"
)

func storedRows(t *testing.T, db, resource string) int {
	t.Helper()
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	rows, _ := readResourceRows(s, resource)
	return len(rows)
}

// TestFanoutErrorFieldElementKeepsStoredRows: Codex's input. An element with
// a populated error field is not a row; stored p1 and p2 stay.
func TestFanoutErrorFieldElementKeepsStoredRows(t *testing.T) {
	_, db := startFakeCIPP(t, map[string][]string{
		"/ListConditionalAccessPolicies": {
			`[{"id":"p1"},{"id":"p2"}]`,
			`{"Results":[{"id":"p1"},{"id":"error-1","error":"HTTP 403"}]}`,
		},
	})
	args := []string{"fanout", "--endpoint", "/ListConditionalAccessPolicies", "--tenant", "t.example", "--save", "--db", db, "--no-cache"}
	if _, err := runCLI(t, args...); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, args...); err == nil {
		t.Fatal("an element with a populated error field was accepted as a row")
	}
	if n := storedRows(t, db, "conditionalaccesspolicies"); n != 2 {
		t.Fatalf("stored rows changed: %d left, want p1 and p2", n)
	}
}

// TestFanoutEnvelopeErrorFieldKeepsStoredRows: an envelope "error":"HTTP 403"
// beside a partial Results list refuses the response; stored p1 and p2 stay.
func TestFanoutEnvelopeErrorFieldKeepsStoredRows(t *testing.T) {
	_, db := startFakeCIPP(t, map[string][]string{
		"/ListConditionalAccessPolicies": {
			`[{"id":"p1"},{"id":"p2"}]`,
			`{"Results":[{"id":"p1"}],"error":"HTTP 403"}`,
		},
	})
	args := []string{"fanout", "--endpoint", "/ListConditionalAccessPolicies", "--tenant", "t.example", "--save", "--db", db, "--no-cache"}
	if _, err := runCLI(t, args...); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, args...); err == nil {
		t.Fatal("a list with an envelope error field was accepted")
	}
	if n := storedRows(t, db, "conditionalaccesspolicies"); n != 2 {
		t.Fatalf("stored rows changed: %d left, want p1 and p2", n)
	}
}

// TestTenantEnvelopeErrorFieldKeepsStoredTenants: the same envelope on the
// tenant list refuses the fleet; stored a and b stay and nothing fans out.
func TestTenantEnvelopeErrorFieldKeepsStoredTenants(t *testing.T) {
	f, db := startFakeCIPP(t, map[string][]string{
		"/ListTenants": {
			`[{"defaultDomainName":"a.example","customerId":"a"},{"defaultDomainName":"b.example","customerId":"b"}]`,
			`{"Results":[{"defaultDomainName":"a.example","customerId":"a"}],"error":"HTTP 403"}`,
		},
		"/ListMFAUsers": {`[{"id":"u1","UPN":"x@example"}]`},
	})
	args := []string{"fanout", "--endpoint", "/ListMFAUsers", "--all-tenants", "--save", "--db", db, "--no-cache"}
	if _, err := runCLI(t, args...); err != nil {
		t.Fatal(err)
	}
	before := countSeen(f, "/ListMFAUsers")
	if _, err := runCLI(t, args...); err == nil {
		t.Fatal("a tenant list with an envelope error field was used as the fleet")
	}
	if after := countSeen(f, "/ListMFAUsers"); after != before {
		t.Fatalf("fanout ran against a partial fleet (%d more read(s))", after-before)
	}
	if n := storedRows(t, db, "tenants"); n != 2 {
		t.Fatalf("the stored tenant list changed: %d left, want a and b", n)
	}
}

// TestTenantGraphErrorFieldsAreTenantData: CIPP's tenant records carry
// LastGraphError, LastGraphTokenError and GraphErrorCount for a tenant whose
// Graph token failed. That tenant is still in the fleet, so the list is used
// and both tenants are read.
func TestTenantGraphErrorFieldsAreTenantData(t *testing.T) {
	f, db := startFakeCIPP(t, map[string][]string{
		"/ListTenants":  {`[{"defaultDomainName":"a.example","customerId":"a","LastGraphError":"invalid_client:AADSTS7000215: Invalid client secret provided.","LastGraphTokenError":"AADSTS7000215","GraphErrorCount":3},{"defaultDomainName":"b.example","customerId":"b","LastGraphError":"","GraphErrorCount":0}]`},
		"/ListMFAUsers": {`[{"id":"u1","UPN":"x@example"}]`},
	})
	if _, err := runCLI(t, "fanout", "--endpoint", "/ListMFAUsers", "--all-tenants", "--save", "--db", db, "--no-cache"); err != nil {
		t.Fatalf("a tenant list with Graph error fields was refused: %v", err)
	}
	if n := countSeen(f, "/ListMFAUsers"); n != 2 {
		t.Fatalf("fanout read %d tenant(s), want a and b", n)
	}
}

// TestBulkSuccessStateWithErrorFieldIsHeld: Codex's input. A success state
// beside a populated error is conflicting: an error, never resent.
func TestBulkSuccessStateWithErrorFieldIsHeld(t *testing.T) {
	f, _ := startFakeCIPP(t, map[string][]string{
		"/ExecOffboardUser": {`{"Results":[{"state":"success","error":"HTTP 403"}]}`},
	})
	csvPath := writeOffboardCSV(t, "u")
	for i := 0; i < 2; i++ {
		if _, err := runCLI(t, "bulk", "--from", csvPath, "--execute", "--resume"); err == nil {
			t.Fatal("a success state with a populated error was recorded as success")
		}
	}
	if n := countSeen(f, "/ExecOffboardUser"); n != 1 {
		t.Fatalf("the conflicting write was resent: %d request(s)", n)
	}
}

// TestBulkRefusesReadOnlyCheckpoint: Codex's input. This CSV's checkpoint
// exists with mode 0400 in a writable directory; bulk --execute refuses to
// start and sends nothing.
func TestBulkRefusesReadOnlyCheckpoint(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode bits do not restrict writes on Windows the same way")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores file mode bits")
	}
	f, _ := startFakeCIPP(t, map[string][]string{
		"/ExecOffboardUser": {`{"Results":[{"state":"success","message":"Successfully reset password; failed to disable sign-in"}]}`},
	})
	csvPath := writeOffboardCSV(t, "u")
	raw, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	cpPath, err := bulkCheckpointPath(csvPath, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cpPath, []byte(`{"completed":[]}`), 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(cpPath, 0o600) })
	for i := 0; i < 2; i++ {
		if _, err := runCLI(t, "bulk", "--from", csvPath, "--execute"); err == nil {
			t.Fatal("bulk --execute started with a read-only checkpoint")
		}
	}
	if n := countSeen(f, "/ExecOffboardUser"); n != 0 {
		t.Fatalf("a write was sent with no way to hold it: %d request(s)", n)
	}
}
