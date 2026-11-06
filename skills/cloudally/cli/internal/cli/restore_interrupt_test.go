// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// Ctrl-C while restore watch's status request is in flight is an
// interrupted watch: exit 8 with the watch-again command, as it is during
// the interval wait.
func TestRestoreWatchInterruptedDuringFetchExits8(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	batchID := submitOneMailbox(t, "a@contoso.com")

	reached := make(chan struct{})
	var once sync.Once
	api.set(func(f *fakeRestoreAPI) {
		f.onList = func(r *http.Request) {
			once.Do(func() { close(reached) })
			<-r.Context().Done() // the request is held until the client gives up
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-reached
		cancel()
	}()
	stdout, _, err := runAuthCmdCtx(t, ctx, "restore", "watch", "--batch", batchID, "--interval", "10ms", "--json")
	if err == nil || ExitCode(err) != ExitJobPending {
		t.Fatalf("Ctrl-C during the status request: want exit 8, got %v", err)
	}
	if want := "Check again later with: cloudally-cli restore watch --batch " + batchID; !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), "interrupted") {
		t.Fatalf("message should say it was interrupted and print %q:\n%v", want, err)
	}
	var w watchOutput
	if jerr := json.Unmarshal([]byte(stdout), &w); jerr != nil || w.Done || w.Tally.Pending != 1 {
		t.Fatalf("summary should still print the last tally: done %v tally %+v (%v)\n%s", w.Done, w.Tally, jerr, stdout)
	}
}

// The other direction: a real API error with a live context is still an
// API error, not an interrupted watch.
func TestRestoreWatchAPIErrorIsNotInterrupted(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	batchID := submitOneMailbox(t, "a@contoso.com")
	api.set(func(f *fakeRestoreAPI) { f.listStatus = http.StatusForbidden })
	_, _, err := runAuthCmd(t, "", "restore", "watch", "--batch", batchID, "--once", "--json")
	if err == nil || ExitCode(err) == ExitJobPending || strings.Contains(err.Error(), "Check again later") {
		t.Fatalf("a 403 on the status list must not read as an interrupted watch, got %v", err)
	}
	if want := ExitCode(authErr(context.Canceled)); ExitCode(err) != want {
		t.Fatalf("exit %d, want %d", ExitCode(err), want)
	}
}

// A --retry-unknown resend the API refuses (here 401) answers only that
// resend: the first send may have been accepted, so the row and its guard
// claim stay UNKNOWN and a plain --resume never sends it again.
func TestRestoreRetryUnknownRefusalStaysUnknown(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	api.setStatus("b@contoso.com", http.StatusBadGateway)
	args := submitArgs("--date", "2026-10-01", "--entities", "a@contoso.com,b@contoso.com", "--concurrency", "1")
	if _, _, err := runAuthCmd(t, "", args...); err == nil {
		t.Fatal("first submit should report b UNKNOWN")
	}
	api.setStatus("b@contoso.com", http.StatusUnauthorized)
	_, _, err := runAuthCmd(t, "", append(append([]string{}, args...), "--resume", "--retry-unknown")...)
	if err == nil {
		t.Fatal("a refused resend should not succeed")
	}
	if b := ledgerRowByEntity(t, "b@contoso.com"); b.SubmitStatus != jobUnknown || b.Status != jobUnknown {
		t.Fatalf("after a refused --retry-unknown, b = status %s submit %s; want UNKNOWN", b.Status, b.SubmitStatus)
	}
	g, gerr := openRestoreGuard(context.Background())
	if gerr != nil {
		t.Fatal(gerr)
	}
	var state string
	_ = g.db.QueryRow(`SELECT state FROM restore_claims WHERE entity = 'b@contoso.com' AND superseded = 0 ORDER BY id DESC LIMIT 1`).Scan(&state)
	g.Close()
	if state != claimUnknown {
		t.Fatalf("b's guard claim = %s, want UNKNOWN", state)
	}

	api.setStatus("b@contoso.com", http.StatusOK)
	if _, _, err := runAuthCmd(t, "", append(append([]string{}, args...), "--resume")...); err == nil {
		t.Fatal("a plain resume with b still UNKNOWN should exit non-zero")
	}
	if got := api.entityPosts("b@contoso.com"); got != 2 {
		t.Fatalf("plain --resume resent b: posts %d, want 2", got)
	}
	// Only an explicit --retry-unknown sends it again.
	if _, stderr, err := runAuthCmd(t, "", append(append([]string{}, args...), "--resume", "--retry-unknown")...); err != nil {
		t.Fatalf("explicit retry: %v (%s)", err, stderr)
	}
	if got := api.entityPosts("b@contoso.com"); got != 3 {
		t.Fatalf("b posts %d, want 3", got)
	}
}
