// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// Contract 2(c), and contract 1's held-before-the-sink rule for request and
// response credentials. Every outbound request, and every redirect hop of
// one, passes through Transport: the credentials it carries (auth headers,
// cookies, Basic parts, declared headers and form or JSON fields) are held,
// then its URL, headers and body are checked as one unit against every held
// value that may not go to its origin. Over plain http, and on a redirect hop
// to a different origin, no held value may go at all. A refused request is
// never sent. Set-Cookie values and declared response fields are held before
// the response is returned to the program.

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
)

type transport struct{ inner http.RoundTripper }

// errRefusedTransport is the refusal of a base transport whose wire the kit
// cannot see, returned before any byte is sent: one that sets
// GetProxyConnectHeader (a proxy header chosen per dial), DialTLSContext or
// DialTLS (a TLS connection the kit cannot read inside), or a proxy that is
// not http or https (SOCKS, whose handshake is not HTTP). No connector sets
// any of them; the kit refuses them rather than checking them.
var errRefusedTransport = errors.New("credkit: the outbound guard refuses this transport: GetProxyConnectHeader, DialTLSContext, DialTLS and proxies other than http and https are not supported")

// Transport wraps inner (http.DefaultTransport when nil) in the outbound
// guard. Wrap the transport of every HTTP client the program builds:
//
//	client := &http.Client{Transport: credkit.Transport(base)}
func Transport(inner http.RoundTripper) http.RoundTripper {
	if inner == nil {
		inner = http.DefaultTransport
	}
	return &transport{inner: inner}
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	// An *http.Transport is cloned once, and the clone is what the kit
	// checks and what sends: its config cannot change in between.
	base, isTransport := t.inner.(*http.Transport)
	var c *http.Transport
	if isTransport {
		c = base.Clone()
		if c.GetProxyConnectHeader != nil || c.DialTLSContext != nil || c.DialTLS != nil {
			if req.Body != nil {
				_ = req.Body.Close()
			}
			return nil, errRefusedTransport
		}
	}
	var body []byte
	if req.Body != nil && req.Body != http.NoBody {
		b, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			// The bytes read before the error are held as the request's
			// would be (kit13rv: a read that ends in an error skipped
			// holding them, and the program may print what it sent).
			part := req.Clone(req.Context())
			part.Body, part.ContentLength = io.NopCloser(bytes.NewReader(b)), int64(len(b))
			holdRequest(part, b)
			markUnknown() // kit14rv: what it did not read is unknown
			return nil, err
		}
		body = b
	}
	out := req.Clone(req.Context())
	if req.Body != nil && req.Body != http.NoBody {
		out.Body = io.NopCloser(bytes.NewReader(body))
		out.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
		out.ContentLength = int64(len(body))
	}
	if !holdRequest(out, body) {
		markUnknown()                       // kit14rv: the values a coding the kit cannot read carries are unknown
		return nil, withheldError{SinkHTTP} // a declared coding the kit cannot read
	}
	o, ok := originOf(out.URL)
	strict := !ok || (o.scheme != "https" && o.scheme != "wss")
	if req.Response != nil && req.Response.Request != nil {
		if prev, ok := originOf(req.Response.Request.URL); !ok || prev != o {
			strict = true
		}
	}
	// A proxy is a second recipient. The kit resolves it once and pins it
	// on the clone, so the proxy it judges is the one the clone dials. Its
	// own credentials are held (contract 1), so the kit knows them for every
	// sink, before the matchers are built.
	var pu *url.URL
	if isTransport && c.Proxy != nil {
		// The callback gets a copy: what it sets on the request never
		// reaches the wire unheld (kit15rv: send what you read).
		p, err := c.Proxy(out.Clone(out.Context()))
		if err != nil {
			return nil, err
		}
		pu = p
	}
	var proxyM *matcher
	var po origin
	if isTransport {
		c.Proxy = nil
		if pu != nil {
			if pu.Scheme != "http" && pu.Scheme != "https" {
				return nil, errRefusedTransport
			}
			pinned := pu
			c.Proxy = func(*http.Request) (*url.URL, error) { return pinned, nil }
			holdProxyCreds(pu, c.ProxyConnectHeader)
			po, ok = originOf(pu)
			if !ok {
				return nil, withheldError{SinkHTTP}
			}
			var unknown bool
			if proxyM, unknown = originMatcher(po, po.scheme != "https"); unknown {
				return nil, withheldError{SinkHTTP}
			}
		}
	}
	// A program's own RoundTripper puts the request out where the kit cannot
	// see: the barrier spans it from before the check below to the end of
	// the inner RoundTrip, and a change to the held set waits for it unless
	// the request as judged would not be withheld now (held.go settle;
	// kit15rv rounds 4 and 5: begun after the check, a value held during
	// the check was not waited for).
	var judgedReq atomic.Pointer[judged]
	end := func() {}
	if !isTransport {
		e := emitBeginIf(func() bool {
			j := judgedReq.Load()
			if j == nil {
				return true // not yet judged: waited for
			}
			m2, unknown := originMatcher(o, strict)
			return unknown || m2 != j.m && m2.check(j.unit) != clean
		})
		ended := false
		end = func() {
			if !ended {
				ended = true
				emitEnd(e)
			}
		}
		defer end()
	}
	m, unknown := originMatcher(o, strict)
	if unknown {
		return nil, withheldError{SinkHTTP}
	}
	// The request as the destination receives it: the URL, headers and body
	// as given, and for an *http.Transport the bytes Go writes (render). This
	// is where the request inside a CONNECT tunnel is judged, since the wire
	// check cannot see inside the TLS the proxy forwards.
	var wire []byte
	if isTransport {
		if wire, ok = render(out, body, c, "", false); !ok {
			return nil, withheldError{SinkHTTP}
		}
	}
	unit := requestUnit(out, body, wire) // as judged: re-judged later from these bytes
	if m.check(unit) != clean {
		return nil, withheldError{SinkHTTP}
	}
	judgedReq.Store(&judged{m, unit})
	// The proxy receives a unit of its own, judged against its origin: the
	// CONNECT for an https target, the whole request for an http one.
	tunnel := out.URL.Scheme == "https"
	if pu != nil {
		pu1, ok := proxyUnit(out, body, c, pu, tunnel)
		if !ok || proxyM.check(pu1) != clean {
			return nil, withheldError{SinkHTTP}
		}
	}
	inner := t.inner
	if isTransport {
		// Every connection the clone dials goes to the proxy when there is
		// one, whichever dialer Go uses for it, and to the target otherwise.
		connM, connBody := m, body
		if pu != nil {
			connM = proxyM
			if tunnel {
				connBody = nil // the CONNECT carries no body; the request is inside the tunnel
			}
		}
		// The connection re-judges the request when the held set changed
		// since these checks, under the sinks' barrier (held.go heldMu):
		// a value held while the request is on its way is not sent
		// (kit15rv round 3, held-set-update-race, blunt).
		rejudge := func() (*matcher, bool) {
			m2, unknown := originMatcher(o, strict)
			if unknown || m2 != m && m2.check(unit) != clean {
				return nil, false
			}
			if pu == nil {
				return m2, true
			}
			pm, unknown := originMatcher(po, po.scheme != "https")
			if unknown {
				return nil, false
			}
			if pm != proxyM {
				if pu1, ok := proxyUnit(out, body, c, pu, tunnel); !ok || pm.check(pu1) != clean {
					return nil, false
				}
			}
			return pm, true
		}
		inner = wireTransport(c, connM, connBody, rejudge)
	}
	var resp *http.Response
	var err error
	if isTransport {
		resp, err = inner.RoundTrip(out) // its connection re-judges each write (checkConn)
	} else {
		resp, err = inner.RoundTrip(out)
		end() // before the response is held: its values may settle
	}
	if err != nil {
		return nil, err
	}
	holdResponse(resp, o)
	return resp, nil
}

// judged is a request as the kit judged it, and the matcher it was judged
// with.
type judged struct {
	m    *matcher
	unit []byte
}

// holdProxyCreds holds a proxy's own credentials (contract 1): the user and
// password of its userinfo, and every auth value in ProxyConnectHeader
// whatever the case of its key, bound to the proxy's origin, so they are
// held at every sink and allowed only to the proxy itself.
func holdProxyCreds(pu *url.URL, connectHeader http.Header) {
	reg.Lock()
	defer reg.Unlock()
	var allowed []string
	if po, ok := originOf(pu); ok {
		allowed = []string{po.scheme + "://" + po.host + ":" + po.port}
	}
	if u := pu.User; u != nil {
		holdAllowedLocked("proxy-user", allowed, u.Username())
		if p, ok := u.Password(); ok {
			holdAllowedLocked("proxy-userinfo", allowed, p)
		}
	}
	holdHeaderAllowedLocked(connectHeader, allowed)
}

// proxyUnit is what the proxy receives, as Go writes it: for an https
// target, the CONNECT request (its line, Host and User-Agent, the
// ProxyConnectHeader and the Proxy-Authorization from the proxy userinfo);
// for an http target, the whole request in absolute form with that
// Proxy-Authorization. The CONNECT is built as Go's transport builds it,
// with the target written by Request.Write (the Punycode host and port).
func proxyUnit(req *http.Request, body []byte, c *http.Transport, pu *url.URL, tunnel bool) ([]byte, bool) {
	auth := ""
	if u := pu.User; u != nil {
		pass, _ := u.Password()
		auth = "Basic " + base64.StdEncoding.EncodeToString([]byte(u.Username()+":"+pass))
	}
	if !tunnel {
		return render(req, body, c, auth, true)
	}
	h := c.ProxyConnectHeader.Clone()
	if h == nil {
		h = http.Header{}
	}
	if auth != "" {
		h["Proxy-Authorization"] = []string{auth}
	}
	connect := &http.Request{Method: "CONNECT", URL: &url.URL{}, Header: h,
		Host: net.JoinHostPort(req.URL.Hostname(), portOf(req.URL))}
	var b bytes.Buffer
	if connect.Write(&b) != nil {
		return nil, false
	}
	return b.Bytes(), true
}

// render is req as Go's transport writes it to the wire, CRLF and all:
// Request.write, the code the transport runs (the request line, the Punycode
// Host, User-Agent, the transfer headers, the headers sorted), then the
// lines the transport adds from c before the blank line that ends the head
// (Accept-Encoding, Connection: close, and for a proxy Proxy-Authorization),
// then the body. absolute is the form an http proxy receives. It reports
// false when Go would not write the request at all.
func render(req *http.Request, body []byte, c *http.Transport, proxyAuth string, absolute bool) ([]byte, bool) {
	r := req.Clone(req.Context())
	if r.Body != nil && r.Body != http.NoBody {
		r.Body = io.NopCloser(bytes.NewReader(body))
	}
	extra := http.Header{}
	if !c.DisableCompression && r.Header.Get("Accept-Encoding") == "" && r.Header.Get("Range") == "" && r.Method != "HEAD" {
		extra["Accept-Encoding"] = []string{"gzip"}
	}
	conn := r.Header["Connection"]
	first := ""
	if len(conn) > 0 {
		first = conn[0]
	}
	// The clone the kit sends through has keep-alives off (wireTransport).
	if !r.Close && !hasToken(first, "close") && !(r.Header.Get("Upgrade") != "" && valuesHaveToken(conn, "upgrade")) {
		extra["Connection"] = []string{"close"}
	}
	if proxyAuth != "" {
		extra["Proxy-Authorization"] = []string{proxyAuth}
	}
	var b bytes.Buffer
	var err error
	if absolute {
		err = r.WriteProxy(&b)
	} else {
		err = r.Write(&b)
	}
	if err != nil {
		return nil, false
	}
	w := b.Bytes()
	i := bytes.Index(w, []byte("\r\n\r\n"))
	if i < 0 {
		return nil, false
	}
	var e bytes.Buffer
	_ = extra.Write(&e)
	return slices.Concat(w[:i+2], e.Bytes(), w[i+2:]), true
}

// hasToken is net/http's: whether v holds token, case-insensitively, at
// boundaries of space, tab or comma.
func hasToken(v, token string) bool {
	for sp := 0; sp+len(token) <= len(v); sp++ {
		end := sp + len(token)
		if (sp == 0 || isTokenBoundary(v[sp-1])) && (end == len(v) || isTokenBoundary(v[end])) &&
			strings.EqualFold(v[sp:end], token) {
			return true
		}
	}
	return false
}

func isTokenBoundary(b byte) bool { return b == ' ' || b == ',' || b == '\t' }

// valuesHaveToken is httpguts.HeaderValuesContainsToken: a comma-separated
// element of any value equals token, case-insensitively.
func valuesHaveToken(vs []string, token string) bool {
	for _, v := range vs {
		for _, e := range strings.Split(v, ",") {
			if strings.EqualFold(strings.Trim(e, " \t"), token) {
				return true
			}
		}
	}
	return false
}

// requestUnit is one outbound request as a unit: the URL, the host, the
// headers and trailers as the program set them, CRLF-separated, then wire,
// the request as the transport writes it (render), body included. With no
// wire (a custom http.RoundTripper, which adds nothing the kit can model)
// the body follows the blank line that ends the given head.
func requestUnit(req *http.Request, body, wire []byte) []byte {
	var b bytes.Buffer
	b.WriteString(req.Method + " " + req.URL.String() + "\r\n")
	b.WriteString(req.Method + " " + req.URL.RequestURI() + " HTTP/1.1\r\n")
	if req.Host != "" {
		b.WriteString("Host: " + req.Host + "\r\n")
	}
	b.WriteString("Host: " + req.URL.Host + "\r\n")
	for k := range req.Trailer {
		b.WriteString("Trailer: " + k + "\r\n")
	}
	_ = req.Header.Write(&b)
	for _, t := range req.Trailer {
		b.WriteString(strings.Join(t, "\r\n") + "\r\n")
	}
	b.WriteString("\r\n")
	if wire != nil {
		b.Write(wire)
	} else {
		b.Write(body)
	}
	return b.Bytes()
}

// wireTransport sets up c, the clone, so the kit sees the exact bytes it
// writes: HTTP/1.1 forced (Protocols too) and one request per connection, and every
// connection, plain or TLS, wrapped in a checkConn that checks the request
// head, with body after it, against m before any byte leaves (contract
// 2(c), 4, 5). A head that carries a value m holds is refused; nothing is
// written. TLS is the kit's own (DialTLSContext), so the kit reads the
// plaintext; a base transport with a TLS dialer of its own is refused
// before this (RoundTrip).
func wireTransport(c *http.Transport, m *matcher, body []byte, rejudge func() (*matcher, bool)) *http.Transport {
	c.DisableKeepAlives = true
	c.ForceAttemptHTTP2 = false
	c.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	// HTTP/1.1 only, whatever Protocols the base sets: HTTP/2 (h2 or h2c)
	// sends the head as HPACK frames the wire check cannot read.
	c.Protocols = new(http.Protocols)
	c.Protocols.SetHTTP1(true)
	if c.TLSClientConfig == nil {
		c.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	} else {
		c.TLSClientConfig = c.TLSClientConfig.Clone()
	}
	c.TLSClientConfig.NextProtos = []string{"http/1.1"}
	dial := c.DialContext
	if dial == nil && c.Dial != nil {
		d := c.Dial
		dial = func(_ context.Context, network, addr string) (net.Conn, error) { return d(network, addr) }
	}
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}
	c.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := dial(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		return &checkConn{Conn: conn, m: m, body: body, rejudge: rejudge}, nil
	}
	c.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		raw, err := dial(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		cfg := c.TLSClientConfig.Clone()
		if cfg.ServerName == "" && !cfg.InsecureSkipVerify {
			host, _, e := net.SplitHostPort(addr)
			if e != nil {
				host = addr
			}
			cfg.ServerName = host
		}
		tc := tls.Client(raw, cfg)
		if err := tc.HandshakeContext(ctx); err != nil {
			_ = raw.Close()
			return nil, err
		}
		return &checkConn{Conn: tc, m: m, body: body, rejudge: rejudge}, nil
	}
	return c
}

// checkConn checks the request head written to it, with body after it,
// against m before it reaches the wire. Writes are buffered until the head
// ends (CRLF CRLF); if the head and body carry a value m holds, the
// connection is closed and the write fails, so nothing leaks. body is the
// request body as given (nil for a CONNECT); the bytes that follow the head
// pass through once it is checked.
type checkConn struct {
	net.Conn
	m      *matcher
	body   []byte
	buf    []byte
	done   bool
	failed bool
	// rejudge is every check of the request against the held set now, and
	// the connection's matcher then (RoundTrip); false is withheld.
	rejudge func() (*matcher, bool)
}

func (c *checkConn) Write(p []byte) (int, error) {
	if c.failed {
		return 0, withheldError{SinkHTTP}
	}
	defer emitEnd(emitBegin()) // the barrier, to the end of the write (held.go heldMu)
	if c.rejudge != nil {
		m, ok := c.rejudge()
		if !ok {
			c.failed = true
			_ = c.Conn.Close()
			return 0, withheldError{SinkHTTP}
		}
		c.m = m
	}
	if c.done {
		return c.Conn.Write(p)
	}
	c.buf = append(c.buf, p...)
	i := bytes.Index(c.buf, []byte("\r\n\r\n"))
	if i < 0 {
		if len(c.buf) > maxUnit { // a head this large is not HTTP/1.1: refuse
			c.failed = true
			_ = c.Conn.Close()
			return 0, withheldError{SinkHTTP}
		}
		return len(p), nil
	}
	if c.m.check(slices.Concat(c.buf[:i+4], c.body)) != clean {
		c.failed = true
		_ = c.Conn.Close()
		return 0, withheldError{SinkHTTP}
	}
	if _, err := c.Conn.Write(c.buf); err != nil {
		return 0, err
	}
	c.done, c.buf = true, nil
	return len(p), nil
}

// holdRequest holds the credentials a request carries (contract 1): in its
// headers and trailers alike, its URL, and its body as written and as its
// headers declare it (acquire.go). It reports false when a declared coding
// cannot be read, or the request is not one the kit reads the one way
// (requestWhole), and the request must not be sent.
func holdRequest(req *http.Request, body []byte) bool {
	reg.Lock()
	defer reg.Unlock()
	holdHeaderLocked(req.Header)
	holdHeaderLocked(req.Trailer)
	if u := req.URL.User; u != nil {
		holdLocked("basic-user", u.Username())
		if p, ok := u.Password(); ok {
			holdLocked("userinfo", p)
		}
	}
	q := sentQuery(req.URL)
	holdFieldsLocked([]byte(q), reg.decl.CredentialFields, "field:", nil)
	rs, ok := readings(body, req.Header)
	for _, r := range rs {
		holdFieldsLocked(r, reg.decl.CredentialFields, "field:", nil)
	}
	// Send what you read (loop ruling 8): a request the kit does not read
	// the one unambiguous way is not sent.
	return ok && requestWhole(q, body, req.Header, req.Trailer)
}

// sentQuery is the query the request line carries: what follows the first
// '?' of the request URI (kit15rv round 2: an Opaque URL carries its own,
// outside RawQuery).
func sentQuery(u *url.URL) string {
	_, q, _ := strings.Cut(u.RequestURI(), "?")
	return q
}

// holdHeaderLocked holds the credentials in one block of request headers or
// trailers: auth headers (and the parts of Basic auth), declared and
// config-named headers, each as written and with MIME words decoded, and
// every cookie value.
func holdHeaderLocked(h http.Header) { holdHeaderAllowedLocked(h, nil) }

// holdHeaderAllowedLocked is holdHeaderLocked with each value bound to
// allowed as well (a proxy's own headers are allowed to the proxy).
func holdHeaderAllowedLocked(h http.Header, allowed []string) {
	holdCookiesLocked(h, allowed)
	for name, vs := range h {
		n := strings.ToLower(name)
		if n != "authorization" && n != "proxy-authorization" && !reg.headers[n] && !containsFold(reg.decl.CredentialHeaders, n) {
			continue
		}
		for _, v := range vs {
			holdAllowedLocked("header:"+n, allowed, headerReadings(v)...)
		}
	}
	if user, pass, ok := (&http.Request{Header: h}).BasicAuth(); ok {
		holdAllowedLocked("basic-user", allowed, user)
		holdAllowedLocked("header:authorization", allowed, pass)
	}
}

// holdCookiesLocked holds every cookie value in one block of headers or
// trailers, of a request or a response (bound to extra as well), read
// leniently from each Cookie and Set-Cookie line (cookieValues).
func holdCookiesLocked(h http.Header, extra []string) {
	for name, vs := range h {
		set := strings.EqualFold(name, "Set-Cookie")
		if !set && !strings.EqualFold(name, "Cookie") {
			continue
		}
		for _, v := range vs {
			for _, r := range headerReadings(v) {
				for _, c := range cookieValues(r, set) {
					holdAllowedLocked("cookie:"+c[0], extra, c[1:]...)
				}
			}
		}
	}
}

// cookieValues is each cookie in a Cookie line, or a Set-Cookie line's
// cookie (its first pair): the name, then the value as written and
// unquoted. A piece with no "=" is a value with no name. It does not ask
// Go's cookie parser, which drops a value it finds invalid that a server
// still receives.
func cookieValues(line string, set bool) [][]string {
	pairs := strings.Split(line, ";")
	if set {
		pairs = pairs[:1]
	}
	var out [][]string
	for _, p := range pairs {
		name, v, ok := strings.Cut(p, "=")
		if !ok {
			name, v = "", p
		}
		v = strings.TrimSpace(v)
		out = append(out, []string{strings.TrimSpace(name), v, strings.Trim(v, `"`)})
	}
	return out
}

// holdFieldsLocked holds every value beneath a key named in names, in a JSON
// (or any key-value) document or a form body.
func holdFieldsLocked(b []byte, names []string, prefix string, extra []string) {
	if len(names) == 0 || len(b) == 0 {
		return
	}
	for _, v := range readConfig(b) {
		if v.stray {
			continue // unclassified text in a body is not a field
		}
		for _, k := range v.keys {
			if containsFold(names, k) {
				holdAllowedLocked(prefix+k, extra, v.value, v.raw)
				break
			}
		}
	}
	for k, vs := range formPairs(string(b)) {
		if containsFold(names, strings.ToLower(k)) {
			holdAllowedLocked(prefix+strings.ToLower(k), extra, vs...)
		}
	}
}

// containsFold reports whether names holds name, compared case-insensitively
// by the last segment of a dotted name.
func containsFold(names []string, name string) bool {
	for _, n := range names {
		segs := splitKey(n)
		if strings.EqualFold(segs[len(segs)-1], name) || strings.EqualFold(n, name) {
			return true
		}
	}
	return false
}
