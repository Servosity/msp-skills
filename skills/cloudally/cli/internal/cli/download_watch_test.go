// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cloudally-pp-cli/internal/cliutil/testenv"
)

// watchStub is a partner API and customer portal whose job listing either
// blocks until the request is cancelled, answers "still building", or
// fails with a server error.
type watchStub struct {
	api, portal *httptest.Server
	listed      chan struct{}
	// release unblocks a blocked listing once the test is done, because the
	// server does not always see the client give up on the connection.
	release chan struct{}
}

func newWatchStub(t *testing.T, mode string) *watchStub {
	t.Helper()
	s := &watchStub{listed: make(chan struct{}, 16), release: make(chan struct{})}
	s.api = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/partner-access-token") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"accessToken":"handoff-token"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(s.api.Close)
	mux := http.NewServeMux()
	mux.HandleFunc("/newapp", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "authed"})
		_, _ = w.Write([]byte(`<html><head><meta name="_csrf" content="csrf-1"/><meta name="_csrf_header" content="X-CSRF-TOKEN"/></head></html>`))
	})
	mux.HandleFunc("/application/service/getRestoreDownloadStatus", func(w http.ResponseWriter, r *http.Request) {
		if mode == "pending" {
			// Signal once the answer is on its way, so the watcher is
			// between checks when the test cancels.
			defer func() { s.listed <- struct{}{} }()
		} else {
			s.listed <- struct{}{}
		}
		switch mode {
		case "block":
			select {
			case <-r.Context().Done():
			case <-s.release:
			}
			return
		case "error":
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		status := "IN_PROCESS"
		if after, ok := strings.CutPrefix(mode, "status="); ok {
			status = after
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":[{"taskId":"job-9","type":"EXPORT","status":"` + status + `","account":"jane@contoso.com","source":"MS365","subsource":"O365MAIL"}]}`))
	})
	s.portal = httptest.NewServer(mux)
	t.Cleanup(s.portal.Close)
	t.Cleanup(func() { close(s.release) }) // runs before Close
	return s
}

// runWatch runs download watch under ctx and returns its error and output.
func runWatch(t *testing.T, ctx context.Context, s *watchStub, extra ...string) (string, error) {
	t.Helper()
	t.Setenv("CLOUDALLY_BASE_URL", s.api.URL)
	t.Setenv("CLOUDALLY_ACCESS_TOKEN", "partner-token")
	t.Setenv("CLOUDALLY_CLIENT_ID", "cid")
	t.Setenv("CLOUDALLY_CLIENT_SECRET", "csecret")
	cmd := RootCmd()
	args := append([]string{"download", "watch", "--account", "5f1c2a", "--job", "job-9", "--portal-url", s.portal.URL}, extra...)
	cmd.SetArgs(args)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.ExecuteContext(ctx)
	return out.String() + errOut.String(), err
}

// cancelAfterFirstListing cancels ctx once the stub has seen (or, for a
// pending job, answered) a listing, after settle.
func cancelAfterFirstListing(t *testing.T, s *watchStub, cancel context.CancelFunc, settle time.Duration) {
	t.Helper()
	go func() {
		select {
		case <-s.listed:
		case <-time.After(10 * time.Second):
		}
		time.Sleep(settle)
		cancel()
	}()
}

func assertWatchInterrupted(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("an interrupted watch returned no error")
	}
	if code := ExitCode(err); code != ExitJobPending {
		t.Fatalf("exit code = %d, want %d (as --once while not ready): %v", code, ExitJobPending, err)
	}
	msg := err.Error()
	want := "cloudally-cli download watch --account 5f1c2a --job job-9"
	if !strings.Contains(msg, want) {
		t.Fatalf("error should name the command to run again (%q): %v", want, err)
	}
	if strings.Contains(msg, "context canceled") {
		t.Fatalf("error leaks the raw context error: %v", err)
	}
}

// Ctrl-C while a status check is in flight.
func TestDownloadWatchInterruptMidRequestExits8(t *testing.T) {
	isolateAuthState(t)
	s := newWatchStub(t, "block")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancelAfterFirstListing(t, s, cancel, 0)
	_, err := runWatch(t, ctx, s)
	assertWatchInterrupted(t, err)
}

// Ctrl-C while waiting for the next check.
func TestDownloadWatchInterruptBetweenChecksExits8(t *testing.T) {
	isolateAuthState(t)
	s := newWatchStub(t, "pending")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancelAfterFirstListing(t, s, cancel, 300*time.Millisecond)
	_, err := runWatch(t, ctx, s, "--interval", "1h")
	assertWatchInterrupted(t, err)
	if !strings.Contains(err.Error(), "--interval 1h0m0s") {
		t.Fatalf("the command to run again should keep --interval: %v", err)
	}
}

// The other direction: a real API failure with a live context keeps its own
// error and exit code, and --once on a job still building still exits 8.
func TestDownloadWatchRealFailureIsNotAnInterrupt(t *testing.T) {
	isolateAuthState(t)
	s := newWatchStub(t, "error")
	_, err := runWatch(t, context.Background(), s, "--once")
	if err == nil {
		t.Fatal("a failing status check returned no error")
	}
	if code := ExitCode(err); code == ExitJobPending {
		t.Fatalf("a real API failure exited %d like an interrupt: %v", code, err)
	}
	if strings.Contains(err.Error(), "interrupted") {
		t.Fatalf("a real API failure was reported as an interrupt: %v", err)
	}

	pending := newWatchStub(t, "pending")
	_, err = runWatch(t, context.Background(), pending, "--once")
	var ce *cliError
	if !errors.As(err, &ce) || ce.code != ExitJobPending || strings.Contains(err.Error(), "interrupted") {
		t.Fatalf("--once on a building job = %v, want exit %d not ready", err, ExitJobPending)
	}
}

// Any status carrying a failure word is a finished (failed) job; only
// READY_FOR_DOWNLOAD is ready; a status with no failure word that is not
// listed stays "still running".
func TestExportJobFinishedTreatsFailureWordsAsTerminal(t *testing.T) {
	cases := []struct {
		status          string
		finished, ready bool
	}{
		{"READY_FOR_DOWNLOAD", true, true},
		{" ready_for_download ", true, true},
		{"FAILED", true, false},
		{"CANCELLED", true, false},
		{"CANCELED", true, false},
		{"PARTIALLY_SUCCEEDED", true, false},
		{"COMPLETED", true, false},
		{"ABORTED", true, false},
		{"aborted", true, false},
		{"STOPPED", true, false},
		{"ERROR", true, false},
		{"EXPORT_ERROR", true, false},
		{"FAILED_WITH_ERRORS", true, false},
		{"CANCELED_BY_USER", true, false},
		{"PARTIALLY_FAILED", true, false},
		{"IN_PROCESS", false, false},
		{"INPROCESS", false, false},
		{"QUEUED", false, false},
		{"PENDING", false, false},
		{"DISPATCHED", false, false},
		{"RUNNING", false, false},
		{"SOMETHING_NEW", false, false},
		{"", false, false},
	}
	for _, tc := range cases {
		if got := exportJobFinished(tc.status); got != tc.finished {
			t.Errorf("exportJobFinished(%q) = %v, want %v", tc.status, got, tc.finished)
		}
		if got := exportJobReady(tc.status); got != tc.ready {
			t.Errorf("exportJobReady(%q) = %v, want %v", tc.status, got, tc.ready)
		}
	}
}

// End to end: download watch --once on an ABORTED job reports a finished,
// failed job (exit 6), not "still ABORTED; check again later" (exit 8).
func TestDownloadWatchOnceOnAbortedJobExits6(t *testing.T) {
	isolateAuthState(t)
	s := newWatchStub(t, "status=ABORTED")
	_, err := runWatch(t, context.Background(), s, "--once")
	if code := ExitCode(err); code != 6 {
		t.Fatalf("exit code = %d, want 6 for an aborted job: %v", code, err)
	}
	if err == nil || !strings.Contains(err.Error(), "finished as ABORTED") {
		t.Fatalf("error should say the job finished as ABORTED: %v", err)
	}
}

// download fetch picks up the same rule through exportJobFinished: an
// ABORTED job is reported FAILED, not NOT_READY waiting for a retry.
func TestDownloadFetchReportsAnAbortedJobAsFailed(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	s.mu.Lock()
	s.jobs = append(s.jobs, dlJob{JobID: "j-aborted", Account: jane, SubSource: "O365MAIL", Status: "ABORTED", Snapshot: 20221231042000})
	s.mu.Unlock()
	out, _, _ := runDL(t, s, fetchArgs(t.TempDir())...)
	var got []string
	for _, o := range decodeOutcomes(t, out) {
		if o.JobID == "j-aborted" {
			got = append(got, o.Status)
		}
	}
	if len(got) != 1 || got[0] != fetchFailed {
		t.Fatalf("aborted job outcome = %v, want [%s]", got, fetchFailed)
	}
}
