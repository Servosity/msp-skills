// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
)

type fleetSpawnRecorder struct {
	mu    sync.Mutex
	specs []fleetChildSpec
	reply func(spec fleetChildSpec) fleetChildOutcome
}

func (r *fleetSpawnRecorder) calls() []fleetChildSpec {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]fleetChildSpec(nil), r.specs...)
}

// fleetStubSpawn replaces the process spawner for one test.
func fleetStubSpawn(t *testing.T, reply func(spec fleetChildSpec) fleetChildOutcome) *fleetSpawnRecorder {
	t.Helper()
	rec := &fleetSpawnRecorder{reply: reply}
	prev := fleetSpawn
	fleetSpawn = func(_ context.Context, spec fleetChildSpec) fleetChildOutcome {
		rec.mu.Lock()
		rec.specs = append(rec.specs, spec)
		rec.mu.Unlock()
		if rec.reply == nil {
			return fleetChildOutcome{Stdout: []byte("[]")}
		}
		return rec.reply(spec)
	}
	t.Cleanup(func() { fleetSpawn = prev })
	return rec
}

func fleetEnvValue(env []string, key string) (string, bool) {
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		if k == key {
			return v, true
		}
	}
	return "", false
}

// fleetAccountOf names the account a stubbed child was started for.
func fleetAccountOf(spec fleetChildSpec) string {
	cfg, _ := fleetEnvValue(spec.Env, "ZOOM_PHONE_CONFIG")
	return filepath.Base(filepath.Dir(cfg))
}

func TestFleetRefusesUnsafeCommands(t *testing.T) {
	acctTestHome(t)
	acctMustAdd(t, "acme", "A1", "C1", "s3cret")
	acctMustAdd(t, "globex", "A2", "C2", "s3cret")
	rec := fleetStubSpawn(t, nil)

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"write command without --allow-writes", []string{"fleet", "--", "users", "delete-call-forward-setting", "abc"}, "writes or returns secrets"},
		{"secret-returning read without --allow-writes", []string{"fleet", "--", "devices", "get-adevice", "dev1"}, "writes or returns secrets"},
		{"accounts excluded", []string{"fleet", "--", "accounts", "list"}, "accounts"},
		{"auth excluded", []string{"fleet", "--allow-writes", "--", "auth", "status"}, "auth"},
		{"fleet excluded", []string{"fleet", "--", "fleet", "--", "doctor"}, "itself"},
		{"unknown command", []string{"fleet", "--", "no-such-command"}, "unknown command"},
		{"unknown subcommand", []string{"fleet", "--", "users", "no-such-sub"}, "unknown command"},
		{"--config", []string{"fleet", "--", "doctor", "--config", "/tmp/other.toml"}, "--config"},
		{"--home=", []string{"fleet", "--", "doctor", "--home=/tmp/other"}, "--home"},
		{"--profile", []string{"fleet", "--", "doctor", "--profile", "p"}, "--profile"},
		{"--db", []string{"fleet", "--", "sync", "--db", "/tmp/shared.db"}, "--db"},
		{"--receipt-file", []string{"fleet", "--", "users", "list-phone", "--receipt-file=/tmp/r.json"}, "--receipt-file"},
		{"--audit-dir", []string{"fleet", "--", "users", "list-phone", "--audit-dir", "/tmp/a"}, "--audit-dir"},
		{"--deliver", []string{"fleet", "--", "users", "list-phone", "--deliver", "webhook:https://example.com/h"}, "--deliver"},
		{"export -o shorthand", []string{"fleet", "--allow-writes", "--", "export", "-o", "/tmp/out.jsonl"}, "--output"},
		{"import -i (mcp:write-flags)", []string{"fleet", "--allow-writes", "--", "import", "-i", "/etc/passwd"}, "--input"},
		{"bad account name", []string{"fleet", "--accounts", "../x", "--", "doctor"}, "invalid account name"},
		{"offboard without --allow-writes", []string{"fleet", "--accounts", "acme", "--", "offboard", "jane@example.com"}, "writes or returns secrets"},
		{"offboard on every account", []string{"fleet", "--allow-writes", "--", "offboard", "jane@example.com"}, "exactly one account"},
		{"offboard on two accounts", []string{"fleet", "--allow-writes", "--accounts", "acme,globex", "--", "offboard", "jane@example.com"}, "exactly one account"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, errOut, code := acctRun(t, "", tc.args...)
			if code != 2 || !strings.Contains(errOut, tc.want) {
				t.Fatalf("exit=%d stderr=%q, want exit 2 mentioning %q", code, errOut, tc.want)
			}
		})
	}
	_, _, code := acctRun(t, "", "fleet", "--accounts", "nope", "--", "doctor")
	if code != 3 {
		t.Fatalf("unknown account exit=%d, want 3", code)
	}
	if n := len(rec.calls()); n != 0 {
		t.Fatalf("refused commands spawned %d children", n)
	}

	// A real "--" ends flag parsing: what follows is the child's positional data.
	if tgt, err := fleetResolveTarget(RootCmd(), []string{"users", "list-phone", "--", "--db"}, false); err != nil || tgt.parsed.Terminator != 2 {
		t.Fatalf("flag after a real -- refused or terminator wrong: %v %+v", err, tgt.parsed)
	}
}

// Round 2 item 1: the child-arg check follows the child's own flag parser. A
// "--" (or a flag-looking token) consumed as a string flag's VALUE does not end
// flag parsing, so the flags after it are still checked.
func TestFleetChildArgParserFollowsFlagValues(t *testing.T) {
	acctTestHome(t)
	acctMustAdd(t, "acme", "A1", "C1", "s3cret")
	refused := []struct {
		name string
		args []string
		want string
	}{
		{"-- as --site-id value", []string{"users", "list-phone", "--json", "--site-id", "--", "--config=/dev/null", "--home=/x"}, "--config"},
		{"-- as --site-id value, home only", []string{"users", "list-phone", "--site-id", "--", "--home=/x"}, "--home"},
		{"--site-id=-- inline", []string{"users", "list-phone", "--site-id=--", "--config=/dev/null"}, "--config"},
		{"-- as shorthand value", []string{"export", "-o", "--", "--db", "/tmp/x.db"}, "--output"},
		{"unknown flag", []string{"users", "list-phone", "--no-such-flag"}, "unknown flag"},
		{"value missing at end", []string{"users", "list-phone", "--site-id"}, "needs a value"},
	}
	rec := fleetStubSpawn(t, nil)
	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			_, errOut, code := acctRun(t, "", append([]string{"fleet", "--allow-writes", "--"}, tc.args...)...)
			if code != 2 || !strings.Contains(errOut, tc.want) {
				t.Fatalf("exit=%d stderr=%q, want exit 2 mentioning %q", code, errOut, tc.want)
			}
		})
	}
	if n := len(rec.calls()); n != 0 {
		t.Fatalf("refused bypasses spawned %d children", n)
	}

	allowed := []struct {
		name  string
		args  []string
		child []string
	}{
		// --config=/dev/null is site-id's value here, exactly as pflag reads it.
		{"flag-looking value", []string{"users", "list-phone", "--site-id", "--config=/dev/null"}, []string{"users", "list-phone", "--site-id", "--config=/dev/null", "--json"}},
		// Inline value, then a real terminator: what follows is positional.
		{"--site-id=-- then real --", []string{"users", "list-phone", "--site-id=--", "--", "--config=/dev/null"}, []string{"users", "list-phone", "--site-id=--", "--json", "--", "--config=/dev/null"}},
		{"--json as a value is not --json", []string{"users", "list-phone", "--site-id", "--json"}, []string{"users", "list-phone", "--site-id", "--json", "--json"}},
	}
	for _, tc := range allowed {
		t.Run(tc.name, func(t *testing.T) {
			rec := fleetStubSpawn(t, nil)
			_, errOut, code := acctRun(t, "", append([]string{"fleet", "--"}, tc.args...)...)
			if code != 0 {
				t.Fatalf("exit=%d stderr=%s", code, errOut)
			}
			calls := rec.calls()
			if len(calls) != 1 || !reflect.DeepEqual(calls[0].Args, tc.child) {
				t.Fatalf("child args = %+v, want %q", calls, tc.child)
			}
		})
	}
}

// Item 4: sync (and local-write commands) run without --allow-writes; offboard
// runs with --allow-writes on exactly one named account; writes run with
// --allow-writes.
func TestFleetAllowedCommands(t *testing.T) {
	acctTestHome(t)
	acctMustAdd(t, "acme", "A1", "C1", "s3cret")
	acctMustAdd(t, "globex", "A2", "C2", "s3cret")

	cases := []struct {
		name     string
		args     []string
		accounts []string
		child    []string
	}{
		{"sync", []string{"fleet", "--", "sync"}, []string{"acme", "globex"}, []string{"sync", "--json"}},
		{"doctor", []string{"fleet", "doctor"}, []string{"acme", "globex"}, []string{"doctor", "--json"}},
		{"write with --allow-writes", []string{"fleet", "--allow-writes", "--", "users", "delete-call-forward-setting", "abc"}, []string{"acme", "globex"}, []string{"users", "delete-call-forward-setting", "abc", "--json"}},
		{"offboard on one account", []string{"fleet", "--allow-writes", "--accounts", "globex", "--", "offboard", "jane@example.com"}, []string{"globex"}, []string{"offboard", "jane@example.com", "--json"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := fleetStubSpawn(t, nil)
			_, errOut, code := acctRun(t, "", tc.args...)
			if code != 0 {
				t.Fatalf("exit=%d stderr=%s", code, errOut)
			}
			calls := rec.calls()
			got := make([]string, 0, len(calls))
			for _, c := range calls {
				got = append(got, fleetAccountOf(c))
				if !reflect.DeepEqual(c.Args, tc.child) {
					t.Fatalf("child args = %q, want %q", c.Args, tc.child)
				}
			}
			if strings.Join(sortedCopy(got), ",") != strings.Join(tc.accounts, ",") {
				t.Fatalf("ran on %v, want %v", got, tc.accounts)
			}
		})
	}
	root := RootCmd()
	syncCmd, _, err := root.Find([]string{"sync"})
	if err != nil {
		t.Fatal(err)
	}
	if access := fleetCommandAccess(root, syncCmd); access != fleetAccessLocalWrite {
		t.Fatalf("sync access = %s", access)
	}
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

// Item 1: the child inherits no ZOOM_PHONE_* variable outside the allowlist.
func TestFleetChildArgvAndEnv(t *testing.T) {
	acctTestHome(t)
	path := acctMustAdd(t, "acme", "A1", "C1", "s3cret")
	// A parent shell pointed at some other tenant and host must not leak.
	parent := map[string]string{
		"ZOOM_PHONE_ACCOUNT_ID":          "PARENT_ACCT",
		"ZOOM_PHONE_CLIENT_ID":           "PARENT_CID",
		"ZOOM_PHONE_CLIENT_SECRET":       "PARENT_SECRET",
		"ZOOM_PHONE_DATA_DIR":            "/parent/data",
		"ZOOM_PHONE_CONFIG":              "/parent/config.toml",
		"ZOOM_PHONE_BASE_URL":            "https://PARENT_HOST.example/v2",
		"ZOOM_PHONE_TOKEN_URL":           "https://PARENT_HOST.example/oauth/token",
		"ZOOM_PHONE_OAUTH_SCOPE":         "PARENT_SCOPE",
		"ZOOM_PHONE_FEEDBACK_ENDPOINT":   "https://PARENT_HOST.example/fb",
		"ZOOM_PHONE_MCP_HTTP_TOKEN":      "PARENT_MCP_TOKEN",
		"ZOOM_PHONE_SOMETHING_NEW_LATER": "PARENT_FUTURE",
		"ZOOM_PHONE_NO_LEARN":            "1",
		"ZOOM_PHONE_USER_AGENT":          "msp-agent/1",
		"UNRELATED_VAR":                  "kept",
	}
	for k, v := range parent {
		t.Setenv(k, v)
	}
	rec := fleetStubSpawn(t, nil)

	_, errOut, code := acctRun(t, "", "fleet", "--", "users", "list-phone", "--page-size", "5")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errOut)
	}
	calls := rec.calls()
	if len(calls) != 1 {
		t.Fatalf("spawned %d children, want 1", len(calls))
	}
	spec := calls[0]
	exe, _ := os.Executable()
	if spec.Path != exe {
		t.Fatalf("child path = %q, want os.Executable %q", spec.Path, exe)
	}
	if want := []string{"users", "list-phone", "--page-size", "5", "--json"}; !reflect.DeepEqual(spec.Args, want) {
		t.Fatalf("child args = %q, want %q", spec.Args, want)
	}
	zoomVars := map[string]string{}
	for _, kv := range spec.Env {
		k, v, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(strings.ToUpper(k), "ZOOM_PHONE_") {
			if _, dup := zoomVars[k]; dup {
				t.Fatalf("%s set twice in child env", k)
			}
			zoomVars[k] = v
		}
	}
	want := map[string]string{
		"ZOOM_PHONE_CONFIG":     path,
		"ZOOM_PHONE_HOME":       filepath.Dir(path),
		"ZOOM_PHONE_NO_LEARN":   "1",
		"ZOOM_PHONE_USER_AGENT": "msp-agent/1",
	}
	if !reflect.DeepEqual(zoomVars, want) {
		t.Fatalf("child ZOOM_PHONE_* env = %v, want exactly %v", zoomVars, want)
	}
	if strings.Contains(strings.Join(spec.Env, "\n"), "PARENT_") {
		t.Fatalf("a parent override value reached the child env")
	}
	if v, _ := fleetEnvValue(spec.Env, "UNRELATED_VAR"); v != "kept" {
		t.Fatalf("non-ZOOM_PHONE env var dropped: %q", v)
	}
}

// Item 3: with an account selected via `accounts path --env`, fleet still runs
// every account and each child gets its own home, not the selected one.
func TestFleetSeesAllAccountsWhenOneIsSelected(t *testing.T) {
	acctTestHome(t)
	acme := acctMustAdd(t, "acme", "A1", "C1", "s1")
	globex := acctMustAdd(t, "globex", "A2", "C2", "s2")
	t.Setenv("ZOOM_PHONE_HOME", filepath.Dir(acme))
	t.Setenv("ZOOM_PHONE_CONFIG", acme)
	rec := fleetStubSpawn(t, nil)

	_, errOut, code := acctRun(t, "", "fleet", "--", "doctor")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errOut)
	}
	homes := map[string]string{}
	for _, c := range rec.calls() {
		h, _ := fleetEnvValue(c.Env, "ZOOM_PHONE_HOME")
		homes[fleetAccountOf(c)] = h
	}
	if len(homes) != 2 || homes["acme"] != filepath.Dir(acme) || homes["globex"] != filepath.Dir(globex) {
		t.Fatalf("children homes = %v", homes)
	}
}

func TestFleetChildArgsJSONFlag(t *testing.T) {
	cases := []struct {
		in, want []string
	}{
		{[]string{"doctor"}, []string{"doctor", "--json"}},
		{[]string{"users", "list-phone", "--agent"}, []string{"users", "list-phone", "--agent"}},
		{[]string{"users", "list-phone", "--json"}, []string{"users", "list-phone", "--json"}},
		{[]string{"users", "list-phone", "--", "--json"}, []string{"users", "list-phone", "--json", "--", "--json"}},
	}
	for _, tc := range cases {
		tgt, err := fleetResolveTarget(RootCmd(), tc.in, false)
		if err != nil {
			t.Fatalf("resolve %q: %v", tc.in, err)
		}
		if got := fleetChildArgs(tc.in, tgt.parsed); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("fleetChildArgs(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Item 6: per-account rows in account order; some failed -> exit 6, all
// failed -> exit 5, none failed -> 0.
func TestFleetMergesResultsAndExitCodes(t *testing.T) {
	acctTestHome(t)
	acctMustAdd(t, "acme", "A1", "C1", "s1")
	acctMustAdd(t, "globex", "A2", "C2", "s2")
	failing := map[string]bool{"globex": true}
	fleetStubSpawn(t, func(spec fleetChildSpec) fleetChildOutcome {
		name := fleetAccountOf(spec)
		if failing[name] {
			return fleetChildOutcome{Stdout: []byte("not json\n"), Stderr: []byte("error: HTTP 401 invalid token\n"), ExitCode: 4}
		}
		return fleetChildOutcome{Stdout: []byte(`[{"id":"u-` + name + `","email":"jane@example.com"}]` + "\n")}
	})

	out, _, code := acctRun(t, "", "fleet", "--json", "--", "users", "list-phone")
	if code != 6 {
		t.Fatalf("one of two failed: exit=%d, want 6", code)
	}
	var results []struct {
		Account   string          `json:"account"`
		AccountID string          `json:"account_id"`
		ExitCode  int             `json:"exit_code"`
		OK        bool            `json:"ok"`
		Result    json.RawMessage `json:"result"`
		Stderr    string          `json:"stderr"`
	}
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		t.Fatalf("output not a JSON array: %v\n%s", err, out)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results: %s", len(results), out)
	}
	a, g := results[0], results[1]
	if a.Account != "acme" || a.AccountID != "A1" || a.ExitCode != 0 || !a.OK || a.Stderr != "" {
		t.Fatalf("acme row = %+v", a)
	}
	var rows []map[string]any
	if err := json.Unmarshal(a.Result, &rows); err != nil || len(rows) != 1 || rows[0]["id"] != "u-acme" {
		t.Fatalf("acme result not parsed JSON: %s", a.Result)
	}
	if g.Account != "globex" || g.AccountID != "A2" || g.ExitCode != 4 || g.OK || g.Stderr != "error: HTTP 401 invalid token" {
		t.Fatalf("globex row = %+v", g)
	}
	var raw string
	if err := json.Unmarshal(g.Result, &raw); err != nil || raw != "not json" {
		t.Fatalf("globex result = %s, want raw string \"not json\"", g.Result)
	}

	failing["acme"] = true
	out, _, code = acctRun(t, "", "fleet", "--json", "--", "users", "list-phone")
	if code != 5 || json.Unmarshal([]byte(out), &results) != nil || len(results) != 2 || results[0].OK || results[1].OK {
		t.Fatalf("all failed: exit=%d out=%s, want 5 with both rows", code, out)
	}

	failing = map[string]bool{}
	_, _, code = acctRun(t, "", "fleet", "--json", "--", "users", "list-phone")
	if code != 0 {
		t.Fatalf("none failed: exit=%d", code)
	}

	// --accounts runs only the named accounts, in the order given.
	out, _, _ = acctRun(t, "", "fleet", "--json", "--accounts", "globex,acme", "--", "users", "list-phone")
	if err := json.Unmarshal([]byte(out), &results); err != nil || len(results) != 2 || results[0].Account != "globex" || results[1].Account != "acme" {
		t.Fatalf("--accounts order: %s", out)
	}
}

func TestFleetOutcomeErr(t *testing.T) {
	ok := fleetResult{OK: true}
	bad := fleetResult{OK: false, ExitCode: 5}
	cases := []struct {
		results []fleetResult
		want    int
	}{
		{nil, 0},
		{[]fleetResult{ok, ok}, 0},
		{[]fleetResult{ok, bad}, 6},
		{[]fleetResult{bad}, 5},
		{[]fleetResult{bad, bad}, 5},
	}
	for _, tc := range cases {
		err := fleetOutcomeErr("users list-phone", tc.results)
		got := 0
		if err != nil {
			got = ExitCode(err)
		}
		if got != tc.want {
			t.Errorf("results %+v: exit %d, want %d", tc.results, got, tc.want)
		}
	}
}

func TestFleetNoAccountsIsEmptyArray(t *testing.T) {
	acctTestHome(t)
	rec := fleetStubSpawn(t, nil)
	out, errOut, code := acctRun(t, "", "fleet", "--json", "--", "doctor")
	if code != 0 || strings.TrimSpace(out) != "[]" {
		t.Fatalf("exit=%d out=%q, want [] and 0", code, out)
	}
	if !strings.Contains(errOut, "accounts connect") {
		t.Fatalf("stderr hint missing 'accounts connect': %q", errOut)
	}
	if n := len(rec.calls()); n != 0 {
		t.Fatalf("spawned %d children with no accounts", n)
	}
}

func TestFleetDryRunPlansWithoutSpawning(t *testing.T) {
	acctTestHome(t)
	path := acctMustAdd(t, "acme", "A1", "C1", "s3cret")
	rec := fleetStubSpawn(t, nil)
	out, _, code := acctRun(t, "", "fleet", "--dry-run", "--json", "--", "users", "list-phone")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	var plan fleetPlan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("plan not JSON: %v\n%s", err, out)
	}
	if !plan.DryRun || plan.Command != "users list-phone" || !plan.ReadOnly || plan.Access != fleetAccessReadOnly || len(plan.Accounts) != 1 ||
		plan.Accounts[0].Account != "acme" || plan.Accounts[0].Config != path ||
		!reflect.DeepEqual(plan.Accounts[0].Args, []string{"users", "list-phone", "--json"}) {
		t.Fatalf("plan = %+v", plan)
	}
	if n := len(rec.calls()); n != 0 {
		t.Fatalf("dry-run spawned %d children", n)
	}
}

func TestFleetStderrTailIsBounded(t *testing.T) {
	long := strings.Repeat("x", 5000) + "LAST-LINE"
	got := fleetTail([]byte(long), fleetStderrTailBytes)
	if len(got) > fleetStderrTailBytes+3 || !strings.HasSuffix(got, "LAST-LINE") || !strings.HasPrefix(got, "...") {
		t.Fatalf("tail len=%d suffix ok=%v", len(got), strings.HasSuffix(got, "LAST-LINE"))
	}
	if got := fleetTail([]byte("  short \n"), fleetStderrTailBytes); got != "short" {
		t.Fatalf("short tail = %q", got)
	}
}
