// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"zoom-phone-pp-cli/internal/config"
)

// The refresh exchange's transport error never shows the redirect Location.
func TestZoomR1RefreshTransportErrorMasked(t *testing.T) {
	const marker = "unheld-refresh-location-0001"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://zoom.us/%ZZ/"+marker)
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	cfg := &config.Config{BaseURL: srv.URL, TokenURL: srv.URL + "/oauth/token", ClientID: "cid-r1", ClientSecret: "refresh-client-secret-0001", RefreshToken: "refresh-token-r1-0001"}
	c := New(cfg, 5*time.Second, 0)
	err := c.refreshAccessToken(context.Background())
	if err == nil {
		t.Fatal("expected the refresh exchange to fail")
	}
	for _, needle := range []string{marker, "%ZZ"} {
		if strings.Contains(err.Error(), needle) {
			t.Fatalf("the redirect Location is shown: %v", err)
		}
	}
}
