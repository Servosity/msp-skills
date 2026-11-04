// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Class test (credential-routing): a credentialed request goes only to the
// server the client was configured for. Whatever a caller changes after that
// (a template var such as --path-context server=..., the base URL) or a server
// answers (a redirect to another host), a second host never sees a request.

package client

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"netsapiens-pp-cli/internal/config"
)

const pinKey = "RoutingApiKey55443322"

// pinServers starts the configured server A (its handler is h) and a second
// host B that counts every request it receives.
func pinServers(t *testing.T, h http.HandlerFunc) (a *httptest.Server, b *httptest.Server, bHits *atomic.Int32) {
	t.Helper()
	bHits = &atomic.Int32{}
	b = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(b.Close)
	if h == nil {
		h = func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		}
	}
	a = httptest.NewServer(h)
	t.Cleanup(a.Close)
	return a, b, bHits
}

func pinClient(t *testing.T, a *httptest.Server) *Client {
	t.Helper()
	cfg := &config.Config{
		BaseURL:          "http://{server}/ns-api/v2",
		TemplateVars:     map[string]string{"server": strings.TrimPrefix(a.URL, "http://")},
		NetsapiensApiKey: pinKey,
		Headers:          map[string]string{"X-NS-Partner": "partner-header-value"},
	}
	c := New(cfg, 5*time.Second, 0)
	c.NoCache = true
	c.cacheDir = t.TempDir()
	t.Setenv("NETSAPIENS_RETRY_MAX", "0")
	return c
}

func TestClassCredentialNeverLeavesTheConfiguredServer(t *testing.T) {
	t.Run("a template var changed after configuration (path-context server=)", func(t *testing.T) {
		a, b, bHits := pinServers(t, nil)
		c := pinClient(t, a)
		c.Config.TemplateVars["server"] = strings.TrimPrefix(b.URL, "http://")
		_, _ = c.Get(context.Background(), "/domains", nil)
		if n := bHits.Load(); n != 0 {
			t.Fatalf("the credentialed request reached a second host (%d requests)", n)
		}
	})
	t.Run("a base URL changed after configuration", func(t *testing.T) {
		a, b, bHits := pinServers(t, nil)
		c := pinClient(t, a)
		c.BaseURL = b.URL + "/ns-api/v2"
		_, _ = c.Get(context.Background(), "/domains", nil)
		if n := bHits.Load(); n != 0 {
			t.Fatalf("the credentialed request reached a second host (%d requests)", n)
		}
	})
	t.Run("the configured server redirects to another host", func(t *testing.T) {
		var b *httptest.Server
		a, b, bHits := pinServers(t, func(w http.ResponseWriter, r *http.Request) {
			_, port, _ := net.SplitHostPort(strings.TrimPrefix(b.URL, "http://"))
			http.Redirect(w, r, "http://vendor-b.test:"+port+"/ns-api/v2/domains", http.StatusFound)
		})
		// vendor-b.test is a public host as far as the dial guard can tell; its
		// connection lands on B.
		defer swapDialResolve(func(context.Context, string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("198.51.100.7")}, nil
		})()
		defer swapDialPinned(func(ctx context.Context, network, addr string) (net.Conn, error) {
			u, _ := url.Parse(b.URL)
			return (&net.Dialer{}).DialContext(ctx, network, u.Host)
		})()
		c := pinClient(t, a)
		_, _ = c.Get(context.Background(), "/domains", nil)
		if n := bHits.Load(); n != 0 {
			t.Fatalf("a redirect off the configured server was followed (%d requests reached the other host)", n)
		}
	})
	t.Run("the configured server still answers", func(t *testing.T) {
		a, _, _ := pinServers(t, nil)
		c := pinClient(t, a)
		if _, err := c.Get(context.Background(), "/domains", nil); err != nil {
			t.Fatalf("a request to the configured server was refused: %v", err)
		}
	})
}
