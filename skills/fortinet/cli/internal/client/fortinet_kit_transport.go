// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring).

package client

import "net/http"

// TuneTransport applies f to a clone of this client's base *http.Transport and
// installs the clone, still inside the credential kit's guard when the client
// had one. Hand code that adjusts TLS or timeouts (--insecure, the fleet
// header timeout) goes through here: the guard hides the *http.Transport it
// wraps. It reports whether a base transport was found.
func (c *Client) TuneTransport(f func(*http.Transport)) bool {
	if c == nil || c.HTTPClient == nil {
		return false
	}
	switch t := c.HTTPClient.Transport.(type) {
	case guardedTransport:
		tr := t.inner.Clone()
		f(tr)
		c.HTTPClient.Transport = guardTransport(tr)
		return true
	case *http.Transport:
		tr := t.Clone()
		f(tr)
		c.HTTPClient.Transport = tr
		return true
	}
	return false
}

// SendOutsideKit takes the credential kit's outbound guard off this client.
// Only a fleet device's client calls it: its token goes to the device URL the
// operator's devices file names, and the kit's origins are fixed at build time
// (internal/credwire). That client keeps its own pinning: https only, no
// redirect off the device's origin, and no configured credential but the
// device's own token.
func (c *Client) SendOutsideKit() {
	if c == nil || c.HTTPClient == nil {
		return
	}
	if t, ok := c.HTTPClient.Transport.(guardedTransport); ok {
		c.HTTPClient.Transport = t.inner
	}
}
