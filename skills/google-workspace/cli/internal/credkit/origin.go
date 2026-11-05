// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// Contract 2(c) definitions: an origin is the scheme, host and port, with the
// scheme's default port when none is written; the host is compared after
// lowercasing and IDNA conversion (canonicalHost).

import (
	"net/url"
	"strings"
	"unicode/utf8"
)

type origin struct{ scheme, host, port string }

func originOf(u *url.URL) (origin, bool) {
	h, ok := canonicalHost(u.Hostname())
	if !ok {
		return origin{}, false
	}
	return origin{scheme: strings.ToLower(u.Scheme), host: h, port: portOf(u)}, true
}

func portOf(u *url.URL) string {
	if p := u.Port(); p != "" {
		return strings.TrimLeft(p, "0")
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "ws":
		return "80"
	case "https", "wss":
		return "443"
	}
	return ""
}

// matches reports whether o is the allowed origin a.
func (a allowedOrigin) matches(o origin) bool {
	if a.scheme != o.scheme || a.port != o.port {
		return false
	}
	if !a.wildcard {
		return a.host == o.host
	}
	return strings.HasSuffix(o.host, "."+a.host) && len(o.host) > len(a.host)+1
}

// canonicalHost is host as the transport dials it, lowercased. net/http
// dials an ASCII host as written and converts any other through IDNA
// tables this kit does not carry, and a near miss there sends a
// credential to a different host. So a host outside ASCII has no form here
// (ok false): a request to one is judged as if to no allowed origin, and an
// allowed origin is written as its ASCII (Punycode) host, where IDNA
// conversion is lowercasing.
func canonicalHost(host string) (string, bool) {
	host = strings.TrimSuffix(host, ".")
	if host == "" || !isASCII(host) {
		return "", false
	}
	return strings.ToLower(host), true
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}
