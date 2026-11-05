// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"zoom-phone-pp-cli/internal/client"
	"zoom-phone-pp-cli/internal/cliutil"
	"zoom-phone-pp-cli/internal/store"
)

// Credential fields (device SIP passwords, desk-phone and port-order PINs,
// smartphone activation codes, Team Chat channel tokens) are redacted from every
// JSON API response at the transport, before the client, its response cache,
// the store, MCP tools or any command sees them. Enforcing it per response
// (not per command or operation id) closes path collisions such as a detail
// call whose id resolves to /phone/common_areas/activation_codes. View these
// values in the Zoom web portal.
func init() {
	registerClientHook(func(c *client.Client) error {
		if c.HTTPClient == nil {
			return nil
		}
		base := c.HTTPClient.Transport
		if base == nil {
			base = cliutil.OutboundGuard(nil) // credkit-wiring
		}
		if _, ok := base.(zoomSecretRedactTransport); !ok {
			c.HTTPClient.Transport = zoomSecretRedactTransport{base: base}
		}
		return nil
	})
}

// zoomWithheldBody replaces a response body the redaction cannot read. The
// status stays, so the client still classifies 401/403/404/429; nothing of
// the body (or its headers) is repeated.
func zoomWithheldBody(n int) []byte {
	return []byte(fmt.Sprintf(`{"code":"zoom_phone_body_withheld","message":"the response body (%d bytes) was withheld: it is not JSON, so credential redaction could not run"}`, n))
}

type zoomSecretRedactTransport struct{ base http.RoundTripper }

func (t zoomSecretRedactTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil || resp == nil || resp.Body == nil {
		return resp, err
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	body, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close() // fully read (or failed); a close error adds nothing
	if readErr != nil {
		resp.Body = io.NopCloser(bytes.NewReader(body))
		return resp, readErr
	}
	// The client sets Accept-Encoding itself, so bodies can arrive compressed:
	// decode here (failing closed on encodings the client cannot read either)
	// and hand the client plain bytes.
	if enc := resp.Header.Get("Content-Encoding"); strings.TrimSpace(enc) != "" {
		decoded, decErr := client.ZoomDecodeContentEncoding(enc, body)
		if decErr != nil {
			return nil, decErr
		}
		body = decoded
		resp.Header.Del("Content-Encoding")
		resp.Uncompressed = true
	}
	// What the body is, not what the response calls it, decides. Normalize
	// every body exactly as the client will (UTF-8 BOM, JSONP/XSSI prefixes)
	// and parse it, redacting by decoded key names; no literal-byte pre-scan,
	// so escaped keys cannot slip past.
	// A body the redaction cannot read is never passed on (zoom-phone-secret-
	// holder): on a non-2xx status it is replaced by a fixed JSON notice (the
	// status still drives the client's error handling), and on a 2xx it is
	// refused unless the request is one of the file downloads. That covers
	// plain text, XML, form bodies, HTML pages (whose title the client would
	// quote), binary media types, NUL-bearing bodies and malformed JSON.
	jsonLabelled := ct == "" || strings.Contains(ct, "json")
	normalized := client.ZoomSanitizeJSONResponse(body)
	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	if len(bytes.TrimSpace(normalized)) > 0 {
		readable := false
		if jsonLabelled || zoomLooksLikeJSON(normalized) {
			// Redaction holds every value it removes; token fields (a
			// mint response) are held but left for the client to read.
			redacted, changed, err := store.RedactSecretsStrict(normalized)
			switch {
			case err == nil && changed:
				body, readable = redacted, true
			case err == nil:
				readable = true
			case success:
				return nil, fmt.Errorf("refusing a malformed JSON response from %s (credential redaction could not run)", req.URL.Path)
			}
		}
		switch {
		case readable:
		case !success:
			body = zoomWithheldBody(len(body))
			resp.Header.Set("Content-Type", "application/json")
		case zoomDownloadRequest(req):
		default:
			return nil, fmt.Errorf("refusing a non-JSON response from %s: Zoom documents JSON for this endpoint (credential redaction could not run)", req.URL.Path)
		}
	}
	resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
	resp.ContentLength = int64(len(body))
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return resp, nil
}

func zoomLooksLikeJSON(body []byte) bool {
	t := bytes.TrimSpace(bytes.TrimPrefix(body, []byte{0xEF, 0xBB, 0xBF}))
	return len(t) > 0 && (t[0] == '{' || t[0] == '[')
}

// zoomDownloadTemplates are the Zoom Phone endpoints that return files
// (recordings, voicemail audio, fax documents and cover images, transcripts)
// rather than JSON. "*" is exactly one path segment.
var zoomDownloadTemplates = [][]string{
	{"phone", "recording", "download", "*"},
	{"phone", "voice_mails", "download", "*"},
	{"phone", "fax", "cover_pages", "download", "*"},
	{"phone", "recording_transcript", "download", "*"},
	{"phone", "fax", "logs", "*", "file", "*"},
}

// zoomDownloadRequest reports whether req (or, for a redirect hop, the API
// request that started it) is one of the file downloads. It matches the
// escaped path segment by segment after the first /phone/, and a segment
// that could resolve elsewhere (., .., an encoded slash or backslash) never
// matches.
func zoomDownloadRequest(req *http.Request) bool {
	for i := 0; i < 20 && req != nil && req.Response != nil && req.Response.Request != nil; i++ {
		req = req.Response.Request
	}
	if req == nil || req.URL == nil {
		return false
	}
	p := req.URL.EscapedPath()
	idx := strings.Index(p, "/phone/")
	if idx < 0 {
		return false
	}
	segs := strings.Split(p[idx+1:], "/")
	for _, tmpl := range zoomDownloadTemplates {
		if len(segs) != len(tmpl) {
			continue
		}
		ok := true
		for i, want := range tmpl {
			got := segs[i]
			if want == "*" {
				lower := strings.ToLower(got)
				if got == "" || got == "." || got == ".." || strings.Contains(lower, "%2f") || strings.Contains(lower, "%5c") || strings.Contains(got, "\\") || strings.Contains(lower, "%2e") {
					ok = false
					break
				}
				continue
			}
			if got != want {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}
