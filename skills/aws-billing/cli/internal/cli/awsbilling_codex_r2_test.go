// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"aws-billing-pp-cli/internal/awsx"
	"aws-billing-pp-cli/internal/credkit"
	"aws-billing-pp-cli/internal/store"
)

// Codex r2 class credential-handed-to-another-program (gaps in the round-1
// fix, fixed after the stop rule, unreviewed): everything a helper receives
// (its environment, argv and stdin) is checked as one set of parts, so a held
// credential split between an inherited variable and an argument never
// reaches it (neither part alone reads as held); and an arbitrary variable is
// not inherited at all (the environment is an allowlist).
func TestR2HelperEnvironmentAndArgvAreCheckedTogether(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	secret := "bbR2SplitSecret/4kL7mN0pQ3rS6tU9vW2x"
	credkit.Hold("config:aws_secret_access_key", secret)
	head, tail := secret[:18], secret[18:]
	saved := os.Environ()
	os.Clearenv()
	t.Cleanup(func() {
		os.Clearenv()
		for _, kv := range saved {
			if k, v, ok := strings.Cut(kv, "="); ok {
				_ = os.Setenv(k, v)
			}
		}
	})
	for _, name := range []string{"SLACK_REPORT_LABEL", "REPORT_LABEL"} {
		os.Clearenv()
		_ = os.Setenv("PATH", "/usr/bin:/bin")
		_ = os.Setenv("HOME", "/home/r2") // a fixed path: random path words would decide the round-1 check
		_ = os.Setenv(name, tail)
		cmd := exec.Command("slack-pp-cli", "messages", "post_message", "--channel="+head, "--text=hello")
		if err := prepareHelper(cmd, nil); err == nil {
			if env := strings.Join(cmd.Env, "\n"); strings.Contains(env, tail) {
				t.Errorf("%s: a helper was prepared with %q in its environment and %q in its argv: together they spell a held credential", name, tail, head)
			}
		}
	}
}

// The round-1 fix checked the environment's variables put together with
// every word of a full environment, so one-character words spelled any held
// value and every helper was refused once a credential was held (a Slack post
// or a PDF never started in a real shell). A full environment with a held
// credential and ordinary arguments starts the helper.
func TestR2HelperStartsInAFullEnvironment(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	credkit.Hold("config:aws_secret_access_key", "bbR2FullEnvSecret/9hG6fE3dC0bA7zY4xW")
	for i := 0; i < 60; i++ {
		t.Setenv(fmt.Sprintf("BB_R2_FILLER_%02d", i), fmt.Sprintf("/opt/r2/%c/%d/v%c-%d/lib:/usr/share/%c", 'a'+i%26, i, 'A'+i%26, i*7, '0'+i%10))
	}
	cmd := exec.Command("slack-pp-cli", "messages", "post_message", "--channel=C123", "--text=AWS bill: $999.99 (+0.0% vs prior)")
	if err := prepareHelper(cmd, nil); err != nil {
		t.Errorf("a helper with ordinary arguments was refused in a full environment: %v", err)
	}
}

// Codex r2 class cache-answer-not-the-request (a gap in the round-1 fix,
// fixed after the stop rule, unreviewed): a stored answer is replaced in one
// transaction, so a reader sees the old answer whole or the new one whole,
// never the rows of half a replacement or none under a coverage record. A
// replacement that fails part way leaves the old answer stored and covered
// (it used to commit the deletion first), and so does an account list.
func TestR2ReplacementIsAllOrNothing(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "atomic.db")
	db, err := store.OpenWithContext(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	pr := periodRange{Start: "2026-09-01", End: "2026-10-01", Label: "September", Granularity: "MONTHLY"}
	b := []store.CostBucket{{Start: "2026-09-01", End: "2026-10-01"}}
	ans := awsx.CostAnswer{Buckets: []awsx.CostBucket{{Start: "2026-09-01", End: "2026-10-01"}},
		Lines: []awsx.CostLine{{PeriodStart: "2026-09-01", PeriodEnd: "2026-10-01", Granularity: "MONTHLY", AccountID: "111111111111", Service: "HQ-Service-A", AmountUSD: 100, Unit: "USD"}}}
	if err := storeCostAnswer(db, "111111111111", costByAccountService, "MONTHLY", ans); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceCostBuckets("111111111111", costByAccountService.id, "MONTHLY", b, []json.RawMessage{json.RawMessage(`{"service":"no id"}`)}); err == nil {
		t.Fatal("a replacement with a row that cannot be stored succeeded")
	}
	o := awsReadOpts{dbPath: path, localOnly: true, sess: &awsSession{}}
	lines, _, ok, err := storedCostLines(ctx, o, pr, costByAccountService)
	if err != nil || !ok || len(lines) != 1 || lines[0].AmountUSD != 100 {
		t.Errorf("after a failed replacement the stored $100 answer is gone or changed: ok=%v err=%v lines=%v", ok, err, lines)
	}
	acct, _ := json.Marshal(awsx.Account{ID: store.AccountRowPrefix("111111111111") + "222222222222", AccountID: "222222222222", Name: "HQ-Member", Status: "ACTIVE"})
	if err := db.ReplaceAccountList("111111111111", []json.RawMessage{acct}); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceAccountList("111111111111", []json.RawMessage{json.RawMessage(`{"name":"no id"}`)}); err == nil {
		t.Fatal("an account list with a row that cannot be stored succeeded")
	}
	accts, ok, err := storedAccounts(ctx, o, db, "")
	if err != nil || !ok || len(accts) != 1 || accts[0].AccountID != "222222222222" {
		t.Errorf("after a failed replacement the stored account list is gone or changed: ok=%v err=%v accounts=%v", ok, err, accts)
	}
}

// While one process re-stores a complete $100 September answer again and
// again, a local read that finds September covered always reads the whole
// $100 (a regression guard; the race window is too narrow to hit reliably).
func TestR2StoredAnswerIsReplacedAndReadWhole(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "race.db")
	db, err := store.OpenWithContext(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	pr := periodRange{Start: "2026-09-01", End: "2026-10-01", Label: "September", Granularity: "MONTHLY"}
	ans := awsx.CostAnswer{Buckets: []awsx.CostBucket{{Start: "2026-09-01", End: "2026-10-01"}}}
	for i, svc := range []string{"HQ-Service-A", "HQ-Service-B", "HQ-Service-C", "HQ-Service-D"} {
		ans.Lines = append(ans.Lines, awsx.CostLine{PeriodStart: "2026-09-01", PeriodEnd: "2026-10-01", Granularity: "MONTHLY",
			AccountID: "111111111111", Service: svc, AmountUSD: []float64{40, 30, 20, 10}[i], Unit: "USD"})
	}
	if err := storeCostAnswer(db, "111111111111", costByAccountService, "MONTHLY", ans); err != nil {
		t.Fatal(err)
	}
	o := awsReadOpts{dbPath: path, localOnly: true, sess: &awsSession{}}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = storeCostAnswer(db, "111111111111", costByAccountService, "MONTHLY", ans)
		}
	}()
	deadline := time.Now().Add(3 * time.Second)
	reads, wrong := 0, 0
	for time.Now().Before(deadline) && wrong == 0 {
		lines, _, ok, err := storedCostLines(ctx, o, pr, costByAccountService)
		if err != nil || !ok {
			continue // not covered right now: a live read would answer; never a wrong total
		}
		reads++
		var total float64
		for _, l := range lines {
			total += l.AmountUSD
		}
		if total != 100 {
			wrong++
			t.Errorf("a covered September read summed $%.2f over %d line(s) while the answer was being re-stored; want $100", total, len(lines))
		}
	}
	close(stop)
	wg.Wait()
	if reads == 0 {
		t.Fatal("no read found September covered; the test did not run")
	}
}

// Codex r2 P2 (redacted rows counted as complete coverage): a group whose
// key repeats a held credential is refused by the kit's SQL driver, so the
// whole replacement fails and the period stays uncovered. A read never
// counts a period as covered while one of its groups is missing.
func TestR2WithheldGroupKeyLeavesThePeriodUncovered(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	secret := "qZ7vX2mK9wL4pR6tN1sB/hJ3d" // no 8-character piece of the test name or its temp path
	credkit.Hold("config:aws_secret_access_key", secret)
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "withheld.db")
	db, err := store.OpenWithContext(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ans := awsx.CostAnswer{Buckets: []awsx.CostBucket{{Start: "2026-09-01", End: "2026-10-01"}}}
	for svc, amt := range map[string]float64{"HQ-Service-A": 60, "HQ-Service " + secret: 40} {
		ans.Lines = append(ans.Lines, awsx.CostLine{PeriodStart: "2026-09-01", PeriodEnd: "2026-10-01", Granularity: "MONTHLY",
			AccountID: "111111111111", Service: svc, AmountUSD: amt, Unit: "USD"})
	}
	if err := storeCostAnswer(db, "111111111111", costByAccountService, "MONTHLY", ans); !errors.Is(err, credkit.ErrWithheld) {
		t.Fatalf("storing an answer with a held group key: want credkit.ErrWithheld, got %v", err)
	}
	o := awsReadOpts{dbPath: path, localOnly: true, sess: &awsSession{}}
	lines, _, ok, err := storedCostLines(ctx, o, periodRange{Start: "2026-09-01", End: "2026-10-01", Label: "September", Granularity: "MONTHLY"}, costByAccountService)
	if ok || len(lines) != 0 {
		t.Errorf("September reads as covered with %d line(s) (err=%v) after its answer was refused", len(lines), err)
	}
}
