// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"cloudally-pp-cli/internal/cliutil/testenv"
)

// TestNovelRestoreSubmitHelpWires smoke-tests that the restore submit command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelRestoreSubmitHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"restore", "submit", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("restore submit --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "submit"} {
		if !strings.Contains(help, want) {
			t.Fatalf("restore submit --help missing %q in output:\n%s", want, help)
		}
	}
}

func TestRestoreBodyShape(t *testing.T) {
	body := restoreBody("64b0e9", "MS365", "jane.doe@contoso.com", "ext-jane", 20261001120000, submitOptions{includeDeleted: true, targetAccount: "archive@contoso.com"})
	raw, _ := json.Marshal(body)
	var back map[string]any
	_ = json.Unmarshal(raw, &back)
	if back["taskId"] != "64b0e9" || back["account"] != "jane.doe@contoso.com" || back["source"] != "MS365" || back["extid"] != "ext-jane" {
		t.Fatalf("body = %s", raw)
	}
	if back["date"].(float64) != 20261001120000 || back["includeDeleted"] != true || back["includeLegalHold"] != false || back["targetAccount"] != "archive@contoso.com" {
		t.Fatalf("body = %s", raw)
	}
	if _, ok := back["restoreOption"]; ok {
		t.Fatalf("empty restoreOption must be omitted: %s", raw)
	}
	minimal := restoreBody("64b0e9", "MS365", "jane.doe@contoso.com", "", 20261001120000, submitOptions{})
	if _, ok := minimal["extid"]; ok {
		t.Fatal("empty extid must be omitted")
	}
}

func TestBatchKeyDeterministic(t *testing.T) {
	a := batchKey(scopeDistributor, "5f1c2a", "64b0e9", []string{"Jane.Doe@contoso.com", "john.smith@contoso.com"}, []string{"2026-10-02", "2026-10-01"})
	b := batchKey(scopeDistributor, "5f1c2a", "64b0e9", []string{"john.smith@contoso.com", "jane.doe@contoso.com"}, []string{"2026-10-01", "2026-10-02"})
	if a != b {
		t.Fatalf("order and case should not change the key: %s vs %s", a, b)
	}
	if c := batchKey(scopeReseller, "5f1c2a", "64b0e9", []string{"jane.doe@contoso.com"}, []string{"2026-10-01"}); c == a {
		t.Fatal("different scope must change the key")
	}
	id := newBatchID(time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC))
	if !strings.HasPrefix(id, "20261015-120000-") || len(id) != len("20261015-120000-ab12") {
		t.Fatalf("batch id = %q", id)
	}
}

func TestRateLimiterSpacesCalls(t *testing.T) {
	lim := newRateLimiter(50)
	start := time.Now()
	for i := 0; i < 5; i++ {
		if err := lim.wait(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if elapsed := time.Since(start); elapsed < 60*time.Millisecond {
		t.Fatalf("5 calls at 50/s should take at least 80ms minus jitter, took %s", elapsed)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := newRateLimiter(1).wait(ctx); err == nil {
		t.Fatal("cancelled context should abort the wait")
	}
}

func TestRestoreSubmitDryRunPrintsOneBodyPerRow(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"restore", "submit", "--account", "5f1c2a", "--task", "64b0e9", "--date", "2026-10-01", "--date", "2026-10-02",
		"--entities", "jane.doe@contoso.com,john.smith@contoso.com", "--dry-run"})
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("dry-run error = %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("want 4 body lines, got %d:\n%s", len(lines), out.String())
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &body); err != nil {
		t.Fatalf("line 0 not JSON: %v", err)
	}
	if body["account"] != "jane.doe@contoso.com" || body["date"].(float64) != 20261001235959 {
		t.Fatalf("line 0 = %s", lines[0])
	}
	if !strings.Contains(errOut.String(), "/v2/distributors/accounts/5f1c2a/recovery/restore") {
		t.Fatalf("stderr should name the path: %q", errOut.String())
	}
}

func TestRestoreSubmitValidation(t *testing.T) {
	testenv.Isolate(t)
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"restore", "submit", "--date", "2026-10-01"}, "--task"},
		{[]string{"restore", "submit", "--task", "64b0e9"}, "--date"},
		{[]string{"restore", "submit", "--task", "64b0e9", "--date", "2026-10-01", "--concurrency", "0"}, "--concurrency"},
		{[]string{"restore", "submit", "--reseller", "--task", "64b0e9", "--date", "2026-10-01"}, "--reseller requires --account"},
	}
	for _, tc := range cases {
		cmd := RootCmd()
		cmd.SetArgs(tc.args)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%v: want error containing %q, got %v", tc.args, tc.want, err)
		}
		if ExitCode(err) != 2 {
			t.Fatalf("%v: exit code %d, want 2", tc.args, ExitCode(err))
		}
	}
}

func TestResumeKeyIgnoresAccount(t *testing.T) {
	ents, dates := []string{"Jane.Doe@contoso.com"}, []string{"2026-10-01"}
	if resumeKey(scopeAccount, "64b0e9", ents, dates) != resumeKey(scopeAccount, "64b0e9", []string{"jane.doe@contoso.com"}, dates) {
		t.Fatal("resume key should ignore entity case")
	}
	if batchKey(scopeAccount, "acct-a", "64b0e9", ents, dates) == batchKey(scopeAccount, "acct-b", "64b0e9", ents, dates) {
		t.Fatal("batch key must include the account")
	}
	if resumeKey(scopeAccount, "64b0e9", nil, dates) == resumeKey(scopeAccount, "64b0e9", ents, dates) {
		t.Fatal("resume key must include the entity filter")
	}
}
