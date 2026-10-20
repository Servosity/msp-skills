// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/saas-alerts/handfixes.json
// under the "webhook-subscription-token-withheld" entry.

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
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
