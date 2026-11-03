// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import "net/http"

// Handfix jsm-config-headers-stay-on-origin. The config file's [headers]
// table is operator-supplied and may hold a secret under any name, so every
// such header is treated as credential material.

// offOriginHeaders is everything a redirect hop that leaves the configured
// origin (scheme, host or port changes) keeps. Every other header, configured
// or not, is dropped: an allowlist, so a header name nobody listed fails closed.
var offOriginHeaders = map[string]bool{
	"Accept":            true,
	"Accept-Encoding":   true,
	"Accept-Language":   true,
	"Content-Length":    true,
	"Content-Type":      true,
	"User-Agent":        true,
	"X-Experimentalapi": true,
}

// jsmStripOffOriginHeaders drops every configured header by name (its value
// is the operator's, whatever the name), then every header not allowlisted.
func jsmStripOffOriginHeaders(c *Client, req *http.Request) {
	if c != nil && c.Config != nil {
		for name := range c.Config.Headers {
			req.Header.Del(name)
		}
	}
	for name := range req.Header {
		if !offOriginHeaders[http.CanonicalHeaderKey(name)] {
			req.Header.Del(name)
		}
	}
}

// jsmConfigHeadersPlaintext refuses a request that carries a configured header
// over plaintext http to a non-loopback host, as the press already does for
// Authorization and cookies.
func jsmConfigHeadersPlaintext(c *Client, req *http.Request) error {
	if c == nil || c.Config == nil {
		return nil
	}
	for name := range c.Config.Headers {
		if req.Header.Get(name) != "" {
			return plaintextCredentialError(req.URL, true)
		}
	}
	return nil
}
