// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package portal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
)

// ErrUnsafeDownloadURL is returned when the portal hands back a download
// link, or a redirect from one, that this CLI refuses to request.
var ErrUnsafeDownloadURL = errors.New("portal: refusing an unsafe download url")

// ErrDownloadSize is returned when a download does not match the size the
// portal reported for it.
var ErrDownloadSize = errors.New("portal: download size does not match the reported size")

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
		return nil
	}
	if reason := blockedAddrReason(addr); reason != "" {
		return fmt.Errorf("%w: host %s is a %s address", ErrUnsafeDownloadURL, addr.WithZone(""), reason)
	}
	return nil
}

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
// is an error. A short error page served with a 200 therefore never passes
// for a finished archive.
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return 0, fmt.Errorf("portal: building the download request: %w", scrubURLError(err))
	}
	// A presigned URL carries its own authorisation; sending the session
	// cookie or CSRF header to object storage would be pointless and
	// would leak them to a third party.
	resp, err := s.downloadClient().Do(req)
	if err != nil {
		return 0, fmt.Errorf("portal: downloading chunk %s: %w", exportDataID, scrubURLError(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return 0, fmt.Errorf("portal: downloading chunk %s: status %d: %s",
			exportDataID, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if !want.Known() {
		n, err := io.Copy(w, resp.Body)
		if err != nil {
			return n, fmt.Errorf("portal: streaming chunk %s after %d bytes: %w", exportDataID, n, err)
		}
		return n, nil
	}
	if resp.ContentLength >= 0 && !want.Matches(resp.ContentLength) {
		return 0, fmt.Errorf("%w: chunk %s is %d bytes but the portal reported about %d",
			ErrDownloadSize, exportDataID, resp.ContentLength, want.Bytes)
	}
	ceiling := want.Bytes + want.Tolerance
	n, err := io.Copy(w, io.LimitReader(resp.Body, ceiling+1))
	if err != nil {
		return n, fmt.Errorf("portal: streaming chunk %s after %d bytes: %w", exportDataID, n, err)
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

// downloadClient returns a client with no timeout, for transfers that can
// run for hours. The session's own client has a per-request timeout suited
// to service calls, which would abort a large archive mid-stream. Every
// redirect hop is held to DownloadURLPolicy, the same as the first URL.
func (s *Session) downloadClient() *http.Client {
	inner := s.hc.CheckRedirect
	return &http.Client{
		Transport: s.hc.Transport,
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
	}
}
