// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package netguard

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strconv"
	"sync"
	"testing"
)

func TestCheckAddrRefusesEveryNonPublicClass(t *testing.T) {
	refused := []string{
		"127.0.0.1", "127.255.255.254", // loopback
		"10.0.0.1", "172.16.0.1", "172.31.255.255", "192.168.1.1", // RFC 1918
		"169.254.169.254", "169.254.0.1", // link-local, cloud metadata
		"100.64.0.1", "100.127.255.254", // CGNAT
		"0.0.0.0", "0.1.2.3", // this network
		"192.0.0.192",                              // Oracle metadata, IETF protocol block
		"192.0.2.1", "198.51.100.1", "203.0.113.1", // documentation
		"198.18.0.1",                   // benchmarking
		"224.0.0.1", "255.255.255.255", // multicast, broadcast
		"240.0.0.1", // reserved
		"::", "::1", // unspecified, loopback
		"fc00::1", "fd00:ec2::254", // unique local, AWS metadata over IPv6
		"fe80::1",         // link-local
		"ff02::1",         // multicast
		"::ffff:10.0.0.1", // IPv4-mapped private
		"::ffff:127.0.0.1",
		"64:ff9b::a00:1",              // NAT64 of 10.0.0.1
		"64:ff9b::a9fe:a9fe",          // NAT64 of 169.254.169.254
		"2002:a00:1::1",               // 6to4 of 10.0.0.1
		"2001::1",                     // Teredo
		"2001:db8::1",                 // documentation
		"64:ff9b:1::1",                // local-use NAT64
		"::127.0.0.1",                 // IPv4-compatible, deprecated (::/96)
		"::a00:1",                     // IPv4-compatible 10.0.0.1
		"::ffff:0:7f00:1",             // IPv4-translated (::ffff:0:0:0/96)
		"4000::1",                     // unassigned, outside 2000::/3
		"100::1",                      // discard
		"3fff::1", "3fff:fff:ffff::1", // documentation, RFC 9637 (3fff::/20)
		"2001:3::1", "2001:1ff::1", // IETF protocol assignments 2001::/23, RFC 2928
		"2001:4:112::1", // AS112-v6 inside 2001::/23
		"5f00::1",       // SRv6 SIDs, outside 2000::/3
	}
	for _, s := range refused {
		a := netip.MustParseAddr(s)
		if err := CheckAddr(a); !errors.Is(err, ErrRefused) {
			t.Errorf("CheckAddr(%s) = %v, want refused", s, err)
		}
	}
	allowed := []string{"93.184.215.14", "1.1.1.1", "8.8.8.8", "2606:4700:4700::1111", "64:ff9b::808:808", "2002:808:808::1"}
	for _, s := range allowed {
		if err := CheckAddr(netip.MustParseAddr(s)); err != nil {
			t.Errorf("CheckAddr(%s) = %v, want allowed", s, err)
		}
	}
}

type dialRecorder struct {
	mu    sync.Mutex
	addrs []string
}

func (d *dialRecorder) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	d.mu.Lock()
	d.addrs = append(d.addrs, addr)
	d.mu.Unlock()
	return nil, errors.New("test dialer: no network")
}

func fakeResolver(m map[string][]string) Resolver {
	return func(ctx context.Context, host string) ([]netip.Addr, error) {
		var out []netip.Addr
		for _, s := range m[host] {
			out = append(out, netip.MustParseAddr(s))
		}
		if len(out) == 0 {
			return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
		}
		return out, nil
	}
}

func TestDialContextRefusesBeforeConnecting(t *testing.T) {
	rec := &dialRecorder{}
	g := &Guard{Dial: rec.dial, Resolve: fakeResolver(map[string][]string{
		"public.test":   {"93.184.215.14"},
		"internal.test": {"10.0.0.5"},
		"mixed.test":    {"93.184.215.14", "192.168.1.10"}, // DNS rebinding shape
		"metadata.test": {"169.254.169.254"},
	})}
	cases := []struct{ addr, why string }{
		{"internal.test:443", "private answer"},
		{"mixed.test:443", "one private answer among public ones"},
		{"metadata.test:443", "metadata address"},
		{"127.0.0.1:443", "loopback literal"},
		{"[::1]:443", "IPv6 loopback literal"},
		{"localhost:443", "loopback name"},
		{"app.localhost:443", "loopback subdomain"},
		{"public.test:8443", "non-443 port"},
		{"public.test:22", "ssh port"},
	}
	for _, c := range cases {
		if _, err := g.DialContext(context.Background(), "tcp", c.addr); !errors.Is(err, ErrRefused) {
			t.Errorf("%s (%s): err = %v, want refused", c.addr, c.why, err)
		}
	}
	if len(rec.addrs) != 0 {
		t.Fatalf("refused dials reached the network: %v", rec.addrs)
	}
	if _, err := g.DialContext(context.Background(), "udp", "public.test:443"); !errors.Is(err, ErrRefused) {
		t.Errorf("udp dial: err = %v, want refused", err)
	}
	// A public host on 443 is dialed at the checked address, not re-resolved.
	_, _ = g.DialContext(context.Background(), "tcp", "public.test:443")
	if len(rec.addrs) != 1 || rec.addrs[0] != "93.184.215.14:443" {
		t.Fatalf("public dial went to %v, want the checked address", rec.addrs)
	}
}

func TestWithPortsWidensOnlyNamedPorts(t *testing.T) {
	g := New().WithPorts(8443)
	if err := g.CheckPort(8443); err != nil {
		t.Fatalf("8443: %v", err)
	}
	if err := g.CheckPort(443); err != nil {
		t.Fatalf("443: %v", err)
	}
	if err := g.CheckPort(22); !errors.Is(err, ErrRefused) {
		t.Fatalf("22: %v, want refused", err)
	}
	if err := New().CheckPort(8443); !errors.Is(err, ErrRefused) {
		t.Fatalf("default guard allowed 8443")
	}
}

func TestCheckURL(t *testing.T) {
	g := New()
	for _, bad := range []string{"http://example.com/", "https://example.com:8443/", "ftp://example.com/", "https:///x", "https://user:pw@example.com/", ""} {
		if err := g.CheckURL(bad); !errors.Is(err, ErrRefused) {
			t.Errorf("CheckURL(%q) = %v, want refused", bad, err)
		}
	}
	if err := g.CheckURL("https://rdap.org/domain/example.com"); err != nil {
		t.Errorf("https URL refused: %v", err)
	}
}

// loopbackGuard accepts only the test server's loopback address and port,
// and resolves internal.test to a private address.
func loopbackGuard(t *testing.T, srv *httptest.Server) *Guard {
	t.Helper()
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	g := New().WithPorts(port)
	g.AllowAddr = func(a netip.Addr) bool { return a == netip.MustParseAddr("127.0.0.1") }
	g.Resolve = fakeResolver(map[string][]string{
		"server.test":   {"127.0.0.1"},
		"internal.test": {"10.0.0.5"},
	})
	return g
}

func TestHTTPClientRefusesRedirectToPrivateHTTPAndOtherPorts(t *testing.T) {
	var target string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ok" {
			_, _ = w.Write([]byte("ok"))
			return
		}
		http.Redirect(w, r, target, http.StatusFound)
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	g := loopbackGuard(t, srv)
	// HTTPClient's transport is wrapped by the credential kit, so the
	// guard's own transport and redirect check are assembled here to trust
	// httptest's certificate.
	tr := g.Transport()
	tlsCfg := srv.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	tlsCfg.ServerName = "example.com" // httptest's certificate name
	tr.TLSClientConfig = tlsCfg
	c := &http.Client{Transport: tr, CheckRedirect: g.CheckRedirect}
	base := "https://server.test:" + u.Port()

	resp, err := c.Get(base + "/ok")
	if err != nil {
		t.Fatalf("direct GET: %v", err)
	}
	resp.Body.Close()

	for _, tc := range []struct{ target, why string }{
		{"https://internal.test:" + u.Port() + "/", "redirect to a name that resolves private"},
		{"https://10.0.0.7:" + u.Port() + "/", "redirect to a private literal"},
		{"https://169.254.169.254/latest/meta-data/", "redirect to the metadata address"},
		{"http://server.test:" + u.Port() + "/ok", "redirect to plain http"},
		{"https://server.test:8443/ok", "redirect to a port not allowed"},
	} {
		target = tc.target
		resp, err := c.Get(base + "/redirect")
		if err == nil {
			resp.Body.Close()
			t.Errorf("%s: followed, want refused", tc.why)
			continue
		}
		if !errors.Is(err, ErrRefused) {
			t.Errorf("%s: err = %v, want refused", tc.why, err)
		}
	}
}

func TestInstallGuardsAnExistingClient(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer srv.Close()
	c := srv.Client() // dials loopback directly before Install
	New().Install(c)
	if _, err := c.Get(srv.URL); !errors.Is(err, ErrRefused) {
		t.Fatalf("installed guard let a loopback request through: %v", err)
	}
}

func TestSplitHostPort(t *testing.T) {
	cases := []struct {
		in   string
		host string
		port int
		bad  bool
	}{
		{"Example.COM", "example.com", 443, false},
		{"example.com.", "example.com", 443, false},
		{"example.com:8443", "example.com", 8443, false},
		{"[2606:4700::1111]:443", "2606:4700::1111", 443, false},
		{"2606:4700::1111", "2606:4700::1111", 443, false},
		{"example.com:0", "", 0, true},
		{"example.com:99999", "", 0, true},
		{"https://example.com", "", 0, true},
		{"user@example.com", "", 0, true},
		{"", "", 0, true},
	}
	for _, c := range cases {
		h, p, err := SplitHostPort(c.in)
		if c.bad {
			if err == nil {
				t.Errorf("SplitHostPort(%q) accepted", c.in)
			}
			continue
		}
		if err != nil || h != c.host || p != c.port {
			t.Errorf("SplitHostPort(%q) = %q, %d, %v", c.in, h, p, err)
		}
	}
}

// The operator's base-URL origin is the one exception, matched exactly:
// same host and port only, from a parsed http(s) URL with no userinfo.
func TestOperatorOriginIsTheOnlyException(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	ctx := context.Background()
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	if _, err := New().DialContext(ctx, "tcp", addr); err == nil {
		t.Fatal("default guard reached a loopback port")
	}
	g, err := New().WithOperatorOrigin("http://127.0.0.1:" + strconv.Itoa(port) + "/rdap")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := g.DialContext(ctx, "tcp", addr)
	if err != nil {
		t.Fatalf("operator origin refused: %v", err)
	}
	conn.Close()
	for _, other := range []string{
		net.JoinHostPort("127.0.0.1", strconv.Itoa(port+1)),
		net.JoinHostPort("127.0.0.2", strconv.Itoa(port)),
		net.JoinHostPort("localhost", strconv.Itoa(port)),
		"169.254.169.254:80",
	} {
		if _, err := g.DialContext(ctx, "tcp", other); err == nil || !errors.Is(err, ErrRefused) {
			t.Errorf("%s: not refused under an operator origin: %v", other, err)
		}
	}
	for _, bad := range []string{"ftp://127.0.0.1:21", "http://user:pw@127.0.0.1:8080", "127.0.0.1:8080", "http://", "http://127.0.0.1:0", "http://127.0.0.1:99999"} {
		if og, err := New().WithOperatorOrigin(bad); err == nil || og.operator != nil {
			t.Errorf("%q accepted as an operator origin", bad)
		}
	}
	if g, _ := New().WithOperatorOrigin("https://RDAP.Example.TEST."); !g.isOperatorOrigin("rdap.example.test", 443) || g.isOperatorOrigin("rdap.example.test", 80) {
		t.Error("https origin without a port is host:443 only, case and trailing dot folded")
	}
}
