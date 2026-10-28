// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (Codex r1 P1 "secret in error text"): a 2xx HTML body's title
// becomes error text, so it must pass the same credential mask as every
// other server-derived error string.

package client

import (
	"context"
	"html"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"blackpoint-pp-cli/internal/config"
)

func TestHTMLSuccessBodyNeverEchoesCredential(t *testing.T) {
	const token = "test-secret-token-XXXXXXXX"
	for name, title := range map[string]string{
		"raw":         token,
		"html-entity": strings.ReplaceAll(html.EscapeString(token), "X", "&#88;"),
		"in-sentence": "Signed in as " + token + " - CompassOne",
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				_, _ = w.Write([]byte("<html><head><title>" + title + "</title></head><body></body></html>"))
			}))
			defer srv.Close()
			c := New(&config.Config{BaseURL: srv.URL, BlackpointApiToken: token}, 5*time.Second, 0)
			c.NoCache = true
			_, err := c.Get(context.Background(), "/tenants", nil)
			if err == nil {
				t.Fatal("an HTML body on a JSON endpoint must be an error")
			}
			if strings.Contains(err.Error(), token) {
				t.Fatalf("error text carries the API token: %v", err)
			}
		})
	}
}
