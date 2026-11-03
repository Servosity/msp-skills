// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Keeps FortiOS secrets out of everything this CLI returns, stores or hands to
// an agent. FortiOS answers cmdb reads with its stored secrets in "ENC <...>"
// form (RADIUS secrets, LDAP bind passwords, IPsec PSKs, the HA password,
// admin and local-user password hashes, API-user keys), and some builds return
// them in clear text when asked with ?plain-text-password=1. The guard wraps
// every FortiOS client (CLI, fleet, MCP) through the client-hook seam:
// it refuses plain-text-password requests and rewrites every JSON "ENC ..."
// string to "ENC (redacted)" before the client, cache, store or MCP sees it.
// The full-configuration backup is the one deliberate exception: `fleet backup`
// exists to keep that file, and `fleet changes` redacts it before diffing.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"fortinet-pp-cli/internal/client"
)

// encLiteral is the fallback for a body that is not valid JSON: a quote
// followed by "ENC " spelled in any JSON escaping, then the value up to (not
// including) the next unescaped quote or the end of the body. It does not
// depend on how earlier quotes pair up, and it leaves the closing quote in
// place, so that quote can still open the next match: a stray quote in an
// error page or a malformed body cannot hide a secret from it, and a body cut
// off inside the value is still redacted. Over-redacting a non-JSON body is
// harmless.
var encLiteral = regexp.MustCompile(`(?s)"(?:E|\\u0045)(?:N|\\u004[eE])(?:C|\\u0043)(?: |\\u0020)(?:[^"\\]|\\.)*\\?`)

// redactENC replaces every JSON string whose DECODED value starts with "ENC "
// with "ENC (redacted)", leaving every other byte of the body as it was (key
// order and formatting included). A body that is not valid JSON keeps the
// spans found before the parse error and then gets the literal fallback.
func redactENC(body []byte) []byte {
	// Scan what the client will parse: it drops a BOM or XSSI prefix first,
	// and either would otherwise stop the tokenizer before the secrets.
	clean := client.SanitizeJSONResponse(body)
	base := int64(len(body) - len(clean))
	dec := json.NewDecoder(bytes.NewReader(clean))
	dec.UseNumber() // a number float64 cannot hold must not end the scan early
	var spans [][2]int64
	prev := int64(0)
	parsed := true
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			parsed = false
			break
		}
		end := dec.InputOffset()
		if v, ok := tok.(string); ok && strings.HasPrefix(v, "ENC ") {
			// Only whitespace, ':' or ',' sit between the previous token and
			// this string's opening quote.
			if q := bytes.IndexByte(clean[prev:end], '"'); q >= 0 {
				spans = append(spans, [2]int64{base + prev + int64(q), base + end})
			}
		}
		prev = end
	}
	out := body
	if len(spans) > 0 {
		var b bytes.Buffer
		last := int64(0)
		for _, sp := range spans {
			b.Write(body[last:sp[0]])
			b.WriteString(redactedENC)
			last = sp[1]
		}
		b.Write(body[last:])
		out = b.Bytes()
	}
	if !parsed {
		out = encLiteral.ReplaceAll(out, []byte(redactedENC[:len(redactedENC)-1]))
	}
	return out
}

// maxInflatedBody caps a body the transport decompressed: a guard against a
// decompression bomb, set well above any real FortiOS cmdb table.
const maxInflatedBody = 256 << 20

// backupPath is the full-configuration download, the one read left unredacted.
const backupPath = "/api/v2/monitor/system/config/backup"

// isBackup matches the backup endpoint exactly. A cmdb object name containing
// "/monitor/system/config/backup" (escaped or not) must not match.
func isBackup(u *url.URL) bool {
	p := u.EscapedPath()
	i := strings.Index(p, "/api/v2/")
	return i >= 0 && p[i:] == backupPath
}

// asksPlainText reports whether the query asks FortiOS for clear-text secrets,
// in any percent-encoding. Decoding is lenient (a malformed escape elsewhere
// must not switch the check off) and repeats so double-encoding is caught too.
func asksPlainText(rawQuery string) bool {
	q := strings.ToLower(rawQuery)
	for i := 0; i < 3; i++ {
		if strings.Contains(q, "plain-text-password") {
			return true
		}
		q = strings.ToLower(lenientUnescape(q))
	}
	return strings.Contains(q, "plain-text-password")
}

// lenientUnescape decodes every valid %XX and '+' and keeps anything else as is.
func lenientUnescape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '+':
			b.WriteByte(' ')
		case s[i] == '%' && i+2 < len(s) && isHex(s[i+1]) && isHex(s[i+2]):
			b.WriteByte(unhex(s[i+1])<<4 | unhex(s[i+2]))
			i += 2
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func isHex(c byte) bool {
	return '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F'
}

func unhex(c byte) byte {
	switch {
	case c >= 'a':
		return c - 'a' + 10
	case c >= 'A':
		return c - 'A' + 10
	}
	return c - '0'
}

const redactedENC = `"ENC (redacted)"`

type secretGuard struct{ next http.RoundTripper }

func (g *secretGuard) RoundTrip(r *http.Request) (*http.Response, error) {
	if asksPlainText(r.URL.RawQuery) {
		// Answer locally with a 400 so the client reports it once instead of
		// retrying it as a network error; nothing is sent to the device.
		return refusal(r, http.StatusBadRequest, "plain-text-password would return stored secrets in clear text"), nil
	}
	// A caller-set Accept-Encoding turns off net/http's transparent gzip and
	// would hand the guard compressed bytes it cannot scan. Without it the
	// transport still asks for gzip and inflates the body before we read it.
	r = r.Clone(r.Context())
	r.Header.Del("Accept-Encoding")
	resp, err := g.next.RoundTrip(r)
	if err != nil || resp.Body == nil || isBackup(r.URL) {
		return resp, err
	}
	if enc := strings.TrimSpace(resp.Header.Get("Content-Encoding")); enc != "" && !strings.EqualFold(enc, "identity") {
		_ = resp.Body.Close()
		return refusal(r, http.StatusUnsupportedMediaType, enc+"-encoded response cannot be checked for secrets"), nil
	}
	var src io.Reader = resp.Body
	if resp.Uncompressed {
		src = io.LimitReader(resp.Body, maxInflatedBody+1)
	}
	body, err := io.ReadAll(src)
	_ = resp.Body.Close()
	if err != nil {
		return nil, err
	}
	if resp.Uncompressed && len(body) > maxInflatedBody {
		return nil, fmt.Errorf("refusing %s: decompressed response exceeds %d bytes", r.URL.Path, maxInflatedBody)
	}
	body = redactENC(body)
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.Header.Del("Content-Length")
	return resp, nil
}

// refusal answers a request locally with a 4xx, so the client reports it once
// (it retries transport errors and 5xx, not 4xx).
func refusal(r *http.Request, code int, why string) *http.Response {
	body, _ := json.Marshal(map[string]any{"http_status": code, "status": "error", "error": "refused by fortinet-cli: " + why})
	return &http.Response{
		StatusCode: code, Status: fmt.Sprintf("%d %s", code, http.StatusText(code)),
		Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
		Header:        http.Header{"Content-Type": {"application/json"}},
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       r,
	}
}

func guardSecrets(c *client.Client) {
	if c == nil || c.HTTPClient == nil {
		return
	}
	if _, done := c.HTTPClient.Transport.(*secretGuard); done {
		return
	}
	next := c.HTTPClient.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	// Binary downloads (fleet backup) use client.StreamingHTTPClient, which
	// drops the whole-request Timeout and sets ResponseHeaderTimeout only on a
	// bare *http.Transport; under the guard it cannot see one. Set it here so a
	// device that accepts the connection and never answers still times out.
	if tr, ok := next.(*http.Transport); ok && tr.ResponseHeaderTimeout == 0 {
		tr = tr.Clone()
		tr.ResponseHeaderTimeout = c.ConfiguredTimeout()
		next = tr
	}
	c.HTTPClient.Transport = &secretGuard{next: next}
}
