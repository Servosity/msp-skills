// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written regression test for the clientid-secret-containment hand-fix.
// See skills/datagate/handfixes.json.

package client

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"datagate-cli/internal/config"
)

// DataGate's ClientId is a vendor-designated secret carried in the generic
// Headers map. Go replays custom headers verbatim across a redirect, so the
// CheckRedirect guard has to drop the whole injected set. Header-name keyword
// rules do not catch "ClientId", which is exactly why this is asserted.
func TestCheckRedirectStripsInjectedHeadersOffOrigin(t *testing.T) {
	const secret = "11111111-2222-3333-4444-555555555555"

	newReq := func(raw string) *http.Request {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("parse %s: %v", raw, err)
		}
		return &http.Request{URL: u, Header: http.Header{
			"Authorization": []string{"Bearer tok"},
			"Clientid":      []string{secret},
		}}
	}

	cfg := &config.Config{
		BaseURL: "https://api.dgportal.net",
		Headers: map[string]string{"ClientId": secret},
	}
	c := New(cfg, 5*time.Second, 0)

	for _, tc := range []struct {
		name       string
		next, from string
		wantStrip  bool
	}{
		{"foreign host", "https://evil.example/x", "https://api.dgportal.net/a", true},
		{"protocol downgrade", "http://api.dgportal.net/x", "https://api.dgportal.net/a", true},
		{"same origin", "https://api.dgportal.net/x", "https://api.dgportal.net/a", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := newReq(tc.next)
			via := []*http.Request{newReq(tc.from)}
			if err := c.HTTPClient.CheckRedirect(req, via); err != nil {
				t.Fatalf("CheckRedirect: %v", err)
			}
			got := req.Header.Get("ClientId")
			if tc.wantStrip && got != "" {
				t.Fatalf("ClientId secret leaked to %s: %q", tc.next, got)
			}
			if !tc.wantStrip && got != secret {
				t.Fatalf("ClientId dropped on a same-origin hop: %q", got)
			}
			if tc.wantStrip && req.Header.Get("Authorization") != "" {
				t.Fatal("Authorization survived an off-origin hop")
			}
		})
	}
}

// The secret must not survive into any rendered output.
func TestMaskRedactsInjectedHeaderValues(t *testing.T) {
	const secret = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	c := New(&config.Config{
		BaseURL: "https://api.dgportal.net",
		Headers: map[string]string{"ClientId": secret},
	}, 5*time.Second, 0)

	body := `{"error":"bad clientId ` + secret + `"}`
	if got := c.maskCredentialText(body); strings.Contains(got, secret) {
		t.Fatalf("ClientId echoed unmasked: %q", got)
	}
}
