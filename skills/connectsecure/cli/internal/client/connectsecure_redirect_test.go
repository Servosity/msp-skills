// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: the encoded client secret must not follow a cross-origin redirect.

package client

import (
	"context"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"connectsecure-pp-cli/internal/config"
)

// TestClientAuthTokenDroppedOnCrossOriginRedirect guards the hand-fix
// redirect-strips-client-auth-token: Client-Auth-Token is a reversible
// base64 of tenant + client ID + client secret, so a 307 to another origin
// must not carry it (Go only strips Authorization and Cookie by itself).
func TestClientAuthTokenDroppedOnCrossOriginRedirect(t *testing.T) {
	got := make(chan string, 1)
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get("Client-Auth-Token")
		_, _ = w.Write([]byte(`{"status":true,"data":{}}`))
	}))
	defer other.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://attacker.example"+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()

	c := New(&config.Config{BaseURL: origin.URL}, 5*time.Second, 0)
	c.NoCache = true
	// Route the off-origin hostname to the capture server; a hostname (not an
	// IP literal) is what a real attacker-controlled redirect would use.
	c.HTTPClient.Transport = &http.Transport{DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		if addr == "attacker.example:80" {
			addr = other.Listener.Addr().String()
		}
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}}
	_, _ = c.GetWithHeaders(context.Background(), "/r/company/companies", nil, map[string]string{"Client-Auth-Token": "SECRET"})
	select {
	case h := <-got:
		if h != "" {
			t.Fatalf("Client-Auth-Token followed a cross-origin redirect: %q", h)
		}
	default:
		t.Fatal("redirect was not followed; the test did not exercise the cross-origin hop")
	}
}

// TestWriteBodyNotReplayedCrossOrigin guards the hand-fix
// redirect-refuses-cross-origin-body: a 307/308 replays the request body, so
// a write carrying a password must not follow a redirect to another origin.
func TestWriteBodyNotReplayedCrossOrigin(t *testing.T) {
	got := make(chan string, 1)
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Method
		_, _ = w.Write([]byte(`{"status":true,"data":{}}`))
	}))
	defer other.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://attacker.example"+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()

	c := New(&config.Config{BaseURL: origin.URL}, 5*time.Second, 0)
	c.NoCache = true
	c.HTTPClient.Transport = &http.Transport{DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		if addr == "attacker.example:80" {
			addr = other.Listener.Addr().String()
		}
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}}
	_, _, err := c.PostWithParamsAndHeaders(context.Background(), "/r/company/scheduler", nil, map[string]any{"password": "SENTINEL"}, nil)
	select {
	case m := <-got:
		t.Fatalf("a %s body followed a cross-origin redirect", m)
	default:
	}
	if err == nil {
		t.Fatal("cross-origin body redirect returned no error")
	}
}

// TestWriteNotDowngradedCrossOrigin guards the same hand-fix for 301/302/303:
// Go rewrites the write into a bodiless GET before CheckRedirect runs, so the
// write never lands and must not be reported as a success.
func TestWriteNotDowngradedCrossOrigin(t *testing.T) {
	for _, code := range []int{http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther} {
		got := make(chan string, 1)
		other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got <- r.Method
			_, _ = w.Write([]byte(`{"status":true,"data":{"id":9}}`))
		}))
		origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://attacker.example"+r.URL.Path, code)
		}))
		c := New(&config.Config{BaseURL: origin.URL}, 5*time.Second, 0)
		c.NoCache = true
		c.HTTPClient.Transport = &http.Transport{DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if addr == "attacker.example:80" {
				addr = other.Listener.Addr().String()
			}
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		}}
		_, _, err := c.PostWithParamsAndHeaders(context.Background(), "/r/company/scheduler", nil, map[string]any{"password": "SENTINEL"}, nil)
		select {
		case m := <-got:
			t.Errorf("%d: a POST reached the other origin as %s", code, m)
		default:
		}
		if err == nil {
			t.Errorf("%d: a cross-origin write redirect returned no error", code)
		}
		other.Close()
		origin.Close()
	}
}

// TestWriteNotDowngradedSameOrigin guards the 301/302 half of the same
// hand-fix on one origin: the write is replayed as a GET, so it never landed.
// A 303 (the write was applied; fetch the result) is still followed.
func TestWriteNotDowngradedSameOrigin(t *testing.T) {
	for code, wantErr := range map[int]bool{http.StatusMovedPermanently: true, http.StatusFound: true, http.StatusSeeOther: false} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/landed" {
				_, _ = w.Write([]byte(`{"status":true,"data":{"id":9}}`))
				return
			}
			http.Redirect(w, r, "/landed", code)
		}))
		c := New(&config.Config{BaseURL: srv.URL}, 5*time.Second, 0)
		c.NoCache = true
		_, _, err := c.PostWithParamsAndHeaders(context.Background(), "/w/company/tags", nil, map[string]any{"name": "x"}, nil)
		if (err != nil) != wantErr {
			t.Errorf("%d: err = %v, want error %v", code, err, wantErr)
		}
		srv.Close()
	}
}

// TestErrorBodyWithheldWhenItEchoesASecret guards handfixes.json
// error-body-redacts-secrets: whatever form the server echoes a request
// secret in (raw, JSON-escaped, plain text, cut at the truncation point,
// a multipart key file, a short value), the error text never carries it.
func TestErrorBodyWithheldWhenItEchoesASecret(t *testing.T) {
	mode := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusBadRequest)
		switch mode {
		case "text":
			_, _ = w.Write([]byte("body was " + string(body)))
		case "straddle":
			_, _ = w.Write([]byte(strings.Repeat("x", 4090) + string(body)))
		default:
			_, _ = w.Write([]byte(`{"status":false,"message":"rejected ` + r.Header.Get("Client-Auth-Token") + `","echo":` + strconv.Quote(string(body)) + `}`))
		}
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL}, 5*time.Second, 0)
	c.NoCache = true
	keyFile := filepath.Join(t.TempDir(), "key.pem")
	if err := os.WriteFile(keyFile, []byte("-----BEGIN "+"PRIVATE KEY-----\nKEYSENTINEL\n-----END PRIVATE KEY-----\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		mode, name string
		secrets    []string
		call       func() error
	}{
		{"json", "html-escaped password and nested params", []string{"Pa&ss<w0rd>9", "PARAMSENTINEL", "CATSENTINEL"}, func() error {
			_, _, err := c.PostWithParamsAndHeaders(context.Background(), "/w/company/scheduler", nil,
				map[string]any{"name": "job", "password": "Pa&ss<w0rd>9", "params": map[string]any{"key": "PARAMSENTINEL"}},
				map[string]string{"Client-Auth-Token": "CATSENTINEL"})
			return err
		}},
		{"text", "quote in a plain-text echo", []string{`Tr0ub"leQ7x`, `Tr0ub\"leQ7x`}, func() error {
			_, _, err := c.PostWithParamsAndHeaders(context.Background(), "/w/company/scheduler", nil, map[string]any{"password": `Tr0ub"leQ7x`}, nil)
			return err
		}},
		{"json", "short password", []string{"ab1"}, func() error {
			_, _, err := c.PostWithParamsAndHeaders(context.Background(), "/w/company/scheduler", nil, map[string]any{"password": "ab1"}, nil)
			return err
		}},
		{"straddle", "secret cut by truncation", []string{"LONGPASSWORD_SENT"}, func() error {
			_, _, err := c.PostWithParamsAndHeaders(context.Background(), "/w/company/scheduler", nil, map[string]any{"password": "LONGPASSWORD_SENTINEL_XYZ"}, nil)
			return err
		}},
		{"text", "multipart secret field", []string{"MPPASSSENTINEL"}, func() error {
			_, _, err := c.PostMultipartWithHeaders(context.Background(), "/w/company/custom_domain", map[string]string{"password": "MPPASSSENTINEL"}, nil, nil)
			return err
		}},
		{"text", "multipart private key", []string{"KEYSENTINEL", "PRIVATE KEY"}, func() error {
			_, _, err := c.PostMultipartWithHeaders(context.Background(), "/w/company/custom_domain", map[string]string{"domain_name": "p.example"}, map[string]string{"ssl_privkey": keyFile}, nil)
			return err
		}},
	}
	for _, tc := range cases {
		mode = tc.mode
		err := tc.call()
		if err == nil {
			t.Fatalf("%s: want an API error", tc.name)
		}
		for _, sec := range tc.secrets {
			if strings.Contains(err.Error(), sec) {
				t.Errorf("%s: error text leaks %q: %v", tc.name, sec, err)
			}
		}
	}
}

// TestErrorBodyKeptWhenNothingEchoes: an error that carries no request secret
// stays readable, including boolean diagnostics under secret-like names.
func TestErrorBodyKeptWhenNothingEchoes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"status":false,"message":"name & tags < 3 required","token_expired":true}`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL}, 5*time.Second, 0)
	c.NoCache = true
	_, _, err := c.PostWithParamsAndHeaders(context.Background(), "/w/company/tags", nil, map[string]any{"name": "x"}, nil)
	if err == nil || !strings.Contains(err.Error(), "name & tags < 3 required") || !strings.Contains(err.Error(), `"token_expired":true`) {
		t.Fatalf("readable error body lost: %v", err)
	}
}

// TestWriteRedirectChainChecksTheLastHop: after a same-origin 303 the write
// landed and the follow-up is a plain GET, so a later 302 is not a failure.
func TestWriteRedirectChainChecksTheLastHop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/w/company/tags":
			http.Redirect(w, r, "/step1", http.StatusSeeOther)
		case "/step1":
			http.Redirect(w, r, "/landed", http.StatusFound)
		default:
			_, _ = w.Write([]byte(`{"status":true,"data":{"id":9}}`))
		}
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL}, 5*time.Second, 0)
	c.NoCache = true
	if _, _, err := c.PostWithParamsAndHeaders(context.Background(), "/w/company/tags", nil, map[string]any{"name": "x"}, nil); err != nil {
		t.Fatalf("303 then 302 after an applied write: %v", err)
	}
}

// TestErrorBodyWithheldForForeignEncoders drives the client with a Python-style
// echo (ensure_ascii): the display path decodes \u00e9, so the check must too.
func TestErrorBodyWithheldForForeignEncoders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"status": false, "message": "password 'Caf\u00e9\/Pa\u002Bss_SENT' rejected"}`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL}, 5*time.Second, 0)
	c.NoCache = true
	_, _, err := c.PostWithParamsAndHeaders(context.Background(), "/w/company/scheduler", nil, map[string]any{"password": "Café/Pa+ss_SENT"}, nil)
	if err == nil || strings.Contains(err.Error(), "Café/Pa+ss_SENT") || !strings.Contains(err.Error(), "withheld") {
		t.Fatalf("foreign-encoded echo not withheld: %v", err)
	}
}

// TestErrorBodyKeepsShortNonSecretParams: a port or type next to the password
// in params must not withhold an unrelated validation error.
func TestErrorBodyKeepsShortNonSecretParams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"status":false,"message":"name already exists","code":1022}`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL}, 5*time.Second, 0)
	c.NoCache = true
	_, _, err := c.PostWithParamsAndHeaders(context.Background(), "/r/company/credentials", nil,
		map[string]any{"name": "c1", "params": map[string]any{"port": 22, "type": "ssh", "username": "root", "password": "test-secret-long-enough"}}, nil)
	if err == nil || !strings.Contains(err.Error(), "name already exists") {
		t.Fatalf("validation error lost: %v", err)
	}
}

// TestDecodedClientAuthTokenIsASecret: a server that echoes the decoded
// tenant+client_id:client_secret must not print the client secret.
func TestDecodedClientAuthTokenIsASecret(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"bad credential acme+cid123:SESSIONSECRET_PLAIN_77"}`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL}, 5*time.Second, 0)
	c.NoCache = true
	cat := base64.StdEncoding.EncodeToString([]byte("acme+cid123:SESSIONSECRET_PLAIN_77"))
	_, _, err := c.PostWithParamsAndHeaders(context.Background(), "/w/authorize", nil, nil, map[string]string{"Client-Auth-Token": cat})
	if err == nil || strings.Contains(err.Error(), "SESSIONSECRET_PLAIN_77") {
		t.Fatalf("decoded credential leaked: %v", err)
	}
}

// TestWriteStatusFalseIsAnError guards handfixes.json write-status-false-is-an-error.
func TestWriteStatusFalseIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":false,"message":"password 'test-secret-200-false' does not meet the policy"}`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL}, 5*time.Second, 0)
	c.NoCache = true
	_, _, err := c.PostWithParamsAndHeaders(context.Background(), "/w/company/scheduler", nil, map[string]any{"password": "test-secret-200-false"}, nil)
	if err == nil {
		t.Fatal("a 200 status:false write was reported as success")
	}
	if strings.Contains(err.Error(), "test-secret-200-false") {
		t.Fatalf("the echoed password leaked: %v", err)
	}
	// A refused read is a failure too, never an empty result to print or cache.
	if raw, err := c.Get(context.Background(), "/r/company/companies", nil); err == nil {
		t.Fatalf("a 200 status:false GET was returned as data: %s", raw)
	}
}

// TestHTMLSuccessSummaryWithheldWhenItEchoes: a 2xx HTML page whose title
// echoes a request secret must not print it in the "returned HTML" error.
func TestHTMLSuccessSummaryWithheldWhenItEchoes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>Invalid value Pa&amp;ss_HTMLSENT</title></head><body>x</body></html>`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL}, 5*time.Second, 0)
	c.NoCache = true
	_, _, err := c.PostWithParamsAndHeaders(context.Background(), "/w/company/scheduler", nil, map[string]any{"password": "Pa&ss_HTMLSENT"}, nil)
	if err == nil || strings.Contains(err.Error(), "HTMLSENT") {
		t.Fatalf("HTML summary leaked the secret: %v", err)
	}
}

// TestWriteStatusFalseBehindBOMIsAnError: a BOM or XSSI prefix must not hide
// the failure envelope.
func TestWriteStatusFalseBehindBOMIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("\ufeff" + `{"status":false,"message":"refused"}`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL}, 5*time.Second, 0)
	c.NoCache = true
	if _, _, err := c.PostWithParamsAndHeaders(context.Background(), "/w/company/tags", nil, map[string]any{"name": "x"}, nil); err == nil {
		t.Fatal("a BOM-prefixed 200 status:false write was reported as success")
	}
}

// TestHTMLSummaryWithheldWhenItEchoesTheBearerToken: the CLI's own token is
// a secret too, on the 2xx HTML path as well as the error path.
func TestHTMLSummaryWithheldWhenItEchoesTheBearerToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>bad ` + r.Header.Get("Authorization") + `</title></head><body>x</body></html>`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, AccessToken: "test-token-sentinel-0001", ConnectsecureUserId: "1"}, 5*time.Second, 0)
	c.NoCache = true
	_, _, err := c.PostWithParamsAndHeaders(context.Background(), "/w/company/tags", nil, map[string]any{"name": "x"}, nil)
	if err == nil || strings.Contains(err.Error(), "test-token-sentinel") {
		t.Fatalf("HTML summary leaked the bearer token: %v", err)
	}
}

// TestErrorBodyParamsRule: a long value under params with an unknown key is
// withheld; a benign key (username) naming the account stays readable.
func TestErrorBodyParamsRule(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusBadRequest)
		if strings.Contains(string(body), "administrator") {
			_, _ = w.Write([]byte(`{"status":false,"message":"login failed for user administrator"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":false,"message":"bad key AKIAEXAMPLE123456"}`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL}, 5*time.Second, 0)
	c.NoCache = true
	_, _, err := c.PostWithParamsAndHeaders(context.Background(), "/w/company/integration_credentials", nil, map[string]any{"params": map[string]any{"access_key_id": "AKIAEXAMPLE123456"}}, nil)
	if err == nil || strings.Contains(err.Error(), "AKIAEXAMPLE123456") {
		t.Fatalf("unknown params value leaked: %v", err)
	}
	_, _, err = c.PostWithParamsAndHeaders(context.Background(), "/r/company/credentials", nil, map[string]any{"params": map[string]any{"username": "administrator", "password": "Xy9!kQ#2"}}, nil)
	if err == nil || !strings.Contains(err.Error(), "login failed for user administrator") {
		t.Fatalf("a benign username withheld the error: %v", err)
	}
}

// TestStatusFalseBehindBOMMasksOtherSecretFields: the field mask runs on the
// sanitized body, so a stored password in a conflict envelope stays masked.
func TestStatusFalseBehindBOMMasksOtherSecretFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("\ufeff" + `{"status":false,"message":"conflict","data":{"password":"STORED_OTHER_SENTINEL_77"}}`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL}, 5*time.Second, 0)
	c.NoCache = true
	_, _, err := c.PostWithParamsAndHeaders(context.Background(), "/w/company/scheduler", nil, map[string]any{"name": "j"}, nil)
	if err == nil || strings.Contains(err.Error(), "STORED_OTHER_SENTINEL_77") {
		t.Fatalf("stored password printed: %v", err)
	}
}

// TestParamsURLIsASecret: a webhook URL under params is the credential.
func TestParamsURLIsASecret(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"status":false,"message":"could not reach https://hooks.slack.com/services/T000/B000/SLACKSECRET9xyz"}`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL}, 5*time.Second, 0)
	c.NoCache = true
	_, _, err := c.PostWithParamsAndHeaders(context.Background(), "/w/integration/integration_credentials", nil, map[string]any{"params": map[string]any{"url": "https://hooks.slack.com/services/T000/B000/SLACKSECRET9xyz"}}, nil)
	if err == nil || strings.Contains(err.Error(), "SLACKSECRET9xyz") {
		t.Fatalf("webhook URL printed: %v", err)
	}
}

// TestGetStatusFalseEchoIsAnError: a 200 status:false GET that echoes the
// CLI's token is an error (never cached, never printed as data).
func TestGetStatusFalseEchoIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":false,"message":"session expired for ` + strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") + `"}`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, AccessToken: "test-token-sentinel-0001", ConnectsecureUserId: "1"}, 5*time.Second, 0)
	c.NoCache = true
	if raw, err := c.Get(context.Background(), "/r/company/companies", nil); err == nil || strings.Contains(string(raw)+err.Error(), "test-token-sentinel") {
		t.Fatalf("status:false echo returned as data or leaked: %s %v", raw, err)
	}
}

// TestConfiguredHeaderSecretIsWithheld: a credential set in the config
// file's headers is sent on every request, so an error echoing it is withheld
// like any other request secret.
func TestConfiguredHeaderSecretIsWithheld(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"bad key ` + r.Header.Get("X-Api-Key") + `"}`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, AccessToken: "test-token-0001", ConnectsecureUserId: "1", Headers: map[string]string{"X-Api-Key": "test-header-sentinel-0001"}}, 5*time.Second, 0)
	c.NoCache = true
	if _, err := c.Get(context.Background(), "/r/company/companies", nil); err == nil || strings.Contains(err.Error(), "test-header-sentinel") {
		t.Fatalf("configured header secret leaked: %v", err)
	}
	c.Config.Headers = map[string]string{"X-Trace": "test-header-sentinel-0001"} // not a credential header: stays readable
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"bad trace ` + r.Header.Get("X-Trace") + `"}`))
	}))
	defer srv2.Close()
	c.Config.BaseURL = srv2.URL
	c.BaseURL = srv2.URL
	if _, err := c.Get(context.Background(), "/r/company/companies", nil); err == nil || !strings.Contains(err.Error(), "test-header-sentinel") {
		t.Fatalf("non-credential header echo should stay readable: %v", err)
	}
}

// TestConfiguredSecretHeaderDroppedOnCrossOriginRedirect guards
// redirect-strips-client-auth-token for config-file headers: an X-Api-Key set
// in config.toml must not ride a redirect to another origin.
func TestConfiguredSecretHeaderDroppedOnCrossOriginRedirect(t *testing.T) {
	got := make(chan string, 1)
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get("X-Api-Key") + r.Header.Get("X-CS-Signature")
		_, _ = w.Write([]byte(`{"status":true,"data":{}}`))
	}))
	defer other.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://attacker.example"+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	// X-CS-Signature has no secret-looking name: every configured header stays home.
	c := New(&config.Config{BaseURL: origin.URL, Headers: map[string]string{"X-Api-Key": "HDRSECRET_42", "X-CS-Signature": "SIG_42"}}, 5*time.Second, 0)
	c.NoCache = true
	c.HTTPClient.Transport = &http.Transport{DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		if addr == "attacker.example:80" {
			addr = other.Listener.Addr().String()
		}
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}}
	_, _ = c.Get(context.Background(), "/r/company/companies", nil)
	select {
	case h := <-got:
		if h != "" {
			t.Fatalf("a configured header followed a cross-origin redirect: %q", h)
		}
	default:
		t.Fatal("redirect was not followed; the test did not exercise the cross-origin hop")
	}
}

// TestRedirectOntoCredentialPathRefused: a same-origin hop from an ordinary
// read onto a credential path would cache or store credential rows under the
// original resource, so it is refused.
func TestRedirectOntoCredentialPathRefused(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/r/company/companies" {
			http.Redirect(w, r, "/r/company/credentials", http.StatusFound)
			return
		}
		hit = true
		_, _ = w.Write([]byte(`{"status":true,"data":[{"id":1,"password":"CREDSENTINEL"}]}`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL}, 5*time.Second, 0)
	c.NoCache = true
	raw, err := c.Get(context.Background(), "/r/company/companies", nil)
	if err == nil || hit || strings.Contains(string(raw), "CREDSENTINEL") {
		t.Fatalf("redirect onto a credential path was followed: hit=%v err=%v body=%s", hit, err, raw)
	}
}

// TestConfiguredBearerTokenEchoWithheld: a configured "Authorization: Bearer
// <tok>" override contributes the bare token to the withhold set, so an
// error echoing only the token is withheld.
func TestConfiguredBearerTokenEchoWithheld(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"bad token ` + strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") + `"}`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, Headers: map[string]string{"Authorization": "Bearer zq81Kx"}}, 5*time.Second, 0)
	c.NoCache = true
	if _, err := c.Get(context.Background(), "/r/company/companies", nil); err == nil || strings.Contains(err.Error(), "zq81Kx") {
		t.Fatalf("bare bearer token leaked: %v", err)
	}
}

// TestRefusedRedirectDoesNotEchoTheSubmittedSecret: a refused redirect's
// error names its Location; a pod that puts the submitted password there must
// not get it printed (Codex round 19).
func TestRefusedRedirectDoesNotEchoTheSubmittedSecret(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://attacker.example/error?password=test-pw-sentinel-0001", http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	c := New(&config.Config{BaseURL: origin.URL}, 5*time.Second, 0)
	c.NoCache = true
	_, _, err := c.PostWithParamsAndHeaders(context.Background(), "/r/company/scheduler", nil, map[string]any{"name": "nightly", "password": "test-pw-sentinel-0001"}, nil)
	if err == nil || strings.Contains(err.Error(), "test-pw-sentinel") || !strings.Contains(err.Error(), "withheld") {
		t.Fatalf("want a withheld redirect error, got %v", err)
	}
}

// TestTransportErrorKeptWhenOnlyOurURLMatches: the request URL is built by the
// CLI, so a secret that happens to appear in it does not hide a DNS failure
// (adversarial round 21).
func TestTransportErrorKeptWhenOnlyOurURLMatches(t *testing.T) {
	c := New(&config.Config{BaseURL: "http://coincidence-sentinel-host.invalid"}, 5*time.Second, 0)
	c.NoCache = true
	_, _, err := c.PostWithParamsAndHeaders(context.Background(), "/r/company/scheduler", nil, map[string]any{"name": "nightly", "password": "coincidence-sentinel-host"}, nil)
	if err == nil || strings.Contains(err.Error(), "withheld") {
		t.Fatalf("want the real transport cause, got %v", err)
	}
}

// TestRedirectEchoOfSecretContainingTheHostIsWithheld: the host strip must not
// cut the host out of an echoed secret that contains it (adversarial round 22).
func TestRedirectEchoOfSecretContainingTheHostIsWithheld(t *testing.T) {
	var pw string
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/error?password="+pw, http.StatusFound)
	}))
	defer origin.Close()
	pw = "pw-" + strings.TrimPrefix(origin.URL, "http://") + "-tail"
	c := New(&config.Config{BaseURL: origin.URL}, 5*time.Second, 0)
	c.NoCache = true
	_, _, err := c.PostWithParamsAndHeaders(context.Background(), "/r/company/scheduler", nil, map[string]any{"name": "nightly", "password": pw}, nil)
	if err == nil || strings.Contains(err.Error(), pw) {
		t.Fatalf("want the echoed secret withheld, got %v", err)
	}
}
