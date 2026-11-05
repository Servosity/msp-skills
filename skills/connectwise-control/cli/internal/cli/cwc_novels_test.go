// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"connectwise-control-pp-cli/internal/cliutil/testenv"
)

// runNovel runs one command against a fake instance that answers every POST
// with answer, and returns stdout, the error, the raw request bodies and paths.
func runNovel(t *testing.T, answer string, args ...string) (string, error, []string, []string) {
	t.Helper()
	var bodies []string
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		raw, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(raw))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(answer))
	}))
	t.Cleanup(srv.Close)
	testenv.Isolate(t)
	t.Setenv("CONNECTWISE_CONTROL_BASE_URL", srv.URL)
	t.Setenv("CONNECTWISE_CONTROL_USERNAME", "novelUnitUser-4c1a")
	t.Setenv("CONNECTWISE_CONTROL_PASSWORD", "novelUnitPass-9e2b7d")
	cwcNow = func() time.Time { return time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { cwcNow = time.Now })
	cmd := RootCmd()
	cmd.SetArgs(append(args, "--json"))
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.Execute()
	return out.String(), err, bodies, paths
}

func decodeReport(t *testing.T, out string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("output is not one JSON object: %v\n%s", err, out)
	}
	return m
}

func names(t *testing.T, v any) []string {
	t.Helper()
	items, _ := v.([]any)
	out := []string{}
	for _, it := range items {
		m, _ := it.(map[string]any)
		out = append(out, m["name"].(string))
	}
	return out
}

func TestSessionsStaleSortsAndSeparates(t *testing.T) {
	// The documented wire envelope, PascalCase as the instance sends it.
	answer := `{"ResponseInfoMap":{"HostSessionInfo":{"Sessions":[
		{"SessionID":"a","Name":"recent","LastConnectedEventTime":"2026-10-20T00:00:00Z"},
		{"SessionID":"b","Name":"old","LastConnectedEventTime":"2026-08-01T00:00:00Z"},
		{"SessionID":"c","Name":"oldest","LastConnectedEventTime":"/Date(1704067200000)/"},
		{"SessionID":"d","Name":"never"},
		{"SessionID":"e","Name":"garbled","LastConnectedEventTime":"last tuesday"},
		{"SessionID":"f","Name":"edge","LastConnectedEventTime":"2026-09-29T12:00:00Z"}
	]}}}`
	out, err, bodies, paths := runNovel(t, answer, "sessions", "stale")
	if err != nil {
		t.Fatalf("sessions stale: %v\n%s", err, out)
	}
	if len(paths) != 1 || paths[0] != "/Services/PageService.ashx/GetLiveData" {
		t.Fatalf("paths = %v", paths)
	}
	// GetLiveData's documented positional body (spec description; the cited
	// PowerShell module's Get-CWCSession sends the same), Access = 2.
	if want := `[{"ActionCenterInfo":{},"HostSessionInfo":{"filter":null,"findSessionID":null,"sessionGroupPathParts":["All Machines"],"sessionLimit":0,"sessionType":2}},0]`; bodies[0] != want {
		t.Fatalf("request body = %s, want %s", bodies[0], want)
	}
	rep := decodeReport(t, out)
	if got := strings.Join(names(t, rep["stale"]), ","); got != "oldest,old,edge" {
		t.Fatalf("stale = %s, want oldest,old,edge (oldest first, cutoff inclusive)", got)
	}
	if got := strings.Join(names(t, rep["no_connection_recorded"]), ","); got != "never" {
		t.Fatalf("no_connection_recorded = %s", got)
	}
	if got := strings.Join(names(t, rep["unreadable_timestamp"]), ","); got != "garbled" {
		t.Fatalf("unreadable_timestamp = %s", got)
	}
	if rep["total_sessions"].(float64) != 6 || rep["stale_count"].(float64) != 3 {
		t.Fatalf("totals = %v / %v", rep["total_sessions"], rep["stale_count"])
	}
}

func TestSessionsStaleReadsTheBareSpecArray(t *testing.T) {
	answer := `[{"sessionID":"x","name":"dark","lastConnectedEventTime":"2025-01-01T00:00:00Z"}]`
	out, err, bodies, _ := runNovel(t, answer, "sessions", "stale", "--session-type", "Support", "--days", "90")
	if err != nil {
		t.Fatalf("sessions stale: %v", err)
	}
	if !strings.Contains(bodies[0], `"sessionType":0`) {
		t.Fatalf("request body = %s, want sessionType 0 (Support)", bodies[0])
	}
	if got := strings.Join(names(t, decodeReport(t, out)["stale"]), ","); got != "dark" {
		t.Fatalf("stale = %s", got)
	}
}

// An answer in any other shape is an error, never "no stale sessions".
func TestSessionsStaleFailsClosedOnAnUnknownShape(t *testing.T) {
	for _, answer := range []string{`{}`, `null`, `{"ResponseInfoMap":{}}`, `{"ResponseInfoMap":{"HostSessionInfo":{}}}`, `[1,2]`, `"ok"`, `not json`, `[] []`} {
		out, err, _, _ := runNovel(t, answer, "sessions", "stale")
		if err == nil {
			t.Errorf("answer %s: want an error, got output %s", answer, out)
		}
	}
}

func TestSessionsStaleRefusesBadFlags(t *testing.T) {
	for _, args := range [][]string{{"--days", "0"}, {"--session-type", "Desktop"}} {
		_, err, _, paths := runNovel(t, `[]`, append([]string{"sessions", "stale"}, args...)...)
		if err == nil || len(paths) != 0 {
			t.Fatalf("%v: err=%v requests=%d", args, err, len(paths))
		}
	}
}

func TestAccessReviewFlagsAndReportsUnknowns(t *testing.T) {
	answer := `{"Users":[
		{"UserName":"ok","SecurityGroups":["Administrators"],"IsDisabled":false,"IsPasswordExpired":false},
		{"UserName":"gone","SecurityGroups":["Techs"],"IsDisabled":true,"IsPasswordExpired":true},
		{"UserName":"orphan","SecurityGroups":[],"IsDisabled":false,"IsPasswordExpired":false},
		{"UserName":"partial","IsDisabled":"true"}
	],"Roles":["Administrators","Techs"]}`
	out, err, bodies, paths := runNovel(t, answer, "security", "access-review")
	if err != nil {
		t.Fatalf("access-review: %v\n%s", err, out)
	}
	if paths[0] != "/Services/SecurityService.ashx/GetSecurityConfigurationInfo" || bodies[0] != "[]" {
		t.Fatalf("request = %s %s, want the documented empty positional array", paths[0], bodies[0])
	}
	rep := decodeReport(t, out)
	for k, want := range map[string]float64{"total_users": 4, "flagged_users": 2, "disabled": 1, "password_expired": 1, "no_security_group": 1, "users_with_unknown_fields": 1} {
		if rep[k].(float64) != want {
			t.Errorf("%s = %v, want %v", k, rep[k], want)
		}
	}
	byName := map[string]map[string]any{}
	for _, it := range rep["users"].([]any) {
		u := it.(map[string]any)
		byName[u["user_name"].(string)] = u
	}
	// A string "true" is not a boolean: unknown, never read as either value.
	if got, _ := json.Marshal(byName["partial"]["unknown"]); string(got) != `["disabled","password_expired","security_groups"]` {
		t.Errorf("partial unknown = %s", got)
	}
	if got, _ := json.Marshal(byName["gone"]["findings"]); string(got) != `["disabled","password_expired"]` {
		t.Errorf("gone findings = %s", got)
	}

	out, err, _, _ = runNovel(t, answer, "security", "access-review", "--flagged-only")
	if err != nil {
		t.Fatal(err)
	}
	if n := len(decodeReport(t, out)["users"].([]any)); n != 3 {
		t.Errorf("--flagged-only listed %d users, want 3", n)
	}
}

func TestAccessReviewFailsClosedOnAnUnknownShape(t *testing.T) {
	for _, answer := range []string{`{}`, `[]`, `null`, `{"users":null}`, `{"users":[1]}`, `{"users":{}}`} {
		out, err, _, _ := runNovel(t, answer, "security", "access-review")
		if err == nil {
			t.Errorf("answer %s: want an error, got output %s", answer, out)
		}
	}
}

func TestNovelsDryRunSendsNothing(t *testing.T) {
	var hits atomic.Int32
	for _, args := range [][]string{{"sessions", "stale", "--dry-run"}, {"security", "access-review", "--dry-run"}} {
		_, err, _, paths := runNovel(t, `[]`, args...)
		hits.Add(int32(len(paths)))
		if err != nil {
			t.Errorf("%v: %v", args, err)
		}
	}
	if hits.Load() != 0 {
		t.Fatalf("--dry-run sent %d request(s)", hits.Load())
	}
}
