// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/servosity/handfixes.json under
// the "cross-host-redirect-strips-all-credentials" entry. See
// docs/reprint-survival.md.

package client

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"servosity-msp-pp-cli/internal/config"
)

// TestCrossHostRedirectStripsEveryCredentialHeader proves the redirect gate
// drops more than Authorization. This connector sends X-Servosity-Mfa on the
// MFA-gated endpoints and copies arbitrary operator-configured headers from
// config.headers, and Go replays both to whatever host a 3xx names. Deleting
// only Authorization handed the second credential to that host.
//
// It drives the client's own CheckRedirect with the headers Go has already
// copied onto the next hop. An end-to-end httptest pair cannot be used: both
// servers listen on loopback, and the private-address redirect guard refuses
// that hop before any header policy runs. 203.0.113.10 (TEST-NET-3) is an
// off-origin public literal, so the guard passes it without a DNS lookup.
func TestCrossHostRedirectStripsEveryCredentialHeader(t *testing.T) {
	cfg := &config.Config{BaseURL: "https://api.servosity.com/api/v1", ServosityMspToken: "partner-secret"}
	c := New(cfg, 10*time.Second, 0)
	if c.HTTPClient == nil || c.HTTPClient.CheckRedirect == nil {
		t.Fatal("New did not install a CheckRedirect policy")
	}

	origin, err := http.NewRequest(http.MethodGet, "https://api.servosity.com/api/v1/redirect-me", nil)
	if err != nil {
		t.Fatal(err)
	}
	next, err := http.NewRequest(http.MethodGet, "https://203.0.113.10/landed", nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{
		"Authorization":   "Token partner-secret",
		"X-Servosity-Mfa": "123456",
		"X-Api-Key":       "operator-api-key",
		"X-Auth-Token":    "operator-auth-token",
		"Cookie":          "session=abc",
		"X-AuthToken":     "proxy-secret-abcdef",
		"X-Tenant-Name":   "acme",
		"Accept":          "application/json",
		"User-Agent":      "servosity-cli",
	} {
		next.Header.Set(name, value)
	}
	if err := c.HTTPClient.CheckRedirect(next, []*http.Request{origin}); err != nil {
		t.Fatalf("CheckRedirect() error = %v", err)
	}

	// Every header outside the content-negotiation allowlist is dropped: a
	// credential can hide under any name (X-AuthToken has no separator), and
	// operator config.headers are not meant for another origin either.
	for _, name := range []string{"Authorization", "X-Servosity-Mfa", "X-Api-Key", "X-Auth-Token", "Cookie", "X-AuthToken", "X-Tenant-Name"} {
		if got := next.Header.Get(name); got != "" {
			t.Errorf("cross-host redirect carried %s = %q to a different host", name, got)
		}
	}
	for name, want := range map[string]string{"Accept": "application/json", "User-Agent": "servosity-cli"} {
		if got := next.Header.Get(name); got != want {
			t.Errorf("%s = %q, want %q; content-negotiation headers may cross", name, got, want)
		}
	}
}

// TestSameHostRedirectKeepsAuthorization pins the other side: an in-host
// redirect must still authenticate, or every paginated or trailing-slash
// redirect the API issues would start returning 401.
func TestSameHostRedirectKeepsAuthorization(t *testing.T) {
	var landed http.Header
	mux := http.NewServeMux()
	mux.HandleFunc("/redirect-me", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/landed", http.StatusFound)
	})
	mux.HandleFunc("/landed", func(w http.ResponseWriter, r *http.Request) {
		landed = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := &config.Config{BaseURL: srv.URL, ServosityMspToken: "partner-secret"}
	c := New(cfg, 10*time.Second, 0)

	if _, err := c.Get(context.Background(), "/redirect-me", nil); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if landed == nil {
		t.Fatal("redirect target was never reached")
	}
	if got := landed.Get("Authorization"); got == "" {
		t.Fatal("same-host redirect lost the Authorization header; every in-host redirect would 401")
	}
}

// TestIsCredentialHeader documents the classification rule directly, including
// the names that must keep crossing.
func TestIsCredentialHeader(t *testing.T) {
	credential := []string{
		"Authorization", "authorization", "Proxy-Authorization", "Cookie",
		"X-Servosity-Mfa", "X-Api-Key", "X-API-KEY", "X-Auth-Token",
		"X-Client-Secret", "X-Session-Id", "X-Csrf-Token", "X-Amz-Signature",
		"X-Hub-Signature-256", "X_Access_Token", "X-Otp-Code",
		"X-AuthToken", "X-Apikey", "X-Sessionid", "X-Clientsecret",
	}
	for _, name := range credential {
		if !isCredentialHeader(name) {
			t.Errorf("isCredentialHeader(%q) = false, want true", name)
		}
	}
	benign := []string{
		"Accept", "Content-Type", "Content-Length", "User-Agent", "Accept-Encoding",
		"X-Request-Id", "X-Tenant-Name", "X-Api-Version", "If-None-Match",
		"X-Assignee", "X-Correlation-Id", "X-Page-Size",
	}
	for _, name := range benign {
		if isCredentialHeader(name) {
			t.Errorf("isCredentialHeader(%q) = true, want false; stripping it would break legitimate cross-host handoffs", name)
		}
	}
}

// TestErrorBodyMasksAnEchoedMFAHeader pins that a credential sent as a
// per-request header (X-Servosity-Mfa on the MFA-gated endpoints) is masked
// when the server echoes it in an error, like the token is.
func TestErrorBodyMasksAnEchoedMFAHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		mfa := r.Header.Get("X-Servosity-Mfa")
		_, _ = w.Write([]byte(`{"detail":"Invalid MFA code: ` + mfa + ` (` + base64.StdEncoding.EncodeToString([]byte(mfa)) + `)"}`))
	}))
	defer srv.Close()

	c := New(&config.Config{BaseURL: srv.URL, ServosityMspToken: "partner-secret"}, 10*time.Second, 0)
	c.NoCache = true
	_, err := c.GetWithHeaders(context.Background(), "/backups/17/encryption-key/", nil, map[string]string{"X-Servosity-Mfa": "123456"})
	if err == nil {
		t.Fatal("want the 403 as an error")
	}
	if strings.Contains(err.Error(), "123456") || strings.Contains(err.Error(), "MTIzNDU2") {
		t.Fatalf("error echoes the MFA code: %v", err)
	}
}
