// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// The classes the kit4rv Codex round 2 left open (.review/kit4rv-codex-r2.md),
// closed as the owner decided on 2026-10-31 (.review/kit5-decisions.md): the
// outbound transport fails closed instead of being patched (decision 2), the
// matcher and config members are fixed as classes (decision 3), and the
// run-decoder every-end reading is a named reading bound (decision 1).

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

// Class proxy-selection-rechecked (contract 2(c); decision 2a): the proxy
// the kit resolved and checked is the one the transport dials. A Proxy
// callback that answers differently the second time cannot move the
// request to a proxy the kit never judged.
func TestKit5ProxyPinned(t *testing.T) {
	px, hits := tcpProxy(t)
	resetForTest(Declarations{Origins: []Origin{{Source: "minted:t", Allowed: []string{"https://api.example"}}}})
	Hold("minted:t", "secret!!xy")
	var calls int64
	tr := &http.Transport{
		Proxy: func(*http.Request) (*url.URL, error) {
			if atomic.AddInt64(&calls, 1) == 1 {
				return &url.URL{Scheme: "https", Host: "api.example"}, nil // checked: the credential's own origin
			}
			return &url.URL{Scheme: "http", Host: px}, nil // never checked
		},
		ProxyConnectHeader: http.Header{"X-Debug": {"secret!!xy"}},
	}
	_ = get(&http.Client{Transport: Transport(tr)}, "https://api.example/")
	if *hits != 0 {
		t.Errorf("the transport dialed a proxy the kit never checked (hits=%d)", *hits)
	}
	if calls != 1 {
		t.Errorf("the Proxy callback ran %d times; the kit resolves it once and pins it", calls)
	}
}

// Class proxy-credentials-unheld (contract 1, 2(c); decision 2b): a base
// transport whose proxy headers or TLS connections the kit cannot see is
// refused with an error before any byte is sent, so those members are
// removed rather than checked.
func TestKit5RefusedTransportMembers(t *testing.T) {
	srv, hits := testServer(t, false, nil)
	resetForTest(Declarations{})
	tlsDial := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
	}
	for name, tr := range map[string]*http.Transport{
		"GetProxyConnectHeader": {DialContext: dialTo(t, srv),
			GetProxyConnectHeader: func(context.Context, *url.URL, string) (http.Header, error) { return nil, nil }},
		"DialTLSContext": {DialContext: dialTo(t, srv), DialTLSContext: tlsDial},
		"DialTLS": {DialContext: dialTo(t, srv),
			DialTLS: func(network, addr string) (net.Conn, error) { return tlsDial(context.Background(), network, addr) }},
	} {
		before := *hits
		err := get(&http.Client{Transport: Transport(tr)}, srv.URL)
		if !errors.Is(err, errRefusedTransport) {
			t.Errorf("%s: not refused (err=%v)", name, err)
		}
		if *hits != before {
			t.Errorf("%s: the request reached the server", name)
		}
	}
	// Codex's input: the callback's Proxy-Authorization never goes out.
	px, phits := tcpProxy(t)
	tr := &http.Transport{
		Proxy: http.ProxyURL(&url.URL{Scheme: "http", Host: px}),
		GetProxyConnectHeader: func(context.Context, *url.URL, string) (http.Header, error) {
			return http.Header{"Proxy-Authorization": {"Bearer secret!!"}}, nil
		},
	}
	if err := get(&http.Client{Transport: Transport(tr)}, "https://api.example/"); !errors.Is(err, errRefusedTransport) {
		t.Errorf("GetProxyConnectHeader with a proxy: not refused (err=%v)", err)
	}
	if *phits != 0 {
		t.Error("GetProxyConnectHeader with a proxy: the CONNECT reached the proxy")
	}
	// A proxy the kit cannot read the wire of (SOCKS) is refused too.
	socks := &http.Transport{Proxy: http.ProxyURL(&url.URL{Scheme: "socks5", Host: px})}
	if err := get(&http.Client{Transport: Transport(socks)}, "http://target.example/"); !errors.Is(err, errRefusedTransport) {
		t.Errorf("socks5 proxy: not refused (err=%v)", err)
	}
	if *phits != 0 {
		t.Error("socks5 proxy: the handshake reached the proxy")
	}
}

// Class proxy-credentials-unheld (contract 1, 2(c); decision 2c): every auth
// value in ProxyConnectHeader is held whatever the case of its key, so a
// plain-http proxy that would carry it is refused and it is caught at every
// other sink.
func TestKit5ProxyConnectHeaderAnyCase(t *testing.T) {
	px, hits := tcpProxy(t)
	resetForTest(Declarations{})
	tr := &http.Transport{
		Proxy: http.ProxyURL(&url.URL{Scheme: "http", Host: px}),
		ProxyConnectHeader: http.Header{
			"proxy-authorization": {"Bearer lc-secret-5150"},
			"authorization":       {"Basic " + base64.StdEncoding.EncodeToString([]byte("pu:pw-secret-7711"))},
			"cookie":              {"sid=cookie-secret-3131"},
		},
	}
	if err := get(&http.Client{Transport: Transport(tr)}, "https://api.example/"); err == nil {
		t.Error("lowercase proxy auth keys went to a plain-http proxy")
	}
	if *hits != 0 {
		t.Errorf("the CONNECT reached the proxy (hits=%d)", *hits)
	}
	for _, v := range []string{"lc-secret-5150", "pw-secret-7711", "cookie-secret-3131"} {
		if !isHeld(v) {
			t.Errorf("%q in ProxyConnectHeader was never held", v)
		}
	}
}

// Class proxy-origin-unchecked (contract 2(b), 2(c); decision 2d): the
// connection to an https proxy, whichever dialer Go uses for it, and the
// CONNECT unit with the headers Go adds (User-Agent), are judged against
// the proxy's origin, not the destination's.
func TestKit5HTTPSProxyJudgedAsProxy(t *testing.T) {
	proxy, hits := testServer(t, true, nil) // an https proxy: counts the CONNECTs it is sent
	resetForTest(Declarations{Origins: []Origin{{Source: "minted:t", Allowed: []string{"https://api.example"}}}})
	Hold("minted:t", "zGo-http-client/1.1")
	pu, _ := url.Parse(proxy.URL)
	tr := &http.Transport{
		Proxy:           http.ProxyURL(pu),
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // test proxy
	}
	if err := get(&http.Client{Transport: Transport(tr)}, "https://api.example/"); err == nil {
		t.Error("the CONNECT's default User-Agent went to an https proxy off the value's origin")
	}
	if *hits != 0 {
		t.Errorf("the https proxy received the CONNECT (hits=%d)", *hits)
	}
}

// Class charset-start-boundary-loss (contract 3, Candidates, Charsets and
// Numbers; decision 3): a charset reading starts at every byte offset, so a
// number spelling of a short digit-only value counts anywhere in it, and a
// UTF-16 or UTF-32 reading's bytes count a short value anywhere too.
func TestKit5CharsetAnyOffset(t *testing.T) {
	holding(t, "1000")
	for name, u := range map[string][]byte{
		"cp037 x1e3 (Codex)": {0xa7, 0xf1, 0x85, 0xf3},
		"cp037 x1,000":       {0xa7, 0xf1, 0x6b, 0xf0, 0xf0, 0xf0},
		"UTF-16BE x1e3":      []byte("\x00x\x001\x00e\x003"),
		"UTF-16BE x1,000":    []byte("\x00x\x001\x00,\x000\x000\x000"),
		"UTF-32LE x1.0e3":    utf32le("x1.0e3"),
	} {
		if !withheld(u) {
			t.Errorf("%s: a spelling of 1000 at a charset offset released", name)
		}
	}
	holding(t, "abcd")
	for name, u := range map[string][]byte{
		"UTF-16BE xabcd (Codex)": []byte("\x00x\x00a\x00b\x00c\x00d"),
		"UTF-16LE xabcd":         []byte("x\x00a\x00b\x00c\x00d\x00"),
		"UTF-32BE xabcd":         []byte("\x00\x00\x00x\x00\x00\x00a\x00\x00\x00b\x00\x00\x00c\x00\x00\x00d"),
	} {
		if !withheld(u) {
			t.Errorf("%s: abcd at a wide reading's offset released", name)
		}
	}
}

func utf32le(s string) []byte {
	var b []byte
	for _, r := range s {
		b = append(b, byte(r), byte(r>>8), byte(r>>16), byte(r>>24))
	}
	return b
}

// Class config-reader-drops-values (contract 1; decision 3): YAML node
// properties (a tag, an anchor, in any order) mark the node after them
// wherever its scalar is: on the following line after properties on their
// own line, after blank and comment lines, or after properties in any
// order. A tagged mapping's children are still read as keys.
func TestKit5ConfigPendingTag(t *testing.T) {
	for _, tc := range []struct{ content, field, value string }{
		{"token:\n  !!str\n  ???\n", "token", "???"}, // Codex's input
		{"token:\n  !!str\n\n  # note\n  ???\n", "token", "???"},
		{"token:\n  &a\n  ???\n", "token", "???"},
		{"token:\n  !!str &a ???\n", "token", "???"},
		{"token:\n  &a !!str ???\n", "token", "???"},
		{"token: &a\n  ???\n", "token", "???"},
		{"token: &a !!str\n  ???\n", "token", "???"},
		{"token:\n  !!str\n  \"?!?\"\n", "token", "?!?"},
		{"auth: !!map\n  password: hunter2-x\n", "password", "hunter2-x"},
		{"auth:\n  !!map\n  password: hunter2-y\n", "password", "hunter2-y"},
	} {
		gatherString(t, Declarations{ConfigFields: []string{tc.field}}, "c.yaml", tc.content)
		if !isHeld(tc.value) {
			t.Errorf("%q: %q was dropped", tc.content, tc.value)
		}
	}
}

// Class outbound-envelope-unchecked (contract 2(b), 2(c), 4, 5; decision 3):
// the outbound unit is the request as Go writes it, CRLF and the body after
// the blank line that ends the head, so a piece spanning that terminator is
// caught, on the wire and inside a CONNECT tunnel alike.
func TestKit5OutboundTerminatorPiece(t *testing.T) {
	srv, hits := testServer(t, false, nil)
	resetForTest(Declarations{})
	Hold("minted:t", "!e\r\n\r\nabc")
	c := &http.Client{Transport: Transport(&http.Transport{})}
	if _, err := c.Post(srv.URL, "text/plain", strings.NewReader("abc")); err == nil {
		t.Error("direct: a piece spanning the head terminator and the body went out")
	}
	if *hits != 0 {
		t.Errorf("direct: the request reached the server (hits=%d)", *hits)
	}
	px, phits := tcpProxy(t)
	resetForTest(Declarations{})
	Hold("minted:t", "!e\r\n\r\nabc")
	tc := &http.Client{Transport: Transport(&http.Transport{Proxy: http.ProxyURL(&url.URL{Scheme: "http", Host: px})})}
	if _, err := tc.Post("https://endpoint.example/", "text/plain", strings.NewReader("abc")); err == nil {
		t.Error("tunnel: a piece spanning the head terminator and the body went out")
	}
	if *phits != 0 {
		t.Errorf("tunnel: the CONNECT reached the proxy (hits=%d)", *phits)
	}
}

// Decision 4, contract section 6 item 8 (tracked, never blocking): the
// Punycode Host of the request inside an https CONNECT tunnel. The owner
// ruled it advisory because the kit imports no IDNA encoder. The kit now
// renders the request with Go's own Request.Write, which writes the
// Punycode Host, so this input is caught and the test runs rather than
// skips (.review/kit5-decisions.md); a miss here is still P2 by item 8.
func TestKit5TunnelPunycodeHost(t *testing.T) {
	px, hits := tcpProxy(t)
	resetForTest(Declarations{})
	Hold("minted:t", "xxn--bcher-kva")
	req, _ := http.NewRequest("GET", "https://endpoint.example/", nil)
	req.Host = "bücher.example"
	c := &http.Client{Transport: Transport(&http.Transport{Proxy: http.ProxyURL(&url.URL{Scheme: "http", Host: px})})}
	if resp, err := c.Do(req); err == nil {
		resp.Body.Close()
		t.Error("the tunneled Punycode Host went out unchecked")
	}
	if *hits != 0 {
		t.Errorf("the CONNECT reached the proxy (hits=%d)", *hits)
	}
}

// Decision 1 (contract 3, Reading bounds, runDecoderSpans): a run decoder
// reads its maximal run at every alignment; it does not read every (start,
// end) span so that a later greedy decoder sees every possible end. These
// two kit4rv round-2 inputs reach the held value only through such an end.
func TestKit5RunDecoderSpansBound(t *testing.T) {
	t.Skip("out of scope by the reading bound runDecoderSpans (contract 3, Reading bounds; owner amendment 2026-10-31)")
	holding(t, "secret!¬")
	if !withheld([]byte("c2VjcmV0ISZub3Rpbjs=")) { // the prefix span reads "secret!&not"
		t.Error("base64 prefix -> &not released")
	}
	holding(t, "abcdefg\x04")
	if !withheld([]byte("XDE0MWJjZGVmZ1wwNDE=")) { // the prefix span reads "\\141bcdefg\\04"
		t.Error("base64 prefix -> mixed octal escapes released")
	}
}

// Class config-reader-drops-values (contract 1), the members found before
// the kit5 review: node properties at the start of a block item are passed
// over before the word, so a comment after them leaves the scalar pending,
// a verbatim tag is one property, and a tagged or anchored key stays a key.
func TestKit5ConfigItemProperties(t *testing.T) {
	for _, tc := range []struct{ content, field, value string }{
		{"token:\n  !!str # note\n  ???\n", "token", "???"},
		{"token: !!str # note\n  ???\n", "token", "???"},
		{"token:\n  &a # note\n  ???\n", "token", "???"},
		{"token:\n  !!str\n  # c\n  ???\n", "token", "???"},
		{"token:\n  !<tag:yaml.org,2002:str>\n  ???\n", "token", "???"},
		{"token:\n  !!str\t???\n", "token", "???"},
		{"- token:\n    !!str\n    ???\n", "token", "???"},
		{"a:\n  token:\n    !!str\n    ???\n", "token", "???"},
		{"token: !!str\n  '?!?'\n", "token", "?!?"},
		{"token:\n  !!str\n  ?!? # c\n", "token", "?!?"},
		{"token: !!str\n  ? x\n", "token", "? x"},
		{"!!str token: ???\n", "token", "???"},
		{"&k token: ???\n", "token", "???"},
		{"a:\n  !!str token: ???\n", "token", "???"},
		{"token: !<tag:yaml.org,2002:str> ???\n", "token", "???"},
		{"token:\n  !<tag:yaml.org,2002:str> ???\n", "token", "???"},
		{"token:\n  !!str\n  &a\n  ???\n", "token", "???"},
		{"token: !!str &a # c\n\n  ???\n", "token", "???"},
		{"base: &b\n  token: ???\nother: *b\n", "token", "???"},
	} {
		gatherString(t, Declarations{ConfigFields: []string{tc.field}}, "c.yaml", tc.content)
		if !isHeld(tc.value) {
			t.Errorf("%q: %q was dropped", tc.content, tc.value)
		}
	}
}

// Class outbound-envelope-unchecked (contract 2(c), 4, 5): the wire check
// reads HTTP/1.1 text, so the clone speaks HTTP/1.1 whatever Protocols the
// base transport sets. Over HTTP/2 the head goes out as HPACK frames after
// the preface, which the wire check passes, and Go's HTTP/2 client adds a
// User-Agent the rendered HTTP/1.1 unit does not carry.
func TestKit5ProtocolsHTTP1Only(t *testing.T) {
	var proto int64
	var ua atomic.Value
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.StoreInt64(&proto, int64(r.ProtoMajor))
		ua.Store(r.UserAgent())
	}))
	srv.Config.Protocols = new(http.Protocols)
	srv.Config.Protocols.SetHTTP1(true)
	srv.Config.Protocols.SetUnencryptedHTTP2(true)
	srv.Start()
	t.Cleanup(srv.Close)
	resetForTest(Declarations{})
	Hold("minted:t", "client/2.0")
	p := new(http.Protocols)
	p.SetUnencryptedHTTP2(true)
	_ = get(&http.Client{Transport: Transport(&http.Transport{Protocols: p})}, srv.URL)
	if got, _ := ua.Load().(string); strings.Contains(got, "client/2") {
		t.Errorf("the request went out over HTTP/%d carrying %q", atomic.LoadInt64(&proto), got)
	}
}
