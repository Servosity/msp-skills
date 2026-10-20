// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"cloudally-pp-cli/internal/cliutil/testenv"
)

// downloadSubmitArgs is a new --latest batch for the stub's task.
func downloadSubmitArgs(extra ...string) []string {
	return append([]string{"download", "submit", "--account", "acct-1", "--task", "task-1", "--latest", "--yes", "--json"}, extra...)
}

// ledgerKeys lists the batch ledgers on disk.
func ledgerKeys(t *testing.T) []string {
	t.Helper()
	dir, err := batchDir()
	if err != nil {
		t.Fatal(err)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	var keys []string
	for _, m := range matches {
		keys = append(keys, strings.TrimSuffix(filepath.Base(m), ".json"))
	}
	return keys
}

// TestDownloadSubmitHasNoWholeRunDeadline runs a batch with --timeout 1s
// and a reconcile wait longer than that: the wait must complete, and the
// batch key must be on screen and on disk before the first create.
func TestDownloadSubmitHasNoWholeRunDeadline(t *testing.T) {
	testenv.Isolate(t)
	prevDelay := reconcileDelay
	reconcileDelay = 1500 * time.Millisecond
	t.Cleanup(func() { reconcileDelay = prevDelay })
	s := newDLStub(t)
	s.taskEntities = []string{"a@contoso.com"}
	s.listCreated = true
	var ledgersAtFirstCreate int
	s.onCreate = func(map[string]any) {
		if ledgersAtFirstCreate == 0 {
			ledgersAtFirstCreate = len(ledgerKeys(t)) + 1000
		}
	}
	_, stderr, err := runDL(t, s, downloadSubmitArgs("--timeout", "1s")...)
	if err != nil {
		t.Fatalf("a batch whose reconcile wait outlasts --timeout must finish: %v\n%s", err, stderr)
	}
	keys := ledgerKeys(t)
	if len(keys) != 1 || ledgersAtFirstCreate != 1001 {
		t.Fatalf("the ledger should exist before the first create: ledgers=%v at-create=%d", keys, ledgersAtFirstCreate-1000)
	}
	announce := strings.Index(stderr, "batch "+keys[0]+" recorded")
	requested := strings.Index(stderr, "requested 2 archive(s)")
	if announce < 0 || requested < 0 || announce > requested {
		t.Fatalf("the batch key must be printed before the creates:\n%s", stderr)
	}
	if !strings.Contains(stderr, "--batch "+keys[0]+" --resume --yes") {
		t.Fatalf("the resume command should be printed:\n%s", stderr)
	}
	b, _ := loadBatch(keys[0])
	if b.count(dlCreated) != 2 {
		t.Fatalf("reconcile should have run to the end: %+v", b.Rows)
	}
}

// TestDownloadSubmitRefusesAnIdenticalRerun covers the re-run guard in one
// process: the same command without --resume is refused while the earlier
// batch owns archives, --new-batch overrides it, and a batch whose creates
// were all refused does not block.
func TestDownloadSubmitRefusesAnIdenticalRerun(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	s.taskEntities = []string{"a@contoso.com", "b@contoso.com"}
	if _, stderr, err := runDL(t, s, downloadSubmitArgs("--no-reconcile")...); err != nil {
		t.Fatalf("first run: %v\n%s", err, stderr)
	}
	first := ledgerKeys(t)
	if n := len(s.createdAccounts()); n != 4 || len(first) != 1 {
		t.Fatalf("first run: creates=%d ledgers=%v", n, first)
	}

	_, _, err := runDL(t, s, downloadSubmitArgs("--no-reconcile")...)
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "--batch "+first[0]+" --resume --yes") || !strings.Contains(err.Error(), "--new-batch") {
		t.Fatalf("an identical re-run must be refused with the resume command, got %v", err)
	}
	if n := len(s.createdAccounts()); n != 4 || len(ledgerKeys(t)) != 1 {
		t.Fatalf("a refused re-run created %d / wrote %d ledgers", n-4, len(ledgerKeys(t))-1)
	}
	// A different filter is a different command, but its archives were all
	// created by the first batch, so it creates none of them again.
	if _, stderr, err := runDL(t, s, downloadSubmitArgs("--no-reconcile", "--sub-sources", "O365MAIL")...); err != nil {
		t.Fatalf("a different sub-source filter should run: %v\n%s", err, stderr)
	}
	if n := len(s.createdAccounts()); n != 4 {
		t.Fatalf("creates after the filtered run = %d, want 4 (nothing created twice)", n)
	}
	if _, _, err := runDL(t, s, downloadSubmitArgs("--new-batch", "--resume", "--batch", first[0])...); err == nil || ExitCode(err) != 2 {
		t.Fatalf("--new-batch with --resume: want exit 2, got %v", err)
	}
	if _, stderr, err := runDL(t, s, downloadSubmitArgs("--no-reconcile", "--new-batch")...); err != nil {
		t.Fatalf("--new-batch: %v\n%s", err, stderr)
	}
	if n := len(s.createdAccounts()); n != 8 || len(ledgerKeys(t)) != 3 {
		t.Fatalf("--new-batch should create every export again: creates=%d ledgers=%d", n, len(ledgerKeys(t)))
	}

	// A batch whose every create was refused owns nothing.
	testenv.Isolate(t)
	s2 := newDLStub(t)
	s2.taskEntities = []string{"c@contoso.com"}
	s2.createCode["c@contoso.com"] = 401
	if _, _, err := runDL(t, s2, downloadSubmitArgs("--no-reconcile")...); err == nil {
		t.Fatal("a run whose creates were all refused should not exit 0")
	}
	delete(s2.createCode, "c@contoso.com")
	if _, stderr, err := runDL(t, s2, downloadSubmitArgs("--no-reconcile")...); err != nil {
		t.Fatalf("an earlier batch with only refused creates must not block: %v\n%s", err, stderr)
	}
}

// helperEnv names the environment a child process reads.
const (
	helperFlag  = "CLOUDALLY_DL_HELPER"
	helperArgs  = "CLOUDALLY_DL_HELPER_ARGS"
	helperHome  = "CLOUDALLY_DL_HELPER_HOME"
	helperPause = "CLOUDALLY_DL_HELPER_PAUSE"
)

// TestHelperDownloadSubmitProcess is not a test: it is the second process
// in the race tests below, run by re-executing this test binary.
func TestHelperDownloadSubmitProcess(t *testing.T) {
	if os.Getenv(helperFlag) != "1" {
		t.Skip("helper process only")
	}
	for _, k := range []string{"HOME", "USERPROFILE"} {
		_ = os.Setenv(k, os.Getenv(helperHome))
	}
	if d, err := time.ParseDuration(os.Getenv(helperPause)); err == nil {
		rerunGuardPause = d
	}
	var args []string
	_ = json.Unmarshal([]byte(os.Getenv(helperArgs)), &args)
	cmd := RootCmd()
	cmd.SetArgs(args)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.Execute()
	fmt.Fprint(os.Stderr, errOut.String())
	if err == nil {
		os.Exit(0)
	}
	fmt.Fprintln(os.Stderr, "ERROR:", err)
	os.Exit(ExitCode(err))
}

// childResult is one helper process's outcome.
type childResult struct {
	code   int
	stderr string
}

// runChildren starts one helper process per argument list, spaced by
// stagger, against the stub and the current isolated home, and waits for
// all of them.
func runChildren(t *testing.T, s *dlStub, pause, stagger time.Duration, argLists ...[]string) []childResult {
	t.Helper()
	home := os.Getenv("HOME")
	results := make([]childResult, len(argLists))
	var wg sync.WaitGroup
	for i, args := range argLists {
		raw, _ := json.Marshal(append(args, "--portal-url", s.portal.URL))
		c := exec.Command(os.Args[0], "-test.run=^TestHelperDownloadSubmitProcess$", "-test.count=1")
		c.Env = append(os.Environ(), helperFlag+"=1", helperArgs+"="+string(raw), helperHome+"="+home,
			helperPause+"="+pause.String(), "CLOUDALLY_BASE_URL="+s.api.URL)
		var errOut bytes.Buffer
		c.Stderr = &errOut
		wg.Add(1)
		go func(i int, c *exec.Cmd, errOut *bytes.Buffer) {
			defer wg.Done()
			err := c.Run()
			code := 0
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else if err != nil {
				code = -1
			}
			results[i] = childResult{code: code, stderr: errOut.String()}
		}(i, c, &errOut)
		time.Sleep(stagger)
	}
	wg.Wait()
	return results
}

// TestDownloadSubmitRerunGuardHoldsAcrossProcesses starts the same submit
// in two processes at once, each pausing between its check and its save:
// one batch is created, the other process is refused, and no export is
// created twice.
func TestDownloadSubmitRerunGuardHoldsAcrossProcesses(t *testing.T) {
	if testing.Short() {
		t.Skip("starts child processes")
	}
	testenv.Isolate(t)
	s := newDLStub(t)
	s.mu.Lock()
	s.taskEntities = []string{"a@contoso.com", "b@contoso.com"}
	s.mu.Unlock()
	args := downloadSubmitArgs("--no-reconcile")
	res := runChildren(t, s, 700*time.Millisecond, 0, args, args)
	codes := map[int]int{}
	for _, r := range res {
		codes[r.code]++
	}
	if codes[0] != 1 || codes[2] != 1 {
		t.Fatalf("want one run to succeed and one refused (exit 2), got %+v", res)
	}
	if n := len(s.createdAccounts()); n != 4 {
		t.Fatalf("creates = %d, want 4 (each export once)", n)
	}
	if keys := ledgerKeys(t); len(keys) != 1 {
		t.Fatalf("ledgers = %v, want one", keys)
	}

	// --new-batch in both: two batches in the same second, both kept.
	res = runChildren(t, s, 0, 0, downloadSubmitArgs("--no-reconcile", "--new-batch"), downloadSubmitArgs("--no-reconcile", "--new-batch"))
	for _, r := range res {
		if r.code != 0 {
			t.Fatalf("--new-batch runs should both succeed: %+v", res)
		}
	}
	if keys := ledgerKeys(t); len(keys) != 3 {
		t.Fatalf("two batches started together must get their own ledgers, got %v", keys)
	}
}

// TestDownloadSubmitConcurrentResumesDoNotOverlap resumes one batch from two
// processes while the first is mid-create: the second is refused, and no
// row is sent twice.
func TestDownloadSubmitConcurrentResumesDoNotOverlap(t *testing.T) {
	if testing.Short() {
		t.Skip("starts child processes")
	}
	testenv.Isolate(t)
	s := newDLStub(t)
	newSubmitBatch(t, "20221231-000000-shared", "a@contoso.com", "b@contoso.com")
	s.mu.Lock()
	s.createDelay = 2 * time.Second
	s.mu.Unlock()
	args := []string{"download", "submit", "--batch", "20221231-000000-shared", "--resume", "--yes", "--no-reconcile", "--json"}
	res := runChildren(t, s, 0, 700*time.Millisecond, args, args)
	if res[0].code != 0 {
		t.Fatalf("the first resume should succeed: %+v", res[0])
	}
	if res[1].code != 2 || !strings.Contains(res[1].stderr, errBatchInUse.Error()) {
		t.Fatalf("the second resume should be refused as in use: %+v", res[1])
	}
	if got := sortedCopy(s.createdAccounts()); len(got) != 2 || got[0] == got[1] {
		t.Fatalf("each row must be sent once, got %v", got)
	}
}

// TestDownloadSubmitResumeKeepsIncludeFlags proves a resumed create asks
// for the same deleted and legal-hold items the first run did, and that a
// conflicting flag is refused.
func TestDownloadSubmitResumeKeepsIncludeFlags(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	s.taskEntities = []string{"a@contoso.com", "b@contoso.com"}
	s.createCode["b@contoso.com"] = 401
	if _, _, err := runDL(t, s, downloadSubmitArgs("--no-reconcile", "--include-deleted", "--include-legal-hold")...); err == nil {
		t.Fatal("b@ was refused, so the first run should not exit 0")
	}
	keys := ledgerKeys(t)
	if len(keys) != 1 {
		t.Fatalf("ledgers = %v", keys)
	}
	delete(s.createCode, "b@contoso.com")
	s.mu.Lock()
	s.createBodies = nil
	s.mu.Unlock()

	if _, _, err := runDL(t, s, "download", "submit", "--batch", keys[0], "--resume", "--include-legal-hold=false", "--yes", "--no-reconcile"); err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "--include-legal-hold=false (batch recorded true)") {
		t.Fatalf("a conflicting include flag must be refused, got %v", err)
	}
	if n := len(s.createBodies); n != 0 {
		t.Fatalf("a refused resume sent %d create(s)", n)
	}
	if _, stderr, err := runDL(t, s, "download", "submit", "--batch", keys[0], "--resume", "--yes", "--no-reconcile"); err != nil {
		t.Fatalf("resume: %v\n%s", err, stderr)
	}
	if len(s.createBodies) != 2 {
		t.Fatalf("want b@'s two sub-sources re-sent, got %d", len(s.createBodies))
	}
	for _, body := range s.createBodies {
		if body["account"] != "b@contoso.com" || body["includeDeleted"] != "true" || body["includeLegalHold"] != "true" {
			t.Fatalf("a resumed create must keep the recorded include flags: %v", body)
		}
	}
}

// TestDownloadSubmitRetryUnknownChecksThePortalFirst proves --retry-unknown
// looks for the job even with --no-reconcile, and does not send one that
// is already there.
func TestDownloadSubmitRetryUnknownChecksThePortalFirst(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	b := newSubmitBatch(t, "20221231-000000-retry", "a@contoso.com")
	b.Rows[0].Status, b.Rows[0].Label = dlUnknown, rowLabelPrefix+"retry1"
	if err := saveBatch(b); err != nil {
		t.Fatal(err)
	}
	s.jobs = append(s.jobs, dlJob{JobID: "job-a", Account: "a@contoso.com", SubSource: "O365MAIL", Status: "INPROCESS", Snapshot: b.Rows[0].SnapshotRaw, Label: b.Rows[0].Label})
	if _, stderr, err := runDL(t, s, "download", "submit", "--batch", b.BatchKey, "--resume", "--retry-unknown", "--no-reconcile", "--yes"); err != nil {
		t.Fatalf("resume: %v\n%s", err, stderr)
	}
	if got := s.createdAccounts(); len(got) != 0 {
		t.Fatalf("a job already on the portal was sent again: %v", got)
	}
	if after, _ := loadBatch(b.BatchKey); after.Rows[0].Status != dlCreated {
		t.Fatalf("the row should be CREATED: %+v", after.Rows[0])
	}
}

// TestDownloadBatchKeysAreConfined covers --batch traversal and a doctored
// ledger.
func TestDownloadBatchKeysAreConfined(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	for _, key := range []string{"../x", "../../etc/passwd", "20221231-000000/../x", "..", "a.json", ""} {
		if _, err := loadBatch(key); err == nil {
			t.Fatalf("loadBatch(%q) accepted a bad key", key)
		}
	}
	if _, _, err := runDL(t, s, "download", "submit", "--batch", "../x", "--resume", "--yes"); err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "not a batch key") {
		t.Fatalf("submit --batch ../x: want a usage error, got %v", err)
	}
	if _, _, err := runDL(t, s, "download", "fetch", "--account", "acct-1", "--dest", t.TempDir(), "--year", "2022", "--batch", "../x"); err == nil || ExitCode(err) != 2 {
		t.Fatalf("fetch --batch ../x: want a usage error, got %v", err)
	}
	// A ledger that names a different key is refused, so it cannot steer
	// later saves.
	b := newSubmitBatch(t, "20221231-000000-honest", "a@contoso.com")
	path, _ := batchPath(b.BatchKey)
	raw, _ := os.ReadFile(path)
	doctored := bytes.Replace(raw, []byte(`"batch_key": "20221231-000000-honest"`), []byte(`"batch_key": "../../outside"`), 1)
	if err := os.WriteFile(path, doctored, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadBatch(b.BatchKey); err == nil || !strings.Contains(err.Error(), "names itself") {
		t.Fatalf("a doctored ledger must be refused, got %v", err)
	}
	if err := saveBatch(&downloadBatch{BatchKey: "../../outside"}); err == nil {
		t.Fatal("saveBatch accepted a traversal key")
	}
	// A ledger that is a link out of the folder is refused.
	outside := filepath.Join(t.TempDir(), "evil.json")
	_ = os.WriteFile(outside, []byte(`{"batch_key":"20221231-000000-linked","rows":[]}`), 0o600)
	link, _ := batchPath("20221231-000000-linked")
	if err := os.Symlink(outside, link); err == nil {
		if _, err := loadBatch("20221231-000000-linked"); err == nil {
			t.Fatal("a ledger that links out of the batch folder was read")
		}
	}
}

func TestNewBatchKeyIsUniqueWithinASecond(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	seen := map[string]bool{}
	for i := 0; i < 2000; i++ {
		k := newBatchKey(now, "5F1C2A-../")
		if err := validBatchKey(k); err != nil {
			t.Fatalf("generated key %q is not valid: %v", k, err)
		}
		if seen[k] {
			t.Fatalf("key %q generated twice in one second", k)
		}
		seen[k] = true
	}
	if k := newBatchKey(now, ""); validBatchKey(k) != nil {
		t.Fatalf("a key with no account prefix is invalid: %q", k)
	}
}
