// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: guards handfixes.json scoped-bearer-in-secret-set.

package client

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"watchguard-pp-cli/internal/config"
)

// A customer's audience-scoped token is substituted at the transport, so the
// configured credential's echo guards never saw it: an HTML 200 that echoes it
// and a 403 that echoes its base64 must both be withheld from the error text.
func TestAudienceScopedBearerEchoWithheld(t *testing.T) {
	const scoped = "scoped-sentinel-token-0002-abcdefghijklmnop"
	scopedMu.Lock()
	scopedBearers["Bearer "+scoped] = true
	scopedMu.Unlock()
	t.Cleanup(func() {
		scopedMu.Lock()
		delete(scopedBearers, "Bearer "+scoped)
		scopedMu.Unlock()
	})
	got := authSecrets("Bearer own-token-0001")
	if !contains(got, scoped) || !contains(got, "Bearer "+scoped) || !contains(got, "own-token-0001") {
		t.Fatalf("authSecrets misses the scoped bearer: %q", got)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/html":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><head><title>portal ` + scoped + `</title></head><body>x</body></html>`))
		default:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"denied for ` + base64.StdEncoding.EncodeToString([]byte(scoped)) + `"}`))
		}
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, AccessToken: "own-token-0001"}, 5*time.Second, 0)
	c.NoCache = true
	for _, path := range []string{"/rest/html", "/rest/err"} {
		_, err := c.Get(context.Background(), path, nil)
		if err == nil {
			t.Fatalf("%s: expected an error", path)
		}
		if strings.Contains(err.Error(), scoped) || strings.Contains(err.Error(), base64.StdEncoding.EncodeToString([]byte(scoped))) {
			t.Fatalf("%s leaked the scoped token: %v", path, err)
		}
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
