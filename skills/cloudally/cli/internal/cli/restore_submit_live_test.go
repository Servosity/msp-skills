// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloudally-pp-cli/internal/client"
	"cloudally-pp-cli/internal/cliutil"
	"cloudally-pp-cli/internal/config"
)

// fakeBackupRaw is the compact date every fake entity was last backed up.
const fakeBackupRaw = int64(20260930120000)

// fakeRestoreAPI serves one backup task and counts every restore POST by
// entity. statusFor overrides the POST status per entity; backupRaw and
// entities can change between runs to model a task that moved on.
type fakeRestoreAPI struct {
	srv        *httptest.Server
	posts      atomic.Int32
	mu         sync.Mutex
	byEntity   map[string]int
	restoreErr int
	statusFor  map[string]int
	entities   []string
	backupRaw  int64
}

func newFakeRestoreAPI(t *testing.T, restoreStatus int) *fakeRestoreAPI {
	t.Helper()
	f := &fakeRestoreAPI{
		byEntity:   map[string]int{},
		restoreErr: restoreStatus,
		statusFor:  map[string]int{},
		entities:   []string{"a@contoso.com", "b@contoso.com", "c@contoso.com", "d@contoso.com", "e@contoso.com"},
		backupRaw:  fakeBackupRaw,
	}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/backup-tasks/64b0e9"):
			f.mu.Lock()
			var ents []string
			for _, e := range f.entities {
				ents = append(ents, fmt.Sprintf(`{"entity":%q,"lastBackupDates":{"mail":%d}}`, e, f.backupRaw))
			}
			f.mu.Unlock()
			_, _ = fmt.Fprintf(w, `{"id":"64b0e9","source":"MS365","entities":[%s]}`, strings.Join(ents, ","))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/recovery/restore"):
			f.posts.Add(1)
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			entity, _ := body["account"].(string)
			f.mu.Lock()
			f.byEntity[entity]++
			status := f.restoreErr
			if s, ok := f.statusFor[entity]; ok {
				status = s
			}
			f.mu.Unlock()
			if status != 0 && status != http.StatusOK {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"errorMessage":"denied"}`))
				return
			}
			_, _ = w.Write([]byte(`{"needRedirect":false}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.srv.Close)
	t.Setenv("CLOUDALLY_CLIENT_ID", "cid")
	t.Setenv("CLOUDALLY_CLIENT_SECRET", "csecret")
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	cfg.BaseURL = f.srv.URL
	if err := cfg.SaveTokens("cid", "csecret", "account-token", "", time.Now().Add(24*time.Hour)); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	return f
}

func (f *fakeRestoreAPI) setStatus(entity string, status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statusFor[entity] = status
}

func (f *fakeRestoreAPI) entityPosts(entity string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.byEntity[entity]
}

// ledgerRows returns every restore_jobs row, or nil when there is no ledger.
func ledgerRows(t *testing.T) []restoreJob {
	t.Helper()
	path := defaultDBPath(restoreCLIName)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	db, err := openLedger(context.Background())
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	defer db.Close()
	rows, err := db.DB().Query(`SELECT ` + jobColumns + ` FROM restore_jobs ORDER BY id`)
	if err != nil {
		t.Fatalf("query ledger: %v", err)
	}
	defer rows.Close()
	var out []restoreJob
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, j)
	}
	return out
}

func ledgerBatchCount(t *testing.T) int {
	t.Helper()
	if _, err := os.Stat(defaultDBPath(restoreCLIName)); os.IsNotExist(err) {
		return 0
	}
	db, err := openLedger(context.Background())
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	defer db.Close()
	var n int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM restore_batches`).Scan(&n); err != nil {
		t.Fatalf("count batches: %v", err)
	}
	return n
}

func ledgerStatusByEntity(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, j := range ledgerRows(t) {
		out[j.Entity] = j.Status
	}
	return out
}

func ledgerExec(t *testing.T, query string, args ...any) {
	t.Helper()
	db, err := openLedger(context.Background())
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	defer db.Close()
	if _, err := db.DB().Exec(query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func TestRestoreSubmitResumeNeverResends(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	args := []string{"restore", "submit", "--task", "64b0e9", "--date", "2026-10-01", "--concurrency", "3", "--rate", "1000", "--yes", "--json"}
	if _, stderr, err := runAuthCmd(t, "", args...); err != nil {
		t.Fatalf("first submit: %v (%s)", err, stderr)
	}
	if got := api.posts.Load(); got != 5 {
		t.Fatalf("first submit posted %d times, want 5", got)
	}
	if _, stderr, err := runAuthCmd(t, "", append(args, "--resume")...); err != nil {
		t.Fatalf("resume: %v (%s)", err, stderr)
	}
	if got := api.posts.Load(); got != 5 {
		t.Fatalf("resume re-posted: total %d, want 5", got)
	}
	for e, n := range api.byEntity {
		if n != 1 {
			t.Fatalf("%s restored %d times", e, n)
		}
	}
}

func TestRestoreSubmitWithoutYesSendsNothing(t *testing.T) {
	cases := map[string][]string{
		"plain":    nil,
		"agent":    {"--agent"},
		"no-input": {"--no-input"},
		"json":     {"--json"},
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			isolateAuthState(t)
			api := newFakeRestoreAPI(t, 0)
			args := append([]string{"restore", "submit", "--task", "64b0e9", "--date", "2026-10-01", "--rate", "1000"}, extra...)
			_, stderr, err := runAuthCmd(t, "", args...)
			if err == nil || ExitCode(err) != 2 {
				t.Fatalf("want exit 2, got %v", err)
			}
			if !strings.Contains(err.Error(), "add --yes to submit these 5 restores") {
				t.Fatalf("error = %v", err)
			}
			for _, want := range []string{"rows:     5", "entities: 5", "dates:    2026-10-01", "scope:    account", "account:"} {
				if !strings.Contains(stderr, want) {
					t.Fatalf("summary missing %q:\n%s", want, stderr)
				}
			}
			if got := api.posts.Load(); got != 0 {
				t.Fatalf("posted %d restores without --yes", got)
			}
			if n := ledgerBatchCount(t); n != 0 {
				t.Fatalf("ledger has %d batches without --yes", n)
			}
			if rows := ledgerRows(t); len(rows) != 0 {
				t.Fatalf("ledger has %d rows without --yes", len(rows))
			}
		})
	}
}

func TestRestoreSubmitVerifyEnvSendsNothing(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	t.Setenv(cliutil.VerifyEnvVar, "1")
	if _, stderr, err := runAuthCmd(t, "", "restore", "submit", "--task", "64b0e9", "--date", "2026-10-01", "--yes"); err != nil {
		t.Fatalf("submit: %v (%s)", err, stderr)
	}
	if got := api.posts.Load(); got != 0 {
		t.Fatalf("verify mode posted %d restores", got)
	}
}

func TestRestoreSubmitDogfoodHarnessSendsNothing(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	t.Setenv(cliutil.DogfoodEnvVar, "1")
	if !cliutil.IsAnyHarness() {
		t.Fatal("dogfood env should count as a harness")
	}
	stdout, stderr, err := runAuthCmd(t, "", "restore", "submit", "--task", "64b0e9", "--date", "2026-10-01", "--entities", "a@contoso.com", "--yes")
	if err != nil {
		t.Fatalf("submit: %v (%s)", err, stderr)
	}
	if got := api.posts.Load(); got != 0 {
		t.Fatalf("dogfood harness posted %d restores", got)
	}
	if !strings.Contains(stderr, "dogfood harness: would POST") || !strings.Contains(stdout, `"account":"a@contoso.com"`) {
		t.Fatalf("want would-send output, got stdout=%q stderr=%q", stdout, stderr)
	}
	if n := ledgerBatchCount(t); n != 0 {
		t.Fatalf("harness run wrote %d batches", n)
	}
}

func TestRestoreSubmitServerErrorIsUnknownAndNotResent(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	api.setStatus("b@contoso.com", http.StatusInternalServerError)
	args := []string{"restore", "submit", "--task", "64b0e9", "--date", "2026-10-01", "--entities", "a@contoso.com,b@contoso.com,c@contoso.com",
		"--concurrency", "1", "--rate", "1000", "--yes", "--json"}
	stdout, _, err := runAuthCmd(t, "", args...)
	if err == nil || ExitCode(err) != 5 {
		t.Fatalf("want apiErr exit 5, got %v", err)
	}
	if got := api.posts.Load(); got != 3 {
		t.Fatalf("first run posted %d, want 3", got)
	}
	var out submitOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout)
	}
	if out.Submitted != 2 || out.Unknown != 1 || out.Counts[jobSubmitted] != 2 || out.Counts[jobUnknown] != 1 {
		t.Fatalf("output counts = submitted %d unknown %d counts %v", out.Submitted, out.Unknown, out.Counts)
	}
	if st := ledgerStatusByEntity(t); st["b@contoso.com"] != jobUnknown || st["a@contoso.com"] != jobSubmitted {
		t.Fatalf("ledger statuses = %v", st)
	}

	// Plain resume leaves the UNKNOWN row alone and says so.
	_, stderr, err := runAuthCmd(t, "", append(args, "--resume")...)
	if err == nil || ExitCode(err) != 5 {
		t.Fatalf("resume with an UNKNOWN row should still exit 5, got %v", err)
	}
	if got := api.posts.Load(); got != 3 {
		t.Fatalf("resume re-posted: total %d, want 3", got)
	}
	if !strings.Contains(stderr, "these 1 restores may already have been accepted; check 'restore watch --batch") ||
		!strings.Contains(stderr, "--retry-unknown only if they are missing") {
		t.Fatalf("stderr should explain the UNKNOWN row:\n%s", stderr)
	}

	// --retry-unknown without --resume is a usage error and sends nothing.
	if _, _, err := runAuthCmd(t, "", append(args, "--retry-unknown")...); err == nil || ExitCode(err) != 2 {
		t.Fatalf("--retry-unknown without --resume: want exit 2, got %v", err)
	}

	// --retry-unknown sends exactly the UNKNOWN row once more.
	api.setStatus("b@contoso.com", http.StatusOK)
	if _, stderr, err := runAuthCmd(t, "", append(args, "--resume", "--retry-unknown")...); err != nil {
		t.Fatalf("retry-unknown: %v (%s)", err, stderr)
	}
	if got := api.posts.Load(); got != 4 {
		t.Fatalf("retry-unknown total posts %d, want 4", got)
	}
	if api.entityPosts("b@contoso.com") != 2 || api.entityPosts("a@contoso.com") != 1 {
		t.Fatalf("per-entity posts = %v", api.byEntity)
	}
	if st := ledgerStatusByEntity(t); st["b@contoso.com"] != jobSubmitted {
		t.Fatalf("after retry ledger = %v", st)
	}
}

func TestRestoreSubmitCleanRejectionIsFailedAndResent(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	api.setStatus("b@contoso.com", http.StatusTooManyRequests)
	args := []string{"restore", "submit", "--task", "64b0e9", "--date", "2026-10-01", "--entities", "a@contoso.com,b@contoso.com",
		"--concurrency", "1", "--rate", "1000", "--yes", "--json"}
	if _, _, err := runAuthCmd(t, "", args...); err == nil || ExitCode(err) != 5 {
		t.Fatalf("want exit 5, got %v", err)
	}
	if st := ledgerStatusByEntity(t); st["b@contoso.com"] != jobFailed {
		t.Fatalf("429 should be FAILED: %v", st)
	}
	api.setStatus("b@contoso.com", http.StatusOK)
	if _, stderr, err := runAuthCmd(t, "", append(args, "--resume")...); err != nil {
		t.Fatalf("resume: %v (%s)", err, stderr)
	}
	if api.entityPosts("b@contoso.com") != 2 || api.entityPosts("a@contoso.com") != 1 {
		t.Fatalf("per-entity posts = %v", api.byEntity)
	}
}

func TestRestoreSubmitCrashedSubmittingRowNotResent(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	args := []string{"restore", "submit", "--task", "64b0e9", "--date", "2026-10-01", "--entities", "a@contoso.com,b@contoso.com,c@contoso.com",
		"--rate", "1000", "--yes", "--json"}
	if _, stderr, err := runAuthCmd(t, "", args...); err != nil {
		t.Fatalf("first submit: %v (%s)", err, stderr)
	}
	// Model a run that died: b was claimed (SUBMITTING) and maybe sent, c
	// was never reached (PLANNED).
	ledgerExec(t, `UPDATE restore_jobs SET status = ? WHERE entity = ?`, jobSubmitting, "b@contoso.com")
	ledgerExec(t, `UPDATE restore_jobs SET status = ?, submitted_at = NULL WHERE entity = ?`, jobPlanned, "c@contoso.com")
	_, stderr, err := runAuthCmd(t, "", append(args, "--resume")...)
	if err == nil || ExitCode(err) != 5 {
		t.Fatalf("resume with a crashed row should exit 5, got %v", err)
	}
	if got := api.posts.Load(); got != 4 {
		t.Fatalf("total posts %d, want 4 (only c re-sent)", got)
	}
	if api.entityPosts("b@contoso.com") != 1 || api.entityPosts("c@contoso.com") != 2 {
		t.Fatalf("per-entity posts = %v", api.byEntity)
	}
	if st := ledgerStatusByEntity(t); st["b@contoso.com"] != jobUnknown || st["c@contoso.com"] != jobSubmitted {
		t.Fatalf("ledger = %v", st)
	}
	if !strings.Contains(stderr, "still SUBMITTING") || !strings.Contains(stderr, "may already have been accepted") {
		t.Fatalf("stderr = %s", stderr)
	}
}

func TestRestoreSubmitResumeKeepsRecordedBackup(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	args := []string{"restore", "submit", "--task", "64b0e9", "--date", "2026-10-01", "--rate", "1000", "--yes", "--json"}
	if _, stderr, err := runAuthCmd(t, "", args...); err != nil {
		t.Fatalf("first submit: %v (%s)", err, stderr)
	}
	// A newer backup inside the same cutoff now exists for every entity.
	api.mu.Lock()
	api.backupRaw = 20261001120000
	api.mu.Unlock()
	_, stderr, err := runAuthCmd(t, "", append(args, "--resume")...)
	if err != nil {
		t.Fatalf("resume: %v (%s)", err, stderr)
	}
	if got := api.posts.Load(); got != 5 {
		t.Fatalf("resume after a newer backup posted again: total %d, want 5", got)
	}
	rows := ledgerRows(t)
	if len(rows) != 5 {
		t.Fatalf("ledger rows = %d, want 5", len(rows))
	}
	for _, r := range rows {
		if r.ResolvedRaw != fakeBackupRaw {
			t.Fatalf("%s resolved_raw changed to %d", r.Entity, r.ResolvedRaw)
		}
	}
	if !strings.Contains(stderr, "the batch keeps its recorded backup") {
		t.Fatalf("stderr should note the newer backup:\n%s", stderr)
	}

	// The task gained a mailbox: only it is added and sent, in the same batch.
	api.mu.Lock()
	api.entities = append(api.entities, "f@contoso.com")
	api.mu.Unlock()
	stdout, stderr, err := runAuthCmd(t, "", append(args, "--resume")...)
	if err != nil {
		t.Fatalf("resume with a new entity: %v (%s)", err, stderr)
	}
	if got := api.posts.Load(); got != 6 || api.entityPosts("f@contoso.com") != 1 {
		t.Fatalf("total posts %d, f posts %d; want 6 and 1", got, api.entityPosts("f@contoso.com"))
	}
	if !strings.Contains(stderr, "were added: f@contoso.com") {
		t.Fatalf("stderr should name the added entity:\n%s", stderr)
	}
	var out submitOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !out.Resumed || len(out.AddedEntities) != 1 || out.Total != 6 {
		t.Fatalf("output = resumed %v added %v total %d", out.Resumed, out.AddedEntities, out.Total)
	}
	if n := ledgerBatchCount(t); n != 1 {
		t.Fatalf("batches = %d, want 1", n)
	}
}

func TestRestoreSubmitPartnerScopeNeedsAccount(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	if err := saveAuthContext(&authContext{Scope: authScopePartner, PartnerAccessToken: "p"}); err != nil {
		t.Fatal(err)
	}
	_, _, err := runAuthCmd(t, "", "restore", "submit", "--task", "64b0e9", "--date", "2026-10-01", "--rate", "1000", "--yes")
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "need an account token") {
		t.Fatalf("want usage error about the account token, got %v", err)
	}
	if got := api.posts.Load(); got != 0 {
		t.Fatalf("partner scope posted %d restores", got)
	}
	if n := ledgerBatchCount(t); n != 0 {
		t.Fatalf("ledger has %d batches", n)
	}
}

func TestRestoreSubmitRecordsAccountAndRefusesMismatchedResume(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	if err := saveAuthContext(&authContext{Scope: authScopeAccount, AccountID: "acct-a", PartnerAccessToken: "p"}); err != nil {
		t.Fatal(err)
	}
	args := []string{"restore", "submit", "--task", "64b0e9", "--date", "2026-10-01", "--rate", "1000", "--yes", "--json"}

	// The confirmation summary names the account.
	_, stderr, err := runAuthCmd(t, "", "restore", "submit", "--task", "64b0e9", "--date", "2026-10-01")
	if err == nil || ExitCode(err) != 2 || !strings.Contains(stderr, "account:  acct-a (use-account)") {
		t.Fatalf("summary should name the account: err=%v\n%s", err, stderr)
	}

	stdout, stderr, err := runAuthCmd(t, "", args...)
	if err != nil {
		t.Fatalf("submit: %v (%s)", err, stderr)
	}
	var out submitOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.AccountID != "acct-a" || out.AuthScope != batchAuthUseAccount {
		t.Fatalf("batch account = %q (%q)", out.AccountID, out.AuthScope)
	}
	hist, _, err := runAuthCmd(t, "", "restore", "history", "--json")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	var batches []batchSummary
	if err := json.Unmarshal([]byte(hist), &batches); err != nil || len(batches) != 1 || batches[0].AccountID != "acct-a" || batches[0].Submitted != 5 {
		t.Fatalf("history = %s (%v)", hist, err)
	}

	if err := saveAuthContext(&authContext{Scope: authScopeAccount, AccountID: "acct-b", PartnerAccessToken: "p"}); err != nil {
		t.Fatal(err)
	}
	ledgerExec(t, `UPDATE restore_jobs SET status = ? WHERE entity = ?`, jobPlanned, "a@contoso.com")
	_, _, err = runAuthCmd(t, "", append(args, "--resume")...)
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "different account") {
		t.Fatalf("mismatched resume: want exit 2, got %v", err)
	}
	if got := api.posts.Load(); got != 5 {
		t.Fatalf("mismatched resume posted: total %d, want 5", got)
	}
}

func TestRestoreSubmitAuthFailureStopsDispatch(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, http.StatusUnauthorized)
	stdout, _, err := runAuthCmd(t, "", "restore", "submit", "--task", "64b0e9", "--date", "2026-10-01", "--concurrency", "1", "--rate", "1000", "--yes", "--json")
	if err == nil {
		t.Fatal("want an auth error")
	}
	if got := api.posts.Load(); got != 1 {
		t.Fatalf("posted %d times after a 401, want 1", got)
	}
	var out submitOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout)
	}
	planned := 0
	for _, r := range out.Rows {
		if r.Status == jobPlanned {
			planned++
		}
	}
	if planned != 4 || out.Failed != 1 {
		t.Fatalf("planned=%d failed=%d, want 4 and 1", planned, out.Failed)
	}
}

func TestRestoreSubmitPlanFileScopeMismatch(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	planPath := filepath.Join(t.TempDir(), "plan.json")
	plan := restorePlan{Scope: scopeDistributor, AccountID: "acct-a", TaskID: "64b0e9", Source: "MS365", Dates: []string{"2026-10-01"},
		Rows: []planRow{{Entity: "a@contoso.com", Requested: "2026-10-01", ResolvedRaw: 20260930120000, Status: planOK}}}
	if err := writePlanFile(planPath, plan); err != nil {
		t.Fatal(err)
	}
	cases := [][]string{
		{"--account", "acct-b"},
		{"--account", "acct-a", "--reseller"},
		{"--task", "other"},
	}
	for _, extra := range cases {
		args := append([]string{"restore", "submit", "--plan-file", planPath, "--yes"}, extra...)
		_, _, err := runAuthCmd(t, "", args...)
		if err == nil || cliErrCode(t, err) != 2 {
			t.Fatalf("%v: want usage error, got %v", extra, err)
		}
	}
	if got := api.posts.Load(); got != 0 {
		t.Fatalf("mismatched plan posted %d restores", got)
	}
	bad := plan
	bad.Rows = []planRow{{Entity: "a@contoso.com", Requested: "2026-10-01", ResolvedRaw: 0, Status: planOK}}
	if err := writePlanFile(planPath, bad); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runAuthCmd(t, "", "restore", "submit", "--plan-file", planPath, "--dry-run"); err == nil {
		t.Fatal("plan row with resolved_raw 0 should be rejected")
	}
	_ = os.Remove(planPath)
}

func TestClassifySubmitFailure(t *testing.T) {
	cases := []struct {
		status    int
		connected bool
		want      string
	}{
		{http.StatusBadRequest, true, jobFailed},
		{http.StatusUnauthorized, true, jobFailed},
		{http.StatusNotFound, true, jobFailed},
		{http.StatusTooManyRequests, true, jobFailed},
		{http.StatusRequestTimeout, true, jobUnknown},
		{http.StatusInternalServerError, true, jobUnknown},
		{http.StatusBadGateway, true, jobUnknown},
		{0, true, jobUnknown},
		{0, false, jobFailed},
	}
	for _, tc := range cases {
		if got := classifySubmitFailure(tc.status, tc.connected); got != tc.want {
			t.Fatalf("classify(%d, connected=%v) = %s, want %s", tc.status, tc.connected, got, tc.want)
		}
	}
}

// TestSubmitOneTransportOutcomes drives submitOne against a refused port
// (nothing left this process: FAILED) and a server that drops the
// connection after reading the request (it may have been accepted: UNKNOWN).
func TestSubmitOneTransportOutcomes(t *testing.T) {
	isolateAuthState(t)
	newClient := func(base string) *client.Client {
		cfg, err := config.Load("")
		if err != nil {
			t.Fatal(err)
		}
		cfg.BaseURL = base
		cfg.ClientID, cfg.ClientSecret, cfg.AccessToken = "cid", "csecret", "tok"
		return client.New(cfg, 5*time.Second, 0)
	}
	body := restoreBody("64b0e9", "MS365", "a@contoso.com", "", fakeBackupRaw, submitOptions{})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	refused := "http://" + ln.Addr().String()
	_ = ln.Close()
	res := submitOne(context.Background(), newClient(refused), "/v2/recovery/restore", body, restoreJob{Entity: "a@contoso.com"})
	if res.status != jobFailed {
		t.Fatalf("connection refused: status %s (%s), want FAILED", res.status, res.errText)
	}

	var got atomic.Int32
	drop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Add(1)
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("no hijacker")
			return
		}
		conn, _, err := hj.Hijack()
		if err == nil {
			_ = conn.Close()
		}
	}))
	defer drop.Close()
	res = submitOne(context.Background(), newClient(drop.URL), "/v2/recovery/restore", body, restoreJob{Entity: "a@contoso.com"})
	if res.status != jobUnknown || got.Load() != 1 {
		t.Fatalf("dropped connection: status %s (%s), server saw %d; want UNKNOWN and 1", res.status, res.errText, got.Load())
	}
}
