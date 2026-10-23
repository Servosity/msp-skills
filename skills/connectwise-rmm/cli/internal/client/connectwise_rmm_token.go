// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: ConnectWise Asio OAuth2 client_credentials token request.

package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"connectwise-rmm-pp-cli/internal/cliutil"
)

var tokenFieldRe = regexp.MustCompile(`("(?:access_token|refresh_token|client_secret|id_token)"\s*:\s*")(?:\\.|[^"\\])*(")`)

// RedactTokenBody strips the client secret and token-bearing JSON fields from
// a token-endpoint error body before it can reach output.
func RedactTokenBody(body, secret string) string {
	if secret != "" {
		body = strings.ReplaceAll(body, secret, "<redacted>")
		if quoted, err := json.Marshal(secret); err == nil {
			body = strings.ReplaceAll(body, string(quoted[1:len(quoted)-1]), "<redacted>")
		}
	}
	body = tokenFieldRe.ReplaceAllString(body, "${1}<redacted>${2}")
	// Hand-fix (handfixes.json secret-echo-decoded-funnel): an echo of the
	// secret the replacements above missed, in any encoding the shared funnel
	// decodes, withholds the whole body before the cut.
	return cliutil.SanitizeErrorBody(cliutil.FunnelWithhold(body, []string{secret}))
}

// ApplyAsioRequestHeaders adds the Origin and per-request X-Request-ID headers
// ConnectWise University lists as required for the platform APIs (the
// MSPbots Asio MCP client sends both on every call). Existing values win.
func ApplyAsioRequestHeaders(req *http.Request) {
	if req.Header.Get("Origin") == "" && req.URL != nil {
		req.Header.Set("Origin", req.URL.Scheme+"://"+req.URL.Host)
	}
	if req.Header.Get("X-Request-ID") == "" {
		var b [16]byte
		if _, err := rand.Read(b[:]); err == nil {
			b[6] = (b[6] & 0x0f) | 0x40
			b[8] = (b[8] & 0x3f) | 0x80
			h := hex.EncodeToString(b[:])
			req.Header.Set("X-Request-ID", h[0:8]+"-"+h[8:12]+"-"+h[12:16]+"-"+h[16:20]+"-"+h[20:])
		}
	}
}

// NewAsioTokenRequest builds the token request ConnectWise Asio accepts.
// Asio wants client_id and client_secret in a JSON body; a form-encoded body
// (with or without HTTP Basic) is rejected with HTTP 400 "failed to decode
// request invalid JSON" (probed 2026-10-13; CW University "Authentication and
// Authorization"; tfournet/asio-cli and Nango both send JSON).
func NewAsioTokenRequest(ctx context.Context, tokenURL, clientID, clientSecret, scope, userAgent string) (*http.Request, error) {
	body := map[string]string{
		"grant_type":    "client_credentials",
		"client_id":     clientID,
		"client_secret": clientSecret,
	}
	if scope != "" {
		body["scope"] = scope
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	return req, nil
}
