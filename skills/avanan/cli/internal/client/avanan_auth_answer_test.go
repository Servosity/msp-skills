// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): regression guards for handfixes.json entry
// "credkit-wiring" (class secret-echo).

package client

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestAuthAnswerIsHeldBeforeItIsParsed: a token the handshake returns is held
// the moment the answer arrives, before any parsing that could fail and quote
// it, and no handshake error carries answer text.
func TestAuthAnswerIsHeldBeforeItIsParsed(t *testing.T) {
	for _, tc := range []struct{ name, base, path, body, token string }{
		{"infinity, unparseable expiresIn (codex input)", "https://cloudinfra-gw.portal.checkpoint.com", "/v2/auth/external",
			`{"data":{"token":"nb-infinity-sess-111","expiresIn":"nb-infinity-sess-111"}}`, "nb-infinity-sess-111"},
		{"legacy, failed envelope", "https://smart-api-production-1-us.avanan.net", "/v1.0/auth",
			`{"responseEnvelope":{"responseCode":401},"responseData":"nb-legacy-sess-222"}`, "nb-legacy-sess-222"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := &handshakeRecorder{reply: func(r *http.Request) *http.Response {
				return textResponse(r, http.StatusOK, tc.body, http.Header{"Content-Type": {"application/json"}})
			}}
			tr, _ := newTestTransport(t, rt, tc.base)
			req, _ := http.NewRequest(http.MethodGet, tc.base+"/v1.0/scopes", nil)
			_, _, err := tr.Mint(req)
			if err == nil {
				t.Fatal("the handshake accepted a malformed answer")
			}
			if strings.Contains(err.Error(), tc.token) {
				t.Errorf("the handshake error quotes the answer: %v", err)
			}
			if !funnelHolds(tc.token) {
				t.Errorf("the token in the answer was not held")
			}
		})
	}
}

// TestDataRequestsNeverReachTheAuthEndpoint: only the handshake's own client
// talks to the auth endpoints. A data request to one, directly or through a
// same-host redirect, is refused, so a freshly issued token never comes back
// as data.
func TestDataRequestsNeverReachTheAuthEndpoint(t *testing.T) {
	const base = "https://smart-api-production-1-us.avanan.net"
	const fresh = "nb-redirect-issued-333"
	auths := 0
	rt := &handshakeRecorder{reply: func(r *http.Request) *http.Response {
		switch {
		case r.URL.Path == "/v1.0/auth" && auths == 0:
			auths++
			return textResponse(r, http.StatusOK, "nb-handshake-sess-444", nil)
		case r.URL.Path == "/v1.0/auth":
			return textResponse(r, http.StatusOK, fresh, nil)
		case r.URL.Path == "/v1.0/scopes":
			return textResponse(r, http.StatusFound, "", http.Header{"Location": {"/v1.0/auth"}})
		}
		return textResponse(r, http.StatusNotFound, "", nil)
	}}
	tr, _ := newTestTransport(t, rt, base)
	resp, err := (&http.Client{Transport: tr}).Get(base + "/v1.0/scopes")
	if err == nil {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		t.Fatalf("a redirect to the auth endpoint was followed; body %q", body)
	}
	if strings.Contains(err.Error(), fresh) {
		t.Errorf("the error carries the issued token: %v", err)
	}
	for _, path := range []string{"/v1.0/auth", "/V1.0/Auth/", "/v2/auth/external"} {
		req, _ := http.NewRequest(http.MethodGet, base+path, nil)
		if resp, err := tr.RoundTrip(req); err == nil {
			_ = resp.Body.Close()
			t.Errorf("a data request to %s was sent", path)
		}
	}
}
