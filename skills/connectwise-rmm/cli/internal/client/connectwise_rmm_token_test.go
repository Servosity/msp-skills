// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectwise-rmm-pp-cli/internal/config"
)

func TestNewAsioTokenRequestSendsJSONCredentials(t *testing.T) {
	req, err := NewAsioTokenRequest(context.Background(), "https://example.test/v1/token", "0e31abc", "s3cret", "platform.devices.read", "ua/1")
	if err != nil {
		t.Fatal(err)
	}
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if _, _, ok := req.BasicAuth(); ok {
		t.Fatal("token request must not use HTTP Basic; Asio reads credentials from the JSON body")
	}
	raw, _ := io.ReadAll(req.Body)
	var body map[string]string
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("body is not JSON: %v (%s)", err, raw)
	}
	want := map[string]string{"grant_type": "client_credentials", "client_id": "0e31abc", "client_secret": "s3cret", "scope": "platform.devices.read"}
	for k, v := range want {
		if body[k] != v {
			t.Errorf("%s = %q, want %q", k, body[k], v)
		}
	}
	req, _ = NewAsioTokenRequest(context.Background(), "https://example.test/v1/token", "id", "sec", "", "")
	raw, _ = io.ReadAll(req.Body)
	noScope := map[string]string{}
	_ = json.Unmarshal(raw, &noScope)
	if _, ok := noScope["scope"]; ok {
		t.Error("empty scope must be omitted")
	}
}

func TestRedactTokenBody(t *testing.T) {
	got := RedactTokenBody(`{"error":"bad","client_secret":"s3cret","access_token":"opaque-tok","echo":"s3cret"}`, "s3cret")
	for _, leak := range []string{"s3cret", "opaque-tok"} {
		if strings.Contains(got, leak) {
			t.Errorf("redacted body still contains %q: %s", leak, got)
		}
	}
	escaped := RedactTokenBody(`{"client_secret":"prefix\"SECRET-SUFFIX","echo":"prefix\"SECRET-SUFFIX"}`, `prefix"SECRET-SUFFIX`)
	if strings.Contains(escaped, "SECRET-SUFFIX") {
		t.Errorf("JSON-escaped secret leaked: %s", escaped)
	}
	if !strings.Contains(got, `"error":"bad"`) {
		t.Errorf("non-secret fields must survive: %s", got)
	}
}

func TestApplyAsioRequestHeaders(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://openapi.service.itsupport247.net/v1/rate_limits", nil)
	ApplyAsioRequestHeaders(req)
	if req.Header.Get("Origin") != "https://openapi.service.itsupport247.net" {
		t.Errorf("Origin = %q", req.Header.Get("Origin"))
	}
	id := req.Header.Get("X-Request-ID")
	if len(id) != 36 || strings.Count(id, "-") != 4 {
		t.Errorf("X-Request-ID = %q, want a UUID", id)
	}
	req.Header.Set("X-Request-ID", "caller")
	ApplyAsioRequestHeaders(req)
	if req.Header.Get("X-Request-ID") != "caller" {
		t.Error("an existing X-Request-ID must win")
	}
}

// TestDryRunNeverMintsAToken pins handfixes.json dry-run-never-mints: a
// --dry-run preview with client credentials and no cached token must not POST
// the client secret to the token URL.
func TestDryRunNeverMintsAToken(t *testing.T) {
	t.Setenv("CONNECTWISE_RMM_HOME", t.TempDir())
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = io.WriteString(w, `{"access_token":"t","expires_in":3600}`)
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, TokenURL: srv.URL + "/token", ConnectwiseRmmClientId: "id-1234567", ConnectwiseRmmClientSecret: "test-secret"}, 5*time.Second, 0)
	c.DryRun = true
	if _, err := c.Get(context.Background(), "/api/platform/v1/company/companies", nil); err != nil {
		t.Fatalf("dry-run Get: %v", err)
	}
	if hits != 0 {
		t.Fatalf("dry-run sent %d request(s); a preview must stay offline", hits)
	}
}
