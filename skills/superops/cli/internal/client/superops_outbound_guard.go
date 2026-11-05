// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): every API client's transport passes the shared
// credential kit's outbound guard (credkit-wiring), which refuses a request that
// carries a held credential off its declared origins, over plain http or across
// a redirect to another origin.

package client

import (
	"net/http"

	"superops-pp-cli/internal/credkit"
)

// OutboundGuard is the kit's transport. A variable so in-process tests, which
// run no credkit.Start and so have no declared origins, can stand up loopback
// servers; the built binaries are the proof of the guard itself.
var OutboundGuard = credkit.Transport

// outboundTransport keeps the base transport beside the guard, so
// StreamingHTTPClient can clone it with another header timeout.
type outboundTransport struct {
	http.RoundTripper
	base *http.Transport
}

func guardOutboundTransport(rt http.RoundTripper) http.RoundTripper {
	base, _ := rt.(*http.Transport)
	return &outboundTransport{RoundTripper: OutboundGuard(rt), base: base}
}

// unguardedTransport is the *http.Transport under rt, guarded or not.
func unguardedTransport(rt http.RoundTripper) (*http.Transport, bool) {
	switch t := rt.(type) {
	case *http.Transport:
		return t, true
	case *outboundTransport:
		return t.base, t.base != nil
	}
	return nil, false
}
