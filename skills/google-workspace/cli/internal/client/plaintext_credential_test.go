// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: covers credentialWouldTravelInPlaintext (handfixes.json
// no-credentials-over-plaintext-http).

package client

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"google-workspace-pp-cli/internal/config"
)

func TestCredentialWouldTravelInPlaintext(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://admin.googleapis.com/x": false,
		"http://127.0.0.1:18777/x":       false,
		"http://[::1]:80/x":              false,
		"http://localhost:8080/x":        false,
		"http://192.0.2.10/x":            true,
		"http://admin.googleapis.com/x":  true,
	} {
		u, _ := url.Parse(raw)
		if got := credentialWouldTravelInPlaintext(u); got != want {
			t.Errorf("%s: got %v want %v", raw, got, want)
		}
	}
}

type countingTransport struct{ calls int }

func (ct *countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	ct.calls++
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader("{}")), Request: r}, nil
}

// TestClientRefusesCredentialOverPlaintext pins the call site: the request
// must be refused before it reaches the transport, not just the helper.
func TestClientRefusesCredentialOverPlaintext(t *testing.T) {
	for base, wantRefused := range map[string]bool{"http://192.0.2.10": true, "http://127.0.0.1:18777": false} {
		ct := &countingTransport{}
		c := New(&config.Config{BaseURL: base, GoogleWorkspaceToken: "ya29.test"}, time.Second, 0)
		c.NoCache = true
		c.HTTPClient.Transport = ct
		_, err := c.Get(context.Background(), "/admin/directory/v1/users", nil)
		refused := err != nil && strings.Contains(err.Error(), "refusing to send credentials over plain HTTP")
		if refused != wantRefused || (wantRefused && ct.calls != 0) {
			t.Errorf("%s: refused=%v transport calls=%d err=%v", base, refused, ct.calls, err)
		}
	}
}
