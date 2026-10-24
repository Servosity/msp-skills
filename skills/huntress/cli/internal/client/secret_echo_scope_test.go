// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/huntress/handfixes.json under
// the "secret-echo-decoded-funnel" entry. The shared funnel withholds whole
// credential values in error text; these tests pin the two widenings made at
// this connector's call sites: a PARTIAL echo of a credential, and an echo
// inside a SUCCESSFUL response body (which reaches stdout, MCP tool results
// and the local store).

package client

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"huntress-pp-cli/internal/config"
)

const (
	echoTestKey    = "test-key-partial-echo-check"
	echoTestSecret = "abcdefghijklmnopqrstuvwx"
)

func echoTestClient(t *testing.T, status int, body string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c := New(&config.Config{BaseURL: srv.URL, HuntressApiKey: echoTestKey, HuntressApiSecret: echoTestSecret}, time.Second, 0)
	c.HTTPClient = srv.Client()
	c.NoCache = true
	return c
}

// A server that echoes part of the secret (here its first eight characters)
// must not get that fragment into the error the CLI or MCP server prints.
func TestPartialCredentialEchoIsWithheldFromErrors(t *testing.T) {
	for name, fragment := range map[string]string{
		"prefix":        echoTestSecret[:8],
		"middle":        echoTestSecret[8:18],
		"base64-prefix": base64.StdEncoding.EncodeToString([]byte(echoTestSecret[:12])),
		"key-fragment":  echoTestKey[3:14],
	} {
		t.Run(name, func(t *testing.T) {
			c := echoTestClient(t, http.StatusBadRequest, `{"error":"rejected `+fragment+`"}`)
			_, err := c.Get(context.Background(), "/v1/account", nil)
			if err == nil {
				t.Fatal("want an error for HTTP 400")
			}
			if strings.Contains(err.Error(), fragment) {
				t.Fatalf("error repeats a credential fragment %q: %v", fragment, err)
			}
		})
	}
}

// A successful body that carries a credential, in any encoding the funnel
// decodes, must not be returned with it: it would reach stdout, an MCP tool
// result, and the local store.
func TestCredentialEchoIsWithheldFromSuccessBodies(t *testing.T) {
	for name, echo := range map[string]string{
		"base64-secret": base64.StdEncoding.EncodeToString([]byte(echoTestSecret)),
		"raw-secret":    echoTestSecret,
		"partial":       echoTestSecret[4:14],
	} {
		t.Run(name, func(t *testing.T) {
			c := echoTestClient(t, http.StatusOK, `{"accounts":[{"id":7,"name":"`+echo+`","status":"active"}],"pagination":{}}`)
			got, err := c.Get(context.Background(), "/v1/accounts", nil)
			if err == nil {
				t.Fatalf("success body with a credential echo was returned: %s", got)
			}
			if strings.Contains(err.Error(), echo) {
				t.Fatalf("the refusal repeats the echo: %v", err)
			}
		})
	}
	// Control: an ordinary body comes back byte-identical.
	plain := `{"accounts":[{"id":7,"name":"Acme Dental","status":"active"}],"pagination":{}}`
	c := echoTestClient(t, http.StatusOK, plain)
	got, err := c.Get(context.Background(), "/v1/accounts", nil)
	if err != nil || string(got) != plain {
		t.Fatalf("ordinary body changed: %s (%v)", got, err)
	}
}

// A credential match never rewrites a successful answer: replacing values
// with one shared notice would collapse distinct ids (two agents stored as
// one row). The whole answer is refused instead, and nothing is stored.
func TestSuccessBodyIsNeverRewritten(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"agents":[{"id":12345678},{"id":23456789}],"pagination":{}}`))
	}))
	t.Cleanup(srv.Close)
	c := New(&config.Config{BaseURL: srv.URL, HuntressApiKey: "hk_numeric_key_x", HuntressApiSecret: "1234567890123456"}, time.Second, 0)
	c.HTTPClient = srv.Client()
	c.NoCache = true
	got, err := c.Get(context.Background(), "/v1/agents", nil)
	if err == nil && string(got) != `{"agents":[{"id":12345678},{"id":23456789}],"pagination":{}}` {
		t.Fatalf("a successful answer was rewritten instead of returned whole or refused: %s", got)
	}
}
