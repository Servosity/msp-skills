// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Class credential-sent-to-another-host (review round 2): the client must pin
// the configured scheme, host and port, and refuse anything else before it is
// resolved, dialled or handed a credential.

import (
	"context"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"testing"
)

type r2RecordingTripper struct{ calls atomic.Int32 }

func (r *r2RecordingTripper) RoundTrip(*http.Request) (*http.Response, error) {
	r.calls.Add(1)
	return nil, errors.New("recorded")
}

// A host that only folds to the pinned one under Unicode case rules is
// another host: the transport dials its IDNA form (xn--i-9bb.example.com).
func TestR2PinRefusesUnicodeLookalikeHost(t *testing.T) {
	next := &r2RecordingTripper{}
	tr := originPinTripper{origin: pinnedOrigin("https://{server}/api/v3", map[string]string{"server": "i.example.com"}), next: next}
	for _, raw := range []string{
		"https://İ.example.com/api/v3/assets",
		"https://I.EXAMPLE.COM.evil.test/api/v3/assets",
		"http://i.example.com/api/v3/assets",
		"https://i.example.com:8443/api/v3/assets",
	} {
		req, err := http.NewRequest(http.MethodGet, raw, nil)
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if _, err := tr.RoundTrip(req); err == nil || next.calls.Load() != 0 {
			t.Fatalf("%s: passed the pin (next called %d times)", raw, next.calls.Load())
		}
	}
	req, _ := http.NewRequest(http.MethodGet, "https://I.Example.COM/api/v3/assets", nil)
	_, _ = tr.RoundTrip(req)
	if next.calls.Load() != 1 {
		t.Fatal("the pinned host in another ASCII case was refused")
	}
}

// swapRecordingResolver makes every DNS lookup in this process countable.
func swapRecordingResolver(t *testing.T) *atomic.Int32 {
	t.Helper()
	var lookups atomic.Int32
	prev := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		lookups.Add(1)
		return nil, errors.New("dns disabled in test")
	}}
	t.Cleanup(func() { net.DefaultResolver = prev })
	return &lookups
}

// A redirect is refused before the hop's host is resolved: a DNS query is
// itself a disclosure (a server-chosen label can carry the secret).
func TestR2RedirectRefusedBeforeAnyDNSLookup(t *testing.T) {
	label := hex.EncodeToString([]byte(r1TokenSecret))
	tenant := r1Server(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://"+label+".example.invalid/api/v3/organizations", http.StatusFound)
	})
	r1Env(t, tenant)
	lookups := swapRecordingResolver(t)
	_, _ = r1Run(t, "organizations", "list", "--json")
	if n := lookups.Load(); n != 0 {
		t.Fatalf("%d DNS lookup(s) for a redirect target outside the pinned origin", n)
	}
}

// --path-context cannot move the API host, even to a Unicode look-alike of the
// configured one: nothing is resolved or dialled.
func TestR2PathContextLookalikeNeverResolves(t *testing.T) {
	home := testenvR2(t)
	lookups := swapRecordingResolver(t)
	_, _ = r1Run(t, "sync", "--resources", "organizations", "--path-context", "server=İ.example.com", "--max-pages", "1",
		"--db", filepath.Join(home, "r2.db"))
	if n := lookups.Load(); n != 0 {
		t.Fatalf("%d DNS lookup(s) for a host other than the configured one", n)
	}
}

func testenvR2(t *testing.T) string {
	t.Helper()
	home := r1Env(t, "i.example.com")
	return home
}
