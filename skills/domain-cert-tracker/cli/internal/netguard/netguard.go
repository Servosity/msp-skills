// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package netguard is the one outbound-connection policy for this CLI and its
// MCP server. Every TCP connection the program opens (RDAP, IANA bootstrap,
// crt.sh, TLS probes) goes through Guard.DialContext, which resolves the host,
// refuses the whole dial when ANY resolved address is not a public unicast
// address, refuses ports outside the allowed set, and then connects to the
// address it checked (no second lookup, so a DNS answer cannot change between
// check and connect). HTTP clients built here also refuse non-https URLs and
// re-run the same checks on every redirect hop.
//
// The allowed port set is 443 unless the operator widens it on the command
// line for one invocation; it is never widened from stored data or from an
// MCP tool argument.
package netguard

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"domain-cert-tracker-pp-cli/internal/credkit"
)

// ErrRefused is wrapped by every refusal so callers can tell a policy refusal
// from a network failure.
var ErrRefused = errors.New("refused")

// DefaultPort is the only port allowed without an operator flag.
const DefaultPort = 443

// deniedPrefixes are address blocks that are never a public internet target.
// IsGlobalUnicast-style checks miss several of them (CGNAT, benchmarking,
// documentation, reserved), so the list is explicit.
var deniedPrefixes = mustPrefixes(
	"0.0.0.0/8",       // "this network"
	"10.0.0.0/8",      // RFC 1918
	"100.64.0.0/10",   // CGNAT, RFC 6598
	"127.0.0.0/8",     // loopback
	"169.254.0.0/16",  // link-local, incl. 169.254.169.254 metadata
	"172.16.0.0/12",   // RFC 1918
	"192.0.0.0/24",    // IETF protocol assignments (incl. 192.0.0.192 Oracle metadata)
	"192.0.2.0/24",    // documentation
	"192.88.99.0/24",  // 6to4 relay anycast (deprecated)
	"192.168.0.0/16",  // RFC 1918
	"198.18.0.0/15",   // benchmarking
	"198.51.100.0/24", // documentation
	"203.0.113.0/24",  // documentation
	"224.0.0.0/4",     // multicast
	"240.0.0.0/4",     // reserved, incl. 255.255.255.255
	"::/128",          // unspecified
	"::1/128",         // loopback
	"::ffff:0:0/96",   // IPv4-mapped (checked after Unmap; kept as a backstop)
	"64:ff9b:1::/48",  // local-use NAT64, RFC 8215
	"100::/64",        // discard
	"2001::/23",       // IETF protocol assignments, RFC 2928 (Teredo, benchmarking, ORCHID, AMT, AS112-v6)
	"2001:db8::/32",   // documentation
	"3fff::/20",       // documentation, RFC 9637
	"fc00::/7",        // unique local, RFC 4193 (incl. fd00:ec2::254 metadata)
	"fe80::/10",       // link-local
	"fec0::/10",       // deprecated site-local
	"ff00::/8",        // multicast
)

var globalUnicast6 = netip.MustParsePrefix("2000::/3")

func mustPrefixes(cidrs ...string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(cidrs))
	for _, c := range cidrs {
		out = append(out, netip.MustParsePrefix(c))
	}
	return out
}

// CheckAddr returns nil only for a public unicast address. Embedded IPv4
// addresses (IPv4-mapped, NAT64 64:ff9b::/96, 6to4 2002::/16) are checked as
// the IPv4 address they carry.
func CheckAddr(a netip.Addr) error {
	if !a.IsValid() {
		return fmt.Errorf("%w: invalid address", ErrRefused)
	}
	a = a.Unmap().WithZone("")
	if a.Is6() {
		b := a.As16()
		switch {
		case netip.MustParsePrefix("64:ff9b::/96").Contains(a):
			return CheckAddr(netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]}))
		case netip.MustParsePrefix("2002::/16").Contains(a):
			return CheckAddr(netip.AddrFrom4([4]byte{b[2], b[3], b[4], b[5]}))
		}
	}
	// IPv6 is an allowlist: only global unicast (2000::/3) can be public, so a
	// reserved or deprecated form (IPv4-compatible ::/96, IPv4-translated
	// ::ffff:0:0:0/96, unassigned space) is refused without being listed.
	if a.Is6() && !globalUnicast6.Contains(a) {
		return fmt.Errorf("%w: %s is not a public address (outside 2000::/3)", ErrRefused, a)
	}
	for _, p := range deniedPrefixes {
		if p.Contains(a) {
			return fmt.Errorf("%w: %s is not a public address (%s)", ErrRefused, a, p)
		}
	}
	if !a.IsGlobalUnicast() {
		return fmt.Errorf("%w: %s is not a public unicast address", ErrRefused, a)
	}
	return nil
}

// Resolver looks up the addresses for a host name.
type Resolver func(ctx context.Context, host string) ([]netip.Addr, error)

// Dialer opens the TCP connection to an address that already passed CheckAddr.
type Dialer func(ctx context.Context, network, addr string) (net.Conn, error)

// Guard holds the policy for one invocation.
type Guard struct {
	// Ports allowed for outbound connections. Empty means {443}.
	Ports map[int]bool
	// Resolve and Dial are replaceable for tests; nil uses the system resolver
	// and a plain net.Dialer.
	Resolve Resolver
	Dial    Dialer
	// AllowAddr lets tests accept a loopback test server. Production code
	// never sets it.
	AllowAddr func(netip.Addr) bool
	// operator is the one host:port the operator configured outside any
	// tool argument (the base-URL environment variable). A connection to
	// exactly that host and port skips the public-address and port checks;
	// every other connection is checked as usual.
	operator *origin
}

type origin struct {
	host string
	port int
}

// WithOperatorOrigin returns a copy of g that also reaches the origin of raw,
// an http or https URL the operator configured in the environment. Callers
// pass only a value read from the process environment, never one a tool
// argument or a server response supplied. A value that does not parse as an
// http(s) URL with a host is refused and g is returned unchanged.
func (g *Guard) WithOperatorOrigin(raw string) (*Guard, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return g, fmt.Errorf("%w: base URL: %v", ErrRefused, err)
	}
	scheme := strings.ToLower(u.Scheme)
	if (scheme != "http" && scheme != "https") || u.Hostname() == "" || u.User != nil {
		return g, fmt.Errorf("%w: base URL %q is not an http(s) URL with a host", ErrRefused, raw)
	}
	port := 443
	if scheme == "http" {
		port = 80
	}
	if ps := u.Port(); ps != "" {
		p, err := strconv.Atoi(ps)
		if err != nil || p < 1 || p > 65535 {
			return g, fmt.Errorf("%w: base URL port %q", ErrRefused, ps)
		}
		port = p
	}
	cp := *g
	cp.operator = &origin{host: normHost(u.Hostname()), port: port}
	return &cp, nil
}

func normHost(h string) string {
	return strings.ToLower(strings.TrimSuffix(strings.Trim(strings.TrimSpace(h), "[]"), "."))
}

func (g *Guard) isOperatorOrigin(host string, port int) bool {
	return g.operator != nil && g.operator.port == port && g.operator.host == normHost(host)
}

// New returns the default policy: port 443 only, system resolver.
func New() *Guard { return &Guard{} }

// WithPorts returns a copy of g that also allows ports. Callers pass only
// values the operator typed on the command line for this invocation.
func (g *Guard) WithPorts(ports ...int) *Guard {
	cp := *g
	cp.Ports = map[int]bool{DefaultPort: true}
	for p, ok := range g.Ports {
		cp.Ports[p] = ok
	}
	for _, p := range ports {
		cp.Ports[p] = true
	}
	return &cp
}

func (g *Guard) portAllowed(p int) bool {
	if len(g.Ports) == 0 {
		return p == DefaultPort
	}
	return g.Ports[p]
}

func (g *Guard) checkAddr(a netip.Addr) error {
	if g.AllowAddr != nil && g.AllowAddr(a) {
		return nil
	}
	return CheckAddr(a)
}

// CheckPort refuses a port outside the allowed set.
func (g *Guard) CheckPort(port int) error {
	if !g.portAllowed(port) {
		return fmt.Errorf("%w: port %d is not allowed (only 443; the CLI flag --allow-port widens this for one run)", ErrRefused, port)
	}
	return nil
}

// ResolvePublic resolves host and returns its addresses only when every one
// is public. One non-public answer refuses the host: a name that mixes public
// and private answers is how DNS rebinding reaches internal services.
func (g *Guard) ResolvePublic(ctx context.Context, host string) ([]netip.Addr, error) {
	host = strings.TrimSuffix(strings.TrimSpace(host), ".")
	if host == "" {
		return nil, fmt.Errorf("%w: empty host", ErrRefused)
	}
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return nil, fmt.Errorf("%w: %s is a loopback name", ErrRefused, host)
	}
	if a, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		if err := g.checkAddr(a); err != nil {
			return nil, err
		}
		return []netip.Addr{a.Unmap()}, nil
	}
	resolve := g.Resolve
	if resolve == nil {
		resolve = systemResolve
	}
	addrs, err := resolve(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("lookup %s: no addresses", host)
	}
	for _, a := range addrs {
		if err := g.checkAddr(a); err != nil {
			return nil, fmt.Errorf("%s resolves to a non-public address: %w", host, err)
		}
	}
	return addrs, nil
}

// resolveAny resolves the operator's own base-URL host without the
// public-address check.
func (g *Guard) resolveAny(ctx context.Context, host string) ([]netip.Addr, error) {
	if a, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		return []netip.Addr{a.Unmap()}, nil
	}
	resolve := g.Resolve
	if resolve == nil {
		resolve = systemResolve
	}
	addrs, err := resolve(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("lookup %s: no addresses", host)
	}
	return addrs, nil
}

func systemResolve(ctx context.Context, host string) ([]netip.Addr, error) {
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	return ips, nil
}

// DialContext is a net.Dialer-compatible DialContext that enforces the policy.
func (g *Guard) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	switch network {
	case "tcp", "tcp4", "tcp6":
	default:
		return nil, fmt.Errorf("%w: network %q", ErrRefused, network)
	}
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRefused, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("%w: bad port %q", ErrRefused, portText)
	}
	var addrs []netip.Addr
	if g.isOperatorOrigin(host, port) {
		addrs, err = g.resolveAny(ctx, host)
	} else {
		if err := g.CheckPort(port); err != nil {
			return nil, err
		}
		addrs, err = g.ResolvePublic(ctx, host)
	}
	if err != nil {
		return nil, err
	}
	dial := g.Dial
	if dial == nil {
		d := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
		dial = d.DialContext
	}
	var firstErr error
	for _, a := range addrs {
		conn, err := dial(ctx, network, net.JoinHostPort(a.String(), portText))
		if err == nil {
			return conn, nil
		}
		if firstErr == nil {
			firstErr = err
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	return nil, firstErr
}

// CheckURL refuses anything but an https URL on an allowed port with a host.
// The address check happens at dial time.
func (g *Guard) CheckURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("%w: empty URL", ErrRefused)
	}
	u, err := parseURL(raw)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRefused, err)
	}
	if !strings.EqualFold(u.scheme, "https") {
		return fmt.Errorf("%w: %s is not https", ErrRefused, raw)
	}
	if u.host == "" {
		return fmt.Errorf("%w: %s has no host", ErrRefused, raw)
	}
	return g.CheckPort(u.port)
}

// CheckRedirect enforces CheckURL on every hop and caps the chain.
func (g *Guard) CheckRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return errors.New("stopped after 5 redirects")
	}
	if req == nil || req.URL == nil {
		return fmt.Errorf("%w: empty redirect", ErrRefused)
	}
	return g.CheckURL(req.URL.String())
}

// Transport returns an http.Transport that dials only through the guard and
// never through an environment proxy (a proxy would hide the real target from
// the address check).
func (g *Guard) Transport() *http.Transport {
	return &http.Transport{
		Proxy:                 nil,
		DialContext:           g.DialContext,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 45 * time.Second,
	}
}

// HTTPClient returns an https-only client that enforces the policy on every
// connection and redirect, its requests checked by the credential kit's
// outbound guard (handfixes.json credkit-wiring).
func (g *Guard) HTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: credkit.Transport(g.Transport()), CheckRedirect: g.CheckRedirect}
}

// Install points an existing client at the guard: its transport dials through
// the guard with no proxy, and its CheckRedirect runs the guard first, then
// any check the client already had.
func (g *Guard) Install(c *http.Client) {
	if c == nil {
		return
	}
	if tr, ok := c.Transport.(*http.Transport); ok && tr != nil {
		tr.Proxy = nil
		tr.DialContext = g.DialContext
		tr.DialTLSContext = nil
		tr.DialTLS = nil //nolint:staticcheck // cleared so no legacy hook bypasses the guard
		tr.Dial = nil    //nolint:staticcheck // cleared so no legacy hook bypasses the guard
	} else {
		c.Transport = g.Transport()
	}
	prev := c.CheckRedirect
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := g.CheckRedirect(req, via); err != nil {
			return err
		}
		if prev != nil {
			return prev(req, via)
		}
		return nil
	}
}

type parsedURL struct {
	scheme string
	host   string
	port   int
}

func parseURL(raw string) (parsedURL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return parsedURL{}, err
	}
	out := parsedURL{scheme: strings.ToLower(u.Scheme), host: u.Hostname()}
	if u.User != nil {
		return parsedURL{}, errors.New("URL carries userinfo")
	}
	if p := u.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return parsedURL{}, fmt.Errorf("bad port %q", p)
		}
		out.port = n
	} else if out.scheme == "https" {
		out.port = 443
	} else if out.scheme == "http" {
		out.port = 80
	}
	return out, nil
}

// SplitHostPort parses "host", "host:port", "[v6]" or "[v6]:port" and
// defaults the port to 443.
func SplitHostPort(s string) (string, int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", 0, errors.New("empty host")
	}
	if strings.Contains(s, "/") || strings.Contains(s, "@") {
		return "", 0, fmt.Errorf("%q is not a host[:port]", s)
	}
	host, portText, err := net.SplitHostPort(s)
	if err != nil {
		// No port, or a bare IPv6 literal.
		if a, perr := netip.ParseAddr(strings.Trim(s, "[]")); perr == nil {
			return a.String(), DefaultPort, nil
		}
		if strings.Contains(s, ":") {
			return "", 0, fmt.Errorf("%q is not a host[:port]", s)
		}
		return strings.TrimSuffix(strings.ToLower(s), "."), DefaultPort, nil
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("bad port %q", portText)
	}
	if host == "" {
		return "", 0, errors.New("empty host")
	}
	return strings.TrimSuffix(strings.ToLower(host), "."), port, nil
}
