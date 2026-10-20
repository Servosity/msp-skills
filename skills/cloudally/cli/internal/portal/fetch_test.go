// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package portal

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// zipBody starts the way a zip archive does.
const zipBody = "PK\x03\x04archive"

func TestStrictDownloadURLPolicy(t *testing.T) {
	allowed := []string{
		"https://ca-s3.amazonaws.com/export/job-1.zip?X-Amz-Signature=abc",
		"https://bucket.s3.eu-west-1.amazonaws.com/x.zip",
		"https://8.8.8.8/x.zip",
		"https://[2606:4700::1111]/x.zip",
		"https://storage.example.com:8443/x.zip",
		"HTTPS://Storage.Example.com/x.zip",
	}
	for _, raw := range allowed {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("parse %q: %v", raw, err)
		}
		if err := StrictDownloadURL(u); err != nil {
			t.Fatalf("%s should be allowed: %v", raw, err)
		}
	}
	refused := []string{
		"http://ca-s3.amazonaws.com/x.zip",   // not https
		"ftp://ca-s3.amazonaws.com/x.zip",    // not https
		"file:///etc/passwd",                 // not https, no host
		"https:///x.zip",                     // no host
		"https://127.0.0.1/x.zip",            // loopback
		"https://127.9.9.9:8080/x.zip",       // loopback
		"https://[::1]/x.zip",                // loopback
		"https://[::ffff:127.0.0.1]/x.zip",   // loopback, IPv4-mapped
		"https://169.254.169.254/latest/",    // link-local metadata
		"https://[fe80::1]/x.zip",            // link-local
		"https://[fe80::1%25en0]/x.zip",      // link-local with a zone
		"https://10.1.2.3/x.zip",             // RFC1918
		"https://172.16.0.9/x.zip",           // RFC1918
		"https://192.168.1.10/x.zip",         // RFC1918
		"https://[fd12:3456::1]/x.zip",       // ULA fc00::/7
		"https://0.0.0.0/x.zip",              // unspecified
		"https://0.1.2.3/x.zip",              // this network
		"https://[::]/x.zip",                 // unspecified
		"https://100.100.100.100/x.zip",      // shared address space
		"https://localhost/x.zip",            // this machine by name
		"https://LOCALHOST./x.zip",           // this machine by name
		"https://evil.localhost/x.zip",       // reserved for this machine
		"https://127.1/x.zip",                // shorthand a resolver may accept
		"https://2130706433/x.zip",           // decimal 127.0.0.1
		"https://0x7f000001/x.zip",           // hex 127.0.0.1
		"https://metadata.0x7f.0x1/x.zip",    // hex last label
		"https://192.168.0.1.nip.io.5/x.zip", // numeric last label
		"https://fuse-t:18083/x.zip",         // single label
		"https://storage/x.zip",              // single label
		"https://nas.local/x.zip",            // mDNS
		"https://metadata.google.internal/x", // cloud metadata
		"https://files.lan/x.zip",            // private network
		"https://nas.home.arpa/x.zip",        // home network
		"https://home.arpa/x.zip",            // the suffix itself
		"https://fileserver.corp/x.zip",      // corporate network
		"https://a.b.LOCALHOST/x.zip",        // reserved for this machine
	}
	for _, raw := range refused {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("parse %q: %v", raw, err)
		}
		err = StrictDownloadURL(u)
		if err == nil {
			t.Fatalf("%s should be refused", raw)
		}
		if !errors.Is(err, ErrUnsafeDownloadURL) {
			t.Fatalf("%s: error should wrap ErrUnsafeDownloadURL, got %v", raw, err)
		}
	}
	if err := StrictDownloadURL(nil); err == nil {
		t.Fatal("a nil url should be refused")
	}
}

// openLinkSession opens a session against a fake portal whose
// getDownloadUrl answers with link.
func openLinkSession(t *testing.T, link string) *Session {
	t.Helper()
	f := newFakePortal(t, "tok")
	f.getHandler = func(op string, _ map[string][]string) string {
		if op != downloadURLOperation {
			t.Fatalf("operation = %q", op)
		}
		return `{"success":true,"data":"` + link + `"}`
	}
	srv := f.server()
	t.Cleanup(srv.Close)
	s, err := Open(context.Background(), Config{BaseURL: srv.URL, RatePerSec: -1}, "tok")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s
}

// withDownloadPolicy swaps DownloadURLPolicy for one test, and lets
// connections reach loopback, where the test servers listen.
func withDownloadPolicy(t *testing.T, p func(*url.URL) error) {
	t.Helper()
	prev, prevAddr := DownloadURLPolicy, DownloadAddrPolicy
	DownloadURLPolicy = p
	DownloadAddrPolicy = allowLoopbackAddr
	t.Cleanup(func() { DownloadURLPolicy, DownloadAddrPolicy = prev, prevAddr })
}

func allowLoopbackAddr(a netip.Addr) error {
	if a.IsLoopback() {
		return nil
	}
	return StrictDownloadAddr(a)
}

// allowOnlyHost lets one test server through and holds everything else to
// the strict policy.
func allowOnlyHost(host string) func(*url.URL) error {
	return func(u *url.URL) error {
		if u.Host == host {
			return nil
		}
		return StrictDownloadURL(u)
	}
}

// TestFetchChunkRefusesAnUnsafeLinkBeforeRequesting proves the strict
// default never requests a loopback http link a portal response names, and
// that the same link is fetched once the policy allows it, so the refusal
// is the policy's doing.
func TestFetchChunkRefusesAnUnsafeLinkBeforeRequesting(t *testing.T) {
	var hits atomic.Int32
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write([]byte(zipBody))
	}))
	defer internal.Close()

	s := openLinkSession(t, internal.URL+"/admin")
	var buf bytes.Buffer
	_, err := s.FetchChunk(context.Background(), "d1", &buf, SizeHint{})
	if !errors.Is(err, ErrUnsafeDownloadURL) {
		t.Fatalf("want ErrUnsafeDownloadURL, got %v", err)
	}
	if hits.Load() != 0 || buf.Len() != 0 {
		t.Fatalf("the unsafe link was requested: hits=%d body=%q", hits.Load(), buf.String())
	}

	u, _ := url.Parse(internal.URL)
	withDownloadPolicy(t, allowOnlyHost(u.Host))
	n, err := s.FetchChunk(context.Background(), "d1", &buf, SizeHint{})
	if err != nil || n == 0 || hits.Load() != 1 {
		t.Fatalf("an allowed link should be fetched: n=%d hits=%d err=%v", n, hits.Load(), err)
	}
}

// TestFetchChunkChecksEveryRedirectHop proves an allowed link cannot bounce
// the request to a host the policy refuses.
func TestFetchChunkChecksEveryRedirectHop(t *testing.T) {
	var metadataHits atomic.Int32
	metadata := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		metadataHits.Add(1)
		_, _ = w.Write([]byte("iam-credentials"))
	}))
	defer metadata.Close()
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bounce" {
			http.Redirect(w, r, metadata.URL+"/latest/meta-data/", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write([]byte(zipBody))
	}))
	defer storage.Close()
	su, _ := url.Parse(storage.URL)
	withDownloadPolicy(t, allowOnlyHost(su.Host))

	s := openLinkSession(t, storage.URL+"/bounce")
	var buf bytes.Buffer
	_, err := s.FetchChunk(context.Background(), "d1", &buf, SizeHint{})
	if !errors.Is(err, ErrUnsafeDownloadURL) {
		t.Fatalf("want the redirect refused, got %v", err)
	}
	if metadataHits.Load() != 0 || buf.Len() != 0 {
		t.Fatalf("the redirect target was requested: hits=%d body=%q", metadataHits.Load(), buf.String())
	}
	// The error must not carry the presigned URL.
	if strings.Contains(err.Error(), "bounce") {
		t.Fatalf("error leaks the link: %v", err)
	}

	// A redirect to the allowed host is followed.
	storage2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bounce" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write([]byte(zipBody))
	}))
	defer storage2.Close()
	s2u, _ := url.Parse(storage2.URL)
	withDownloadPolicy(t, allowOnlyHost(s2u.Host))
	s2 := openLinkSession(t, storage2.URL+"/bounce")
	buf.Reset()
	if _, err := s2.FetchChunk(context.Background(), "d1", &buf, SizeHint{}); err != nil || buf.String() != zipBody {
		t.Fatalf("a same-host redirect should be followed: body=%q err=%v", buf.String(), err)
	}
}

func TestParseSizeHint(t *testing.T) {
	h, ok := ParseSizeHint("871.26 MB")
	if !ok || h.Bytes != ParseSizeBytes("871.26 MB") {
		t.Fatalf("871.26 MB = %+v ok=%v", h, ok)
	}
	if !h.Matches(913_581_909) {
		t.Fatalf("the observed live size must match: %+v", h)
	}
	// A rounded size allows one unit in its last printed digit and no more.
	coarse, ok := ParseSizeHint("1.8 GB")
	gib := float64(1 << 30)
	if !ok || !coarse.Matches(int64(1.84*gib)) || !coarse.Matches(int64(1.89*gib)) || coarse.Matches(int64(1.95*gib)) {
		t.Fatalf("1.8 GB tolerance wrong: %+v", coarse)
	}
	if h.Matches(h.Bytes - h.Bytes/100) {
		t.Fatalf("871.26 MB must not accept an archive 1%% short: %+v", h)
	}
	// A byte count is exact: a truncated body is never close enough.
	exact, ok := ParseSizeHint("1000 Bytes")
	if !ok || !exact.Matches(1000) || exact.Matches(990) || exact.Matches(999) || exact.Matches(1001) {
		t.Fatalf("1000 Bytes must match exactly: %+v", exact)
	}
	for _, s := range []string{"", "0 Bytes", "unknown", "-1 MB"} {
		if h, ok := ParseSizeHint(s); ok || h.Known() {
			t.Fatalf("%q should be an unknown size, got %+v", s, h)
		}
	}
	if (SizeHint{}).Matches(0) {
		t.Fatal("an unknown hint matches nothing")
	}
}

// sizedSession serves body from a storage server; chunked forces a body
// with no Content-Length, the way a misbehaving proxy would send one.
func sizedSession(t *testing.T, body []byte, chunked bool) (*Session, *atomic.Int64) {
	t.Helper()
	var served atomic.Int64
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if chunked {
			w.(http.Flusher).Flush()
		}
		for off := 0; off < len(body); off += 4096 {
			end := min(off+4096, len(body))
			n, err := w.Write(body[off:end])
			served.Add(int64(n))
			if err != nil {
				return
			}
		}
	}))
	t.Cleanup(storage.Close)
	su, _ := url.Parse(storage.URL)
	withDownloadPolicy(t, allowOnlyHost(su.Host))
	return openLinkSession(t, storage.URL+"/obj"), &served
}

func TestFetchChunkFailsAShortBodyOfTheWrongSize(t *testing.T) {
	want, _ := ParseSizeHint("1.00 MB")
	for _, chunked := range []bool{false, true} {
		s, _ := sizedSession(t, []byte(`<Error><Code>AccessDenied</Code></Error>`), chunked)
		var buf bytes.Buffer
		_, err := s.FetchChunk(context.Background(), "d1", &buf, want)
		if !errors.Is(err, ErrDownloadSize) {
			t.Fatalf("chunked=%v: want ErrDownloadSize for a 40-byte error page, got %v", chunked, err)
		}
		// With a Content-Length the mismatch is caught before writing.
		if !chunked && buf.Len() != 0 {
			t.Fatalf("a wrong Content-Length should be refused before writing, wrote %d", buf.Len())
		}
	}
}

func TestFetchChunkStopsCopyingPastTheReportedSize(t *testing.T) {
	want, _ := ParseSizeHint("100.00 KB")
	huge := bytes.Repeat([]byte("x"), 4<<20)
	s, _ := sizedSession(t, huge, true)
	var buf bytes.Buffer
	n, err := s.FetchChunk(context.Background(), "d1", &buf, want)
	if !errors.Is(err, ErrDownloadSize) {
		t.Fatalf("want ErrDownloadSize, got %v", err)
	}
	if limit := want.Bytes + want.Tolerance + 1; n > limit || int64(buf.Len()) > limit {
		t.Fatalf("copied %d bytes (buffer %d), want at most %d", n, buf.Len(), limit)
	}
}

func TestFetchChunkAcceptsTheReportedSize(t *testing.T) {
	want, _ := ParseSizeHint("100.00 KB")
	body := bytes.Repeat([]byte("y"), 100<<10)
	for _, chunked := range []bool{false, true} {
		s, _ := sizedSession(t, body, chunked)
		var buf bytes.Buffer
		n, err := s.FetchChunk(context.Background(), "d1", &buf, want)
		if err != nil || n != int64(len(body)) || !bytes.Equal(buf.Bytes(), body) {
			t.Fatalf("chunked=%v: n=%d err=%v", chunked, n, err)
		}
	}
	// An unknown size is copied as it comes.
	s, _ := sizedSession(t, body, true)
	var buf bytes.Buffer
	if n, err := s.FetchChunk(context.Background(), "d1", &buf, SizeHint{}); err != nil || n != int64(len(body)) {
		t.Fatalf("unknown size: n=%d err=%v", n, err)
	}
}

// TestFetchChunkRefusesAHostnameThatResolvesToLoopback lets the URL pass
// its check (as any DNS name does) and proves the connection is still
// refused once the name resolves to this machine.
func TestFetchChunkRefusesAHostnameThatResolvesToLoopback(t *testing.T) {
	var hits atomic.Int32
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(zipBody))
	}))
	defer internal.Close()
	u, _ := url.Parse(internal.URL)
	prev, prevAddr := DownloadURLPolicy, DownloadAddrPolicy
	DownloadURLPolicy = func(*url.URL) error { return nil }
	DownloadAddrPolicy = StrictDownloadAddr
	t.Cleanup(func() { DownloadURLPolicy, DownloadAddrPolicy = prev, prevAddr })

	s := openLinkSession(t, "http://localhost:"+u.Port()+"/admin")
	var buf bytes.Buffer
	_, err := s.FetchChunk(context.Background(), "d1", &buf, SizeHint{})
	if !errors.Is(err, ErrUnsafeDownloadURL) || !strings.Contains(err.Error(), "resolves to a loopback") {
		t.Fatalf("want the connection refused after DNS, got %v", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("the loopback service was reached %d time(s)", hits.Load())
	}
}

// TestFetchChunkLetsAnEnvironmentProxyResolve proves a request sent through
// a proxy is not refused for the proxy's own (here loopback) address: the
// proxy, not this machine, connects to the host.
func TestFetchChunkLetsAnEnvironmentProxyResolve(t *testing.T) {
	var proxied atomic.Value
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxied.Store(r.URL.String())
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write([]byte(zipBody))
	}))
	defer proxy.Close()
	pu, _ := url.Parse(proxy.URL)
	prev, prevAddr := DownloadURLPolicy, DownloadAddrPolicy
	DownloadURLPolicy = func(*url.URL) error { return nil }
	DownloadAddrPolicy = StrictDownloadAddr
	t.Cleanup(func() { DownloadURLPolicy, DownloadAddrPolicy = prev, prevAddr })

	f := newFakePortal(t, "tok")
	f.getHandler = func(string, map[string][]string) string {
		return `{"success":true,"data":"http://storage.example.invalid/obj"}`
	}
	srv := f.server()
	defer srv.Close()
	transport := &http.Transport{Proxy: func(r *http.Request) (*url.URL, error) {
		if r.URL.Hostname() == "storage.example.invalid" {
			return pu, nil
		}
		return nil, nil
	}}
	s, err := Open(context.Background(), Config{BaseURL: srv.URL, RatePerSec: -1, HTTPClient: &http.Client{Transport: transport}}, "tok")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	var buf bytes.Buffer
	if _, err := s.FetchChunk(context.Background(), "d1", &buf, SizeHint{}); err != nil || buf.String() != zipBody {
		t.Fatalf("a proxied download should go through: body=%q err=%v", buf.String(), err)
	}
	if got, _ := proxied.Load().(string); got != "http://storage.example.invalid/obj" {
		t.Fatalf("the proxy saw %q", got)
	}
}

// TestFetchChunkEndsOnlyAStalledStream proves a stream that keeps arriving
// may run longer than the stall timeout in total, and one that goes silent
// is ended.
func TestFetchChunkEndsOnlyAStalledStream(t *testing.T) {
	prevStall := DownloadStallTimeout
	DownloadStallTimeout = 300 * time.Millisecond
	t.Cleanup(func() { DownloadStallTimeout = prevStall })
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write([]byte("PK\x03\x04"))
		w.(http.Flusher).Flush()
		if r.URL.Path == "/stall" {
			select {
			case <-r.Context().Done():
			case <-time.After(10 * time.Second):
			}
			return
		}
		for i := 0; i < 10; i++ {
			time.Sleep(100 * time.Millisecond)
			_, _ = w.Write([]byte("steady"))
			w.(http.Flusher).Flush()
		}
	}))
	defer storage.Close()
	su, _ := url.Parse(storage.URL)
	withDownloadPolicy(t, allowOnlyHost(su.Host))

	start := time.Now()
	var buf bytes.Buffer
	n, err := openLinkSession(t, storage.URL+"/steady").FetchChunk(context.Background(), "d1", &buf, SizeHint{})
	if err != nil || n != int64(4+10*len("steady")) {
		t.Fatalf("a steady stream over %s must finish: n=%d err=%v", time.Since(start), n, err)
	}
	start = time.Now()
	_, err = openLinkSession(t, storage.URL+"/stall").FetchChunk(context.Background(), "d1", &buf, SizeHint{})
	if !errors.Is(err, ErrDownloadStalled) {
		t.Fatalf("want ErrDownloadStalled, got %v", err)
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("the stall took %s to notice", took)
	}
}

func TestParseSizeHintReadsThousandsSeparators(t *testing.T) {
	for in, want := range map[string]string{
		"1,024.50 MB":      "1024.50 MB",
		"12,345,678 Bytes": "12345678 Bytes",
		"2,048 KB":         "2048 KB",
		"1 024.50 MB":      "",
	} {
		h, ok := ParseSizeHint(in)
		if want == "" {
			if ok {
				t.Fatalf("%q should stay unknown, got %+v", in, h)
			}
			continue
		}
		if !ok || h.Bytes != ParseSizeBytes(want) {
			t.Fatalf("ParseSizeHint(%q) = %+v, %v; want the bytes of %q", in, h, ok, want)
		}
	}
}

// TestFetchChunkRefusesAnErrorDocumentOfUnknownSize covers a size that is
// missing or unreadable: a short HTML, XML, JSON or text body is refused,
// an archive-like one is kept.
func TestFetchChunkRefusesAnErrorDocumentOfUnknownSize(t *testing.T) {
	cases := []struct {
		name, ctype, body string
		refuse            bool
	}{
		{"xml error, binary type", "application/octet-stream", `<?xml version="1.0"?><Error><Code>AccessDenied</Code></Error>`, true},
		{"html with a bom", "application/octet-stream", "\xef\xbb\xbf  <html><body>Forbidden</body></html>", true},
		{"json error", "application/octet-stream", `{"error":"expired"}`, true},
		{"text type", "text/plain; charset=utf-8", "PK but really text", true},
		{"zip", "application/zip", zipBody, false},
		{"zip, no type", "", zipBody, false},
	}
	for _, tc := range cases {
		storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", tc.ctype)
			_, _ = w.Write([]byte(tc.body))
		}))
		su, _ := url.Parse(storage.URL)
		withDownloadPolicy(t, allowOnlyHost(su.Host))
		var buf bytes.Buffer
		_, err := openLinkSession(t, storage.URL+"/obj").FetchChunk(context.Background(), "d1", &buf, SizeHint{})
		storage.Close()
		if tc.refuse {
			if !errors.Is(err, ErrNotAnArchive) || buf.Len() != 0 {
				t.Fatalf("%s: want ErrNotAnArchive with nothing written, got err=%v wrote=%d", tc.name, err, buf.Len())
			}
		} else if err != nil || buf.String() != tc.body {
			t.Fatalf("%s: want it kept, got err=%v body=%q", tc.name, err, buf.String())
		}
	}
}
