// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"comet-backup-pp-cli/internal/cliutil"
	"comet-backup-pp-cli/internal/cliutil/testenv"
	"comet-backup-pp-cli/internal/config"
)

func TestCometFormAuth(t *testing.T) {
	cfg := &config.Config{CometBackupUsername: "admin", CometBackupPassword: "pw"}

	// Parameterless call: generator sends a JSON {} body.
	b, ct, err := cometFormAuth(cfg, "/api/v1/admin/list-users", []byte(`{}`), "application/json", false)
	if err != nil || ct != cometFormContentType {
		t.Fatalf("got ct=%q err=%v", ct, err)
	}
	v, _ := url.ParseQuery(string(b))
	if v.Get("Username") != "admin" || v.Get("AuthType") != "Password" || v.Get("Password") != "pw" {
		t.Fatalf("credentials not in body: %v", v)
	}

	// Form body keeps params; caller-supplied credentials are dropped.
	b, _, _ = cometFormAuth(cfg, "/api/v1/admin/get-jobs-for-date-range", []byte("Start=1&End=2&Username=evil&AuthType=SessionKey&SessionKey=x&Token=t"), cometFormContentType, false)
	v, _ = url.ParseQuery(string(b))
	if v.Get("Start") != "1" || v.Get("Username") != "admin" || v.Has("SessionKey") || v.Has("Token") || v.Get("AuthType") != "Password" {
		t.Fatalf("form merge wrong: %v", v)
	}

	// JSON stdin: non-string values become their JSON text; numbers stay exact.
	b, _, _ = cometFormAuth(cfg, "/api/v1/admin/x", []byte(`{"Start":1700000000,"Simple":true,"Paths":["a","b"],"Skip":null}`), "application/json", false)
	v, _ = url.ParseQuery(string(b))
	if v.Get("Start") != "1700000000" || v.Get("Simple") != "true" || v.Get("Paths") != `["a","b"]` || v.Has("Skip") {
		t.Fatalf("json conversion wrong: %v", v)
	}

	// Session key wins; dry-run redacts.
	cfg.CometBackupSessionKey = "sk"
	b, _, _ = cometFormAuth(cfg, "/api/v1/admin/x", nil, "", true)
	v, _ = url.ParseQuery(string(b))
	if v.Get("AuthType") != "SessionKey" || v.Get("SessionKey") != "<redacted>" || v.Has("Password") {
		t.Fatalf("session/redact wrong: %v", v)
	}
	if cometMaskValue(cfg, true) != "" || cometMaskValue(cfg, false) != "sk" {
		t.Fatal("mask value wrong")
	}

	// Non-Comet paths are untouched.
	if b, ct, _ := cometFormAuth(cfg, "/other", []byte("x"), "text/plain", false); string(b) != "x" || ct != "text/plain" {
		t.Fatalf("non-api path modified")
	}

	// A non-object JSON body is an error, not a silent drop.
	if _, _, err := cometFormAuth(cfg, "/api/v1/admin/x", []byte(`[1]`), "application/json", false); err == nil {
		t.Fatal("expected error for array body")
	}
}

func TestCometEnvelopeError(t *testing.T) {
	c := &Client{Config: &config.Config{CometBackupPassword: "hunter2-secret"}}
	cases := []struct {
		body   string
		failed bool
		code   int
	}{
		{`{"Status":200,"Message":"OK"}`, false, 0},
		{`{"Status":403,"Message":"Invalid credentials"}`, true, 403},
		{`{"Status":500,"Message":"No such job","JobID":""}`, true, 500},
		{`{"Status":0,"Message":"weird"}`, false, 0},
		{`{"Status":404,"Message":""}`, false, 0},
		{`{"Status":700,"Message":"odd"}`, true, 500},
		{`{"GUID":"j1","Status":7001}`, false, 0}, // BackupJobDetail: job state, no Message
		{`[{"Status":500,"Message":"x"}]`, false, 0},
		{`not json`, false, 0},
	}
	for _, tc := range cases {
		err := c.cometEnvelopeError("POST", "/api/v1/admin/x", []byte(tc.body))
		if (err != nil) != tc.failed {
			t.Fatalf("%s: failed=%v want %v", tc.body, err != nil, tc.failed)
		}
		if err != nil && err.StatusCode != tc.code {
			t.Fatalf("%s: code=%d want %d", tc.body, err.StatusCode, tc.code)
		}
	}
	err := c.cometEnvelopeError("POST", "/x", []byte(`{"Status":403,"Message":"bad password hunter2-secret"}`))
	if err == nil || strings.Contains(err.Error(), "hunter2-secret") {
		t.Fatalf("envelope error must mask the configured secret: %v", err)
	}
}

// TestCometClientHooks drives the generated client end to end. It fails when a
// reprint drops any of the five hand-fix call sites in client.go.
func TestCometClientHooks(t *testing.T) {
	src, err := os.ReadFile("client.go")
	if err != nil {
		t.Fatalf("reading client.go: %v", err)
	}
	switch n := strings.Count(string(src), "comet-backup hand-fix ("); {
	case n == 0:
		// A fresh regen before apply_client_hooks.py; skip so `generate --validate`
		// does not fail. handfixes.json in the monorepo still gates a lost hook set.
		t.Skip("comet-backup hand-fix hooks not applied; run handfix/apply_client_hooks.py")
	case n != 5:
		t.Fatalf("client.go has %d of 5 comet-backup hand-fix hooks; re-run apply_client_hooks.py on a fresh client.go", n)
	}
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	var gotBody url.Values
	var gotHeader string
	var otherHits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
		w.Write([]byte(`{}`))
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		gotBody, _ = url.ParseQuery(string(raw))
		gotHeader = r.Header.Get("X-Comet-Admin-Password")
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/admin/list-users":
			w.Write([]byte(`["acme"]`))
		case "/api/v1/admin/job/cancel":
			w.Write([]byte(`{"Status":500,"Message":"job not running; password was s3cret-pw"}`))
		case "/api/v1/admin/get-job-log":
			w.Write([]byte(`{"Status":403,"Message":"denied"}`))
		case "/api/v1/admin/redirect":
			// "localhost" is a different origin from 127.0.0.1 and, unlike an IP
			// literal, is not caught by the generated private-destination guard,
			// so only the comet redirect hook can stop the body replay.
			http.Redirect(w, r, strings.Replace(other.URL, "127.0.0.1", "localhost", 1)+"/steal", http.StatusTemporaryRedirect)
		}
	}))
	defer srv.Close()
	cfg := &config.Config{BaseURL: srv.URL, CometBackupUsername: "admin", CometBackupPassword: "s3cret-pw"}
	c := New(cfg, 5*time.Second, 0)
	c.NoCache = true
	ctx := context.Background()

	if _, _, err := c.PostQueryFormWithParams(ctx, "/api/v1/admin/list-users", nil, url.Values{}); err != nil {
		t.Fatalf("list-users: %v", err)
	}
	if gotBody.Get("Username") != "admin" || gotBody.Get("Password") != "s3cret-pw" || gotHeader != "" {
		t.Fatalf("hook body-auth/no-header missing: body=%v header=%q", gotBody, gotHeader)
	}

	_, _, err = c.PostFormWithParams(ctx, "/api/v1/admin/job/cancel", nil, url.Values{"JobID": {"j1"}})
	if err == nil {
		t.Fatal("hook envelope missing: a 2xx failure envelope must be an error")
	}
	if strings.Contains(err.Error(), "s3cret-pw") {
		t.Fatalf("envelope error leaked the password: %v", err)
	}

	_, _, err = c.PostQueryFormWithParamsAndHeaders(ctx, "/api/v1/admin/get-job-log", nil, url.Values{"JobID": {"j1"}}, map[string]string{TextResponseHeader: "true"})
	if err == nil {
		t.Fatal("hook envelope must run before the text-response early return")
	}

	_, _, err = c.PostQueryFormWithParams(ctx, "/api/v1/admin/redirect", nil, url.Values{})
	if err == nil || otherHits.Load() != 0 {
		t.Fatalf("hook redirect missing: cross-origin 307 must be refused (err=%v, other host hits=%d)", err, otherHits.Load())
	}
}

func TestMaskCometText(t *testing.T) {
	c := &Client{Config: &config.Config{CometBackupPassword: "p@ss+word", CometBackupSessionKey: "sk-123"}}
	leaks := []string{
		"bad p@ss+word",
		"Password=p%40ss%2Bword",
		"50% complete Password=p%2540ss%252Bword", // malformed "% c" before a double-encoded secret
		"p%2525252540ss%252525252Bword",           // deep nesting
		"session sk-123 expired",
		"denied p@\x1bss+word",                          // split by an escape byte
		"denied p@\u009bss%2Bword",                      // split by a C1 control, then encoded
		"Password=P%40SS%2bword"[0:0] + "p%40ss%2bword", // lowercase hex
	}
	for _, in := range leaks {
		out := c.MaskCometText(in)
		for _, form := range []string{"p@ss+word", "p%40ss", "p%2540ss", "sk-123", "p@ss word"} {
			if strings.Contains(strings.ToLower(out), strings.ToLower(form)) {
				t.Fatalf("%q leaked through as %q", in, out)
			}
		}
	}
	spaced := &Client{Config: &config.Config{CometBackupPassword: "two words+@"}}
	if out := spaced.MaskCometText("x two%2Bwords%252B%2540 y"); strings.Contains(out, "two") {
		t.Fatalf("double query-encoded secret with a space leaked: %q", out)
	}
	if got := c.MaskCometText("Quota exceeded at 50% (code %zz)"); got != "Quota exceeded at 50% (code %zz)" {
		t.Fatalf("clean text changed: %q", got)
	}
	if got := lenientPercentDecode("a%41%zz%"); got != "aA%zz%" {
		t.Fatalf("lenient decode = %q", got)
	}
}

// TestCometTokenHint: an authentication failure, as an HTTP 403 or as a 2xx
// failure envelope, names the unsupported personal API token only when one
// is configured.
func TestCometTokenHint(t *testing.T) {
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/admin/list-users" {
			w.WriteHeader(http.StatusForbidden)
		}
		w.Write([]byte(`{"Status":403,"Message":"Invalid user account or password"}`))
	}))
	defer srv.Close()
	for secret, want := range map[string]bool{"bkup_0123456789abcdef": true, "ordinary-pw-1": false} {
		c := New(&config.Config{BaseURL: srv.URL, CometBackupUsername: "admin", CometBackupPassword: secret}, 5*time.Second, 0)
		c.NoCache = true
		for _, p := range []string{"/api/v1/admin/list-users", "/api/v1/admin/meta/stats"} {
			_, _, err := c.PostQueryFormWithParams(context.Background(), p, nil, url.Values{})
			if err == nil {
				t.Fatalf("%s: want an auth error", p)
			}
			if got := strings.Contains(err.Error(), "personal API tokens (bkup_...) are not supported"); got != want {
				t.Errorf("%s with %q: hint=%v, want %v (%v)", p, secret[:5], got, want, err)
			}
		}
	}
}

// Every field Comet authenticates with is a credential by name, so no request or
// preview walker can pass one by. Only the account name and the auth type are public.
func TestCometAuthFieldsAreCredentialNames(t *testing.T) {
	for _, f := range cometCredentialFields {
		if want := f != "Username" && f != "AuthType"; cliutil.SecretFieldName(f) != want {
			t.Errorf("SecretFieldName(%q) = %v, want %v", f, !want, want)
		}
	}
}
