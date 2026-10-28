// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"domain-cert-tracker-pp-cli/internal/cliutil"
	"domain-cert-tracker-pp-cli/internal/cliutil/testenv"
	"domain-cert-tracker-pp-cli/internal/config"
)

// The configured auth_header and custom headers belong to base_url's origin
// only. A request whose absolute URL names another host (the per-operation
// crt.sh and IANA servers) and a redirect off the origin carry neither.
func TestCredentialsStayOnBaseOrigin(t *testing.T) {
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	const auth, custom = "Bearer origin-only-s3cret", "custom-s3cret"
	var mu sync.Mutex
	var leaked []string
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if v := r.Header.Get("Authorization"); v != "" {
			leaked = append(leaked, "Authorization="+v)
		}
		if v := r.Header.Get("X-Rdap-Key"); v != "" {
			leaked = append(leaked, "X-Rdap-Key="+v)
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer other.Close()
	var baseSaw string
	base := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		baseSaw = r.Header.Get("Authorization") + "|" + r.Header.Get("X-Rdap-Key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer base.Close()

	c := New(&config.Config{BaseURL: base.URL, AuthHeaderVal: auth, Headers: map[string]string{"X-Rdap-Key": custom}}, 5*time.Second, 0)
	ctx := context.Background()
	if _, err := c.GetNoCache(ctx, "/help", nil); err != nil {
		t.Fatal(err)
	}
	if baseSaw != auth+"|"+custom {
		t.Fatalf("base_url's own origin did not get its credentials: %q", baseSaw)
	}
	// A redirect off the origin (rdap.org always redirects to a registry
	// server) drops both; the hop is driven directly because the generated
	// guard refuses a loopback redirect target before any header is sent.
	pub := New(&config.Config{BaseURL: "https://192.0.2.10", AuthHeaderVal: auth, Headers: map[string]string{"X-Rdap-Key": custom}}, 5*time.Second, 0)
	orig, _ := http.NewRequest(http.MethodGet, "https://192.0.2.10/domain/example.com", nil)
	next, _ := http.NewRequest(http.MethodGet, "https://93.184.215.14/domain/example.com", nil)
	next.Header.Set("Authorization", auth)
	next.Header.Set("X-Rdap-Key", custom)
	if err := pub.HTTPClient.CheckRedirect(next, []*http.Request{orig}); err == nil {
		if a, k := next.Header.Get("Authorization"), next.Header.Get("X-Rdap-Key"); a != "" || k != "" {
			t.Errorf("a redirect off base_url kept credentials: Authorization=%q X-Rdap-Key=%q", a, k)
		}
	}
	same, _ := http.NewRequest(http.MethodGet, "https://192.0.2.10/help", nil)
	if err := pub.HTTPClient.CheckRedirect(same, []*http.Request{orig}); err != nil || same.Header.Get("Authorization") != auth {
		t.Errorf("a same-origin hop lost its auth header: %q (err %v)", same.Header.Get("Authorization"), err)
	}
	// The spec default base leaves absolute per-operation URLs on their host.
	d := New(&config.Config{BaseURL: specDefaultBaseURL, AuthHeaderVal: auth, Headers: map[string]string{"X-Rdap-Key": custom}}, 5*time.Second, 0)
	_, _ = d.GetNoCache(ctx, other.URL+"/?q=example.com", nil)
	mu.Lock()
	defer mu.Unlock()
	if len(leaked) > 0 {
		t.Fatalf("credentials reached a host other than base_url: %v", leaked)
	}
}
