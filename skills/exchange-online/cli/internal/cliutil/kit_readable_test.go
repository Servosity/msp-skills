// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"errors"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"exchange-online-pp-cli/internal/credkit"
)

type recordingTripper struct{ got []*http.Request }

func (r *recordingTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	r.got = append(r.got, req)
	return &http.Response{StatusCode: 204, Body: http.NoBody, Header: http.Header{}, Request: req}, nil
}

// The kit itself is the oracle, not a copy of its rule: every query the helper lets through
// passes the kit's outbound guard, and decodes to what it was written as. This process never
// runs credkit.Start, and these requests carry no credential. A query the kit refused would
// stay refused for the whole process, so one failure shows as many.
func TestCanonicalQueryIsWhatTheKitReads(t *testing.T) {
	inner := &recordingTripper{}
	guard := credkit.Transport(inner)
	send := func(t *testing.T, raw, query string) {
		t.Helper()
		req := &http.Request{Method: http.MethodGet, URL: &url.URL{Scheme: "https", Host: "kit.example", Path: "/x", RawQuery: query}, Header: http.Header{}}
		if _, err := guard.RoundTrip(req); err != nil {
			t.Fatalf("the kit refused %q (from %q): %v", query, raw, err)
		}
	}
	check := func(t *testing.T, raw string) {
		t.Helper()
		want, parseErr := url.ParseQuery(raw)
		got, err := CanonicalQuery(raw)
		if parseErr != nil {
			if err == nil || !errors.Is(err, ErrUnreadableRequest) {
				t.Fatalf("%q: url.ParseQuery refuses it (%v), the helper said %q, %v", raw, parseErr, got, err)
			}
			return
		}
		if err != nil {
			t.Fatalf("%q: %v", raw, err)
		}
		decoded, err := url.ParseQuery(got)
		if err != nil || !reflect.DeepEqual(decoded, want) {
			t.Fatalf("%q became %q, which decodes to %v (%v); want %v", raw, got, decoded, err, want)
		}
		again, err := CanonicalQuery(got)
		if err != nil || again != got {
			t.Fatalf("%q is not settled: %q then %q (%v)", raw, got, again, err)
		}
		send(t, raw, got)
	}
	for _, raw := range []string{
		"", "a", "a=", "=", "=a", "&", "a=1&&b=2", "&a=1&",
		"$skiptoken=abc", "%24skiptoken=abc==", "%24skiptoken='abc'", "%24skiptoken=a(b)",
		"$top=1000&$skip=1000", "$filter=Name eq 'x,y'", "a=b+c", "a=b%20c", "a=b+c%20d", "a=b%2Bc", "a=b%2bc",
		"a=,:/@", "a=%2C%3A%2F%40", "a=,%3A", "a=%00", "a=%FF", "a=\xff", "a=caf\u00e9", "a=~!*()'", "a==b==",
		"a=b;c", "a;b", ";", "a=%zz", "%zz", "a=%2", "a=%", "%=a",
	} {
		check(t, raw)
	}
	alphabet := []string{"a", "Z", "0", "$", "=", "&", "+", "%", "%20", "%2B", "%2b", "%24", "%2C", "%3A", "'", "(", ")", "~", ",", ":", "/", "@", "!", "*", " ", ";", "#", "?", "\u00e9", "\xff", "-", "_", "."}
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 20000; i++ {
		var b strings.Builder
		for n := r.Intn(12); n > 0; n-- {
			b.WriteString(alphabet[r.Intn(len(alphabet))])
		}
		check(t, b.String())
	}
	if len(inner.got) < 1000 {
		t.Fatalf("only %d queries reached the kit's inner transport", len(inner.got))
	}
}

// A piece the kit reads is sent as the server wrote it.
func TestCanonicalQueryLeavesAReadablePieceAlone(t *testing.T) {
	for raw, want := range map[string]string{
		"a=b+c":          "a=b+c",
		"a=b%20c":        "a=b%20c",
		"a=b%2Bc":        "a=b%2Bc",
		"a=,:/@":         "a=,:/@",
		"a=%2C%3A%2F%40": "a=%2C%3A%2F%40",
		"upload":         "upload",
		"a=1&&b=2":       "a=1&&b=2",
		"$a=b+c'":        "%24a=b+c%27",
		"$a=b%20c'":      "%24a=b%20c%27",
		"$a=b c":         "%24a=b%20c",
		"$a=b+c%20d":     "%24a=b%20c%20d",
		"$b=1&$a=2":      "%24b=1&%24a=2",
	} {
		if got, err := CanonicalQuery(raw); err != nil || got != want {
			t.Errorf("%q: got %q, %v; want %q", raw, got, err, want)
		}
	}
}

func TestKitReadableRequestRefusesWhatTheKitWouldNotRead(t *testing.T) {
	build := func(target string, set map[string]string, body string) *http.Request {
		var r io.Reader
		if body != "" {
			r = strings.NewReader(body)
		}
		req, err := http.NewRequest(http.MethodPost, target, r)
		if err != nil {
			t.Fatal(err)
		}
		for k, v := range set {
			req.Header.Set(k, v)
		}
		return req
	}
	for _, tc := range []struct {
		name, target string
		headers      map[string]string
		body, word   string
	}{
		{"semicolon", "https://h.example/x?a=b;c", nil, "", "semicolon"},
		{"bad escape", "https://h.example/x?a=%zz", nil, "", "%zz"},
		{"opaque", "https:h.example?a=b", nil, "", "no host"},
		{"content coding", "https://h.example/x", map[string]string{"Content-Encoding": "gzip"}, "", "Content-Encoding"},
		{"transfer coding", "https://h.example/x", map[string]string{"Transfer-Encoding": "gzip, chunked"}, "", "Transfer-Encoding"},
		{"mime coding", "https://h.example/x", map[string]string{"Content-Transfer-Encoding": "base64"}, "", "Content-Transfer-Encoding"},
		{"bad type", "https://h.example/x", map[string]string{"Content-Type": "a;;b"}, "", "Content-Type"},
		{"charset", "https://h.example/x", map[string]string{"Content-Type": "application/json; charset=latin1"}, "", "latin1"},
		{"form", "https://h.example/x", map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, "", "x-www-form-urlencoded"},
		{"multipart", "https://h.example/x", map[string]string{"Content-Type": "multipart/form-data; boundary=x"}, "", "multipart"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := KitReadableRequest(build(tc.target, tc.headers, tc.body))
			if err == nil || !errors.Is(err, ErrUnreadableRequest) || !strings.Contains(err.Error(), tc.word) {
				t.Fatalf("got %v, want an unreadable request naming %q", err, tc.word)
			}
		})
	}
	// What it accepts: the query rewritten on a clone, the body whole, req untouched.
	orig := build("https://h.example/x?$a=b", map[string]string{"Content-Type": "application/json; charset=utf-8"}, `{"a":1}`)
	out, err := KitReadableRequest(orig)
	if err != nil {
		t.Fatal(err)
	}
	if out.URL.RawQuery != "%24a=b" || orig.URL.RawQuery != "$a=b" {
		t.Errorf("out %q, orig %q", out.URL.RawQuery, orig.URL.RawQuery)
	}
	body, _ := io.ReadAll(out.Body)
	if string(body) != `{"a":1}` {
		t.Errorf("the body arrived as %q", body)
	}
	for _, h := range []map[string]string{nil, {"Content-Encoding": "identity"}, {"Content-Type": "application/json"}} {
		req := build("https://h.example/x?a=1", h, "")
		if out, err := KitReadableRequest(req); err != nil || out != req {
			t.Errorf("%v: got a rewrite or error (%v) for a request the kit reads", h, err)
		}
	}
}
