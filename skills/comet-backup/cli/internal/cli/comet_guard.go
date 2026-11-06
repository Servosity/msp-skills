// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// comet-backup handfixes applied to every API client (CLI and MCP) through
// registerClientHook:
//
//   - api-origin-pinned: every request, redirect hops included, must go to
//     the scheme, host and port the configuration resolved when the client was
//     built: the Comet Server base URL (COMET_BACKUP_BASE_URL, or
//     https://{server} with COMET_BACKUP_SERVER or the config file's
//     template_vars). Nothing given later (a Location header, an MCP argument,
//     a path) can move the admin password or session key, which ride in every
//     request body, to another host.
//   - no-response-cache: the on-disk response cache is off. Comet bodies carry
//     storage destination keys, password hashes and code-signing secrets; the
//     local store keeps them out (store/comet_secrets.go), and raw bodies must
//     not sit in cache files.
//   - response-credentials-held: every credential a request body or a
//     response carries is held before the request is sent or the body reaches
//     any command, so the output chokepoint withholds it like the configured
//     password in every format: a destination's S3, Azure, B2 or SFTP keys and
//     passwords (UserProfileConfig), a bucket ReadWriteKey (BucketProperties),
//     an organization's code-signing secrets, a new user's password. Each is
//     then replaced by name in the body (cliutil.RedactBody), so a listing that
//     carries a password stays readable: the kit withholds a whole unit that
//     repeats a held value.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"comet-backup-pp-cli/internal/client"
	"comet-backup-pp-cli/internal/cliutil"
	"comet-backup-pp-cli/internal/credkit"
)

// ErrOriginChanged marks a request refused because it left the pinned origin.
var ErrOriginChanged = errors.New("request refused: it would leave the configured Comet Server")

func init() {
	registerClientHook(func(c *client.Client) error {
		dropIdempotencyKeys(c)
		guardClient(c)
		return nil
	})
}

// dropIdempotencyKeys is comet-backup hand-fix (no-idempotency-replay). The
// client replays a POST after an ambiguous failure (a 5xx, a dropped or late
// reply) only when the request carries an Idempotency-Key or
// X-Idempotency-Key header, even one marked not replay-safe (client.go
// canRetryAmbiguousFailure; net/http replays such requests too). Comet
// ignores both headers, so one set in the config file's [headers] would have
// re-sent a backup dispatch or a user create up to three more times. Every
// Comet client drops them, so no write is ever replayed.
func dropIdempotencyKeys(c *client.Client) {
	if c == nil || c.Config == nil || len(c.Config.Headers) == 0 {
		return
	}
	kept := make(map[string]string, len(c.Config.Headers))
	for k, v := range c.Config.Headers {
		if strings.EqualFold(k, "Idempotency-Key") || strings.EqualFold(k, "X-Idempotency-Key") {
			continue
		}
		kept[k] = v
	}
	c.Config.Headers = kept
}

func guardClient(c *client.Client) {
	if c == nil || c.HTTPClient == nil {
		return
	}
	c.NoCache = true
	// The base URL may be the spec's https://{server} template: pin the host
	// it resolves to. An unresolved template pins nothing that resolves (the
	// client refuses the request first, naming COMET_BACKUP_SERVER).
	origins := pinnedOrigins(client.ResolvedBaseURL(c.BaseURL, c.CurrentTemplateVars()))
	// The kit sits inside the redactor: it reads the server's own bytes, so a
	// credential in a response is held before the redactor rewrites it, and it
	// never sees the redactor's marker as a credential.
	next := client.OutboundGuard(c.HTTPClient.Transport)
	c.HTTPClient.Transport = originPinTripper{origins: origins, next: responseHoldTripper{next: next}}
	// The generated redirect check may resolve an off-origin hop's host in
	// DNS; a query is itself a disclosure, so the pin runs first.
	generated := c.HTTPClient.CheckRedirect
	c.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := checkPinned(origins, req.URL); err != nil {
			return err
		}
		if generated != nil {
			return generated(req, via)
		}
		return nil
	}
}

// pinnedOrigins returns the normalized origin of each configured URL; an
// unparseable one pins nothing (its requests are refused).
func pinnedOrigins(urls ...string) map[string]bool {
	out := map[string]bool{}
	for _, raw := range urls {
		if u, err := url.Parse(raw); err == nil {
			if o := normalizedOrigin(u); o != "" {
				out[o] = true
			}
		}
	}
	return out
}

func checkPinned(origins map[string]bool, u *url.URL) error {
	if o := normalizedOrigin(u); o == "" || !origins[o] {
		return fmt.Errorf("%w (%s); set COMET_BACKUP_SERVER or COMET_BACKUP_BASE_URL to change servers", ErrOriginChanged, displayOrigin(u))
	}
	return nil
}

// normalizedOrigin is scheme://host:port with ASCII letters lowercased and
// the default port filled in. It is "" (never equal to a pinned origin) for a
// host holding any non-ASCII byte: Unicode case folding and IDNA can make two
// spellings compare equal while the transport dials another name.
func normalizedOrigin(u *url.URL) string {
	if u == nil || u.Host == "" || !isASCII(u.Scheme) || !isASCII(u.Host) {
		return ""
	}
	scheme := asciiLower(u.Scheme)
	port := u.Port()
	if port == "" {
		switch scheme {
		case "https":
			port = "443"
		case "http":
			port = "80"
		}
	}
	return scheme + "://" + net.JoinHostPort(asciiLower(u.Hostname()), port)
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

type originPinTripper struct {
	origins map[string]bool
	next    http.RoundTripper
}

func (t originPinTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := checkPinned(t.origins, req.URL); err != nil {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, err
	}
	return t.next.RoundTrip(req)
}

func displayOrigin(u *url.URL) string {
	if u == nil {
		return "no URL"
	}
	if o := normalizedOrigin(&url.URL{Scheme: u.Scheme, Host: u.Host}); o != "" {
		return o
	}
	return "a non-ASCII host"
}

// responseHoldTripper reads each response (under the client's size cap),
// decodes a copy as the client will (Content-Encoding, then BOM and JSONP/XSSI
// prefixes), holds its credential values with the one shared walker
// (cliutil.HoldCredentialFields: any length, even one equal to a marker) and
// replaces them by name before any command reads it. Request bodies and params
// are held in the client itself (doInternal), before a --dry-run preview or a
// send.
type responseHoldTripper struct{ next http.RoundTripper }

func (t responseHoldTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.next.RoundTrip(req)
	if err != nil || resp == nil || resp.Body == nil {
		return resp, err
	}
	body, rerr := client.ReadBody(resp.Body)
	_ = resp.Body.Close()
	if rerr != nil {
		return nil, rerr
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	plain, derr := client.DecodeBody(resp.Header.Get("Content-Encoding"), body)
	if derr != nil {
		return resp, nil
	}
	cliutil.HoldDocument(plain)
	if redacted := cliutil.RedactBody(plain); !bytes.Equal(redacted, plain) {
		resp.Body = io.NopCloser(bytes.NewReader(redacted))
		resp.ContentLength = int64(len(redacted))
		resp.Header.Del("Content-Encoding")
		resp.Header.Del("Content-Length")
	}
	return resp, nil
}

// sendOffHost is the one way a request leaves for a host other than the Comet
// Server (the feedback endpoint, a webhook). It goes through the credential
// kit's outbound guard, which refuses it when its URL, a header or its body
// carries a held credential, sends once, and never follows a redirect, so no
// hop can carry the request to a host the operator did not configure
// (comet-backup handfix: outbound-sinks-withheld). A 3xx or 4xx/5xx answer is
// an error labelled with what was called.
func sendOffHost(label string, req *http.Request, timeout time.Duration) error {
	httpClient := &http.Client{
		Timeout:   timeout,
		Transport: credkit.Transport(nil), // Hand-fix (handfixes.json credkit-wiring)
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("posting to %s: %w", label, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return fmt.Errorf("%s returned %s (Location: %q); redirects are not followed", label, resp.Status, resp.Header.Get("Location"))
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("%s returned %s", label, resp.Status)
	}
	return nil
}
