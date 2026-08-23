// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/servosity/handfixes.json under
// the "cross-host-redirect-strips-all-credentials" entry. See
// docs/reprint-survival.md.

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"servosity-msp-pp-cli/internal/config"
)

// TestCrossHostRedirectStripsEveryCredentialHeader proves the redirect gate
// drops more than Authorization. This connector sends X-Servosity-Mfa on the
// MFA-gated endpoints and copies arbitrary operator-configured headers from
// config.headers, and Go replays both to whatever host a 3xx names. Deleting
// only Authorization handed the second credential to that host.
func TestCrossHostRedirectStripsEveryCredentialHeader(t *testing.T) {
	var landed http.Header
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		landed = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer final.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL+"/landed", http.StatusFound)
	}))
	defer origin.Close()

	cfg := &config.Config{
		BaseURL:           origin.URL,
		ServosityMspToken: "partner-secret",
		Headers: map[string]string{
			"X-Api-Key":     "operator-api-key",
			"X-Auth-Token":  "operator-auth-token",
			"X-Tenant-Name": "acme",
		},
	}
	c := New(cfg, 10*time.Second, 0)

	_, err := c.GetWithHeaders(context.Background(), "/redirect-me", nil, map[string]string{
		"X-Servosity-Mfa": "123456",
	})
	if err != nil {
		t.Fatalf("GetWithHeaders() error = %v", err)
	}
	if landed == nil {
		t.Fatal("redirect target was never reached")
	}

	for _, name := range []string{"Authorization", "X-Servosity-Mfa", "X-Api-Key", "X-Auth-Token", "Cookie"} {
		if got := landed.Get(name); got != "" {
			t.Errorf("cross-host redirect carried %s = %q to a different host; every credential-classed header must be stripped", name, got)
		}
	}
	// A non-credential header still crosses, so the gate is a credential
	// filter and not a blanket header wipe.
	if got := landed.Get("X-Tenant-Name"); got != "acme" {
		t.Errorf("X-Tenant-Name = %q, want %q; the gate must only strip credential-classed headers", got, "acme")
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
