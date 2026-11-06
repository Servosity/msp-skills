// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"zoom-phone-pp-cli/internal/config"
)

// zoom-phone hand-fix: zoom-phone-store-scope-account-id (header-override). The
// client is the one place every caller shares, the MCP server included, so a
// config [headers] Authorization override stops here in every spelling and
// nothing reaches the network.
func TestZoomConfigAuthorizationHeaderOverrideRefused(t *testing.T) {
	for _, name := range []string{"Authorization", "authorization", "AUTHORIZATION", " Authorization "} {
		t.Run(name, func(t *testing.T) {
			var hits atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
			defer srv.Close()
			c := New(&config.Config{BaseURL: srv.URL, AccessToken: "token-A", Headers: map[string]string{name: "Bearer token-B"}}, 5*time.Second, 0)
			_, err := c.Get(context.Background(), "/phone/users", nil)
			if err == nil || !strings.Contains(err.Error(), "Authorization") {
				t.Fatalf("err = %v, want a refusal naming the Authorization header", err)
			}
			if hits.Load() != 0 {
				t.Fatalf("%d request(s) reached the server", hits.Load())
			}
			if _, err := c.ProbeGet(context.Background(), "/phone/users"); err == nil || hits.Load() != 0 {
				t.Fatalf("doctor's probe: err=%v hits=%d", err, hits.Load())
			}
		})
	}
}
