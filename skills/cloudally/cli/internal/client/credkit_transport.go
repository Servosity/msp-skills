// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring): every HTTP
// client this program builds sends through the shared credential kit's
// outbound guard, so a held credential never goes to an origin outside its
// declared ones, over plain http, or across a redirect to another origin.

package client

import (
	"net/http"

	"cloudally-pp-cli/internal/credkit"
)

// OutboundGuard is the credential kit's outbound guard, around every transport
// a client sends through. Production code never reassigns it. In-process unit
// tests do, to the identity: they never run credkit.Start, so the kit has no
// declarations and no base origin and would refuse every credentialed request
// to a loopback test server. The built binaries' leak tests (internal/credwire)
// prove the guard itself.
var OutboundGuard = credkit.Transport

// guardedTransport is OutboundGuard around inner, with inner kept so a
// streaming client can clone it with its own header timeout and guard the
// clone.
type guardedTransport struct {
	http.RoundTripper
	inner *http.Transport
}

func guardTransport(tr *http.Transport) http.RoundTripper {
	return guardedTransport{RoundTripper: OutboundGuard(tr), inner: tr}
}

// innerTransport is a clone of the plain transport under rt: the one a
// guarded transport wraps, rt itself when it is plain, the default when rt is
// nil. Any other round tripper reports false; the caller guards it as it is.
func innerTransport(rt http.RoundTripper) (*http.Transport, bool) {
	switch t := rt.(type) {
	case nil:
		return http.DefaultTransport.(*http.Transport).Clone(), true
	case guardedTransport:
		return t.inner.Clone(), true
	case *http.Transport:
		return t.Clone(), true
	}
	return nil, false
}

// GuardedTransport is a fresh default transport behind the outbound guard, for
// the clients built outside this package: the CloudAlly login, refresh and
// partner-access-token calls, feedback, --deliver webhooks and probes.
func GuardedTransport() http.RoundTripper {
	return guardTransport(http.DefaultTransport.(*http.Transport).Clone())
}
