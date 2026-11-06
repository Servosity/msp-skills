// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"keeper-pp-cli/internal/cliutil/testenv"
)

// fakeServiceMode answers the allowlisted commands for two managed companies;
// state is changed between runs by the test.
type fakeServiceMode struct {
	mu       sync.Mutex
	twoFA    bool // ada's 2FA
	weak     int  // ada's weak passwords
	failMC   string
	noBW     string // the company without the BreachWatch add-on
	auditErr string // the company whose security audit answers with its error report
	current  string // the switched-to company, "" for the MSP
	commands []string
	extra    int // more locked users without 2FA, so a minority carries 2FA and invites
}

func (f *fakeServiceMode) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("api-key") != "Zq7mW2xR9vT4bN6kL1pS8cH3" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var rq struct {
			Command string `json:"command"`
		}
		_ = json.Unmarshal(body, &rq)
		f.mu.Lock()
		defer f.mu.Unlock()
		f.commands = append(f.commands, rq.Command)
		fields := strings.Fields(rq.Command)
		rows := func(r ...string) {
			fmt.Fprintf(w, `{"status":"success","command":%q,"data":[%s]}`, fields[0], strings.Join(r, ","))
		}
		switch fields[0] {
		case "switch-to-msp":
			if f.current == "" {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"status":"error","error":"Unexpected error: switch-to-msp: Already MSP"}`))
				return
			}
			f.current = ""
			_, _ = w.Write([]byte(`{"status":"success","command":"switch-to-msp","message":"Switched back to MSP","data":null}`))
		case "switch-to-mc":
			if f.current != "" {
				// commands/msp.py SwitchToMcCommand while a company is selected.
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, `{"status":"error","error":"Unexpected error: switch-to-mc: Already switched to Managed Company id=%s"}`, f.current)
				return
			}
			f.current = fields[1]
			fmt.Fprintf(w, `{"status":"success","command":"switch-to-mc","message":"Switched to MC 'Co %s'","data":null}`, fields[1])
		case "msp-info":
			if f.current != "" {
				t.Errorf("msp-info sent inside managed company %s", f.current)
			}
			rows(`{"company_id":4421,"company_name":"Contoso","plan":"business","allocated":5,"active":3}`,
				`{"company_id":5310,"company_name":"Fabrikam","plan":"business","allocated":2,"active":3}`)
		case "enterprise-info", "user-report", "security-audit-report":
			if f.current == "" {
				t.Errorf("%s sent in the MSP context during posture", rq.Command)
			}
			if f.current == f.failMC {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"status":"error","error":"Unexpected error: throttled"}`))
				return
			}
			dom := map[string]string{"4421": "contoso.com", "5310": "fabrikam.com"}[f.current]
			switch {
			case fields[0] == "enterprise-info" && strings.Contains(rq.Command, "--roles"):
				rows(`{"role_id":1,"name":"Admins","admin":true,"enforcements":{}}`)
			case fields[0] == "enterprise-info":
				roles := `["Admins"]`
				if strings.Contains(rq.Command, "--verbose") {
					roles = `[{"role_id":"1","role_name":"Admins"}]` // enterprise-info --users --verbose
				}
				ada := fmt.Sprintf(`{"user_id":1,"email":"ada@%s","name":"Ada","status":"Active","roles":%s`, dom, roles)
				if f.twoFA {
					ada += `,"2fa_enabled":true`
				}
				users := []string{ada + "}", fmt.Sprintf(`{"user_id":2,"email":"bo@%s","status":"Invited"}`, dom)}
				for i := range f.extra {
					users = append(users, fmt.Sprintf(`{"user_id":%d,"email":"u%d@%s","name":"U%d","status":"Locked","roles":%s}`, 10+i, i, dom, i, roles))
				}
				rows(users...)
			case fields[0] == "user-report":
				logins := []string{fmt.Sprintf(`{"email":"ada@%s","status":"Active","last_login":"2026-10-01 10:00:00+00:00"}`, dom),
					fmt.Sprintf(`{"email":"bo@%s","status":"Invited","last_login":"N/A"}`, dom)}
				for i := range f.extra {
					logins = append(logins, fmt.Sprintf(`{"email":"u%d@%s","status":"Locked","last_login":"2025-01-01 10:00:00+00:00"}`, i, dom))
				}
				rows(logins...)
			case strings.Contains(rq.Command, "--breachwatch") && f.current == f.noBW:
				// keepercommander/breachwatch.py validate_reporting.
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"status":"error","error":"Unexpected error: security-audit-report: BreachWatch is not enabled for this enterprise. Please visit https://www.keepersecurity.com/breachwatch.html for more information."}`))
			case fields[0] == "security-audit-report" && f.current == f.auditErr && !strings.Contains(rq.Command, "--breachwatch"):
				// commands/security_audit.py: the error report replaces the scores.
				rows(`{"vault_owner":"a@x.co","error_message":"Decryption fail"}`)
			case strings.Contains(rq.Command, "--breachwatch"):
				rows(fmt.Sprintf(`{"email":"ada@%s","at_risk":0,"passed":3,"ignored":0}`, dom))
			default:
				rows(fmt.Sprintf(`{"email":"ada@%s","weak":%d,"reused":1,"securityScore":70}`, dom, f.weak))
			}
		default:
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"status":"error","error":"Not permitted to perform this function"}`))
		}
	}
}

func runKeeperCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestPostureLifecycle(t *testing.T) {
	testenv.Isolate(t)
	f := &fakeServiceMode{twoFA: true, weak: 0}
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	t.Setenv("KEEPER_BASE_URL", srv.URL)
	t.Setenv("KEEPER_API_KEY", "Zq7mW2xR9vT4bN6kL1pS8cH3")

	out, err := runKeeperCLI(t, "posture", "report", "--json")
	if err != nil {
		t.Fatalf("run 1: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"status": "complete"`) || !strings.Contains(out, `"seats_over_allocated": true`) {
		t.Fatalf("run 1 output:\n%s", out)
	}
	if f.current != "" {
		t.Fatalf("the session was left in managed company %s", f.current)
	}

	// Ada turns 2FA off and gains weak passwords; Fabrikam's reads fail.
	f.mu.Lock()
	f.twoFA, f.weak, f.failMC = false, 3, "5310"
	f.mu.Unlock()
	out, err = runKeeperCLI(t, "posture", "report", "--json")
	if err != nil {
		t.Fatalf("run 2: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"status": "partial"`) || !strings.Contains(out, `"fetch_failures": 1`) {
		t.Fatalf("run 2 output:\n%s", out)
	}

	out, err = runKeeperCLI(t, "events", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"user.twofa_disabled", "4421/ada@contoso.com", "posture.weak_passwords_increased", "posture.twofa_coverage_dropped"} {
		if !strings.Contains(out, want) {
			t.Errorf("events list lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "fabrikam") || strings.Contains(out, `"client": "5310"`) {
		t.Errorf("a failed company read raised events:\n%s", out)
	}

	out, err = runKeeperCLI(t, "posture", "diff", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "later run failed") || !strings.Contains(out, "user.twofa_disabled") {
		t.Errorf("posture diff:\n%s", out)
	}

	out, _ = runKeeperCLI(t, "users", "risk", "--kind", "no-2fa", "--json")
	if !strings.Contains(out, "ada@contoso.com") || !strings.Contains(out, `"admin": true`) {
		t.Errorf("users risk:\n%s", out)
	}
	// Fabrikam's newest good run is run 1: Ada still had 2FA there.
	if strings.Contains(out, "ada@fabrikam.com") {
		t.Errorf("users risk read a failed run:\n%s", out)
	}
	out, _ = runKeeperCLI(t, "users", "hygiene", "--kind", "invited", "--json")
	if !strings.Contains(out, "bo@contoso.com") || !strings.Contains(out, "first_seen") {
		t.Errorf("users hygiene:\n%s", out)
	}
	out, _ = runKeeperCLI(t, "users", "find", "fabrikam", "--json")
	if !strings.Contains(out, `"company_id": "5310"`) {
		t.Errorf("users find:\n%s", out)
	}
	for _, c := range f.commands {
		if strings.Contains(c, "--output") || strings.Contains(c, "--save") || strings.Contains(c, "--apply-action") {
			t.Errorf("sent %q", c)
		}
	}
}

// Failing input (Codex r1): a security audit answered with its error report means
// the numbers are unknown. Stored as zeros, it read as a drop from 5 weak passwords
// to 0, and the weak users left every view.
func TestAuditErrorReportIsNoGoodSnapshot(t *testing.T) {
	testenv.Isolate(t)
	f := &fakeServiceMode{twoFA: true, weak: 5}
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	t.Setenv("KEEPER_BASE_URL", srv.URL)
	t.Setenv("KEEPER_API_KEY", "Zq7mW2xR9vT4bN6kL1pS8cH3")
	if out, err := runKeeperCLI(t, "posture", "report", "--json"); err != nil {
		t.Fatalf("run 1: %v\n%s", err, out)
	}
	f.mu.Lock()
	f.auditErr = "4421"
	f.mu.Unlock()
	for run := 2; run <= 3; run++ {
		out, err := runKeeperCLI(t, "posture", "report", "--json")
		if err != nil {
			t.Fatalf("run %d: %v\n%s", run, err, out)
		}
		if !strings.Contains(out, `"status": "partial"`) || !strings.Contains(out, "error report") {
			t.Errorf("run %d is no partial run naming the error report:\n%s", run, out)
		}
	}
	out, err := runKeeperCLI(t, "events", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "posture.weak_passwords") {
		t.Errorf("the error report read as a password change:\n%s", out)
	}
	if n := strings.Count(out, `"company.security_audit_error"`); n != 1 {
		t.Errorf("%d company.security_audit_error events, want one on the change:\n%s", n, out)
	}
	out, _ = runKeeperCLI(t, "users", "risk", "--kind", "weak", "--json")
	if !strings.Contains(out, "ada@contoso.com") {
		t.Errorf("the weak user left users risk:\n%s", out)
	}
}

// A typed read never runs in a managed company left behind by someone else:
// the session is returned to the MSP first.
func TestMSPReadReturnsAStraySession(t *testing.T) {
	testenv.Isolate(t)
	f := &fakeServiceMode{current: "5310"}
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	t.Setenv("KEEPER_BASE_URL", srv.URL)
	t.Setenv("KEEPER_API_KEY", "Zq7mW2xR9vT4bN6kL1pS8cH3")
	out, err := runKeeperCLI(t, "companies", "list", "--json")
	if err != nil || !strings.Contains(out, "Contoso") || !strings.Contains(out, "returned it to the MSP") {
		t.Fatalf("err %v\n%s", err, out)
	}
	// The read held the per-user session lock for this Service Mode origin (keeper
	// handfix mc-context-serialized).
	lock, err := sessionLockFile(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(lock + ".lock"); err != nil {
		t.Fatalf("no session lock file: %v", err)
	}
}

// Invalid arguments are refused before any request.
func TestInvalidArgumentsSendNothing(t *testing.T) {
	testenv.Isolate(t)
	f := &fakeServiceMode{}
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	t.Setenv("KEEPER_BASE_URL", srv.URL)
	t.Setenv("KEEPER_API_KEY", "Zq7mW2xR9vT4bN6kL1pS8cH3")
	for _, args := range [][]string{
		{"companies", "get", "4421 --output=/tmp/x"},
		{"companies", "get", "Contoso"},
		{"users", "list", "--mc", "1;switch-to-mc 2"},
		{"audit-trail", "events", "--username", "a@b.co --save"},
		{"audit-trail", "events", "--created", "last_7_days --output x"},
		{"billing", "report", "--month", "2026-9"},
		{"users", "last-login", "--days", "0"},
	} {
		if _, err := runKeeperCLI(t, args...); err == nil {
			t.Errorf("%v: no error", args)
		}
	}
	if len(f.commands) != 0 {
		t.Fatalf("sent %v", f.commands)
	}
}

// Codex r1 P2 breachwatch-required: a company without the BreachWatch add-on
// keeps its other posture reads, with breachwatch "not_enabled".
func TestPostureWithoutBreachWatchKeepsTheCompany(t *testing.T) {
	testenv.Isolate(t)
	f := &fakeServiceMode{twoFA: true, noBW: "5310"}
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	t.Setenv("KEEPER_BASE_URL", srv.URL)
	t.Setenv("KEEPER_API_KEY", "Zq7mW2xR9vT4bN6kL1pS8cH3")
	out, err := runKeeperCLI(t, "posture", "report", "--json")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, `"status": "complete"`) || !strings.Contains(out, `"breachwatch": "not_enabled"`) || strings.Contains(out, `"fetch_failures": 1`) {
		t.Fatalf("output:\n%s", out)
	}
}

// --agent (and --compact) must not drop a field the command's projection kept:
// the generated compaction keeps a key only when most rows carry it, so a user
// with 2FA on read as 2FA off, and a pending invite lost its age (adversarial
// n1, rule 4).
func TestAgentOutputKeepsEveryProjectedField(t *testing.T) {
	testenv.Isolate(t)
	f := &fakeServiceMode{twoFA: true, extra: 4}
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	t.Setenv("KEEPER_BASE_URL", srv.URL)
	t.Setenv("KEEPER_API_KEY", "Zq7mW2xR9vT4bN6kL1pS8cH3")

	for _, flag := range []string{"--agent", "--compact"} {
		out, err := runKeeperCLI(t, "users", "list", "--mc", "4421", flag)
		if err != nil {
			t.Fatalf("users list %s: %v\n%s", flag, err, out)
		}
		if !strings.Contains(out, `"2fa_enabled"`) {
			t.Errorf("users list %s dropped 2fa_enabled:\n%s", flag, out)
		}
	}
	if out, err := runKeeperCLI(t, "posture", "report", "--json"); err != nil {
		t.Fatalf("posture report: %v\n%s", err, out)
	}
	for _, flag := range []string{"--agent", "--compact"} {
		out, err := runKeeperCLI(t, "users", "hygiene", "--kind", "invited,locked", flag)
		if err != nil {
			t.Fatalf("users hygiene %s: %v\n%s", flag, err, out)
		}
		if !strings.Contains(out, `"invited_days_at_least"`) || !strings.Contains(out, `"first_seen"`) {
			t.Errorf("users hygiene %s dropped the invite age:\n%s", flag, out)
		}
	}
}
