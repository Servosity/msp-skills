// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package portal

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"syscall"
	"time"
)

// ErrUnsafeDownloadURL is returned when the portal hands back a download
// link, or a redirect from one, that this CLI refuses to request.
var ErrUnsafeDownloadURL = errors.New("portal: refusing an unsafe download url")

// ErrDownloadSize is returned when a download does not match the size the
// portal reported for it.
var ErrDownloadSize = errors.New("portal: download size does not match the reported size")

// ErrNotAnArchive is returned when a download of unknown size turns out to
// be a short HTML, XML, JSON or text document: an error page served with a
// 200, not an archive.
var ErrNotAnArchive = errors.New("portal: the download is an error document, not an archive")

// ErrDownloadStalled is returned when a download receives nothing for
// DownloadStallTimeout.
var ErrDownloadStalled = errors.New("portal: the download stalled")

// DownloadStallTimeout ends a download that has received nothing for this
// long. A whole transfer has no deadline, because a large archive can take
// hours; only silence ends it. Tests shorten it.
var DownloadStallTimeout = 5 * time.Minute

// DownloadAddrPolicy vets the address a download connection actually
// reaches, after the host name is resolved, so a name that resolves to
// this machine or an internal network is refused just as the same address
// written in the URL would be. Tests replace it to reach an httptest
// server; nothing in the shipped binary changes it.
var DownloadAddrPolicy = StrictDownloadAddr

// StrictDownloadAddr refuses the same ranges StrictDownloadURL refuses in
// an IP literal.
func StrictDownloadAddr(a netip.Addr) error {
	if reason := blockedAddrReason(a); reason != "" {
		return fmt.Errorf("%w: the host resolves to a %s address", ErrUnsafeDownloadURL, reason)
	}
	return nil
}

// DownloadURLPolicy vets a server-supplied download URL, and every redirect
// hop from it, before anything is requested. The link comes from a portal
// response, so without this a hostile or compromised response could point
// this machine at its own services or at a cloud metadata endpoint.
//
// Tests replace it to reach an httptest server. Nothing in the shipped
// binary changes it, and no flag or environment variable can.
var DownloadURLPolicy = StrictDownloadURL

// maxDownloadRedirects bounds a redirect chain from object storage.
const maxDownloadRedirects = 5

// cgnatPrefix is the shared address space (RFC 6598). Mesh VPNs hand these
// out to internal hosts, so it is treated like a private range.
var cgnatPrefix = netip.MustParsePrefix("100.64.0.0/10")

// StrictDownloadURL is the production policy: https only, and never a host
// that is a loopback, link-local, private, shared or unspecified IP literal,
// or a name that is reserved for this machine.
//
// It judges what the URL says, not what a name resolves to. Checking the
// resolved address would also refuse a corporate proxy on a private
// address, which is how many MSPs reach the internet.
func StrictDownloadURL(u *url.URL) error {
	if u == nil {
		return fmt.Errorf("%w: no url", ErrUnsafeDownloadURL)
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return fmt.Errorf("%w: scheme %q is not https", ErrUnsafeDownloadURL, u.Scheme)
	}
	host := strings.TrimSuffix(u.Hostname(), ".")
	if host == "" {
		return fmt.Errorf("%w: it names no host", ErrUnsafeDownloadURL)
	}
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") {
		return fmt.Errorf("%w: host %s is this machine", ErrUnsafeDownloadURL, host)
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		// A real host name never ends in an all-numeric or hex label, but
		// a system resolver may read "127.1" or "0x7f000001" as an IPv4
		// address, so those shorthand forms are refused outright.
		if numericLabel(lower) {
			return fmt.Errorf("%w: host %s looks like a numeric address in a non-standard form", ErrUnsafeDownloadURL, host)
		}
		// A name with no dot, or under a suffix reserved for private
		// networks, names a machine on this network. A proxy would resolve
		// it on its own side and reach that network's services, so it is
		// refused whether or not a proxy is in use.
		if !strings.Contains(lower, ".") {
			return fmt.Errorf("%w: host %s is a single-label name for a machine on the local network", ErrUnsafeDownloadURL, host)
		}
		for _, suffix := range localNameSuffixes {
			if lower == suffix[1:] || strings.HasSuffix(lower, suffix) {
				return fmt.Errorf("%w: host %s is under %s, which names private-network machines", ErrUnsafeDownloadURL, host, suffix)
			}
		}
		return nil
	}
	if reason := blockedAddrReason(addr); reason != "" {
		return fmt.Errorf("%w: host %s is a %s address", ErrUnsafeDownloadURL, addr.WithZone(""), reason)
	}
	return nil
}

// localNameSuffixes are DNS suffixes used for machines on a private
// network rather than public object storage.
var localNameSuffixes = []string{".local", ".internal", ".lan", ".home.arpa", ".localhost", ".corp"}

// numericLabel reports whether a host's last label is all digits or a hex
// literal, which DNS host names never are.
func numericLabel(host string) bool {
	last := host
	if i := strings.LastIndexByte(host, '.'); i >= 0 {
		last = host[i+1:]
	}
	if strings.HasPrefix(last, "0x") {
		return true
	}
	if last == "" {
		return false
	}
	for _, r := range last {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// blockedAddrReason names why an address may not be fetched, or "".
func blockedAddrReason(a netip.Addr) string {
	a = a.Unmap()
	switch {
	case a.IsLoopback():
		return "loopback"
	case a.IsUnspecified(), a.Is4() && a.As4()[0] == 0:
		return "unspecified"
	case a.IsLinkLocalUnicast(), a.IsLinkLocalMulticast():
		return "link-local"
	case a.IsPrivate():
		return "private"
	case a.Is4() && cgnatPrefix.Contains(a):
		return "shared (carrier-grade NAT)"
	case a.IsMulticast():
		return "multicast"
	}
	return ""
}

// SizeHint is the size the portal reported for a file, in bytes, with the
// slack its rounding allows. The zero value means the size is unknown.
type SizeHint struct {
	Bytes     int64
	Tolerance int64
}

// ParseSizeHint reads a portal size string such as "871.26 MB". ok is false
// when the string cannot be read or reports nothing, which callers treat as
// an unknown size rather than as zero.
//
// The tolerance is exactly what the printed precision allows and no more: a
// byte count must match exactly, and a rounded size may differ by one unit
// in its last printed digit, whichever way the portal rounded. "871.26 MB"
// is good to about 10 KiB; "1.8 GB" only to about 100 MiB, which is the
// most that string can say.
func ParseSizeHint(reported string) (SizeHint, bool) {
	reported = normaliseSize(reported)
	m := sizePattern.FindStringSubmatch(reported)
	if m == nil {
		return SizeHint{}, false
	}
	b := ParseSizeBytes(reported)
	if b <= 0 {
		return SizeHint{}, false
	}
	var tol int64
	if mult := sizeUnits[strings.ToLower(m[2])]; mult > 1 {
		step := float64(mult)
		if i := strings.IndexByte(m[1], '.'); i >= 0 {
			step /= math.Pow(10, float64(len(m[1])-i-1))
		}
		tol = int64(math.Ceil(step))
	}
	return SizeHint{Bytes: b, Tolerance: tol}, true
}

// groupedNumber matches a size written with thousands separators, such as
// "1,024.50 MB".
var groupedNumber = regexp.MustCompile(`^\s*([0-9]{1,3}(?:,[0-9]{3})+)((?:\.[0-9]+)?\s*[A-Za-z]+)\s*$`)

// normaliseSize turns "1,024.50 MB" into "1024.50 MB" and a no-break space
// into a plain one, so the size check is not lost to formatting.
func normaliseSize(s string) string {
	s = strings.ReplaceAll(s, "\u00a0", " ")
	if m := groupedNumber.FindStringSubmatch(s); m != nil {
		return strings.ReplaceAll(m[1], ",", "") + m[2]
	}
	return s
}

// Known reports whether the hint carries a size at all.
func (h SizeHint) Known() bool { return h.Bytes > 0 }

// Matches reports whether n bytes is the size the portal reported. Callers
// check Known first; an unknown hint matches nothing.
func (h SizeHint) Matches(n int64) bool {
	if !h.Known() {
		return false
	}
	d := n - h.Bytes
	if d < 0 {
		d = -d
	}
	return d <= h.Tolerance
}

// FetchChunk mints a link for one export chunk and streams it to w,
// returning the number of bytes written.
//
// Minting and streaming are deliberately one operation. The link is valid
// for only 60 seconds (see LinkExpiry), so there is no safe way to hand it
// to anything else first. S3 authorises at request time, so once the
// transfer has begun it continues past the expiry even for a long download.
//
// When want is known, a response whose length cannot match it is refused
// before anything is written, copying stops as soon as the body runs past
// the reported size by more than its tolerance, and a body that ends short
// is an error. When it is not known, a short body that reads as an HTML,
// XML, JSON or text document is refused. Either way a short error page
// served with a 200 never passes for a finished archive.
//
// The transfer has no overall deadline; it ends only when it has received
// nothing for DownloadStallTimeout.
func (s *Session) FetchChunk(ctx context.Context, exportDataID string, w io.Writer, want SizeHint) (int64, error) {
	link, err := s.DownloadURL(ctx, exportDataID)
	if err != nil {
		return 0, err
	}
	u, err := url.Parse(link)
	if err != nil {
		return 0, fmt.Errorf("portal: the download link for %s is not a url", exportDataID)
	}
	if err := DownloadURLPolicy(u); err != nil {
		return 0, fmt.Errorf("portal: downloading chunk %s: %w", exportDataID, err)
	}
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	stall := time.AfterFunc(DownloadStallTimeout, func() { cancel(ErrDownloadStalled) })
	defer stall.Stop()
	streamErr := func(n int64, err error) error {
		if errors.Is(context.Cause(ctx), ErrDownloadStalled) {
			return fmt.Errorf("%w: chunk %s received nothing for %s after %d bytes", ErrDownloadStalled, exportDataID, DownloadStallTimeout, n)
		}
		return fmt.Errorf("portal: streaming chunk %s after %d bytes: %w", exportDataID, n, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return 0, fmt.Errorf("portal: building the download request: %w", scrubURLError(err))
	}
	// A presigned URL carries its own authorisation; sending the session
	// cookie or CSRF header to object storage would be pointless and
	// would leak them to a third party.
	client, done := s.downloadClient()
	defer done()
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(context.Cause(ctx), ErrDownloadStalled) {
			return 0, streamErr(0, err)
		}
		return 0, fmt.Errorf("portal: downloading chunk %s: %w", exportDataID, scrubURLError(err))
	}
	defer resp.Body.Close()
	body := &stallReader{r: resp.Body, timer: stall}
	if resp.StatusCode != http.StatusOK {
		text, _ := io.ReadAll(io.LimitReader(body, 512))
		return 0, fmt.Errorf("portal: downloading chunk %s: status %d: %s",
			exportDataID, resp.StatusCode, strings.TrimSpace(string(text)))
	}
	if !want.Known() {
		// Hold the start of the body back until it is clear it is not an
		// error document; an archive never starts like one.
		head, err := io.ReadAll(io.LimitReader(body, sniffLimit+1))
		if err != nil {
			return 0, streamErr(int64(len(head)), err)
		}
		if len(head) <= sniffLimit && looksLikeDocument(head, resp.Header.Get("Content-Type")) {
			return 0, fmt.Errorf("%w: chunk %s is %d bytes of %s and the portal reported no usable size",
				ErrNotAnArchive, exportDataID, len(head), documentKind(head, resp.Header.Get("Content-Type")))
		}
		hn, err := w.Write(head)
		if err != nil {
			return int64(hn), streamErr(int64(hn), err)
		}
		n, err := io.Copy(w, body)
		n += int64(hn)
		if err != nil {
			return n, streamErr(n, err)
		}
		return n, nil
	}
	if resp.ContentLength >= 0 && !want.Matches(resp.ContentLength) {
		return 0, fmt.Errorf("%w: chunk %s is %d bytes but the portal reported about %d",
			ErrDownloadSize, exportDataID, resp.ContentLength, want.Bytes)
	}
	ceiling := want.Bytes + want.Tolerance
	n, err := io.Copy(w, io.LimitReader(body, ceiling+1))
	if err != nil {
		return n, streamErr(n, err)
	}
	if n > ceiling {
		return n, fmt.Errorf("%w: chunk %s ran past %d bytes but the portal reported about %d; stopped",
			ErrDownloadSize, exportDataID, ceiling, want.Bytes)
	}
	if !want.Matches(n) {
		return n, fmt.Errorf("%w: chunk %s ended at %d bytes but the portal reported about %d",
			ErrDownloadSize, exportDataID, n, want.Bytes)
	}
	return n, nil
}

// sniffLimit is the largest body checked for being an error document when
// the portal gave no usable size.
const sniffLimit = 1 << 20

// looksLikeDocument reports whether a body reads as an HTML, XML, JSON or
// text document rather than an archive. A zip starts "PK" and a PST
// "!BDN", never with markup or a brace.
func looksLikeDocument(b []byte, contentType string) bool {
	if mt, _, err := mime.ParseMediaType(contentType); err == nil {
		if strings.HasPrefix(mt, "text/") || mt == "application/json" || mt == "application/xml" ||
			strings.HasSuffix(mt, "+json") || strings.HasSuffix(mt, "+xml") {
			return true
		}
	}
	b = bytes.TrimLeft(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf")), " \t\r\n")
	return len(b) > 0 && (b[0] == '<' || b[0] == '{' || b[0] == '[')
}

// documentKind names what a refused body looked like, for the error.
func documentKind(b []byte, contentType string) string {
	if mt, _, err := mime.ParseMediaType(contentType); err == nil && mt != "application/octet-stream" {
		return mt
	}
	b = bytes.TrimLeft(b, " \t\r\n")
	if len(b) > 0 && (b[0] == '{' || b[0] == '[') {
		return "JSON"
	}
	return "HTML or XML"
}

// stallReader restarts the stall timer each time bytes arrive.
type stallReader struct {
	r     io.Reader
	timer *time.Timer
}

func (s *stallReader) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	if n > 0 {
		s.timer.Reset(DownloadStallTimeout)
	}
	return n, err
}

// downloadClient returns a client with no timeout, for transfers that can
// run for hours. The session's own client has a per-request timeout suited
// to service calls, which would abort a large archive mid-stream. Every
// redirect hop is held to DownloadURLPolicy, the same as the first URL, and
// every connection to DownloadAddrPolicy. done releases its connections.
func (s *Session) downloadClient() (*http.Client, func()) {
	inner := s.hc.CheckRedirect
	transport, done := downloadTransport(s.hc.Transport)
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxDownloadRedirects {
				return fmt.Errorf("portal: the download redirected more than %d times", maxDownloadRedirects)
			}
			if err := DownloadURLPolicy(req.URL); err != nil {
				return err
			}
			if inner != nil {
				return inner(req, via)
			}
			return nil
		},
	}, done
}

// downloadTransport wraps base so that a direct connection is refused when
// the address it reaches (after DNS) is one DownloadAddrPolicy refuses. A
// request that goes through a proxy from the environment is not checked
// that way: the proxy resolves and connects to the host itself, so the
// address this machine connects to is the proxy's, which is often rightly
// on a private network. The URL itself was still checked by
// DownloadURLPolicy.
//
// A caller-supplied RoundTripper that is not an *http.Transport is used as
// it is; only tests supply one.
func downloadTransport(base http.RoundTripper) (http.RoundTripper, func()) {
	if base == nil {
		base = http.DefaultTransport
	}
	t, ok := base.(*http.Transport)
	if !ok {
		return base, func() {}
	}
	direct := t.Clone()
	proxy := direct.Proxy
	direct.Proxy = nil
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second, Control: checkDownloadConn}
	direct.DialContext = dialer.DialContext
	direct.DialTLSContext = nil
	if proxy == nil {
		return direct, direct.CloseIdleConnections
	}
	proxied := t.Clone()
	split := &proxySplit{proxy: proxy, direct: direct, proxied: proxied}
	return split, func() {
		direct.CloseIdleConnections()
		proxied.CloseIdleConnections()
	}
}

// checkDownloadConn runs on every connection attempt, with the address the
// name resolved to.
func checkDownloadConn(_, address string, _ syscall.RawConn) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil {
		return fmt.Errorf("%w: cannot read the connected address", ErrUnsafeDownloadURL)
	}
	return DownloadAddrPolicy(ap.Addr())
}

// proxySplit sends a request through the proxy when the environment names
// one for it, and directly (with the address check) otherwise.
type proxySplit struct {
	proxy           func(*http.Request) (*url.URL, error)
	direct, proxied *http.Transport
}

func (p *proxySplit) RoundTrip(req *http.Request) (*http.Response, error) {
	u, err := p.proxy(req)
	if err != nil {
		return nil, err
	}
	if u != nil {
		return p.proxied.RoundTrip(req)
	}
	return p.direct.RoundTrip(req)
}
