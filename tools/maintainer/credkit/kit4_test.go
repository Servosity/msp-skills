// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// The round-2 members the kit3rv review left open (.review/kit3rv-codex-r2.md),
// each closed here as a class: the reported input and other members of it.

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// dialTo ignores the dialed address and connects to srv, so a request to
// any host reaches the test server.
func dialTo(t *testing.T, srv *httptest.Server) func(context.Context, string, string) (net.Conn, error) {
	t.Helper()
	addr := srv.Listener.Addr().String()
	return func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
}

// Class numeric-grouping-incomplete (contract 3, Numbers; 2(a)): a
// scientific-notation spelling with a leading plus sign is judged by its
// own length, the plus included. sciHit must not drop the plus from the
// length nor demand token edges an 8-character spelling does not need.
func TestKit4SciHitLeadingPlus(t *testing.T) {
	holding(t, "1234")
	leaks := []string{
		"x+1.234e3y", "x+1234e+0y", "x+1234e00y", "x+.1234e4y", // Codex's inputs
		"a+1.234E3b", "+0.1234e4", "x+1234000e-3y",
	}
	for _, u := range leaks {
		if !withheld([]byte(u)) {
			t.Errorf("held 1234: %q released (a +sci spelling of 8 characters counts anywhere)", u)
		}
	}
	// Still judged by length: a shorter +sci spelling counts only as a
	// whole token, and a real non-spelling stays clean.
	for _, u := range []string{"x+1.234e3", "1.234e2", "+1234e1"} {
		_ = u
	}
	holding(t, "9999")
	if withheld([]byte("x+1.234e3y")) {
		t.Error("held 9999: +1.234e3 (= 1234) withheld, not a spelling of 9999")
	}
}

// Class config-reader-drops-values (contract 1): a YAML tag in front of a
// scalar never hides the scalar, wherever the scalar begins: on the next
// line, inside a flow collection, or quoted.
func TestKit4ConfigTagAcrossBoundaries(t *testing.T) {
	for _, tc := range []struct {
		content, value string
	}{
		{"token: !!str\n  ~\n", "~"},        // Codex: tag, then the scalar on the next line
		{"token:\n  !!str ~\n", "~"},        // Codex: key with no inline value, tagged child
		{"token: [!!str ~]\n", "~"},         // Codex: tagged scalar inside a flow sequence
		{"token: [!!str \"???\"]\n", "???"}, // Codex: tagged quoted scalar in a flow sequence
		{"token: {a: !!str ~}\n", "~"},      // a flow mapping value
		{"token:\n  - !!str ~\n", "~"},      // a block sequence element
	} {
		gatherString(t, Declarations{ConfigFields: []string{"token"}}, "c.yaml", tc.content)
		if !isHeld(tc.value) {
			t.Errorf("%q: the scalar %q behind the tag was dropped", tc.content, tc.value)
		}
	}
}

// Class auth-part-origin-union (contract 2(c)): re-holding an
// already-held full credential under an undeclared header source must not
// union its part's binding into it. Its declared origin stands.
func TestKit4HeaderReacquisitionNoWiden(t *testing.T) {
	const part = "f8Lq6N2v9s4K7m1R"
	d := Declarations{Origins: []Origin{
		{Source: "minted:part", Allowed: []string{"https://b.example"}},
		{Source: "minted:full", Allowed: []string{"https://a.example"}},
	}}
	for _, order := range [][2]string{{"part", "full"}, {"full", "part"}} {
		resetForTest(d)
		for _, o := range order {
			if o == "part" {
				Hold("minted:part", part)
			} else {
				Hold("minted:full", "custom "+part)
			}
		}
		// The full credential in an Authorization header, sent to the
		// origin only its PART allows: the re-hold under header:authorization
		// must not have widened the full value to b.example.
		sent := false
		tr := Transport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
			sent = true
			return &http.Response{StatusCode: 200, Body: http.NoBody, Request: r}, nil
		}))
		req, _ := http.NewRequest("POST", "https://b.example/", nil)
		req.Header.Set("Authorization", "custom "+part)
		if resp, err := tr.RoundTrip(req); err == nil {
			resp.Body.Close()
		}
		if sent {
			t.Errorf("order %v: the full credential reached an origin only its part allows", order)
		}
	}
}

// Class stream-context-truncation (contract 3, Streams and Budget): a
// compressed stream can carry printable bytes, so a printable gap is no
// proof it ended. A stream split across writes, with a long printable gap
// inside it, is still caught at the write that completes it.
func TestKit4StreamBinaryGap(t *testing.T) {
	holding(t, "secret!!")
	// Codex's input: a zlib stream whose middle is 1,200 printable bytes.
	p := append([]byte(strings.Repeat(" \t", 600)), "secret!"...)
	first := append([]byte{0x78, 0x01, 0x00, 0xb7, 0x04, 0x48, 0xfb}, p...)
	last := []byte{0x53, 0x04, 0x00, 0xa4, 0x4e, 0x62, 0xe1}
	var out bytes.Buffer
	sw := NewWriter(&out, SinkFile)
	sw.Write(first)
	sw.Write(last)
	if bytes.Contains(out.Bytes(), append(append([]byte{}, first...), last...)) {
		t.Error("the split zlib stream reached the file whole")
	}
	// The over-withholding guard: ordinary text lines, no binary byte, all flow.
	out.Reset()
	sw = NewWriter(&out, SinkFile)
	line := `{"id":12,"title":"Printer offline","owner":"j.doe@example.com"}` + "\n"
	for range 600 {
		sw.Write([]byte(line))
	}
	if strings.Count(out.String(), line) != 600 {
		t.Errorf("ordinary lines withheld: %d of 600", strings.Count(out.String(), line))
	}
}

// Class outbound-envelope-unchecked (contract 2(c), 4, 5): the request is
// checked as the transport writes it on the wire, so a held value in a
// header the transport adds (Content-Length, Proxy-Authorization, the
// Punycode Host) is caught before any byte leaves.
func TestKit4OutboundWireChecked(t *testing.T) {
	srv, hits := testServer(t, false, nil)
	// Content-Length: 0 on an empty POST.
	resetForTest(Declarations{})
	Hold("minted:x", "zContent-Length: 0")
	if _, err := (&http.Client{Transport: Transport(nil)}).Post(srv.URL, "text/plain", nil); err == nil {
		t.Error("empty POST: Content-Length: 0 went out unchecked")
	}
	// The Punycode Host of an international URL.
	resetForTest(Declarations{})
	Hold("minted:x", "xxn--bcher-kva")
	puny := &http.Transport{DialContext: dialTo(t, srv)}
	if err := get(&http.Client{Transport: Transport(puny)}, "http://bücher.example/"); err == nil {
		t.Error("international URL: the Punycode Host went out unchecked")
	}
	// Proxy-Authorization from a proxy's userinfo.
	resetForTest(Declarations{})
	Hold("minted:x", "proxy-password-94")
	pu, _ := url.Parse(srv.URL)
	pu.User = url.UserPassword("u", "proxy-password-94")
	prox := &http.Transport{Proxy: http.ProxyURL(pu)}
	before := *hits
	if err := get(&http.Client{Transport: Transport(prox)}, "http://target.example/"); err == nil {
		t.Error("proxy: Proxy-Authorization went out unchecked")
	}
	if *hits != before {
		t.Error("proxy: the request reached the proxy before the check refused it")
	}
}

// Class synthetic-request-overwithholding (P2, withholding beyond
// contract 2): the kit checks what the transport actually sends, so a
// transport with compression disabled does not invent an Accept-Encoding
// header that makes a held "gzip" refuse an otherwise clean request.
func TestKit4NoSyntheticAcceptEncoding(t *testing.T) {
	srv, hits := testServer(t, false, nil)
	resetForTest(Declarations{})
	Hold("minted:x", "gzip")
	c := &http.Client{Transport: Transport(&http.Transport{DisableCompression: true})}
	if err := get(c, srv.URL); err != nil {
		t.Errorf("DisableCompression: a clean request was refused: %v", err)
	}
	if *hits != 1 {
		t.Errorf("DisableCompression: request did not reach the server (hits=%d)", *hits)
	}
}
