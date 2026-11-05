// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"zoom-phone-pp-cli/internal/cliutil"
)

// ZoomRequireSecureURL refuses plaintext http to a non-loopback host for any URL
// that carries credentials (base URL with the bearer token, token URL with the
// Basic client secret). Loopback test servers may use http; values with any
// other scheme (or none) cannot send credentials in the clear.
func ZoomRequireSecureURL(label, raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid %s %q: %w", label, raw, err)
	}
	if !strings.EqualFold(u.Scheme, "http") || zoomLoopbackHost(u.Hostname()) {
		return nil
	}
	return fmt.Errorf("refusing %s %q: credentials are only sent over https (plain http is allowed for loopback test servers); check ZOOM_PHONE_BASE_URL / ZOOM_PHONE_TOKEN_URL and base_url / token_url in the config file", label, raw)
}

func zoomRequireSecureTokenURL(raw string) error { return ZoomRequireSecureURL("token URL", raw) }

func zoomLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// ZoomDecodeContentEncoding inflates a response body per its Content-Encoding
// header (gzip, deflate; size-capped) using the client's own decoder, so the
// secret-redaction transport sees the same bytes the client parses.
func ZoomDecodeContentEncoding(encoding string, body []byte) ([]byte, error) {
	return decodeContentEncoding(encoding, body)
}

// ZoomSanitizeJSONResponse applies the client's own JSON normalization (UTF-8
// BOM, JSONP/XSSI prefixes) so the redaction transport inspects exactly the
// bytes the client will parse.
func ZoomSanitizeJSONResponse(body []byte) []byte { return sanitizeJSONResponse(body) }

// ZoomIsBinaryResponseContentType is the client's own test for a response it
// hands back base64-wrapped instead of parsing as JSON, so the redaction
// transport and the client agree on what counts as binary.
func ZoomIsBinaryResponseContentType(ct string) bool { return isBinaryResponseContentType(ct) }

// zoomOAuthErrorCodes are the token-endpoint error codes of RFC 6749 section
// 5.2 (plus server_error and temporarily_unavailable), each with a fixed
// description. They are the only response text a token error ever shows.
var zoomOAuthErrorCodes = map[string]string{
	"invalid_request":         "the token request was malformed",
	"invalid_client":          "the client ID or client secret was rejected",
	"invalid_grant":           "the grant (account ID or refresh token) was rejected",
	"unauthorized_client":     "this app may not use this grant; check that it is a Server-to-Server OAuth app",
	"unsupported_grant_type":  "the token endpoint does not support this grant type",
	"invalid_scope":           "the requested scope was rejected",
	"access_denied":           "access was denied",
	"server_error":            "the token endpoint had an internal error",
	"temporarily_unavailable": "the token endpoint is temporarily unavailable",
}

// ZoomTokenEndpointError describes a failed token-endpoint response by its
// HTTP status and, when the body's "error" member is a recognized OAuth error
// code, that code and its fixed description. Nothing else from the body is
// shown: an endpoint that echoes the request (a proxy, a misconfigured or
// hostile endpoint) can return the client secret or refresh token in any
// encoding, so token errors allowlist what may be shown instead of masking.
// Every token path (auth login, the automatic mint, the refresh exchange)
// uses this one function.
func ZoomTokenEndpointError(status int, body []byte) string {
	msg := fmt.Sprintf("HTTP %d", status)
	var parsed struct {
		Error any `json:"error"`
	}
	if json.Unmarshal(sanitizeJSONResponse(body), &parsed) == nil {
		if code, ok := parsed.Error.(string); ok {
			if desc, known := zoomOAuthErrorCodes[code]; known {
				return msg + " (" + code + ": " + desc + ")"
			}
		}
	}
	return msg + " (response body withheld)"
}

// ZoomTokenResponseUnreadable is the error for a token response that does not
// parse; the parser's message is not shown because it can quote the body.
func ZoomTokenResponseUnreadable(status int) error {
	return fmt.Errorf("token endpoint returned HTTP %d with a body that is not a token response (body withheld)", status)
}

// ZoomLooksLikeHTMLDocument is the client's own test for an HTML page, which
// it reports as "API returned HTML instead of JSON" rather than using as data.
func ZoomLooksLikeHTMLDocument(body []byte) bool {
	_, ok := summarizeHTMLDocument(body)
	return ok
}

// MaskCredentialText masks every credential the process holds (and their
// encodings) in text, for output paths outside a client (MCP previews).
func MaskCredentialText(text string) string {
	return (*Client)(nil).maskCredentialText(text)
}

// zoom-phone hand-fix: credkit-wiring (class
// secret-redaction/source-registration, Codex review r1 of the onboard).
//
// ZoomMaskTransportError is a token or API transport error as it may be shown:
// the redirect Location removed (cliutil.ZoomScrubTransportError), then every
// held credential masked, and the text withheld when one survives in any
// encoding (maskCredentialText). auth login, the automatic mint and the
// refresh exchange use it; the API request loop scrubs and masks the same way.
func ZoomMaskTransportError(err error, req *http.Request, extraCredentials ...string) error {
	return (*Client)(nil).zoomMaskTransportError(err, req, extraCredentials...)
}

func (c *Client) zoomMaskTransportError(err error, req *http.Request, extraCredentials ...string) error {
	if err == nil {
		return nil
	}
	var requested *url.URL
	if req != nil {
		requested = req.URL
	}
	scrubbed := cliutil.ZoomScrubTransportError(err, requested)
	text := scrubbed.Error()
	masked := c.maskCredentialText(text, extraCredentials...)
	if masked == text {
		return scrubbed
	}
	out := zoomMaskedTransportError{msg: masked}
	if terminalTransportRefusal(scrubbed) {
		out.sentinel = terminalSentinel(scrubbed)
	}
	return out
}

// zoomMaskTransportErrorLocked and zoomMaskErrorLocked are the token mint
// and refresh forms: both run while the caller holds the token-state lock,
// which maskCredentialText takes to read the config's credentials (a
// sync.Mutex is not reentrant, so a rejected mint would hang). They read those
// credentials directly instead and mask through the lock-free path.
func (c *Client) zoomMaskTransportErrorLocked(err error, req *http.Request, extraCredentials ...string) error {
	return ZoomMaskTransportError(err, req, c.zoomTokenStateCredentialsLocked(extraCredentials)...)
}

func (c *Client) zoomMaskErrorLocked(err error, extraCredentials ...string) error {
	return (*Client)(nil).maskError(err, c.zoomTokenStateCredentialsLocked(extraCredentials)...)
}

func (c *Client) zoomTokenStateCredentialsLocked(extra []string) []string {
	out := append([]string(nil), extra...)
	if c != nil && c.Config != nil {
		out = append(out, c.Config.AuthHeaderVal, c.Config.AccessToken, c.Config.RefreshToken, c.Config.ClientSecret, c.Config.ZoomPhoneClientSecret)
	}
	return out
}

// zoomMaskedTransportError carries masked text; only a refusal sentinel stays
// reachable (errors.Is), never the unmasked error.
type zoomMaskedTransportError struct {
	msg      string
	sentinel error
}

func (e zoomMaskedTransportError) Error() string { return e.msg }
func (e zoomMaskedTransportError) Unwrap() error { return e.sentinel }

// zoomSameOriginReadsOnly is the redirect policy for a client built without a
// base (zoom-phone hand-fix: zoom-phone-same-origin-redirects).
func zoomSameOriginReadsOnly(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	return cliutil.ZoomRedirectRefusal(req, via)
}
