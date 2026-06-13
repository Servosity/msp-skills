// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0.
package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"autotask-pp-cli/internal/config"
)

// TestAllThreeAutotaskHeadersSent pins the fix for the generator's
// multi-apiKey-scheme bug: Autotask requires ApiIntegrationCode, UserName, and
// Secret on EVERY request. The generated client originally blanked UserName and
// Secret immediately after setting them, so only ApiIntegrationCode reached the
// wire and every authenticated call would 401. This test fails if those reset
// lines ever return.
func TestAllThreeAutotaskHeadersSent(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	cfg := &config.Config{
		BaseURL:                       srv.URL,
		AutotaskApiIntegrationCode: "integration-code-123",
		AutotaskPsaUserName:           "api-user@example.com",
		AutotaskPsaSecret:             "secret-456",
	}
	c := New(cfg, 5*time.Second, 0)
	if _, err := c.Get(context.Background(), "/Companies/query", nil); err != nil {
		t.Fatalf("Get: %v", err)
	}

	checks := map[string]string{
		"ApiIntegrationCode": "integration-code-123",
		"UserName":           "api-user@example.com",
		"Secret":             "secret-456",
	}
	for header, want := range checks {
		if g := got.Get(header); g != want {
			t.Errorf("header %s = %q, want %q (all three Autotask headers must reach the wire)", header, g, want)
		}
	}
}

// TestCrossHostRedirectStripsAutotaskHeaders pins the redirect-leak fix: on a
// cross-host 3xx the client must strip ALL THREE custom Autotask credential
// headers, not just ApiIntegrationCode. Go's automatic redirect stripping only
// covers standard headers (Authorization, Cookie), so without the explicit
// deletes a compromised/MITM'd zone host could 302 the client to an attacker
// host and harvest the full UserName+Secret pair.
func TestCrossHostRedirectStripsAutotaskHeaders(t *testing.T) {
	var leaked http.Header
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer attacker.Close()

	// Origin redirects to the attacker host. httptest binds 127.0.0.1:port per
	// server, so the two hosts differ by port — enough for the same-host gate.
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, attacker.URL+r.URL.Path, http.StatusFound)
	}))
	defer origin.Close()

	cfg := &config.Config{
		BaseURL:                       origin.URL,
		AutotaskApiIntegrationCode: "integration-code-123",
		AutotaskPsaUserName:           "api-user@example.com",
		AutotaskPsaSecret:             "secret-456",
	}
	c := New(cfg, 5*time.Second, 0)
	if _, err := c.Get(context.Background(), "/Companies/query", nil); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if leaked == nil {
		t.Fatal("redirect target never received the request")
	}
	for _, header := range []string{"ApiIntegrationCode", "UserName", "Secret"} {
		if g := leaked.Get(header); g != "" {
			t.Errorf("cross-host redirect leaked %s=%q; credential headers must be stripped on cross-host hops", header, g)
		}
	}
}
