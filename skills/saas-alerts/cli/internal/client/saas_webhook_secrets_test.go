// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/saas-alerts/handfixes.json
// under the "webhook-subscription-token-withheld" entry.

package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"saas-alerts-pp-cli/internal/config"
)

func TestWithholdWebhookTokensBlanksEveryToken(t *testing.T) {
	in := `[{"channelId":"ch-1","url":"https://hooks.example.com/a?x=1&y=2","token":"test-secret-1"},{"channelId":"ch-2","nested":{"Token":"test-secret-2"},"token":null}]`
	out := string(withholdWebhookTokens("/outgoingWebhookApi/api/v1/subscriptions", []byte(in)))
	if strings.Contains(out, "test-secret") {
		t.Fatalf("token survived: %s", out)
	}
	if strings.Count(out, WebhookTokenWithheld) != 2 {
		t.Fatalf("want 2 withheld markers: %s", out)
	}
	if !strings.Contains(out, "https://hooks.example.com/a?x=1&y=2") {
		t.Fatalf("url was rewritten: %s", out)
	}
}

func TestWithholdWebhookTokensLeavesOtherAPIsAlone(t *testing.T) {
	in := `{"token":"keep-me"}`
	if out := string(withholdWebhookTokens("/reportApi/api/v1/customers", []byte(in))); out != in {
		t.Fatalf("non-webhook body changed: %s", out)
	}
	if out := string(withholdWebhookTokens("/outgoingWebhookApi/api/v1/subscriptions", []byte("not json"))); out != "not json" {
		t.Fatalf("non-JSON body changed: %s", out)
	}
}

// TestWebhookTokenNeverLeavesTheClient drives the real request path: the
// response a caller receives (and so the cache, sync and MCP) has no token.
func TestWebhookTokenNeverLeavesTheClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"channelId":"ch-1","token":"test-secret-wire"}]`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, AuthHeaderVal: "test-key"}, 5*time.Second, 0)
	c.NoCache = true
	data, err := c.Get(context.Background(), "/outgoingWebhookApi/api/v1/subscriptions", nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "test-secret-wire") || !strings.Contains(string(data), WebhookTokenWithheld) {
		t.Fatalf("token reached the caller: %s", data)
	}
}

// TestWebhookTokenWithheldFromDryRunPreview: the --dry-run request preview of a
// subscription write prints [withheld], not the delivery token.
func TestWebhookTokenWithheldFromDryRunPreview(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stderr
	os.Stderr = w
	c := New(&config.Config{BaseURL: "http://127.0.0.1:1", AuthHeaderVal: "test-key"}, time.Second, 0)
	_, _, derr := c.dryRun("POST", "http://127.0.0.1:1/outgoingWebhookApi/api/v1/subscriptions", "/outgoingWebhookApi/api/v1/subscriptions", nil, []byte(`{"url":"https://hooks.example.com/x","token":"test-secret-preview"}`), nil, "test-key")
	w.Close()
	os.Stderr = saved
	if derr != nil {
		t.Fatal(derr)
	}
	out, _ := io.ReadAll(r)
	if strings.Contains(string(out), "test-secret-preview") || !strings.Contains(string(out), WebhookTokenWithheld) {
		t.Fatalf("dry-run preview leaked the token: %s", out)
	}
}

// TestWithheldPlaceholderIsNeverWritten: copying a listed subscription (token
// [withheld]) into an update must not overwrite the real token.
func TestWithheldPlaceholderIsNeverWritten(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, AuthHeaderVal: "test-key"}, 5*time.Second, 0)
	_, _, err := c.Patch(context.Background(), "/outgoingWebhookApi/api/v1/subscriptions/ch-1", map[string]any{"enabled": true, "token": WebhookTokenWithheld})
	if err == nil || hits != 0 {
		t.Fatalf("placeholder token sent: err=%v hits=%d", err, hits)
	}
	if _, _, err := c.Patch(context.Background(), "/outgoingWebhookApi/api/v1/subscriptions/ch-1", map[string]any{"enabled": true}); err != nil || hits != 1 {
		t.Fatalf("an update without token must pass: err=%v hits=%d", err, hits)
	}
}

// TestWebhookTokenWithheldFromErrorBodies: an error answer that echoes a
// subscription token does not carry it into the returned error.
func TestWebhookTokenWithheldFromErrorBodies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"exists","existing":{"channelId":"ch-1","token":"test-secret-error"}}`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, AuthHeaderVal: "test-key"}, 5*time.Second, 0)
	c.NoCache = true
	_, err := c.Get(context.Background(), "/outgoingWebhookApi/api/v1/subscriptions", nil)
	if err == nil || strings.Contains(err.Error(), "test-secret-error") || !strings.Contains(err.Error(), WebhookTokenWithheld) {
		t.Fatalf("token reached the error: %v", err)
	}
}
