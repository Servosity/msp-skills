// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): regression tests for the fresh Codex round-2
// inputs, pinned by skills/cipp/handfixes.json under
// "tenant-list-message-is-not-a-fleet" and "bulk-conflict-needs-review".

package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"cipp-pp-cli/internal/store"
)

// TestFanoutStatusErrorElementKeepsStoredRows: {"id":"error-1","status":"error"}
// is not a row; stored p1 and p2 stay.
func TestFanoutStatusErrorElementKeepsStoredRows(t *testing.T) {
	_, db := startFakeCIPP(t, map[string][]string{
		"/ListConditionalAccessPolicies": {
			`[{"id":"p1"},{"id":"p2"}]`,
			`{"Results":[{"id":"p1"},{"id":"error-1","status":"error"}]}`,
		},
	})
	args := []string{"fanout", "--endpoint", "/ListConditionalAccessPolicies", "--tenant", "t.example", "--save", "--db", db, "--no-cache"}
	if _, err := runCLI(t, args...); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, args...); err == nil {
		t.Fatal("an element with status error was accepted as a row")
	}
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if rows, _ := readResourceRows(s, "conditionalaccesspolicies"); len(rows) != 2 {
		t.Fatalf("stored rows changed: %d left, want p1 and p2", len(rows))
	}
}

// TestTenantEnvelopeStatusErrorKeepsStoredTenants: an envelope "status":"error"
// beside a partial Results list refuses the fleet; stored a and b stay.
func TestTenantEnvelopeStatusErrorKeepsStoredTenants(t *testing.T) {
	f, db := startFakeCIPP(t, map[string][]string{
		"/ListTenants": {
			`[{"defaultDomainName":"a.example","customerId":"a"},{"defaultDomainName":"b.example","customerId":"b"}]`,
			`{"Results":[{"defaultDomainName":"a.example","customerId":"a"}],"status":"error"}`,
		},
		"/ListMFAUsers": {`[{"id":"u1","UPN":"x@example"}]`},
	})
	args := []string{"fanout", "--endpoint", "/ListMFAUsers", "--all-tenants", "--save", "--db", db, "--no-cache"}
	if _, err := runCLI(t, args...); err != nil {
		t.Fatal(err)
	}
	before := countSeen(f, "/ListMFAUsers")
	if _, err := runCLI(t, args...); err == nil {
		t.Fatal("a tenant list with an envelope status error was used as the fleet")
	}
	if after := countSeen(f, "/ListMFAUsers"); after != before {
		t.Fatalf("fanout ran against a partial fleet (%d more read(s))", after-before)
	}
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if tenants, _ := s.List("tenants", 100); len(tenants) != 2 {
		t.Fatalf("the stored tenant list changed: %d left, want a and b", len(tenants))
	}
}

func writeOffboardCSV(t *testing.T, user string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "offboards.csv")
	body := "action,tenant,params_json\n" +
		`offboard,t.example,"{""tenantFilter"":""t.example"",""user"":[""` + user + `@t.example""],""DisableSignIn"":true}"` + "\n"
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestBulkNeedsReviewHoldStoresNoCIPPData: CIPP_NO_LOCAL_DATA=1, one offboard
// row for alice, a conflicting answer naming alice. The hold is kept (the row
// is not resent) but nothing of the response or the row reaches the disk.
func TestBulkNeedsReviewHoldStoresNoCIPPData(t *testing.T) {
	f, _ := startFakeCIPP(t, map[string][]string{
		"/ExecOffboardUser": {`{"Results":[{"state":"success","message":"Reset password for alice@t.example; failed to remove licenses"}]}`},
	})
	t.Setenv("CIPP_NO_LOCAL_DATA", "1")
	csvPath := writeOffboardCSV(t, "alice")
	if _, err := runCLI(t, "bulk", "--from", csvPath, "--execute"); err == nil {
		t.Fatal("a conflicting write was reported as success")
	}
	if _, err := runCLI(t, "bulk", "--from", csvPath, "--execute"); err == nil {
		t.Fatal("a row that needs review must keep the run non-zero")
	}
	if n := countSeen(f, "/ExecOffboardUser"); n != 1 {
		t.Fatalf("the conflicting write was resent: %d request(s)", n)
	}
	home := os.Getenv("HOME")
	sawHold := false
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || p == csvPath {
			return nil
		}
		if b, rerr := os.ReadFile(p); rerr == nil {
			if strings.Contains(string(b), `"needs_review"`) {
				sawHold = true
			}
			for _, leak := range []string{"alice", "failed to remove licenses", "Reset password"} {
				if strings.Contains(string(b), leak) {
					t.Errorf("%q was written to %s", leak, p)
				}
			}
		}
		return nil
	})
	if !sawHold {
		t.Fatal("no needs-review hold was found under HOME; the leak scan looked in the wrong place")
	}
}

// TestBulkRefusesUnwritableCheckpointDir: with the checkpoints directory not
// writable, bulk --execute refuses to start and sends nothing.
func TestBulkRefusesUnwritableCheckpointDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory mode bits do not restrict writes on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory mode bits")
	}
	f, _ := startFakeCIPP(t, map[string][]string{
		"/ExecOffboardUser": {`{"Results":[{"state":"success","message":"Reset password for alice@t.example; failed to remove licenses"}]}`},
	})
	state := filepath.Join(t.TempDir(), "cipp-state")
	if err := os.MkdirAll(filepath.Join(state, "checkpoints"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(state, "checkpoints"), 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(state, "checkpoints"), 0o700) })
	t.Setenv("CIPP_STATE_DIR", state)
	csvPath := writeOffboardCSV(t, "alice")
	for i := 0; i < 2; i++ {
		if _, err := runCLI(t, "bulk", "--from", csvPath, "--execute"); err == nil {
			t.Fatal("bulk --execute started with an unwritable checkpoint directory")
		}
	}
	if n := countSeen(f, "/ExecOffboardUser"); n != 0 {
		t.Fatalf("a write was sent with no way to hold it: %d request(s)", n)
	}
}
