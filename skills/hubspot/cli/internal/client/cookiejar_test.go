// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"hubspot-pp-cli/internal/config"
)

// Hand-fix (handfixes.json api-client-keeps-cookies): HubSpot's edge mints a
// fresh __cf_bm cookie on every request that carries none, and the
// credential kit holds each one until exit. The API client sends it back, so
// a sync of many pages receives one cookie, not one per page.
func TestAPIClientSendsBackTheEdgeCookie(t *testing.T) {
	var minted atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie("__cf_bm"); err != nil {
			minted.Add(1)
			http.SetCookie(w, &http.Cookie{Name: "__cf_bm", Value: "synthetic-bot-cookie", Path: "/", Secure: true, HttpOnly: true})
		}
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer server.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	c := New(&config.Config{BaseURL: server.URL, AuthHeaderVal: "Bearer synthetic"}, 5*time.Second, 0)
	c.HTTPClient.Transport = server.Client().Transport // trust the test certificate; keep New's jar
	for i := 0; i < 5; i++ {
		if _, err := c.Get(context.Background(), "/crm/v3/objects/companies", map[string]string{"after": string(rune('0' + i))}); err != nil {
			t.Fatal(err)
		}
	}
	if n := minted.Load(); n != 1 {
		t.Fatalf("the edge minted %d cookies over 5 requests, want 1 (the client must send the cookie back)", n)
	}
}

// A cookie goes back only to the host that set it: a Domain=hubapi.com cookie
// from one hubapi.com host never reaches a sibling (Codex synccpu r1).
func TestAPIClientCookieStaysWithTheHostThatSetIt(t *testing.T) {
	jar := hostOnlyCookieJar()
	edge, _ := url.Parse("https://edge.hubapi.com/")
	api, _ := url.Parse("https://api.hubapi.com/")
	jar.SetCookies(edge, []*http.Cookie{{Name: "__cf_bm", Value: "Cookie9Vq2Wm7Rs4Xk8", Domain: "hubapi.com", Path: "/", Secure: true}})
	if got := jar.Cookies(api); len(got) != 0 {
		t.Fatalf("a cookie set by edge.hubapi.com reached api.hubapi.com: %v", got)
	}
	if got := jar.Cookies(edge); len(got) != 1 {
		t.Fatalf("the setting host did not get its cookie back: %v", got)
	}
}
