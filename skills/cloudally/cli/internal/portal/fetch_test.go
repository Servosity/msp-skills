// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package portal

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

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

// withDownloadPolicy swaps DownloadURLPolicy for one test.
func withDownloadPolicy(t *testing.T, p func(*url.URL) error) {
	t.Helper()
	prev := DownloadURLPolicy
	DownloadURLPolicy = p
	t.Cleanup(func() { DownloadURLPolicy = prev })
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
		_, _ = w.Write([]byte("internal-service-secret"))
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
		_, _ = w.Write([]byte("archive"))
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
		_, _ = w.Write([]byte("archive"))
	}))
	defer storage2.Close()
	s2u, _ := url.Parse(storage2.URL)
	withDownloadPolicy(t, allowOnlyHost(s2u.Host))
	s2 := openLinkSession(t, storage2.URL+"/bounce")
	buf.Reset()
	if _, err := s2.FetchChunk(context.Background(), "d1", &buf, SizeHint{}); err != nil || buf.String() != "archive" {
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
