// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"sonicwall-pp-cli/internal/client"
)

// NSM SaaS reports failures in-band: HTTP 200 with
// {"status":{"success":false,"info":[{"code":"E_INVALIDAUTH","message":"..."}]}}
// (observed 2026-10-13 on /api/manager/tenants with no token and with a bogus
// one). Left alone, an expired or idle-timed-out token prints that envelope as
// if it were data and exits 0. This hook rewrites such responses to the HTTP
// status the rest of the CLI already classifies (401 -> exit 4, 404 -> exit 3),
// for both the CLI and the MCP server, without editing generated client code.
func init() {
	registerClientHook(func(c *client.Client) error {
		if c.HTTPClient == nil {
			return nil
		}
		base := c.HTTPClient.Transport
		if base == nil {
			base = http.DefaultTransport
		}
		if _, already := base.(nsmEnvelopeTransport); !already {
			// Redaction sits INSIDE the envelope hook, so it always sees the
			// server's raw body and the 401 text below is built from data that
			// was already redacted (see sonicwall_secret_redact.go).
			c.HTTPClient.Transport = nsmEnvelopeTransport{base: sonicOSRedactTransport{base: base}}
		}
		return nil
	})
}

type nsmEnvelopeTransport struct{ base http.RoundTripper }

// Auth-failure hints the generated error path cannot know: the NSM token is
// minted by nsm-login, and MySonicWall answers a bad key with an empty 401.
const (
	nsmAuthHint = "NSM rejected the session token: run 'sonicwall-cli nsm-login' (tokens last 8 hours, 15 minutes idle; unset SONICWALL_NSM_TOKEN if it holds an old token)"
	mswAuthHint = "MySonicWall rejected the X-API-KEY: check SONICWALL_MSW_API_KEY and that MySonicWall API permission is enabled for your user"
)

func (t nsmEnvelopeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err == nil && resp != nil && resp.StatusCode == http.StatusUnauthorized && resp.Body != nil &&
		strings.EqualFold(req.URL.Hostname(), "api.mysonicwall.com") {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("reading %s response: %w", req.URL.Path, readErr)
		}
		if len(bytes.TrimSpace(body)) == 0 {
			body = []byte(mswAuthHint)
		}
		resp.Body = io.NopCloser(bytes.NewReader(body))
		return resp, nil
	}
	if err != nil || resp == nil || resp.StatusCode < 200 || resp.StatusCode >= 300 || resp.Body == nil {
		return resp, err
	}
	// ponytail: reads the whole success body to inspect it; NSM bodies are small JSON documents.
	body, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		// A truncated body must fail the request, not reach a permissive parser.
		return nil, fmt.Errorf("reading %s response: %w", req.URL.Path, readErr)
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	// The client decodes an explicitly requested Content-Encoding after this
	// hook, so decode a copy with the same rules it uses (gzip, x-gzip, deflate,
	// stacked). Content type is not trusted; the body shape is. If the copy
	// cannot be decoded the client rejects the response itself.
	inspect, derr := decodeForInspection(resp.Header.Get("Content-Encoding"), body)
	if derr != nil {
		return resp, nil
	}
	if status, ok := nsmEnvelopeFailure(inspect); ok {
		resp.StatusCode = status
		resp.Status = http.StatusText(status)
		if status == http.StatusUnauthorized {
			// Lead with the fix, keep NSM's own envelope for diagnosis.
			resp.Body = io.NopCloser(bytes.NewReader(append([]byte(nsmAuthHint+". NSM said: "), bytes.TrimSpace(stripJSONPrefixes(inspect))...)))
			resp.Header.Del("Content-Encoding")
			resp.Header.Set("Content-Type", "text/plain; charset=utf-8")
		}
	}
	return resp, nil
}

// nsmEnvelopeFailure reports whether body is an NSM failure envelope and the
// HTTP status it stands for. Success envelopes (success:true, e.g. the
// /auth/sso token response) and every other JSON shape return ok=false.
func nsmEnvelopeFailure(body []byte) (int, bool) {
	trimmed := bytes.TrimSpace(stripJSONPrefixes(body))
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return 0, false
	}
	var head struct {
		Status *struct {
			Success *bool `json:"success"`
		} `json:"status"`
	}
	if json.Unmarshal(trimmed, &head) != nil || head.Status == nil || head.Status.Success == nil || *head.Status.Success {
		return 0, false
	}
	// The failure is decided; diagnostics are best effort and never undo it.
	code, msg := "", ""
	var diag struct {
		Status struct {
			Info []struct {
				Code    json.RawMessage `json:"code"`
				Message json.RawMessage `json:"message"`
			} `json:"info"`
		} `json:"status"`
	}
	if json.Unmarshal(trimmed, &diag) == nil && len(diag.Status.Info) > 0 {
		code = strings.ToUpper(strings.Trim(string(diag.Status.Info[0].Code), `"`))
		msg = strings.ToLower(strings.Trim(string(diag.Status.Info[0].Message), `"`))
	}
	// ponytail: substring match on NSM's E_* codes; add exact codes as a live tenant shows them.
	switch {
	case strings.Contains(code, "INVALIDAUTH") || strings.Contains(code, "UNAUTHENTICATED") || strings.Contains(msg, "unauthenticated"):
		return http.StatusUnauthorized, true
	case strings.Contains(code, "FORBIDDEN") || strings.Contains(code, "PERMISSION") || strings.Contains(code, "DENIED") || strings.Contains(code, "UNAUTHORIZED"):
		return http.StatusForbidden, true
	case strings.Contains(code, "NOTFOUND") || strings.Contains(code, "NOT_FOUND") || strings.Contains(msg, "not found"):
		return http.StatusNotFound, true
	default:
		return http.StatusBadRequest, true
	}
}

// maxInspectBytes bounds decompression of a body inspected for an envelope.
const maxInspectBytes = 32 << 20

func decodeForInspection(encoding string, body []byte) ([]byte, error) {
	var tokens []string
	for _, part := range strings.Split(encoding, ",") {
		t := strings.ToLower(strings.TrimSpace(part))
		if i := strings.IndexByte(t, ';'); i >= 0 {
			t = strings.TrimSpace(t[:i])
		}
		if t != "" && t != "identity" {
			tokens = append(tokens, t)
		}
	}
	for i := len(tokens) - 1; i >= 0; i-- {
		var r io.Reader
		switch tokens[i] {
		case "gzip", "x-gzip":
			zr, err := gzip.NewReader(bytes.NewReader(body))
			if err != nil {
				return nil, err
			}
			r = zr
		case "deflate":
			if zr, err := zlib.NewReader(bytes.NewReader(body)); err == nil {
				r = zr
			} else {
				r = flate.NewReader(bytes.NewReader(body))
			}
		default:
			return nil, fmt.Errorf("unsupported Content-Encoding %q", tokens[i])
		}
		decoded, err := io.ReadAll(io.LimitReader(r, maxInspectBytes+1))
		if err != nil {
			return nil, err
		}
		if len(decoded) > maxInspectBytes {
			return nil, fmt.Errorf("decoded body too large to inspect")
		}
		body = decoded
	}
	return body, nil
}

// stripJSONPrefixes removes the same UTF-8 BOM and XSSI/JSONP guards the client
// strips before parsing, so the hook judges exactly what the client will read.
func stripJSONPrefixes(body []byte) []byte {
	body = bytes.TrimPrefix(bytes.TrimLeft(body, " \t\r\n"), []byte("\xEF\xBB\xBF"))
	for _, p := range [][]byte{[]byte(")]}'\n"), []byte(")]}'"), []byte("{}&&"), []byte("for(;;);"), []byte("while(1);")} {
		if bytes.HasPrefix(body, p) {
			return bytes.TrimLeft(bytes.TrimPrefix(body, p), " \t\r\n")
		}
	}
	return body
}
