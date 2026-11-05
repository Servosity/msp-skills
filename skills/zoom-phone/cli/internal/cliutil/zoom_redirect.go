// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// zoom-phone hand-fix: zoom-phone-same-origin-redirects (class
// credential-host-redirect, Codex review r1 of the onboard).
//
// Go's client drops the Authorization header on a cross-host redirect but
// replays a 307/308 request's body to the Location, so a PATCH carrying a PIN
// answered `307 Location: https://example.com/collect` sent the PIN there.
// ZoomRedirectRefusal is the one redirect rule every HTTP client in this
// binary that carries a credential or a body applies (the API client and its
// download and upload clones, the token mint, auth login's token client, the
// refresh exchange, the feedback POST; the webhook delivery never follows a
// redirect at all):
//   - a request that is not GET or HEAD is never redirected, even within its
//     origin, because its body would be replayed;
//   - a GET or HEAD follows a redirect only to the origin (scheme, host and
//     effective port) the chain started on.
//
// The refusal names the starting origin only. Go still puts the Location into
// the *url.Error it returns, so every caller passes the error through
// ZoomScrubTransportError before showing it.

// ErrZoomRedirectRefused marks a redirect ZoomRedirectRefusal refused.
var ErrZoomRedirectRefused = errors.New("redirect refused")

// ZoomRedirectRefusal returns nil when req (the next hop) may be followed.
func ZoomRedirectRefusal(req *http.Request, via []*http.Request) error {
	if req == nil || req.URL == nil || len(via) == 0 || via[0] == nil || via[0].URL == nil {
		return fmt.Errorf("%w: the redirect has no origin to compare", ErrZoomRedirectRefused)
	}
	first := via[0]
	origin := strings.ToLower(first.URL.Scheme) + "://" + first.URL.Host
	switch first.Method {
	case "", http.MethodGet, http.MethodHead:
	default:
		return fmt.Errorf("%w: %s to %s was answered with a redirect; a %s is never redirected because its body would be sent to the new location (the location is not shown)",
			ErrZoomRedirectRefused, first.Method, origin, first.Method)
	}
	if !oauthTokenSameOrigin(req.URL, first.URL) {
		return fmt.Errorf("%w: a redirect from %s leaves that origin (the location is not shown)", ErrZoomRedirectRefused, origin)
	}
	return nil
}

// errZoomLocationUnparsable replaces Go's "failed to parse Location header
// %q" text, which quotes the whole header.
var errZoomLocationUnparsable = errors.New("the server answered with a redirect whose Location header does not parse (the header is not shown)")

// ZoomScrubTransportError returns err with every trace of a redirect Location
// removed. Go's *url.Error names the Location when a redirect is refused, and
// quotes the whole header when it does not parse; that text would be printed.
// The returned *url.Error names the URL this program requested instead, and a
// Location parse failure becomes a fixed message. Every other cause
// (timeouts, DNS, TLS, the refusal sentinels) stays reachable through
// errors.Is and errors.As. Callers then mask held credentials in the text.
func ZoomScrubTransportError(err error, requested *url.URL) error {
	if err == nil {
		return nil
	}
	var ue *url.Error
	if !errors.As(err, &ue) || ue == nil {
		return err
	}
	scrubbed := &url.Error{Op: ue.Op, URL: zoomRequestedURL(requested), Err: zoomScrubCause(ue.Err)}
	if err == error(ue) {
		return scrubbed
	}
	// A wrapped *url.Error: replace its text inside the outer message.
	return zoomScrubbedError{msg: strings.ReplaceAll(err.Error(), ue.Error(), scrubbed.Error()), err: scrubbed}
}

type zoomScrubbedError struct {
	msg string
	err error
}

func (e zoomScrubbedError) Error() string { return e.msg }
func (e zoomScrubbedError) Unwrap() error { return e.err }

func zoomRequestedURL(u *url.URL) string {
	if u == nil {
		return "(request)"
	}
	return u.Redacted()
}

func zoomScrubCause(cause error) error {
	if cause == nil {
		return nil
	}
	if strings.Contains(cause.Error(), "Location header") {
		return errZoomLocationUnparsable
	}
	return cause
}
