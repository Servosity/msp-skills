// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// The classes the kit4rv Codex round 1 raised (.review/kit4rv-codex-r1.md),
// each closed as a class: the reported input and other members of it.

import (
	"net"
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"
)

// tcpProxy is a bare TCP listener standing in for an HTTP proxy. The wire
// check refuses the CONNECT or forwarded request head before the proxy must
// answer, so the listener only needs to accept and count connections.
func tcpProxy(t *testing.T) (addr string, hits *int64) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	var n int64
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			atomic.AddInt64(&n, 1)
			c.Close()
		}
	}()
	return l.Addr().String(), &n
}

// Class decoder-length-pruning (contract 3, Character-code lists; 2a): a
// code-list token with leading zeros is judged by its value, not dropped
// for its digit count. A code point is at most seven digits; zeros pad.
func TestKit4rvCodeListLeadingZeros(t *testing.T) {
	holding(t, "secret!!")
	for _, u := range []string{
		"000000115,000000101,000000099,000000114,000000101,000000116,000000033,000000033", // Codex's input
		"0x073,0x065,0x063,0x072,0x065,0x074,0x021,0x021",
		"00115 00101 00099 00114 00101 00116 00033 00033",
	} {
		if !withheld([]byte(u)) {
			t.Errorf("%q released (a leading-zero code list of secret!!)", u)
		}
	}
	// Still bounded: a genuinely oversized token is not a code point.
	holding(t, "abcd")
	if withheld([]byte("999999999999999,97")) {
		t.Error("an oversized token was read as a code point")
	}
}

// Class run-decoder-span-pruning (contract 3, Candidates): a run decoder's
// span to every end, and a code list's every sub-span, are candidates, so a
// value reachable only through an earlier end, or a short value anywhere in
// a decoded list or charset reading, counts.
func TestKit4rvRunDecoderEveryEnd(t *testing.T) {
	// base64 span whose last partial group ends an octal escape short.
	holding(t, "secret!\x04")
	if !withheld([]byte("c2VjcmV0IVwwNDE=")) {
		t.Error("base64 -> octal escape at an earlier end released")
	}
	// A short value inside a decoded code list (Codex: only x..y was queued).
	holding(t, "abcd")
	if !withheld([]byte("120,97,98,99,100,121")) {
		t.Error("abcd inside a decoded code list released")
	}
}

// Class charset-start-boundary-loss (contract 3, Candidates and Charsets):
// a charset reading starts at every byte offset, so a short value at a
// non-zero offset of a reading counts.
func TestKit4rvCharsetEveryOffset(t *testing.T) {
	holding(t, "abcd")
	// IBM Code Page 037 reads 0xa7,0x81..0x84 as "xabcd"; the offset-1
	// reading "abcd" is its own candidate.
	if !withheld([]byte{0xa7, 0x81, 0x82, 0x83, 0x84}) {
		t.Error("abcd at charset offset 1 released")
	}
}

// Class joined-values-incomplete (contract 3, Joined values): a number out
// of float64 range does not error the JSON walk and cut the join short.
func TestKit4rvJoinedBigNumber(t *testing.T) {
	holding(t, "secret!!")
	for _, u := range []string{
		`["secr",1e1000,"et!!"]`, // Codex's input
		`["secr",1e2000,"et","!!"]`,
		`{"a":"secr","n":1e1000,"b":"et!!"}`,
	} {
		if !withheld([]byte(u)) {
			t.Errorf("%q released (joined string values spell secret!!)", u)
		}
	}
}

// Class config-reader-drops-values (contract 1): a YAML tag's scalar is held
// even when a blank or comment line separates it from the tag.
func TestKit4rvConfigTagAcrossBlankLines(t *testing.T) {
	for _, tc := range []struct{ content, value string }{
		{"token: !!str\n\n  ???\n", "???"},     // Codex's input
		{"token: !!str\n  # note\n  ~\n", "~"}, // a comment between
		{"token: !!str\n\n\n  secret-xyz\n", "secret-xyz"},
	} {
		gatherString(t, Declarations{ConfigFields: []string{"token"}}, "c.yaml", tc.content)
		if !isHeld(tc.value) {
			t.Errorf("%q: the scalar %q behind the tag was dropped", tc.content, tc.value)
		}
	}
}

// Class proxy-origin-unchecked (contract 2(c)): the head the proxy receives
// is judged against the proxy's own origin, so a destination credential in
// a CONNECT's Proxy-Authorization does not reach the proxy unchecked.
func TestKit4rvProxyOriginChecked(t *testing.T) {
	px, hits := tcpProxy(t)
	resetForTest(Declarations{Origins: []Origin{{Source: "minted:t", Allowed: []string{"https://api.example"}}}})
	Hold("minted:t", "secret!!xy")
	tr := &http.Transport{
		Proxy:              http.ProxyURL(&url.URL{Scheme: "http", Host: px}),
		ProxyConnectHeader: http.Header{"Proxy-Authorization": {"Bearer secret!!xy"}},
	}
	if err := get(&http.Client{Transport: Transport(tr)}, "https://api.example/"); err == nil {
		t.Error("a destination credential in the CONNECT Proxy-Authorization reached the proxy")
	}
	if *hits > 1 {
		t.Errorf("the proxy received more than the refused connection (hits=%d)", *hits)
	}
}

// Class proxy-credentials-unheld (contract 1, 2(c)): a proxy's own userinfo
// is held, so it is caught at every sink, and a plain-http proxy that would
// carry it in cleartext is refused (a credential over plain http).
func TestKit4rvProxyCredentialsHeld(t *testing.T) {
	px, _ := tcpProxy(t)
	resetForTest(Declarations{})
	pu := &url.URL{Scheme: "http", Host: px, User: url.UserPassword("u", "proxy-password-94")}
	tr := &http.Transport{Proxy: http.ProxyURL(pu)}
	err := get(&http.Client{Transport: Transport(tr)}, "http://target.example/")
	if err == nil {
		t.Error("the proxy password went to a plain-http proxy in cleartext")
	}
	if !isHeld("proxy-password-94") {
		t.Error("the proxy password was never held")
	}
}

// Class outbound-envelope-unchecked (contract 2(b), 2(c), 4, 5): a header
// the transport adds to the tunneled request, which the wire check cannot
// see through the proxy's TLS, is checked by the config-aware synthesis.
func TestKit4rvTunnelAddedHeaderChecked(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:t", "zAccept-Encoding: gzip")
	tr := &http.Transport{Proxy: http.ProxyURL(&url.URL{Scheme: "http", Host: "proxy.invalid:8080"})}
	if err := get(&http.Client{Transport: Transport(tr)}, "https://endpoint.example/"); err == nil {
		t.Error("the tunneled Accept-Encoding the transport adds went unchecked")
	}
}
