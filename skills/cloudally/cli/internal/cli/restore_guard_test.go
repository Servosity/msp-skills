// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// submitArgs is a restore submit for the fake task with the given extras.
func submitArgs(extra ...string) []string {
	return append([]string{"restore", "submit", "--task", "64b0e9", "--rate", "1000", "--yes", "--json"}, extra...)
}

func wantGuardRefusal(t *testing.T, what string, err error) {
	t.Helper()
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "already sent from this state directory") {
		t.Fatalf("%s: want the machine-wide guard refusal (exit 2), got %v", what, err)
	}
}

// (a) The same cutoff spelled three ways resolves to one backup and is one
// restore.
func TestRestoreGuardSameCutoffDifferentSpelling(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	ents := []string{"--entities", "a@contoso.com,b@contoso.com"}
	if _, stderr, err := runAuthCmd(t, "", submitArgs(append(ents, "--date", "2026-10-02")...)...); err != nil {
		t.Fatalf("first submit: %v (%s)", err, stderr)
	}
	for _, spelling := range []string{"2026-10-02T23:59:59Z", "20261002235959"} {
		_, _, err := runAuthCmd(t, "", submitArgs(append(ents, "--date", spelling)...)...)
		wantGuardRefusal(t, spelling, err)
		if !strings.Contains(err.Error(), "2 of the 2 restore(s)") {
			t.Fatalf("%s: refusal should count the restores: %v", spelling, err)
		}
	}
	if got := api.posts.Load(); got != 2 {
		t.Fatalf("restore POSTs = %d, want 2", got)
	}
}

// (b) A partner submit with --account, then the same restore after
// use-account with no --account: other scope kind, other store, one
// restore.
func TestRestoreGuardSpansPartnerAndUseAccount(t *testing.T) {
	isolateAuthState(t)
	resetStoreScope(t)
	api := newFakeRestoreAPI(t, 0)
	binding := authBinding{origin: cloudallyStoreOrigin(api.srv.URL)}
	partnerUser := partnerPrincipal("tech@example.com")
	partner := &authContext{Scope: authScopePartner}
	partner.stashPartner(binding, "account-token", "", time.Time{}, partnerUser)
	partner.bindTo(binding, "account-token", "", partnerUser)
	if err := saveAuthContext(partner); err != nil {
		t.Fatal(err)
	}
	if _, stderr, err := runAuthCmd(t, "", submitArgs("--account", "acc1", "--resolve", "last-backup", "--date", "2026-10-01")...); err != nil {
		t.Fatalf("partner submit: %v (%s)", err, stderr)
	}
	partnerStore := defaultDBPath(restoreCLIName)
	if api.posts.Load() != 5 {
		t.Fatalf("partner submit posted %d", api.posts.Load())
	}

	customer := &authContext{Scope: authScopeAccount, AccountID: "acc1"}
	customer.bindTo(binding, "account-token", "", accountPrincipal(partnerUser, "acc1"))
	if err := saveAuthContext(customer); err != nil {
		t.Fatal(err)
	}
	_, _, err := runAuthCmd(t, "", submitArgs("--date", "2026-10-01")...)
	if customerStore := defaultDBPath(restoreCLIName); customerStore == partnerStore {
		t.Fatalf("both logins share the ledger %s; the case needs separate stores", customerStore)
	}
	wantGuardRefusal(t, "use-account resubmit", err)
	if !strings.Contains(err.Error(), "login acc1 (partner)") {
		t.Fatalf("refusal should name the partner login: %v", err)
	}
	if got := api.posts.Load(); got != 5 {
		t.Fatalf("use-account resubmit posted: total %d, want 5", got)
	}
}

// (c) An entity superset sends only the mailbox never sent, through the
// command the refusal prints.
func TestRestoreGuardEntitySuperset(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	if _, stderr, err := runAuthCmd(t, "", submitArgs("--date", "2026-10-01", "--entities", "a@contoso.com")...); err != nil {
		t.Fatalf("first submit: %v (%s)", err, stderr)
	}
	_, _, err := runAuthCmd(t, "", submitArgs("--date", "2026-10-01", "--entities", "a@contoso.com,b@contoso.com")...)
	wantGuardRefusal(t, "superset", err)
	if api.entityPosts("a@contoso.com") != 1 || api.entityPosts("b@contoso.com") != 0 {
		t.Fatalf("superset sent: %v", api.byEntity)
	}
	const marker = "with none of these restores sent: "
	i := strings.Index(err.Error(), marker)
	if i < 0 {
		t.Fatalf("refusal should print a command for the never-sent mailbox: %v", err)
	}
	line := err.Error()[i+len(marker):]
	line = line[:strings.Index(line, " --yes")+len(" --yes")]
	fields := strings.Fields(line)
	if !strings.Contains(line, "--entities b@contoso.com") || strings.Contains(line, "--new-batch") || fields[0] != "cloudally-cli" {
		t.Fatalf("printed command = %q", line)
	}
	if _, stderr, err := runAuthCmd(t, "", fields[1:]...); err != nil {
		t.Fatalf("printed command %q: %v (%s)", line, err, stderr)
	}
	if api.entityPosts("a@contoso.com") != 1 || api.entityPosts("b@contoso.com") != 1 {
		t.Fatalf("after the printed command: %v", api.byEntity)
	}
}

// (d) Raw tokens get a ledger each; the guard still sees the restore.
func TestRestoreGuardSpansRawTokens(t *testing.T) {
	isolateAuthState(t)
	resetStoreScope(t)
	api := newFakeRestoreAPI(t, 0)
	t.Setenv("CLOUDALLY_ACCESS_TOKEN", "raw-token-one")
	if _, stderr, err := runAuthCmd(t, "", submitArgs("--date", "2026-10-01")...); err != nil {
		t.Fatalf("first token: %v (%s)", err, stderr)
	}
	first := defaultDBPath(restoreCLIName)
	t.Setenv("CLOUDALLY_ACCESS_TOKEN", "raw-token-two")
	_, _, err := runAuthCmd(t, "", submitArgs("--date", "2026-10-01")...)
	if second := defaultDBPath(restoreCLIName); second == first {
		t.Fatalf("both tokens share the ledger %s; the case needs separate stores", second)
	}
	wantGuardRefusal(t, "second token", err)
	if got := api.posts.Load(); got != 5 {
		t.Fatalf("second token posted: total %d, want 5", got)
	}
}

// (e) Two guard handles (two processes' worth of connections and owner
// locks) race for one restore: exactly one claim wins.
func TestRestoreGuardClaimRace(t *testing.T) {
	isolateAuthState(t)
	ctx := context.Background()
	g1, err := openRestoreGuard(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer g1.Close()
	g2, err := openRestoreGuard(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer g2.Close()
	id := newRestoreIdentity("64B0E9", " A@contoso.com", fakeBackupRaw)
	var wins, refusals int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		g := g1
		if i%2 == 1 {
			g = g2
		}
		wg.Add(1)
		go func(i int, g *restoreGuard) {
			defer wg.Done()
			_, err := g.claim(ctx, id, claimOwner{batchID: "batch-" + string(rune('a'+i))}, false, false)
			mu.Lock()
			defer mu.Unlock()
			var held errRestoreClaimed
			switch {
			case err == nil:
				wins++
			case errors.As(err, &held):
				refusals++
			default:
				t.Errorf("claim %d: %v", i, err)
			}
		}(i, g)
	}
	wg.Wait()
	if wins != 1 || refusals != 15 {
		t.Fatalf("wins %d refusals %d, want 1 and 15", wins, refusals)
	}
}

// (e) Two identical submits at the same instant send each restore once.
func TestRestoreGuardConcurrentSubmits(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	api.set(func(f *fakeRestoreAPI) { f.postDelay = 30 * time.Millisecond })
	args := submitArgs("--date", "2026-10-01", "--concurrency", "2")
	var wg sync.WaitGroup
	errs := make([]error, 2)
	start := make(chan struct{})
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, _, errs[i] = runAuthCmd(t, "", args...)
		}(i)
	}
	close(start)
	wg.Wait()
	for _, e := range []string{"a@contoso.com", "b@contoso.com", "c@contoso.com", "d@contoso.com", "e@contoso.com"} {
		if n := api.entityPosts(e); n != 1 {
			t.Fatalf("%s posted %d times by two racing submits (errors %v)", e, n, errs)
		}
	}
	if errs[0] == nil && errs[1] == nil {
		t.Fatal("one of two racing submits should report what it did not send")
	}
}

// A pre-acceptance rejection releases the restore for any later batch; an
// acceptance does not, until --new-batch records a new generation.
func TestRestoreGuardRejectionReleasesAndNewBatchSupersedes(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	api.setStatus("b@contoso.com", http.StatusBadRequest)
	if _, _, err := runAuthCmd(t, "", submitArgs("--date", "2026-10-01", "--entities", "a@contoso.com,b@contoso.com")...); err == nil {
		t.Fatal("first submit should report the rejected row")
	}
	api.setStatus("b@contoso.com", http.StatusOK)
	// b was refused before it ran, so another batch (here: another date
	// spelling) may send it.
	if _, stderr, err := runAuthCmd(t, "", submitArgs("--date", "2026-10-01T23:59:59Z", "--entities", "b@contoso.com")...); err != nil {
		t.Fatalf("resend of a rejected restore: %v (%s)", err, stderr)
	}
	if api.entityPosts("b@contoso.com") != 2 {
		t.Fatalf("b posts = %d, want 2", api.entityPosts("b@contoso.com"))
	}
	_, stderr, err := runAuthCmd(t, "", submitArgs("--date", "2026-10-01T23:59:59Z", "--entities", "a@contoso.com", "--new-batch")...)
	if err != nil {
		t.Fatalf("--new-batch: %v (%s)", err, stderr)
	}
	if !strings.Contains(stderr, "--new-batch sends 1 restore(s) again") {
		t.Fatalf("--new-batch should say what it resends:\n%s", stderr)
	}
	if api.entityPosts("a@contoso.com") != 2 {
		t.Fatalf("a posts = %d, want 2", api.entityPosts("a@contoso.com"))
	}
	g, err := openRestoreGuard(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	var gens, superseded int
	if err := g.db.QueryRow(`SELECT MAX(generation), SUM(superseded) FROM restore_claims WHERE entity = 'a@contoso.com'`).Scan(&gens, &superseded); err != nil {
		t.Fatal(err)
	}
	if gens != 2 || superseded != 1 {
		t.Fatalf("a: generation %d superseded %d; want generation 2 over a kept, superseded generation 1", gens, superseded)
	}
}

// The refusal messages that used to point at --new-batch now count what it
// would resend and point at --entities limited to the never-sent rows; the
// printed command sends exactly those.
func TestRestoreResumeRefusalAdviceNeverResends(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	api.setStatus("e@contoso.com", http.StatusTooManyRequests)
	api.setStatus("d@contoso.com", http.StatusBadGateway)
	base := submitArgs("--date", "2026-10-01", "--concurrency", "1")
	if _, _, err := runAuthCmd(t, "", base...); err == nil {
		t.Fatal("first submit should report the unsent rows")
	}
	// a, b, c accepted; d UNKNOWN; e rejected before it ran.
	_, _, err := runAuthCmd(t, "", append(append([]string{}, base...), "--resume", "--include-deleted")...)
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "different options") {
		t.Fatalf("conflicting resume: want a refusal, got %v", err)
	}
	msg := err.Error()
	if strings.Contains(msg, "start a separate batch with --new-batch") || !strings.Contains(msg, "Do not use --new-batch here: it would send the 4 restore(s)") {
		t.Fatalf("refusal must warn what --new-batch resends, not recommend it:\n%s", msg)
	}
	const marker = "with this run's options: "
	i := strings.Index(msg, marker)
	if i < 0 {
		t.Fatalf("refusal should print a command for the never-sent mailbox:\n%s", msg)
	}
	line := msg[i+len(marker):]
	line = line[:strings.Index(line, " --yes")+len(" --yes")]
	if !strings.Contains(line, "--entities e@contoso.com") || strings.Contains(line, "--resume") || strings.Contains(line, "--new-batch") {
		t.Fatalf("printed command = %q", line)
	}
	api.setStatus("e@contoso.com", http.StatusOK)
	before := api.posts.Load()
	if _, stderr, err := runAuthCmd(t, "", strings.Fields(line)[1:]...); err != nil {
		t.Fatalf("printed command %q: %v (%s)", line, err, stderr)
	}
	if got := api.posts.Load() - before; got != 1 || api.entityPosts("e@contoso.com") != 2 {
		t.Fatalf("printed command sent %d restores (e posts %d); want only e once", got, api.entityPosts("e@contoso.com"))
	}
	if body := api.lastBody("e@contoso.com"); body["includeDeleted"] != true {
		t.Fatalf("e went with %v, want this run's --include-deleted", body)
	}
}

// restore watch keeps an UNKNOWN row open and exits non-zero while it is
// unconfirmed, with --once and when the timeout runs out.
func TestRestoreWatchUnknownIsNotDone(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	api.setStatus("b@contoso.com", http.StatusInternalServerError)
	api.set(func(f *fakeRestoreAPI) { f.taskStatus = "ARCHIVED" })
	stdout, _, err := runAuthCmd(t, "", submitArgs("--date", "2026-10-01", "--entities", "a@contoso.com,b@contoso.com")...)
	if err == nil {
		t.Fatal("submit should report the UNKNOWN row")
	}
	batchID := decodeSubmitOutput(t, stdout).BatchID
	for _, extra := range [][]string{{"--once"}, {"--interval", "10ms", "--max-wait", "60ms"}} {
		out, _, err := runAuthCmd(t, "", append([]string{"restore", "watch", "--batch", batchID, "--json"}, extra...)...)
		if err == nil || ExitCode(err) != 5 || !strings.Contains(err.Error(), "UNKNOWN") {
			t.Fatalf("watch %v: want exit 5 naming the UNKNOWN row, got %v", extra, err)
		}
		var w watchOutput
		if jerr := json.Unmarshal([]byte(out), &w); jerr != nil || w.Done || w.Tally.Unknown != 1 {
			t.Fatalf("watch %v output: done %v unknown %d (%v)\n%s", extra, w.Done, w.Tally.Unknown, jerr, out)
		}
	}
}

// Recovery leaves a row another live run is sending alone, and recovers it
// once that run is gone.
func TestRestoreRecoveryLeavesLiveClaimAlone(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	api.setStatus("b@contoso.com", http.StatusBadGateway)
	args := submitArgs("--date", "2026-10-01", "--entities", "a@contoso.com,b@contoso.com", "--concurrency", "1")
	stdout, _, _ := runAuthCmd(t, "", args...)
	batchID := decodeSubmitOutput(t, stdout).BatchID
	api.setStatus("b@contoso.com", http.StatusOK)

	// Another live process takes b over (as its --retry-unknown would) and
	// is mid-send.
	other, err := openRestoreGuard(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.claim(context.Background(), newRestoreIdentity("64b0e9", "b@contoso.com", fakeBackupRaw), claimOwner{batchID: batchID}, true, false); err != nil {
		t.Fatalf("other run's claim: %v", err)
	}
	ledgerExec(t, `UPDATE restore_jobs SET status = ?, submit_status = ? WHERE entity = ?`, jobSubmitting, jobSubmitting, "b@contoso.com")

	_, stderr, _ := runAuthCmd(t, "", append(append([]string{}, args...), "--resume", "--retry-unknown")...)
	if b := ledgerRowByEntity(t, "b@contoso.com"); b.SubmitStatus != jobSubmitting {
		t.Fatalf("a row a live run is sending was recovered to %s", b.SubmitStatus)
	}
	if !strings.Contains(stderr, "being sent by another submit that is still running") || api.entityPosts("b@contoso.com") != 1 {
		t.Fatalf("b posts %d; stderr:\n%s", api.entityPosts("b@contoso.com"), stderr)
	}

	other.Close() // the other process is gone
	_, stderr, _ = runAuthCmd(t, "", append(append([]string{}, args...), "--resume")...)
	if b := ledgerRowByEntity(t, "b@contoso.com"); b.SubmitStatus != jobUnknown || !strings.Contains(stderr, "still SUBMITTING") {
		t.Fatalf("after its owner ended, b = %s; stderr:\n%s", b.SubmitStatus, stderr)
	}
	if api.entityPosts("b@contoso.com") != 1 {
		t.Fatalf("b posts = %d, want 1", api.entityPosts("b@contoso.com"))
	}
}

// A batch submitted after use-account (account scope, customer id
// recorded) is watched on the /v2 family, not the distributor one.
func TestRestoreWatchUseAccountBatchUsesAccountPaths(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	customer := &authContext{Scope: authScopeAccount, AccountID: "CUSTOMER"}
	customer.bindTo(authBinding{origin: cloudallyStoreOrigin(api.srv.URL)}, "account-token", "",
		accountPrincipal(partnerPrincipal("tech@example.com"), "CUSTOMER"))
	if err := saveAuthContext(customer); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, err := runAuthCmd(t, "", submitArgs("--date", "2026-10-01", "--entities", "a@contoso.com")...)
	if err != nil {
		t.Fatalf("submit: %v (%s)", err, stderr)
	}
	out := decodeSubmitOutput(t, stdout)
	if out.Scope != scopeAccount || out.AccountID != "CUSTOMER" {
		t.Fatalf("batch scope %q account %q", out.Scope, out.AccountID)
	}
	api.set(func(f *fakeRestoreAPI) { f.getPaths = nil })
	_, _, _ = runAuthCmd(t, "", "restore", "watch", "--batch", out.BatchID, "--once", "--json")
	api.mu.Lock()
	paths := append([]string(nil), api.getPaths...)
	api.mu.Unlock()
	if len(paths) == 0 {
		t.Fatal("watch made no request")
	}
	for _, p := range paths {
		if p != "/v2/backup-tasks" {
			t.Fatalf("watch called %s, want /v2/backup-tasks", p)
		}
	}
}

// Errors from every restore subcommand are scrubbed, and keep their exit
// code.
func TestRestoreErrorsAreScrubbed(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	api.set(func(f *fakeRestoreAPI) {
		f.notFoundBody = "{\"errorMessage\":\"\x1b]0;TASK-PWN\x07\x1b[2J\u202enot here\"}"
	})
	stdout, stderr, err := runAuthCmd(t, "", "restore", "plan", "--task", "missing", "--date", "2026-10-01")
	if err == nil {
		t.Fatal("want a not-found error")
	}
	if want := ExitCode(notFoundErr(errors.New("x"))); ExitCode(err) != want {
		t.Fatalf("exit code %d, want %d", ExitCode(err), want)
	}
	var ce *cliError
	if !errors.As(err, &ce) {
		t.Fatalf("scrubbing lost the typed error: %T", err)
	}
	assertTerminalSafe(t, "error", err.Error())
	assertTerminalSafe(t, "stderr", stderr)
	assertTerminalSafe(t, "stdout", stdout)
	if !strings.Contains(err.Error(), "TASK-PWN") {
		t.Fatalf("the body text should still be shown: %v", err)
	}
}
