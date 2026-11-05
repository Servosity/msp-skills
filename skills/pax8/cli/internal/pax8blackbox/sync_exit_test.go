// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package pax8blackbox

// Adversarial review 2026-11-05, class 2: a failed sync never exits 0. The fake
// is the one in paging_test.go; the oracle is the exit code of the real binaries
// and the sync summary they print.

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// summaryOf reads the counts sync reported.
func summaryOf(t *testing.T, stderr string) (success, warned, errored int) {
	t.Helper()
	for _, line := range strings.Split(stderr, "\n") {
		if !strings.Contains(line, `"sync_summary"`) {
			continue
		}
		var s struct {
			Success int `json:"success"`
			Warned  int `json:"warned"`
			Errored int `json:"errored"`
		}
		if err := json.Unmarshal([]byte(line), &s); err != nil {
			t.Fatalf("%v: %s", err, line)
		}
		return s.Success, s.Warned, s.Errored
	}
	t.Fatalf("no sync_summary in:\n%s", stderr)
	return 0, 0, 0
}

// The documented failure path: the token endpoint refuses the pair. The three
// dependents had no parents and counted as successes, so sync exited 0 with an
// empty store. MCP reported no error.
func TestSyncExitRejectedPairFails(t *testing.T) {
	t.Parallel()
	api := newPagedAPI(t, map[string][]string{"/companies": rowsOf("c", 1, "")})
	api.tokenStatus = http.StatusUnauthorized
	e := newEnv(t, api.sv, true)
	rc, out, errOut := e.runRC(t, "sync", "--agent")
	if rc == 0 {
		t.Errorf("sync exited 0 with every resource failed\n%s\n%s", out, errOut)
	}
	if success, _, errored := summaryOf(t, errOut); success != 0 || errored != 9 {
		t.Errorf("success=%d errored=%d, want 0 and 9\n%s", success, errored, errOut)
	}
	// A machine caller keeps the counts on stdout, as on a run that exits 0.
	if !strings.Contains(out, `"errored"`) {
		t.Errorf("the failed run printed no summary on stdout:\n%s", out)
	}
	s := e.mcpSession(t)
	if text, isErr := s.call("sync", map[string]any{}); !isErr {
		t.Errorf("MCP sync reported no error:\n%.800s", text)
	}
}

func TestSyncExitEveryResourceForbiddenFails(t *testing.T) {
	t.Parallel()
	api := newPagedAPI(t, nil)
	api.setFault(func(string, int, int) (int, string, bool) {
		return http.StatusForbidden, `{"status":403,"message":"Forbidden"}`, true
	})
	e := newEnv(t, api.sv, false).store(t)
	rc, out, errOut := e.runRC(t, "sync", "--agent")
	if rc == 0 {
		t.Errorf("sync exited 0 with 403 on every resource\n%s\n%s", out, errOut)
	}
	if success, _, _ := summaryOf(t, errOut); success != 0 {
		t.Errorf("success=%d, want 0\n%s", success, errOut)
	}
}

// A parent that really has no rows leaves its dependent with nothing to do, and
// that is a success.
func TestSyncExitParentWithNoRowsIsASuccess(t *testing.T) {
	t.Parallel()
	api := newPagedAPI(t, nil)
	e := newEnv(t, api.sv, false).store(t)
	rc, out, errOut := e.runRC(t, "sync", "--agent")
	if rc != 0 {
		t.Fatalf("an empty tenant rc=%d\n%s\n%s", rc, out, errOut)
	}
	if success, warned, errored := summaryOf(t, errOut); success != 9 || warned != 0 || errored != 0 {
		t.Errorf("success=%d warned=%d errored=%d, want 9 0 0\n%s", success, warned, errored, errOut)
	}
}

// One parent failing leaves the other resources' data in place, exit 0 as the
// help documents, and its dependent is not reported as a success.
func TestSyncExitDependentOfAFailedParentIsNotASuccess(t *testing.T) {
	t.Parallel()
	api := newPagedAPI(t, map[string][]string{"/companies": rowsOf("c", 2, "")})
	api.setFault(func(p string, _, _ int) (int, string, bool) {
		if p == "/subscriptions" {
			return http.StatusBadRequest, `{"status":400,"message":"no"}`, true
		}
		return 0, "", false
	})
	e := newEnv(t, api.sv, false).store(t)
	rc, _, errOut := e.runRC(t, "sync", "--agent")
	if rc != 0 {
		t.Errorf("a partial failure exits 0 unless --strict, rc=%d\n%s", rc, errOut)
	}
	success, _, errored := summaryOf(t, errOut)
	if errored != 2 || success != 7 {
		t.Errorf("success=%d errored=%d, want 7 and 2 (subscriptions and its usage summaries)\n%s", success, errored, errOut)
	}
	if rc, _, errOut := e.runRC(t, "sync", "--strict", "--agent"); rc == 0 {
		t.Errorf("--strict exited 0\n%s", errOut)
	}
}

// Parents kept from an earlier sync still get their dependents refreshed, but
// with the parent read failing this run the dependent is not a clean success.
func TestSyncExitStaleParentsAreNotASuccess(t *testing.T) {
	t.Parallel()
	api := newPagedAPI(t, map[string][]string{"/subscriptions": rowsOf("s", 2, activeSubFields)})
	e := newEnv(t, api.sv, false).store(t)
	if rc, _, errOut := e.runRC(t, "sync", "--resources", "subscriptions", "--agent"); rc != 0 {
		t.Fatalf("first sync rc=%d\n%s", rc, errOut)
	}
	api.setFault(func(p string, _, _ int) (int, string, bool) {
		if p == "/subscriptions" {
			return http.StatusBadRequest, `{"status":400,"message":"no"}`, true
		}
		return 0, "", false
	})
	rc, _, errOut := e.runRC(t, "sync", "--resources", "subscriptions", "--agent")
	if rc == 0 {
		t.Errorf("every selected resource failed or is unproven and sync exited 0\n%s", errOut)
	}
	if success, _, _ := summaryOf(t, errOut); success != 0 {
		t.Errorf("success=%d, want 0\n%s", success, errOut)
	}
}

// A dependent named on its own, with a parent never read, has not run. Once the
// parent has been read and holds no rows, the same command is a success.
func TestSyncExitParentNeverRead(t *testing.T) {
	t.Parallel()
	api := newPagedAPI(t, nil)
	e := newEnv(t, api.sv, false).store(t)
	rc, out, errOut := e.runRC(t, "sync", "--resources", "contacts", "--agent")
	if rc == 0 {
		t.Errorf("a dependent whose parent was never read exited 0\n%s\n%s", out, errOut)
	}
	if rc, out, errOut := e.runRC(t, "sync", "--resources", "companies", "--agent"); rc != 0 {
		t.Fatalf("reading the parent rc=%d\n%s\n%s", rc, out, errOut)
	}
	if rc, out, errOut := e.runRC(t, "sync", "--resources", "contacts", "--agent"); rc != 0 {
		t.Errorf("the parent was read and holds no rows, rc=%d\n%s\n%s", rc, out, errOut)
	}
}

// A preview reads nothing, so an unread parent is not a failure of the preview.
func TestSyncExitDryRunStaysClean(t *testing.T) {
	t.Parallel()
	api := newPagedAPI(t, nil)
	e := newEnv(t, api.sv, false).store(t)
	for _, args := range [][]string{{"sync", "--dry-run", "--agent"}, {"sync", "--resources", "contacts", "--dry-run", "--agent"}} {
		if rc, out, errOut := e.runRC(t, args...); rc != 0 {
			t.Errorf("%v rc=%d\n%s\n%s", args, rc, out, errOut)
		}
	}
	if n := len(api.requests("/companies")); n != 0 {
		t.Errorf("a dry run sent %d requests", n)
	}
}
