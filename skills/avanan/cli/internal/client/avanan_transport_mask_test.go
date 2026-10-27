// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): review round 2 regression guard.

package client

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// TestTransportMasksEveryIssuedTokenInErrorBodies: the client masks only the
// credentials in Config, but the session token the transport mints lives only
// in the transport. A vendor error body echoing it (in any encoding) reached
// the CLI error and the MCP result. That includes a token the transport has
// since discarded (a 401 whose re-mint failed hands back the stale token's
// echo).
func TestTransportMasksEveryIssuedTokenInErrorBodies(t *testing.T) {
	const minted = "minted-session-XYZ/0123456789+abc"
	echoes := func(tok string) string {
		return strings.Join([]string{
			tok,
			url.QueryEscape(tok),
			base64.StdEncoding.EncodeToString([]byte(tok)),
			strings.ReplaceAll(tok, "/", `\/`),
		}, " | ")
	}
	base := "https://smart-api-production-1-us.avanan.net"

	for _, tc := range []struct {
		name   string
		status int
	}{
		{"400 echo of the live token", http.StatusBadRequest},
		{"401 echo after the re-mint failed", http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			mints := 0
			rt := &handshakeRecorder{reply: func(r *http.Request) *http.Response {
				if r.URL.Path == "/v1.0/auth" {
					mu.Lock()
					mints++
					n := mints
					mu.Unlock()
					if n > 1 {
						return textResponse(r, http.StatusInternalServerError, "", nil)
					}
					return textResponse(r, http.StatusOK, minted, nil)
				}
				return textResponse(r, tc.status, `{"error":"x-av-token: `+echoes(minted)+`"}`, http.Header{"Content-Type": {"application/json"}})
			}}
			tr, _ := newTestTransport(t, rt, base)
			req, _ := http.NewRequest(http.MethodGet, base+"/v1.0/scopes", nil)
			resp, err := tr.RoundTrip(req)
			if err != nil {
				t.Fatalf("RoundTrip: %v", err)
			}
			body, _ := io.ReadAll(resp.Body)
			for _, enc := range strings.Split(echoes(minted), " | ") {
				if strings.Contains(string(body), enc) {
					t.Errorf("error body still carries the minted token (%q):\n%s", enc, body)
				}
			}
			if !strings.Contains(string(body), "x-av-token") {
				t.Errorf("masking dropped the rest of the vendor body: %s", body)
			}
		})
	}
}
