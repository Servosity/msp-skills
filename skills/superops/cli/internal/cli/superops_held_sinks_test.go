// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): report and ledger sinks through the credential
// kit (credkit-wiring).

package cli

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"superops-pp-cli/internal/cliutil/testenv"
	"superops-pp-cli/internal/credkit"
)

// A doctor probe code that is the base64 of a held short value (the round-1
// input: user "sam", code "c2Ft") is never shown; the denial is read from
// every code, so withholding one never hides it (round-2 P2).
func TestDoctorNeverShowsAHeldCodeAndKeepsTheDenial(t *testing.T) {
	if !heldChild(t) {
		return
	}
	const short = "z~Q4kP"
	credkit.Hold("minted:test", short)
	code := base64.StdEncoding.EncodeToString([]byte(short))
	codes, failed, err := doctorGraphQLErrorCodes([]byte(`{"errors":[{"message":"denied","extensions":{"code":"` + code + `"}},{"message":"x","extensions":{"code":"UNAUTHENTICATED"}}]}`))
	if err != nil || !failed || !doctorGraphQLDenied(codes) {
		t.Fatalf("codes %q failed=%v denied=%v err=%v", codes, failed, doctorGraphQLDenied(codes), err)
	}
	if shown := doctorShownCodes(codes); slices.Contains(shown, code) || !slices.Contains(shown, "UNAUTHENTICATED") {
		t.Errorf("shown %q: want UNAUTHENTICATED and not %q", shown, code)
	}
}

// No jobs ledger field keeps a held credential.
func TestJobsLedgerNeverKeepsAHeldCredential(t *testing.T) {
	if !heldChild(t) {
		return
	}
	testenv.Isolate(t)
	const held = "ledgerHeldTok2264"
	credkit.Hold("minted:test", held)
	if err := RecordJob(JobRow{JobID: "job-held-1", Resource: "tasks", Endpoint: "list " + held, Status: "failed", Error: "denied " + held}); !errors.Is(err, credkit.ErrWithheld) {
		t.Fatal("a row repeating a held value was recorded")
	}
	if err := RecordJob(JobRow{JobID: "job-clean-1", Resource: "tasks", Status: "ok"}); err != nil {
		t.Fatal(err)
	}
	p, err := jobsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if strings.Contains(string(b), held) || !strings.Contains(string(b), "job-clean-1") {
		t.Errorf("ledger:\n%s", b)
	}
}

// The round-1 input, held as a vendor response value (synccpu: the one value
// the store refuses): a successful tickets page whose record repeats it. Sync
// refuses that record (nothing of the value reaches the store file), stores
// the others (#612: one refused record no longer drops its batch) and exits
// clean, naming the record it withheld.
func TestSyncNeverStoresARecordThatRepeatsTheToken(t *testing.T) {
	if !heldChild(t) {
		return
	}
	credkit.Hold("response:token", heldResp)
	a, _ := twoTenantFakes(t)
	t.Setenv("SUPEROPS_SUBDOMAIN", "acme-a")
	a.data["getTicketList"][3]["subject"] = "echo " + heldResp
	dir := t.TempDir()
	db := filepath.Join(dir, "held.db")
	out, errOut, err := syncCapturingStderr(t, "sync", "--resources", "tickets", "--db", db)
	if err != nil || !strings.Contains(errOut, "withheld by the credential check") {
		t.Fatalf("want a clean exit naming the withheld record (err %v)\n%s\n%s", err, out, errOut)
	}
	if got, want := storeTicketRows(t, db), len(a.data["getTicketList"])-1; got != want {
		t.Fatalf("stored %d tickets, want %d: every record but the one that repeats the value (err %v)\n%s", got, want, err, out)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "held.db*"))
	for _, p := range files {
		if b, _ := os.ReadFile(p); strings.Contains(string(b), heldResp) {
			t.Fatalf("%s keeps the held value", filepath.Base(p))
		}
	}
}
