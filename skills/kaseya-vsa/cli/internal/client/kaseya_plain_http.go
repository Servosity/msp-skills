// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

// kaseya-vsa handfix chrome-transport-plain-http-h1: the Chrome transport
// sends every request to its h2-over-TLS path first, so an http:// base URL
// failed with "http2: unencrypted HTTP/2 not enabled". Plain http has no TLS
// to negotiate h2 over; serve it on the transport's HTTP/1.1 side. https is
// unchanged. Kept out of chrome.go so that template file stays byte-identical.

import "net/http"

// ServePlainHTTPOverH1 rewires a Chrome-transport client so plain-http
// requests use its HTTP/1.1 transport. Other transports are left as they are.
func ServePlainHTTPOverH1(hc *http.Client) {
	if hc != nil {
		hc.Transport = plainHTTPSplit(hc.Transport)
	}
}

func plainHTTPSplit(rt http.RoundTripper) http.RoundTripper {
	switch t := rt.(type) {
	case *chromeAllowTripper:
		t.next = plainHTTPSplit(t.next)
		return t
	case chromeHeaderTripper:
		t.next = plainHTTPSplit(t.next)
		return t
	case *chromeTransport:
		return schemeSplitTripper{tls: t, plain: t.h1}
	}
	return rt
}

type schemeSplitTripper struct {
	tls   http.RoundTripper
	plain http.RoundTripper
}

func (s schemeSplitTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" {
		return s.plain.RoundTrip(req)
	}
	return s.tls.RoundTrip(req)
}
