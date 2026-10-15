// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: guards handfixes.json error-body-redacts-secrets,
// transport-error-withholds-echoed-secret and dry-run-redacts-secret-body
// (ported from connectsecure).

package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"watchguard-pp-cli/internal/config"
)

const bovpnPath = "/rest/firebox/management/v1/configuration/ACC-1/bovpn/ipsec/tunnels"

// Whatever form the server echoes a request secret in (raw, JSON-escaped,
// plain text, cut at the truncation point, a short value, the API key
// header), the error text never carries it.
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
		case "apikey":
			// A plain-text body carrying the key JSON-escaped (\u003c): not
			// JSON, so nothing re-encodes it, and the raw/URL-escape
			// credential masker does not recognise that form.
			b, _ := json.Marshal(r.Header.Get("WatchGuard-API-Key"))
			_, _ = w.Write([]byte("unknown key " + strings.Trim(string(b), `"`)))
		default:
			_, _ = w.Write([]byte(`{"error":"validation failed","echo":` + strconv.Quote(string(body)) + `}`))
		}
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, AccessToken: "tok-0001", WatchguardApiKey: "APIKEY<SENT>0001"}, 5*time.Second, 0)
	c.NoCache = true
	post := func(body map[string]any) func() error {
		return func() error {
			_, _, err := c.PostWithParamsAndHeaders(context.Background(), bovpnPath, nil, body, nil)
			return err
		}
	}
	cases := []struct {
		mode, name string
		secrets    []string
		call       func() error
	}{
		{"json", "html-escaped psk and nested private key", []string{"Pa&ss<psk>9", "PVTKEYSENTINEL"}, post(map[string]any{"name": "t1", "psk": "Pa&ss<psk>9", "certificate": map[string]any{"pvt_key": "PVTKEYSENTINEL"}})},
		{"text", "quote in a plain-text echo", []string{`Tr0ub"leQ7x`, `Tr0ub\"leQ7x`}, post(map[string]any{"password": `Tr0ub"leQ7x`})},
		{"json", "short otp", []string{"481"}, post(map[string]any{"otp": "481"})},
		{"straddle", "secret cut by truncation", []string{"LONGPASSPHRASE_SENT"}, post(map[string]any{"passphrase": "LONGPASSPHRASE_SENTINEL_XYZ"})},
		{"json", "activation key", []string{"ACTKEYSENTINEL"}, post(map[string]any{"activationKey": "ACTKEYSENTINEL"})},
		{"apikey", "JSON-escaped WatchGuard-API-Key header echo", []string{"APIKEY<SENT>0001", `APIKEY\u003cSENT`, `SENT\u003e0001`}, func() error {
			_, err := c.Get(context.Background(), "/rest/probe", nil)
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

// An error that carries no request secret stays readable, including boolean
// diagnostics under secret-like names; a secret-named field the server added
// on its own is masked, not printed.
func TestErrorBodyKeptWhenNothingEchoes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"name & tags < 3 required","token_expired":true,"psk":"SERVERSIDEPSK"}`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, AccessToken: "tok-0001"}, 5*time.Second, 0)
	c.NoCache = true
	_, _, err := c.PostWithParamsAndHeaders(context.Background(), "/rest/firebox/management/v1/configuration/ACC-1/aliases", nil, map[string]any{"name": "x"}, nil)
	if err == nil || !strings.Contains(err.Error(), "name & tags < 3 required") || !strings.Contains(err.Error(), `"token_expired":true`) {
		t.Fatalf("readable error body lost: %v", err)
	}
	if strings.Contains(err.Error(), "SERVERSIDEPSK") || !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("a secret-named field in the error body must be masked: %v", err)
	}
}

// A 2xx HTML page whose title echoes a request secret must not print it in
// the "returned HTML" error; the CLI's own bearer token counts too.
func TestHTMLSuccessSummaryWithheldWhenItEchoes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`<html><head><title>bad ` + r.Header.Get("Authorization") + `</title></head><body>x</body></html>`))
			return
		}
		_, _ = w.Write([]byte(`<html><head><title>Invalid value Pa&amp;ss_HTMLSENT</title></head><body>x</body></html>`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, AccessToken: "test-token-sentinel-0001"}, 5*time.Second, 0)
	c.NoCache = true
	_, _, err := c.PostWithParamsAndHeaders(context.Background(), bovpnPath, nil, map[string]any{"psk": "Pa&ss_HTMLSENT"}, nil)
	if err == nil || strings.Contains(err.Error(), "HTMLSENT") {
		t.Fatalf("HTML summary leaked the request secret: %v", err)
	}
	if _, err := c.Get(context.Background(), "/rest/probe", nil); err == nil || strings.Contains(err.Error(), "test-token-sentinel") {
		t.Fatalf("HTML summary leaked the bearer token: %v", err)
	}
}

// A refused redirect's error names its Location; a server that puts the
// submitted pre-shared key there must not get it printed, whether the hop
// is refused for leaving the origin with a body, for a private address, or
// for downgrading a write on the same origin.
func TestRefusedRedirectDoesNotEchoTheSubmittedSecret(t *testing.T) {
	var target string
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	for name, loc := range map[string]string{
		"cross-origin":    "http://attacker.example/error?psk=test-psk-sentinel-0001",
		"private address": "http://10.255.255.1/error?psk=test-psk-sentinel-0001",
	} {
		target = loc
		c := New(&config.Config{BaseURL: origin.URL, AccessToken: "tok-0001"}, 5*time.Second, 0)
		c.NoCache = true
		_, _, err := c.PostWithParamsAndHeaders(context.Background(), bovpnPath, nil, map[string]any{"name": "t1", "psk": "test-psk-sentinel-0001"}, nil)
		if err == nil || strings.Contains(err.Error(), "test-psk-sentinel") || !strings.Contains(err.Error(), "withheld") {
			t.Fatalf("%s: want a withheld redirect error, got %v", name, err)
		}
	}
}

// The host strip must not cut the host out of an echoed secret that contains
// it.
func TestRedirectEchoOfSecretContainingTheHostIsWithheld(t *testing.T) {
	var pw string
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/error?password="+pw, http.StatusFound)
	}))
	defer origin.Close()
	pw = "pw-" + strings.TrimPrefix(origin.URL, "http://") + "-tail"
	c := New(&config.Config{BaseURL: origin.URL, AccessToken: "tok-0001"}, 5*time.Second, 0)
	c.NoCache = true
	_, _, err := c.PostWithParamsAndHeaders(context.Background(), "/rest/authpoint/authentication/v1/accounts/ACC-1/resources/r1/transactions", nil, map[string]any{"login": "u", "password": pw}, nil)
	if err == nil || strings.Contains(err.Error(), pw) {
		t.Fatalf("want the echoed secret withheld, got %v", err)
	}
}

// The request URL is built by the CLI, so a secret that happens to appear in
// it does not hide a DNS failure.
func TestTransportErrorKeptWhenOnlyOurURLMatches(t *testing.T) {
	c := New(&config.Config{BaseURL: "http://coincidence-sentinel-host.invalid", AccessToken: "tok-0001"}, 5*time.Second, 0)
	c.NoCache = true
	_, _, err := c.PostWithParamsAndHeaders(context.Background(), bovpnPath, nil, map[string]any{"name": "t1", "psk": "coincidence-sentinel-host"}, nil)
	if err == nil || strings.Contains(err.Error(), "withheld") {
		t.Fatalf("want the real transport cause, got %v", err)
	}
}

// --dry-run masks secret-named body fields before printing the preview.
func TestDryRunRedactsSecretBody(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	c := New(&config.Config{BaseURL: "http://127.0.0.1:1"}, 5*time.Second, 0)
	c.DryRun = true
	body := map[string]any{"name": "t1", "psk": "SENTINEL-PSK", "certificate": map[string]any{"pvt_key": "SENTINEL-PVT", "password": "SENTINEL-PW"}, "pskEnabled": true}
	_, _, _ = c.PostWithParamsAndHeaders(context.Background(), bovpnPath, nil, body, nil)
	os.Stderr = orig
	_ = w.Close()
	out, _ := io.ReadAll(r)
	if strings.Contains(string(out), "SENTINEL") {
		t.Fatalf("dry run echoed a secret:\n%s", out)
	}
	if !strings.Contains(string(out), `"t1"`) || !strings.Contains(string(out), "[redacted]") || !strings.Contains(string(out), "true") {
		t.Fatalf("dry run lost the non-secret preview:\n%s", out)
	}
}
