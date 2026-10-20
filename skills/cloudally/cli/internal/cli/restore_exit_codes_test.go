// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"cloudally-pp-cli/internal/cliutil"
)

// submitOne submits a single mailbox and returns the batch id.
func submitOneMailbox(t *testing.T, entity string) string {
	t.Helper()
	stdout, stderr, err := runAuthCmd(t, "", submitArgs("--date", "2026-10-01", "--entities", entity)...)
	if err != nil {
		t.Fatalf("submit: %v (%s)", err, stderr)
	}
	return decodeSubmitOutput(t, stdout).BatchID
}

func watchJSON(t *testing.T, batchID string, extra ...string) (watchOutput, error) {
	t.Helper()
	out, _, err := runAuthCmd(t, "", append([]string{"restore", "watch", "--batch", batchID, "--json"}, extra...)...)
	var w watchOutput
	if jerr := json.Unmarshal([]byte(out), &w); jerr != nil {
		t.Fatalf("watch output: %v\n%s", jerr, out)
	}
	return w, err
}

// Item 2(a): --once with a restore still running, or submitted with no task
// seen yet, exits 8, never 0.
func TestRestoreWatchOnceStillRunningExits8(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	batchID := submitOneMailbox(t, "a@contoso.com")

	w, err := watchJSON(t, batchID, "--once")
	if err == nil || ExitCode(err) != ExitJobPending || !strings.Contains(err.Error(), "still running") || w.Done || w.Tally.Running != 1 {
		t.Fatalf("running task under --once: want exit 8, got %v (done %v tally %+v)", err, w.Done, w.Tally)
	}

	// A fresh row whose task is not listed yet is pending, also exit 8.
	api.set(func(f *fakeRestoreAPI) { f.tasks = nil })
	submitted := submitOneMailbox(t, "b@contoso.com")
	api.set(func(f *fakeRestoreAPI) { f.tasks = nil })
	w, err = watchJSON(t, submitted, "--once")
	if err == nil || ExitCode(err) != ExitJobPending || w.Tally.Pending != 1 {
		t.Fatalf("no task seen yet under --once: want exit 8, got %v (tally %+v)", err, w.Tally)
	}

	// --max-wait running out with the task still running is also 8.
	if _, err := watchJSON(t, batchID, "--interval", "10ms", "--max-wait", "15ms"); err == nil || ExitCode(err) != ExitJobPending {
		t.Fatalf("--max-wait with a running task: want exit 8, got %v", err)
	}

	api.set(func(f *fakeRestoreAPI) { f.taskStatus = "COMPLETED" })
	done := submitOneMailbox(t, "c@contoso.com")
	if w, err := watchJSON(t, done, "--once"); err != nil || !w.Done {
		t.Fatalf("finished restore: want exit 0 and done, got %v (done %v)", err, w.Done)
	}
}

// Item 2(b): a restore the API accepted but whose RESTORE task never
// appears ends UNMATCHED, which is exit 5, not success.
func TestRestoreWatchUnmatchedExits5(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	batchID := submitOneMailbox(t, "a@contoso.com")
	api.set(func(f *fakeRestoreAPI) { f.tasks = nil })
	w, err := watchJSON(t, batchID, "--interval", "10ms")
	if err == nil || ExitCode(err) != 5 || !strings.Contains(err.Error(), "no RESTORE task was seen") || !strings.Contains(err.Error(), "tasks list --type RESTORE") {
		t.Fatalf("unmatched restore: want exit 5 pointing at tasks list, got %v", err)
	}
	if w.Tally.Unmatched != 1 {
		t.Fatalf("tally = %+v", w.Tally)
	}
}

// A RESTORE task that ends cancelled or in error is not a success.
func TestRestoreWatchCancelledTaskExits5(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	batchID := submitOneMailbox(t, "a@contoso.com")
	for _, status := range []string{"CANCELLED", "ERROR", "FAILED_PARTIALLY"} {
		api.set(func(f *fakeRestoreAPI) { f.taskStatus = status })
		if _, err := watchJSON(t, batchID, "--once"); err == nil || ExitCode(err) != 5 {
			t.Fatalf("task %s: want exit 5, got %v", status, err)
		}
	}
}

// Item 1: a data directory that can be entered but not listed fails the
// run closed instead of looking empty.
func TestRestoreGuardSeedFailsClosedOnUnlistableDir(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permission bits do not stop listing here")
	}
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	writeOriginalLedger(t, "data-legacybuild.db",
		`INSERT INTO restore_batches (batch_id, batch_key, created_at, scope, task_id) VALUES ('old-1', 'k', '2026-10-02T00:00:00Z', 'account', '64b0e9')`,
		`INSERT INTO restore_jobs (batch_id, entity, requested, resolved_raw, resolved, status, http_status, updated_at)
			VALUES ('old-1', 'a@contoso.com', '2026-10-01', 20260930120000, 'x', 'SUBMITTED', 200, 'now')`)
	dir, err := cliutil.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o300); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	_, _, err = runAuthCmd(t, "", submitArgs("--date", "2026-10-01", "--entities", "a@contoso.com")...)
	if err == nil || !strings.Contains(err.Error(), "cannot be listed") || !strings.Contains(err.Error(), "this run sent nothing") {
		t.Fatalf("unlistable data dir: want a fail-closed error, got %v", err)
	}
	if api.posts.Load() != 0 {
		t.Fatalf("posted %d restores past an unlistable ledger directory", api.posts.Load())
	}
}

// Item 3: a refusal on a nearby backup time names both times, says the CLI
// cannot tell, and gives the deliberate --new-batch command, which works.
// An exact match keeps the plain "twice" wording.
func TestRestoreGuardWindowRefusalNamesBothBackups(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	api.set(func(f *fakeRestoreAPI) { f.backupRaw = 20261001020000 })
	if _, stderr, err := runAuthCmd(t, "", submitArgs("--date", "2026-10-01", "--entities", "a@contoso.com")...); err != nil {
		t.Fatalf("first: %v (%s)", err, stderr)
	}
	_, _, err := runAuthCmd(t, "", submitArgs("--date", "2026-10-01T23:59:59Z", "--entities", "a@contoso.com")...)
	wantGuardRefusal(t, "exact", err)
	if strings.Contains(err.Error(), "cannot tell") || !strings.Contains(err.Error(), "would restore those mailboxes twice") {
		t.Fatalf("exact match wording:\n%v", err)
	}

	api.set(func(f *fakeRestoreAPI) { f.backupRaw = 20261001021500 })
	_, _, err = runAuthCmd(t, "", submitArgs("--date", "20261001235959", "--entities", "a@contoso.com")...)
	wantGuardRefusal(t, "nearby", err)
	msg := err.Error()
	for _, want := range []string{"2026-10-01T02:15:00Z", "2026-10-01T02:00:00Z", "15m0s", "cannot tell", "--new-batch"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("nearby refusal should mention %q:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "would restore those mailboxes twice") {
		t.Fatalf("a nearby match must not claim it would restore twice:\n%s", msg)
	}
	args := printedCommand(t, msg, "send it on purpose with: ")
	if !strings.Contains(strings.Join(args, " "), "--entities a@contoso.com --new-batch") {
		t.Fatalf("printed command = %v", args)
	}
	if _, stderr, err := runAuthCmd(t, "", args...); err != nil {
		t.Fatalf("printed --new-batch command: %v (%s)", err, stderr)
	}
	if api.entityPosts("a@contoso.com") != 2 {
		t.Fatalf("a posts = %d, want 2", api.entityPosts("a@contoso.com"))
	}
}

// Item 4: --timeout bounds each portal request, not the plan, so a plan
// whose calendar reads add up past --timeout still resolves.
func TestRestorePlanTimeoutIsPerRequest(t *testing.T) {
	f := &fakeCloudAlly{t: t}
	api := f.apiServerWithHandoff()
	portalSrv := f.portalServer()
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(600 * time.Millisecond)
		req, _ := http.NewRequest(r.Method, portalSrv.URL+r.URL.RequestURI(), r.Body)
		req.Header = r.Header.Clone()
		resp, err := noRedirect.Do(req)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		for k, vs := range resp.Header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	t.Cleanup(slow.Close)

	stdout, stderr, err := runSnapshotCLI(t, api.URL, "restore", "plan", "--account", "acct-1", "--task", "task-1",
		"--date", "2025-12-31", "--resolve", "snapshots", "--portal-url", slow.URL, "--timeout", "1s", "--json")
	if err != nil {
		t.Fatalf("plan: %v (%s)", err, stderr)
	}
	var rows []planRow
	if err := json.Unmarshal([]byte(stdout), &rows); err != nil || len(rows) != 1 {
		t.Fatalf("plan output: %v\n%s", err, stdout)
	}
	if rows[0].Status != planOK || rows[0].ResolvedRaw != 20251231042000 {
		t.Fatalf("plan row = %+v (stderr %s)", rows[0], stderr)
	}
}
