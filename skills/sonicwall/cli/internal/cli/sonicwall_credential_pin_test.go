// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"testing"

	"sonicwall-pp-cli/internal/client"
)

// Each credential goes only to its own vendor host (handfix credential-host-pin),
// whatever path, redirect or base override produced the request.
func TestCredentialHostPin(t *testing.T) {
	const nsm = "https://nsm-uswest.sonicwall.com:443"
	cases := []struct {
		name, url, header, nsm string
		ok                     bool
	}{
		{"msw key to mysonicwall", "https://api.mysonicwall.com/api/hgms/get-cloud-tenants", "X-API-KEY", nsm, true},
		{"msw key rebased onto NSM", "https://nsm-uswest.sonicwall.com/api/hgms/get-cloud-tenants", "X-API-KEY", nsm, false},
		{"msw key to third host", "https://evil.example/api", "X-API-KEY", nsm, false},
		{"msw key over plain http", "http://api.mysonicwall.com/api/x", "X-API-KEY", nsm, false},
		{"msw key other port", "https://api.mysonicwall.com:8443/api/x", "X-API-KEY", nsm, false},
		{"nsm token to NSM", "https://nsm-uswest.sonicwall.com/api/manager/tenants", "Authorization", nsm, true},
		{"nsm token same-host redirect on mysonicwall", "https://api.mysonicwall.com/tenants", "Authorization", nsm, false},
		{"nsm token to other region", "https://nsm-eucentral.sonicwall.com/api/manager/tenants", "Authorization", nsm, false},
		{"nsm token with unresolved base", "https://nsm-uswest.sonicwall.com/api/manager/tenants", "Authorization", "", false},
		{"msw key to loopback mock base", "http://127.0.0.1:9999/api/x", "X-API-KEY", "http://127.0.0.1:9999", true},
		{"nsm token to loopback mock base", "http://127.0.0.1:9999/tenants", "Authorization", "http://127.0.0.1:9999", true},
		{"no credential anywhere", "https://evil.example/", "", nsm, true},
	}
	for _, tc := range cases {
		req, err := http.NewRequest(http.MethodGet, tc.url, nil)
		if err != nil {
			t.Fatal(err)
		}
		if tc.header != "" {
			req.Header.Set(tc.header, "secret-value")
		}
		if got := credentialHostAllowed(req, tc.nsm) == nil; got != tc.ok {
			t.Errorf("%s: allowed=%v, want %v", tc.name, got, tc.ok)
		}
	}
}

// The pin is the innermost transport every CLI and MCP client gets.
func TestCredentialHostPinIsWired(t *testing.T) {
	c := &client.Client{HTTPClient: &http.Client{}}
	if err := ApplyClientHooks(c); err != nil {
		t.Fatal(err)
	}
	env, ok := c.HTTPClient.Transport.(nsmEnvelopeTransport)
	if !ok {
		t.Fatalf("outer transport %T, want nsmEnvelopeTransport", c.HTTPClient.Transport)
	}
	red, ok := env.base.(sonicOSRedactTransport)
	if !ok {
		t.Fatalf("middle transport %T, want sonicOSRedactTransport", env.base)
	}
	if _, ok := red.base.(credentialPinTransport); !ok {
		t.Fatalf("inner transport %T, want credentialPinTransport", red.base)
	}
}
