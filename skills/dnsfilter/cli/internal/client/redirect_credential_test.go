// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "redirect-never-reaches-a-credential-endpoint" entry.

package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"dnsfilter-pp-cli/internal/cliutil"
	"dnsfilter-pp-cli/internal/config"
)

// TestRedirectNeverReachesACredentialEndpoint: a same-origin redirect to the
// uninstall PIN, an API key, a site secret key or the PSA integration GET (a
// write) is refused before the hop is sent, in either mode; an ordinary
// redirect is still followed.
func TestRedirectNeverReachesACredentialEndpoint(t *testing.T) {
	var forbidden atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/user_agents/7":
			http.Redirect(w, r, "/v1/user_agents/uninstall_pin?organization_id=3", http.StatusFound)
		case "/v1/networks/1":
			http.Redirect(w, r, "/v1/psa_integrations/redirect_link?organization_id=3", http.StatusFound)
		case "/v1/networks/4":
			http.Redirect(w, r, "/v1/api%5Fkeys", http.StatusFound)
		case "/v1/networks/2":
			http.Redirect(w, r, "/v1/networks/3", http.StatusFound)
		case "/v1/networks/3":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"id":"3"}}`))
		default:
			forbidden.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`"482915"`))
		}
	}))
	t.Cleanup(server.Close)
	for _, ro := range []string{"", "1"} {
		t.Setenv(cliutil.MCPReadOnlyEnvVar, ro)
		c := New(&config.Config{BaseURL: server.URL, DnsfilterApiKey: "test-key"}, 5*time.Second, 0)
		c.NoCache = true
		for _, p := range []string{"/v1/user_agents/7", "/v1/networks/1", "/v1/networks/4"} {
			if _, err := c.Get(context.Background(), p, nil); !errors.Is(err, ErrRedirectCredentialEndpoint) {
				t.Fatalf("read-only=%q: GET %s followed a redirect to a credential endpoint: %v", ro, p, err)
			}
		}
		if _, err := c.Get(context.Background(), "/v1/networks/2", nil); err != nil {
			t.Fatalf("read-only=%q: an ordinary redirect was refused: %v", ro, err)
		}
	}
	if n := forbidden.Load(); n != 0 {
		t.Fatalf("%d redirected requests reached a credential endpoint", n)
	}
}
